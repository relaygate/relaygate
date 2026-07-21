package ops

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
)

// Baseline collects a read-only host baseline into backups/baseline-*.txt.
func Baseline(root string, outPath string) error {
	_ = config.LoadDotEnv(filepath.Join(root, ".env"))
	env, _ := LoadEnv(root)
	p := config.ResolvePaths(root)
	_ = os.MkdirAll(p.Backups, 0o755)
	if outPath == "" {
		outPath = filepath.Join(p.Backups, "baseline-"+time.Now().Format("20060102-150405")+".txt")
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := func(format string, args ...any) {
		fmt.Fprintf(f, format+"\n", args...)
		fmt.Printf(format+"\n", args...)
	}
	runSection := func(title string, name string, args ...string) {
		w("## %s", title)
		cmd := exec.Command(name, args...)
		cmd.Stdout = f
		cmd.Stderr = f
		_ = cmd.Run()
		// also mirror to stdout roughly
		out, _ := exec.Command(name, args...).CombinedOutput()
		fmt.Print(string(out))
		w("")
	}

	w("# %s runtime baseline collected at %s", env.GatewayName, time.Now().Format(time.RFC3339))
	w("")
	runSection("uname", "uname", "-a")
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		w("## os-release")
		w("%s", string(b))
	}
	runSection("cpu", "nproc")
	runSection("memory", "free", "-h")
	runSection("disk", "df", "-h", "/")
	runSection("addresses", "ip", "-br", "addr")
	runSection("routes", "ip", "route")
	runSection("listening sockets", "ss", "-lntup")
	w("## docker")
	if LookPath("docker") {
		out, _ := exec.Command("docker", "--version").CombinedOutput()
		w("%s", strings.TrimSpace(string(out)))
		out, _ = exec.Command("systemctl", "is-active", "docker").CombinedOutput()
		w("%s", strings.TrimSpace(string(out)))
		out, _ = exec.Command("docker", "ps", "--format", "table {{.Names}}\t{{.Status}}\t{{.Image}}").CombinedOutput()
		w("%s", string(out))
	} else {
		w("docker not installed")
	}
	w("## sysctl")
	out, _ := exec.Command("sysctl",
		"net.core.somaxconn", "net.ipv4.ip_local_port_range",
		"net.core.rmem_max", "net.core.wmem_max", "fs.file-max").CombinedOutput()
	w("%s", string(out))
	w("## firewall")
	if LookPath("nft") {
		out, _ := exec.Command("nft", "list", "ruleset").CombinedOutput()
		w("%s", string(out))
	} else if LookPath("iptables") {
		out, _ := exec.Command("iptables", "-S").CombinedOutput()
		w("%s", string(out))
	}
	fmt.Printf("\n已写入 %s\n", outPath)
	return nil
}

