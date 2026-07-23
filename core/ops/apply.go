package ops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
)

// Apply deploys the full data plane: backup, observability, validate, sysctl, compose up, wait ready.
func Apply(root string) error {
	if err := requireEnvFile(root); err != nil {
		return err
	}
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	if env.GatewayName == "" {
		return fmt.Errorf("set GATEWAY_NAME in .env")
	}

	// First apply / upgrade: seed missing runtime defaults from versioned templates
	// (never overwrite existing resources.yaml without setup --reset-defaults).
	if err := SeedDefaults(root, false); err != nil {
		return err
	}

	fmt.Printf("==> 备份并生成变更摘要 (gateway=%s)\n", env.GatewayName)
	stamp, backupDir, _, err := BackupWithSummary(root, os.Stdout)
	if err != nil {
		return err
	}
	fmt.Printf("备份目录: %s (stamp=%s)\n", backupDir, stamp)

	fmt.Println("==> 渲染并校验")
	if err := Validate(root); err != nil {
		return err
	}

	fmt.Println("==> 应用内核参数（若存在）")
	if err := ApplySysctl(root, env.GatewayName); err != nil {
		warnf("%v", err)
	}

	fmt.Println("==> 安装 tcp-access logrotate（若 root）")
	if err := ApplyLogrotate(root); err != nil {
		warnf("%v", err)
	}

	fmt.Printf("==> compose up (%s)\n", env.GatewayName)
	_ = Compose(root, io.Discard, os.Stderr, "pull")
	if err := Compose(root, os.Stdout, os.Stderr, "up", "-d"); err != nil {
		return err
	}

	fmt.Println("==> 等待 Envoy ready")
	if err := WaitHTTP(env.AdminURL("/ready"), 30, 2*time.Second); err != nil {
		_ = Compose(root, os.Stdout, os.Stderr, "ps")
		return fmt.Errorf("Envoy 未 ready")
	}
	fmt.Println("Envoy ready")
	_ = Compose(root, os.Stdout, os.Stderr, "ps")
	fmt.Println()
	fmt.Printf("部署完成: %s\n", env.GatewayName)
	fmt.Printf("Panel（含 Grafana）: ssh -p %s -L 9000:127.0.0.1:9000 root@%s\n", env.GatewaySSHPort, env.GatewayPublicIP)
	fmt.Println("浏览器: http://127.0.0.1:9000/monitoring （无需隧道 3000）")
	fmt.Println("回滚: relaygate rollback")
	fmt.Println("冒烟: relaygate smoke")
	return nil
}

// ApplySysctl copies packaging/sysctl/gateway.conf when running as root.
func ApplySysctl(root, gatewayName string) error {
	src := filepath.Join(root, config.PackagingDirName, "sysctl", "gateway.conf")
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	dst := fmt.Sprintf("/etc/sysctl.d/99-%s.conf", gatewayName)
	if !IsRoot() {
		fmt.Printf("WARN: 非 root，跳过 sysctl；sudo cp %s %s && sudo sysctl --system\n", src, dst)
		return nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return err
	}
	if err := RunCmd(root, "sysctl", "--system"); err != nil {
		return err
	}
	fmt.Printf("sysctl applied: %s\n", dst)
	return nil
}

// ApplyLogrotate installs /etc/logrotate.d/relaygate-envoy-tcp-access from packaging
// template, substituting the absolute DataDir path for tcp-access.json rotation.
func ApplyLogrotate(root string) error {
	src := filepath.Join(root, config.PackagingDirName, "logrotate", "envoy-tcp-access")
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	dataDir := config.ResolveDataDir(root)
	dst := "/etc/logrotate.d/relaygate-envoy-tcp-access"
	if !IsRoot() {
		fmt.Printf("WARN: 非 root，跳过 logrotate；sudo sed \"s|@DATA_DIR@|%s|g\" %s | sudo tee %s\n", dataDir, src, dst)
		return nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(b), "@DATA_DIR@", dataDir)
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("logrotate installed: %s (DataDir=%s)\n", dst, dataDir)
	return nil
}
