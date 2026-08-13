package dataplane

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/relaygate/relaygate/core/config"
)

// GrafanaEnabled reports whether the with-grafana compose profile is active.
func GrafanaEnabled(env Env) bool {
	if env.GrafanaEnabled == "1" {
		return true
	}
	if env.GrafanaEnabled == "0" {
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
// provisioning tree under packaging/, and a group-readable admin password file
// (Grafana image runs as uid=472 gid=0; 0600 root:root is unreadable in-container).
func EnsureGrafanaRuntime(root string, env Env) error {
	if !GrafanaEnabled(env) {
		return nil
	}

	prov := filepath.Join(root, config.PackagingDirName, "grafana", "provisioning")
	for _, rel := range []string{
		"datasources/prometheus.yml",
		"datasources/loki.yml",
		"dashboards/dashboards.yml",
		"dashboards/json/gateway-overview.json",
		"dashboards/json/tcp-session-logs.json",
	} {
		p := filepath.Join(prov, rel)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("Grafana provisioning 缺失: %s（release 应含 packaging/grafana）", p)
		}
	}

	secretsDir := env.SecretsDir
	if secretsDir == "" {
		secretsDir = config.DefaultSecretsDir
	}
	if err := os.MkdirAll(secretsDir, 0o750); err != nil {
		return fmt.Errorf("创建密钥目录失败: %w", err)
	}

	pwFile := filepath.Join(secretsDir, "grafana_admin_password")
	st, err := os.Stat(pwFile)
	if err != nil || st.Size() == 0 {
		return fmt.Errorf("缺少 %s（先跑 relaygate setup）", pwFile)
	}
	// Ensure container (gid 0) can read; leave ownership to install/setup.
	if err := os.Chmod(pwFile, 0o640); err != nil {
		return fmt.Errorf("无法调整 %s 权限为 0640: %w", pwFile, err)
	}
	return nil
}
