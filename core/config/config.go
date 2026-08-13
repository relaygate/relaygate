// Package config is the single entry for RelayGate paths, defaults, and env loading.
//
// Layout conventions:
//   - ProductRoot: install prefix (/opt/relaygate) or git checkout (packaging/, ui/, …)
//   - DataDir: runtime state only — never part of the source tree layout
//   - PackagingDir: versioned install assets under packaging/
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Production / documented defaults (keep in sync with README and *.example).
const (
	DefaultInstallDir = "/opt/relaygate"
	DefaultSecretsDir = "/etc/relaygate/secrets"
	DefaultSSHPort    = "22"
	DefaultPanelBind  = "0.0.0.0:9000"

	PackagingDirName = "packaging"
	// SharedDirName holds role-agnostic templates (resources, default env).
	SharedDirName   = "shared"
	ComposeFileRel  = "packaging/compose.yaml"
	TemplatesDirRel = "packaging/shared"
	ControlEnvRel   = "packaging/control/env.example"
	NodeEnvRel      = "packaging/node/env.example"

	// InstallDataDirName is the runtime directory name under an install prefix.
	InstallDataDirName = "data"
	// DevDataDirName is the runtime directory under a source checkout (gitignored).
	DevDataDirName = ".runtime"

	// DefaultDrainWaitSec matches packaging/terraform/nlb health_check:
	// unhealthy_threshold (3) × interval (10s) = 30s. Keep in sync with that template.
	DefaultDrainWaitSec = 30
	// RecommendedDrainWaitSec is the minimum advised DRAIN_WAIT under NLB / dual-active.
	RecommendedDrainWaitSec = DefaultDrainWaitSec

	// DefaultXDSEnabled is the .env fallback when XDS_ENABLED is unset (1 = HotApply path).
	DefaultXDSEnabled = "1"
)

// Getenv returns the trimmed env value or fallback when unset/blank.
func Getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// IsSourceTree reports whether root looks like a git/source checkout
// (has Go package sources), as opposed to a release install prefix.
func IsSourceTree(root string) bool {
	_, err := os.Stat(filepath.Join(root, "core", "cmd", "relaygate"))
	return err == nil
}

// ResolveDataDir returns the absolute runtime data directory for root.
//
// Priority:
//  1. RELAYGATE_DATA_DIR (absolute, or relative to root)
//  2. Source checkout → <root>/.runtime
//  3. Install prefix → <root>/data
func ResolveDataDir(root string) string {
	if v := strings.TrimSpace(os.Getenv("RELAYGATE_DATA_DIR")); v != "" {
		if filepath.IsAbs(v) {
			return filepath.Clean(v)
		}
		return filepath.Clean(filepath.Join(root, v))
	}
	if IsSourceTree(root) {
		return filepath.Join(root, DevDataDirName)
	}
	return filepath.Join(root, InstallDataDirName)
}

// Paths holds resolved product + runtime locations for one root.
type Paths struct {
	Root         string
	DataDir      string
	Packaging    string
	Compose      string
	Resources    string
	EnvoyYAML    string
	ForwardPorts string
	Firewall     string
	PromYAML     string
	Inventory    string
	Backups      string
}

// ResolvePaths builds Paths for a product root.
func ResolvePaths(root string) Paths {
	data := ResolveDataDir(root)
	return Paths{
		Root:         root,
		DataDir:      data,
		Packaging:    filepath.Join(root, PackagingDirName),
		Compose:      filepath.Join(root, ComposeFileRel),
		Resources:    filepath.Join(data, "resources.yaml"),
		EnvoyYAML:    filepath.Join(data, "envoy", "envoy.yaml"),
		ForwardPorts: filepath.Join(data, "firewall", "forward-ports.nft"),
		Firewall:     filepath.Join(data, "firewall"),
		PromYAML:     filepath.Join(data, "prometheus", "prometheus.yml"),
		Inventory:    filepath.Join(data, "inventory", "gateways.env"),
		Backups:      filepath.Join(data, "backups"),
	}
}

