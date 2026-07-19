// Package ops implements RelayGate operational workflows (apply, reload, seed,
// firewall, panel install…). It is a Go code package under core/ops — not a
// runtime data directory. Instance state lives in data/; versioned templates in core/deploy/.
package ops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Env holds commonly used gateway settings loaded from .env / process env.
type Env struct {
	GatewayName              string
	GatewayPublicIP          string
	GatewaySSHPort           string
	EnvoyAdminPort           string
	EnvoyImage               string
	PrometheusRemoteWriteURL string
	ComposeProjectName       string
	PanelRole                string
	EnablePanel              string
	EnableGrafana            string
	ImageTag                 string
	SecretsDir               string
	GrafanaURL               string
	DrainWait                int
	ApplyFirewall            string
	NonInteractive           string
	FirewallConfirm          string
	TCPPort                  string
	UDPPort                  string
	Timeout                  string
	Raw                      map[string]string
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// LoadDotEnv loads KEY=VALUE pairs from path into the process environment
// without overriding already-set variables.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}

// LoadEnv loads root/.env (if present) then reads known keys.
func LoadEnv(root string) (Env, error) {
	envPath := filepath.Join(root, ".env")
	if err := LoadDotEnv(envPath); err != nil {
		return Env{}, fmt.Errorf("load .env: %w", err)
	}
	raw := map[string]string{}
	if b, err := os.ReadFile(envPath); err == nil {
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
			raw[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}
	drainWait := 5
	if v := getenv("DRAIN_WAIT", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			drainWait = n
		}
	}
	return Env{
		GatewayName:              getenv("GATEWAY_NAME", "gateway-01"),
		GatewayPublicIP:          getenv("GATEWAY_PUBLIC_IP", "127.0.0.1"),
		GatewaySSHPort:           getenv("GATEWAY_SSH_PORT", getenv("SSH_PORT", "30455")),
		EnvoyAdminPort:           getenv("ENVOY_ADMIN_PORT", "9901"),
		EnvoyImage:               getenv("ENVOY_IMAGE", "envoyproxy/envoy:v1.39.0"),
		PrometheusRemoteWriteURL: getenv("PROMETHEUS_REMOTE_WRITE_URL", ""),
		ComposeProjectName:       getenv("COMPOSE_PROJECT_NAME", ""),
		PanelRole:                getenv("PANEL_ROLE", "primary"),
		EnablePanel:              getenv("ENABLE_PANEL", "1"),
		EnableGrafana:            getenv("ENABLE_GRAFANA", ""),
		ImageTag:                 getenv("IMAGE_TAG", "local"),
		SecretsDir:               getenv("RELAYGATE_SECRETS_DIR", "/etc/relaygate/secrets"),
		GrafanaURL:               getenv("GRAFANA_URL", ""),
		DrainWait:                drainWait,
		ApplyFirewall:            getenv("APPLY_FIREWALL", "0"),
		NonInteractive:           getenv("NONINTERACTIVE", "0"),
		FirewallConfirm:          getenv("FIREWALL_CONFIRM", ""),
		TCPPort:                  getenv("TCP_PORT", "11001"),
		UDPPort:                  getenv("UDP_PORT", "11001"),
		Timeout:                  getenv("TIMEOUT", "3"),
		Raw:                      raw,
	}, nil
}

func (e Env) EnvoyContainer() string {
	return e.GatewayName + "-envoy"
}

func (e Env) AdminURL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%s%s", e.EnvoyAdminPort, path)
}

func requireEnvFile(root string) error {
	path := filepath.Join(root, ".env")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("缺少 .env，请先: relaygate setup 或 cp .env.example .env")
	}
	return nil
}
