package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Rollback restores config from backups/<stamp> or backups/latest and recreates envoy.
func Rollback(root string, stamp string) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	backupDir := filepath.Join(root, "data", "backups")
	var target string
	if stamp != "" {
		target = filepath.Join(backupDir, stamp)
	} else {
		b, err := os.ReadFile(filepath.Join(backupDir, "latest"))
		if err != nil {
			return fmt.Errorf("无备份")
		}
		target = filepath.Join(backupDir, strings.TrimSpace(string(b)))
	}
	st, err := os.Stat(target)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("备份不存在: %s", target)
	}

	fmt.Printf("==> 回滚自 %s (gateway=%s)\n", target, env.GatewayName)
	if b, err := os.ReadFile(filepath.Join(target, "resources.yaml")); err == nil {
		_ = os.MkdirAll(filepath.Join(root, "data"), 0o755)
		_ = os.WriteFile(filepath.Join(root, "data", "resources.yaml"), b, 0o644)
	}
	if b, err := os.ReadFile(filepath.Join(target, "envoy.yaml")); err == nil {
		_ = os.MkdirAll(filepath.Join(root, "data", "envoy"), 0o755)
		_ = os.WriteFile(filepath.Join(root, "data", "envoy", "envoy.yaml"), b, 0o644)
	} else {
		if err := RenderConfig(root, false); err != nil {
			return err
		}
	}
	if b, err := os.ReadFile(filepath.Join(target, "prometheus.yml")); err == nil {
		_ = os.WriteFile(filepath.Join(root, "data", "prometheus", "prometheus.yml"), b, 0o644)
	}

	if err := requireEnvFile(root); err != nil {
		return err
	}

	fmt.Println("==> drain before recreate")
	_ = Drain(root, "fail")
	if err := Validate(root); err != nil {
		return err
	}
	args := append(ComposeArgs(root, true), "up", "-d", "--force-recreate", "--no-deps", "envoy")
	if err := RunCmd(root, "docker", args...); err != nil {
		return err
	}
	if err := WaitHTTP(env.AdminURL("/ready"), 30, 2*time.Second); err != nil {
		return err
	}
	_ = HTTPPost(env.AdminURL("/healthcheck/ok"))
	fmt.Printf("Envoy ready（回滚完成: %s）\n", env.GatewayName)
	fmt.Println("验证: relaygate smoke")
	return nil
}