// FindRoot walks from cwd for a product root. Prefer PANEL_ROOT when set.
// Markers are versioned checkout/install files (not runtime data).
func FindRoot() (string, error) {
	if env := strings.TrimSpace(os.Getenv("PANEL_ROOT")); env != "" {
		return filepath.Clean(env), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	markers := []string{
		filepath.Join(TemplatesDirRel, "resources.example.yaml"),
		filepath.Join(PackagingDirName, "compose.yaml"),
		"go.mod",
	}
	dir := wd
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("cannot locate product root (%s/resources.example.yaml, packaging/compose.yaml, or go.mod)", TemplatesDirRel)
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
	PanelEnabled             string // PANEL_ENABLED (1=主控 Panel；0=纯节点)
	GrafanaEnabled           string // GRAFANA_ENABLED (1=启 with-grafana；空则看 COMPOSE_PROFILES)
	ImageTag                 string
	SecretsDir               string
	GrafanaURL               string
	DataDir                  string
	DrainWait                int
	ApplyFirewall            string // APPLY_FIREWALL：安装/CLI 一次性落地防火墙（≠ SECURITY_AUTO_APPLY）
	NonInteractive           string
	FirewallConfirm          string
	TCPPort                  string
	UDPPort                  string
	Timeout                  string
	// XDSEnabled gates HotApply. Default on; unset/1 = on. Escape hatch: XDS_ENABLED=0 → HardReload.
	XDSEnabled bool
	// XDSPort is the loopback ADS listen port (default 18000).
	XDSPort string
	// SecurityAutoApply controls host-side kernel + firewall after agent pull.
	// Empty = default: on only when PANEL_ENABLED=0 (pure node); off on control (PANEL_ENABLED=1).
	// Explicit 1/0 overrides the default. Never auto-apply host domains on production control unless set to 1.
	SecurityAutoApply string
	Raw               map[string]string
}

// LoadEnv loads root/.env (if present) then reads known keys.
// It also exports RELAYGATE_DATA_DIR into the process env when resolved,
// so docker compose volume mounts can expand ${RELAYGATE_DATA_DIR}.
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
	dataDir := ResolveDataDir(root)
	_ = os.Setenv("RELAYGATE_DATA_DIR", dataDir)

	drainWait := DefaultDrainWaitSec
	if v := Getenv("DRAIN_WAIT", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			drainWait = n
		}
	}
	return Env{
		GatewayName:              Getenv("GATEWAY_NAME", "gateway-01"),
		GatewayPublicIP:          Getenv("GATEWAY_PUBLIC_IP", "127.0.0.1"),
		GatewaySSHPort:           Getenv("GATEWAY_SSH_PORT", Getenv("SSH_PORT", DefaultSSHPort)),
		EnvoyAdminPort:           Getenv("ENVOY_ADMIN_PORT", "9901"),
		EnvoyImage:               Getenv("ENVOY_IMAGE", "envoyproxy/envoy:v1.39.0"),
		PrometheusRemoteWriteURL: Getenv("PROMETHEUS_REMOTE_WRITE_URL", ""),
		ComposeProjectName:       Getenv("COMPOSE_PROJECT_NAME", ""),
		PanelRole:                Getenv("PANEL_ROLE", "primary"),
		PanelEnabled:             Getenv("PANEL_ENABLED", "1"),
		GrafanaEnabled:           Getenv("GRAFANA_ENABLED", ""),
		ImageTag:                 Getenv("IMAGE_TAG", "local"),
		SecretsDir:               Getenv("RELAYGATE_SECRETS_DIR", DefaultSecretsDir),
		GrafanaURL:               Getenv("GRAFANA_URL", ""),
		DataDir:                  dataDir,
		DrainWait:                drainWait,
		ApplyFirewall:            Getenv("APPLY_FIREWALL", "0"),
		NonInteractive:           Getenv("NONINTERACTIVE", "0"),
		FirewallConfirm:          Getenv("FIREWALL_CONFIRM", ""),
		TCPPort:                  Getenv("TCP_PORT", "11001"),
		UDPPort:                  Getenv("UDP_PORT", "11001"),
		Timeout:                  Getenv("TIMEOUT", "3"),
		XDSEnabled:               XDSEnabledFromEnv(),
		XDSPort:                  Getenv("XDS_PORT", "18000"),
		SecurityAutoApply:        Getenv("SECURITY_AUTO_APPLY", ""),
		Raw:                      raw,
	}, nil
}

// HostSecurityAutoApply reports whether agent pull should apply host-side
// kernel, nic, and firewall domains (in addition to gateway HotApply).
// Explicit SECURITY_AUTO_APPLY wins; when unset, defaults to true only for
// pure nodes (PANEL_ENABLED=0). Control hosts (PANEL_ENABLED=1) stay off by default
// so upgrades never rewrite firewall/kernel/nic qdisc on the production control plane.
func (e Env) HostSecurityAutoApply() bool {
	switch strings.ToLower(strings.TrimSpace(e.SecurityAutoApply)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return strings.TrimSpace(e.PanelEnabled) == "0"
	}
}

// XDSEnabledFromEnv reads XDS_ENABLED with DefaultXDSEnabled when unset.
func XDSEnabledFromEnv() bool {
	return envFlagEnabled(Getenv("XDS_ENABLED", DefaultXDSEnabled))
}

// envFlagEnabled treats 1/true/yes/on as enabled; empty and all other values are off.
func envFlagEnabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (e Env) EnvoyContainer() string {
	return e.GatewayName + "-envoy"
}

func (e Env) AdminURL(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%s%s", e.EnvoyAdminPort, path)
}
