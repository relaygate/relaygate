package dataplane

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/confirm"
	"github.com/relaygate/relaygate/core/resources"
)

// writeFirewallRuntime materializes a runnable nft file under DataDir/firewall/
// from the packaging template (SSH port + include path rewritten).
func writeFirewallRuntime(root, sshPort string) (fwDir, runtimePath string, err error) {
	p := config.ResolvePaths(root)
	fwDir = p.Firewall
	if err := os.MkdirAll(fwDir, 0o755); err != nil {
		return "", "", err
	}
	src := filepath.Join(p.Packaging, "firewall", "gateway.nft")
	b, err := os.ReadFile(src)
	if err != nil {
		return "", "", err
	}
	forwardPorts := filepath.Join(fwDir, "forward-ports.nft")
	body := string(b)
	body = strings.ReplaceAll(body, "@INLINE_TCP_NEW_CONN_RATE@", inlineNftablesRate(root, func(n resources.NftablesDefaults) string { return n.TCPNewConnPerIP }))
	body = strings.ReplaceAll(body, "@INLINE_UDP_PPS_RATE@", inlineNftablesRate(root, func(n resources.NftablesDefaults) string { return n.UDPPPSPerIP }))
	body = strings.ReplaceAll(body, "@INLINE_TCP_NEW_CONN_BURST@", inlineNftablesBurst(root, func(n resources.NftablesDefaults) int { return n.TCPBurst }, 60))
	body = strings.ReplaceAll(body, "@INLINE_UDP_PPS_BURST@", inlineNftablesBurst(root, func(n resources.NftablesDefaults) int { return n.UDPBurst }, 1000))
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "include ") {
			lines[i] = fmt.Sprintf(`include %q`, forwardPorts)
			continue
		}
		if strings.HasPrefix(line, "define SSH_PORT = ") {
			lines[i] = "define SSH_PORT = " + sshPort
		}
	}
	runtimePath = filepath.Join(fwDir, fmt.Sprintf(".gateway-%s.nft", sshPort))
	if err := os.WriteFile(runtimePath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", "", err
	}
	return fwDir, runtimePath, nil
}

// Firewall renders and optionally applies nftables rules. apply=false is check-only.
func Firewall(root string, apply bool) error {
	return firewallExec(root, apply, false)
}

// FirewallApplyConfirmed applies nftables after the caller already verified
// 确认 / Confirm (e.g. Panel API). Skips TTY / env confirm prompts.
func FirewallApplyConfirmed(root string) error {
	return firewallExec(root, true, true)
}