// Fleet upgrades gateways from inventory one-by-one via release tar / install.sh --upgrade.
// Flow per host: drain fail → install.sh --upgrade → smoke → drain ok.
// No git fetch/checkout fallback (production installs are tar-based).
func Fleet(root string, gatewaysCSV string) error {
	inventory := getenv("INVENTORY", config.ResolvePaths(root).Inventory)
	if _, err := os.Stat(inventory); err != nil {
		return fmt.Errorf("缺少 inventory: %s（请复制 gateways.env.example → DataDir/inventory/gateways.env）", inventory)
	}
	vars, err := parseInventory(inventory)
	if err != nil {
		return err
	}
	if gatewaysCSV == "" {
		gatewaysCSV = getenv("GATEWAYS", vars["GATEWAY_MATRIX"])
	}
	if gatewaysCSV == "" {
		gatewaysCSV = "gateway-01,gateway-02"
	}
	version, localTar, err := ResolveReleaseSpec(root)
	if err != nil {
		return fmt.Errorf("fleet 需要 release 规格: %w", err)
	}
	sshOpts := strings.Fields(getenv("SSH_OPTS", "-o StrictHostKeyChecking=accept-new -o BatchMode=yes"))
	imageTag := os.Getenv("IMAGE_TAG")
	pauseSec := 10
	if v := os.Getenv("BATCH_PAUSE_SEC"); v != "" {
		fmt.Sscanf(v, "%d", &pauseSec)
	}

	fmt.Println("==> fleet：分批 release-tar / install.sh --upgrade（不用 git）")
	if localTar != "" {
		fmt.Printf("    RELAYGATE_TAR=%s\n", localTar)
	}
	if version != "" {
		fmt.Printf("    RELAYGATE_VERSION=%s\n", version)
	}
	fmt.Printf("    GATEWAYS=%s BATCH_PAUSE_SEC=%d\n", gatewaysCSV, pauseSec)

	for _, gw := range strings.Split(gatewaysCSV, ",") {
		gw = strings.TrimSpace(gw)
		if gw == "" {
			continue
		}
		key := strings.ReplaceAll(gw, "-", "_")
		host := vars["HOST_"+key]
		port := vars["SSH_PORT_"+key]
		if port == "" {
			port = config.DefaultSSHPort
		}
		user := vars["SSH_USER_"+key]
		if user == "" {
			user = "root"
		}
		rdir := vars["REMOTE_DIR_"+key]
		if rdir == "" {
			rdir = config.DefaultInstallDir
		}
		if host == "" {
			return fmt.Errorf("inventory 未定义 HOST_%s（网关 %s）", key, gw)
		}
		fmt.Printf("\n========== 分批升级: %s (%s@%s:%s) ==========\n", gw, user, host, port)
		sshBase := append([]string{}, sshOpts...)
		sshBase = append(sshBase, "-p", port, user+"@"+host)

		remote := func(script string) error {
			args := append([]string{}, sshBase...)
			args = append(args, script)
			if err := RunCmd(root, "ssh", args...); err != nil {
				return fmt.Errorf("%s SSH 失败（%s@%s:%s）: %w", gw, user, host, port, err)
			}
			return nil
		}

		fmt.Println("==> 1/4 drain fail")
		if err := remote(fmt.Sprintf("cd %s && ./bin/relaygate drain fail", shellQuote(rdir))); err != nil {
			return err
		}

		remoteTar := ""
		if localTar != "" {
			remoteTar = "/tmp/relaygate-fleet-" + filepath.Base(localTar)
			fmt.Printf("==> 2/4 scp release tar → %s\n", remoteTar)
			scpArgs := append([]string{}, sshOpts...)
			scpArgs = append(scpArgs, "-P", port, localTar, fmt.Sprintf("%s@%s:%s", user, host, remoteTar))
			if err := RunCmd(root, "scp", scpArgs...); err != nil {
				return fmt.Errorf("%s scp tar 失败（%s@%s）: %w", gw, user, host, err)
			}
		} else {
			fmt.Println("==> 2/4 远端将按 RELAYGATE_VERSION 下载 release tar")
		}

		if imageTag != "" {
			sed := fmt.Sprintf(`cd %s && sed -i 's/^IMAGE_TAG=.*/IMAGE_TAG=%s/' .env || echo IMAGE_TAG=%s >> .env`,
				shellQuote(rdir), imageTag, imageTag)
			_ = remote(sed)
		}

		upgradeCmd := fleetRemoteUpgradeCmd(rdir, version, remoteTar)
		fmt.Println("==> 3/4 install.sh --upgrade")
		if err := remote(upgradeCmd); err != nil {
			return fmt.Errorf("%s 升级失败（无 git 回退）: %w", gw, err)
		}

		fmt.Println("==> 4/4 smoke + drain ok")
		if err := remote(fmt.Sprintf("cd %s && ./bin/relaygate smoke 127.0.0.1 && ./bin/relaygate drain ok", shellQuote(rdir))); err != nil {
			return err
		}
		fmt.Printf("==> %s 完成，BATCH_PAUSE_SEC=%d 后继续下一台\n", gw, pauseSec)
		time.Sleep(time.Duration(pauseSec) * time.Second)
	}
	fmt.Printf("\n全部分批升级完成: %s\n", gatewaysCSV)
	fmt.Println("回滚单台: ssh … 'cd /opt/relaygate && ./bin/relaygate rollback'")
	return nil
}

// fleetRemoteUpgradeCmd builds the remote shell snippet for install.sh --upgrade.
func fleetRemoteUpgradeCmd(remoteDir, version, remoteTar string) string {
	var b strings.Builder
	b.WriteString("set -euo pipefail; ")
	b.WriteString("cd ")
	b.WriteString(shellQuote(remoteDir))
	b.WriteString("; ")
	b.WriteString("export NONINTERACTIVE=1 RELAYGATE_INSTALL_DIR=")
	b.WriteString(shellQuote(remoteDir))
	if version != "" {
		b.WriteString("; export RELAYGATE_VERSION=")
		b.WriteString(shellQuote(version))
	}
	if remoteTar != "" {
		b.WriteString("; export RELAYGATE_TAR=")
		b.WriteString(shellQuote(remoteTar))
	}
	b.WriteString("; ")
	b.WriteString("if [[ ! -f ./install.sh ]]; then ")
	b.WriteString("echo 'ERROR: 远端缺少 install.sh；生产请用 release tar 安装树，fleet 不回退到源码同步' >&2; exit 1; ")
	b.WriteString("fi; ")
	b.WriteString("bash ./install.sh --upgrade -y")
	return b.String()
}

func parseInventory(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return m, sc.Err()
}
