package ops

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
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

	stamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(root, "data", "backups", stamp)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	fmt.Printf("==> 备份到 %s (gateway=%s)\n", backupDir, env.GatewayName)
	for _, rel := range []string{
		"data/envoy/envoy.yaml",
		"core/deploy/compose.yaml",
		"data/resources.yaml",
		"data/prometheus/prometheus.yml",
	} {
		src := filepath.Join(root, rel)
		if b, err := os.ReadFile(src); err == nil {
			_ = os.WriteFile(filepath.Join(backupDir, filepath.Base(rel)), b, 0o644)
		}
	}
	_ = os.WriteFile(filepath.Join(root, "data", "backups", "latest"), []byte(stamp+"\n"), 0o644)

	fmt.Println("==> 渲染并校验")
	if err := Validate(root); err != nil {
		return err
	}

	fmt.Println("==> 应用内核参数（若存在）")
	if err := ApplySysctl(root, env.GatewayName); err != nil {
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

// ApplySysctl copies core/deploy/sysctl/gateway.conf when running as root.
func ApplySysctl(root, gatewayName string) error {
	src := filepath.Join(root, "core", "deploy", "sysctl", "gateway.conf")
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