func firewallExec(root string, apply bool, skipConfirm bool) error {
	if !IsRoot() {
		// Confirm on the unprivileged side (TTY / env) before elevating dataplane.
		if apply && !skipConfirm {
			env, err := LoadEnv(root)
			if err != nil {
				return err
			}
			if err := confirmFirewall(env); err != nil {
				return err
			}
			skipConfirm = true
		}
		args := []string{"firewall-check"}
		if apply {
			args = []string{"firewall-apply"}
		}
		if handled, err := maybePrivilegedReexec(os.Stdout, os.Stderr, args...); handled {
			return err
		}
		return errNeedRootOrHelper()
	}
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	sshPort := env.GatewaySSHPort
	if v := os.Getenv("SSH_PORT"); v != "" {
		sshPort = v
	}
	n, err := strconv.Atoi(sshPort)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("SSH 端口无效: %s", sshPort)
	}

	fmt.Println("==> 渲染端口定义")
	if err := RenderConfig(root, false); err != nil {
		return err
	}

	fwDir, runtimeRuleset, err := writeFirewallRuntime(root, sshPort)
	if err != nil {
		return err
	}

	fmt.Println("==> 语法检查")
	if err := RunCmd(fwDir, "nft", "-c", "-f", filepath.Base(runtimeRuleset)); err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("规则已生成并校验: %s\n", runtimeRuleset)
	fmt.Println("警告：该规则包含 flush ruleset，应用错误可能中断 SSH。")
	fmt.Printf("将保留 SSH/TCP %s；应用前请保持当前会话并准备云控制台。\n", sshPort)

	if !apply {
		fmt.Println("变更分流：ACL / nftables-only → sudo relaygate firewall apply（无需 reload Envoy）")
		fmt.Println("默认未应用。确认无误后执行: sudo relaygate firewall apply")
		fmt.Println("（非交互: sudo APPLY_FIREWALL=1 FIREWALL_CONFIRM=Confirm relaygate firewall apply）")
		fmt.Println("（Panel：运维工具 → 防火墙检查 / 应用防火墙，经 privileged helper）")
		return nil
	}

	if !skipConfirm {
		if err := confirmFirewall(env); err != nil {
			return err
		}
	}

	stamp := time.Now().Format("20060102-150405")
	backup := filepath.Join("/root", "nft-backup-"+stamp+".nft")
	out, err := RunCmdCapture(root, "nft", "list", "ruleset")
	if err != nil {
		return fmt.Errorf("备份 nft ruleset: %w", err)
	}
	if err := os.WriteFile(backup, []byte(out), 0o600); err != nil {
		return err
	}
	restore := strings.TrimSuffix(backup, ".nft") + "-restore.sh"
	script := fmt.Sprintf("#!/usr/bin/env bash\nset -e\nnft -f %q\n", backup)
	if err := os.WriteFile(restore, []byte(script), 0o700); err != nil {
		return err
	}
	backups := config.ResolvePaths(root).Backups
	_ = os.MkdirAll(backups, 0o755)
	_ = os.WriteFile(filepath.Join(backups, "firewall-latest"), []byte(restore+"\n"), 0o600)
	fmt.Printf("旧规则备份: %s\n", backup)
	fmt.Printf("恢复命令: %s\n", restore)

	if err := RunCmd(fwDir, "nft", "-f", filepath.Base(runtimeRuleset)); err != nil {
		return err
	}
	_ = RunCmd(root, "nft", "list", "ruleset")
	fmt.Printf("请立即新开终端测试 SSH: ssh -p %s root@<公网IP>\n", sshPort)
	return nil
}

func confirmFirewall(env Env) error {
	if env.NonInteractive == "1" || os.Getenv("NONINTERACTIVE") == "1" {
		if !confirm.Match(env.FirewallConfirm) && !confirm.Match(os.Getenv("FIREWALL_CONFIRM")) {
			return fmt.Errorf("非交互应用还需 FIREWALL_CONFIRM=Confirm（或「确认」）")
		}
		return nil
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return fmt.Errorf("无交互终端；请设置 NONINTERACTIVE=1 和 FIREWALL_CONFIRM=Confirm")
	}
	defer tty.Close()
	fmt.Fprint(os.Stderr, "输入 确认 或 Confirm 继续: ")
	sc := bufio.NewScanner(tty)
	if !sc.Scan() {
		return fmt.Errorf("已取消")
	}
	if !confirm.Match(sc.Text()) {
		return fmt.Errorf("已取消")
	}
	return nil
}

func inlineNftablesRate(root string, pick func(resources.NftablesDefaults) string) string {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return "30/second"
	}
	res.Defaults.ApplyNftablesDefaults()
	rate := strings.TrimSpace(pick(res.Defaults.Nftables))
	if rate == "" {
		return "30/second"
	}
	return rate
}

func inlineNftablesBurst(root string, pick func(resources.NftablesDefaults) int, fallback int) string {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return strconv.Itoa(fallback)
	}
	res.Defaults.ApplyNftablesDefaults()
	v := pick(res.Defaults.Nftables)
	if v <= 0 {
		v = fallback
	}
	return strconv.Itoa(v)
}

func relaygateBin(root string) string {
	if v := os.Getenv("RELAYGATE_BIN"); v != "" {
		return v
	}
	return filepath.Join(root, "bin", "relaygate")
}
