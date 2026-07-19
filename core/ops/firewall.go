package ops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// writeFirewallRuntime materializes a runnable nft file under data/firewall/
// from the deploy template (SSH port + include path rewritten).
func writeFirewallRuntime(root, sshPort string) (fwDir, runtimePath string, err error) {
	fwDir = filepath.Join(root, "data", "firewall")
	if err := os.MkdirAll(fwDir, 0o755); err != nil {
		return "", "", err
	}
	src := filepath.Join(root, "core", "deploy", "firewall", "gateway.nft")
	b, err := os.ReadFile(src)
	if err != nil {
		return "", "", err
	}
	gamePorts := filepath.Join(fwDir, "game-ports.nft")
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "include ") {
			lines[i] = fmt.Sprintf(`include %q`, gamePorts)
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
	if !IsRoot() {
		return fmt.Errorf("需要 root")
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
		fmt.Println("默认未应用。确认无误后执行: sudo relaygate firewall apply")
		fmt.Println("（非交互: sudo APPLY_FIREWALL=1 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES relaygate firewall apply）")
		return nil
	}

	if err := confirmFirewall(env); err != nil {
		return err
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
	_ = os.MkdirAll(filepath.Join(root, "data", "backups"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "data", "backups", "firewall-latest"), []byte(restore+"\n"), 0o600)
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
	const phrase = "YES_FLUSH_NFTABLES"
	if env.NonInteractive == "1" || os.Getenv("NONINTERACTIVE") == "1" {
		if env.FirewallConfirm != phrase && os.Getenv("FIREWALL_CONFIRM") != phrase {
			return fmt.Errorf("非交互应用还需 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES")
		}
		return nil
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return fmt.Errorf("无交互终端；请设置 NONINTERACTIVE=1 和 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES")
	}
	defer tty.Close()
	fmt.Fprint(os.Stderr, "输入 YES_FLUSH_NFTABLES 继续: ")
	sc := bufio.NewScanner(tty)
	if !sc.Scan() {
		return fmt.Errorf("已取消")
	}
	if strings.TrimSpace(sc.Text()) != phrase {
		return fmt.Errorf("已取消")
	}
	return nil
}

func relaygateBin(root string) string {
	if v := os.Getenv("RELAYGATE_BIN"); v != "" {
		return v
	}
	return filepath.Join(root, "bin", "relaygate")
}
