package dataplane

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
)

// Rollback restores config from backups/<stamp> or backups/latest and recreates envoy.
// When RELAYGATE_PRIVILEGED_HELPER is set and not root, re-execs via sudo (needs docker).
func Rollback(root string, stamp string) error {
	args := []string{"rollback"}
	if stamp != "" {
		args = append(args, stamp)
	}
	if handled, err := maybePrivilegedReexec(os.Stdout, os.Stderr, args...); handled {
		return err
	}
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	p := config.ResolvePaths(root)
	backupDir := p.Backups
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
		_ = os.MkdirAll(p.DataDir, 0o755)
		_ = os.WriteFile(p.Resources, b, 0o644)
	}
	if b, err := os.ReadFile(filepath.Join(target, "envoy.yaml")); err == nil {
		_ = os.MkdirAll(filepath.Dir(p.EnvoyYAML), 0o755)
		_ = os.WriteFile(p.EnvoyYAML, b, 0o644)
	} else {
		if err := RenderConfig(root, false); err != nil {
			return err
		}
	}
	if b, err := os.ReadFile(filepath.Join(target, "prometheus.yml")); err == nil {
		_ = os.MkdirAll(filepath.Dir(p.PromYAML), 0o755)
		_ = os.WriteFile(p.PromYAML, b, 0o644)
	}

	if err := requireEnvFile(root); err != nil {
		return err
	}

	fmt.Println("==> drain before recreate")
	_ = Drain(root, "fail")
	if err := Validate(root); err != nil {
		return err
	}
	composeArgs := append(ComposeArgs(root, true), "up", "-d", "--force-recreate", "--no-deps", "envoy")
	if err := RunCmd(root, "docker", composeArgs...); err != nil {
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
