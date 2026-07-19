package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GrafanaEnabled reports whether the with-grafana compose profile is active.
func GrafanaEnabled(env Env) bool {
	if env.EnableGrafana == "1" {
		return true
	}
	if env.EnableGrafana == "0" {
		return false
	}
	profiles := env.Raw["COMPOSE_PROFILES"]
	if profiles == "" {
		profiles = getenv("COMPOSE_PROFILES", "")
	}
	for _, p := range strings.Split(profiles, ",") {
		if strings.TrimSpace(p) == "with-grafana" {
			return true
		}
	}
	return false
}

// EnsureGrafanaRuntime verifies bind-mount sources for the grafana service:
// provisioning tree under core/deploy, and a group-readable admin password file
// (Grafana image runs as uid=472 gid=0; 0600 root:root is unreadable in-container).
func EnsureGrafanaRuntime(root string, env Env) error {
	if !GrafanaEnabled(env) {
		return nil
	}

	prov := filepath.Join(root, "core", "deploy", "grafana", "provisioning")
	for _, rel := range []string{
		"datasources/prometheus.yml",
		"dashboards/dashboards.yml",
		"dashboards/json/gateway-overview.json",
	} {
		p := filepath.Join(prov, rel)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("Grafana provisioning 缺失: %s（release 应含 core/deploy/grafana）", p)
		}
	}

	secretsDir := env.SecretsDir
	if secretsDir == "" {
		secretsDir = "/etc/relaygate/secrets"
	}
	if err := os.MkdirAll(secretsDir, 0o750); err != nil {
		return fmt.Errorf("创建密钥目录失败: %w", err)
	}

	pwFile := filepath.Join(secretsDir, "grafana_admin_password")
	st, err := os.Stat(pwFile)
	if err != nil || st.Size() == 0 {
		if strings.TrimSpace(getenv("GRAFANA_ADMIN_PASSWORD", "")) != "" {
			return nil
		}
		return fmt.Errorf("缺少 %s（先跑 relaygate setup）或设置 GRAFANA_ADMIN_PASSWORD", pwFile)
	}
	// Ensure container (gid 0) can read; leave ownership to install/setup.
	if err := os.Chmod(pwFile, 0o640); err != nil {
		return fmt.Errorf("无法调整 %s 权限为 0640: %w", pwFile, err)
	}
	return nil
}
