package assets

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Resources struct {
	Meta     Meta      `yaml:"meta"`
	Gateway  Gateway   `yaml:"gateway"`
	Defaults Defaults  `yaml:"defaults"`
	Servers  []Server  `yaml:"servers"`
	Rules    []Rule    `yaml:"rules"`
}

type Meta struct {
	GatewayName  string `yaml:"gateway_name"`
	GameName     string `yaml:"game_name"`
	EnvoyImage   string `yaml:"envoy_image"`
	AdminPort    int    `yaml:"admin_port"`
	AdminAddress string `yaml:"admin_address"`
}

type Gateway struct {
	Name          string `yaml:"name"`
	PublicIP      string `yaml:"public_ip"`
	SSHPort       int    `yaml:"ssh_port"`
	ListenAddress string `yaml:"listen_address"`
}

type Defaults struct {
	BackendTCPPort            int         `yaml:"backend_tcp_port"`
	BackendUDPPort            int         `yaml:"backend_udp_port"`
	TCPIdleTimeout            string      `yaml:"tcp_idle_timeout"`
	UDPIdleTimeout            string      `yaml:"udp_idle_timeout"`
	MaxConnections            int         `yaml:"max_connections"`
	MaxPendingRequests        int         `yaml:"max_pending_requests"`
	TCPLocalRateLimitPerSec   int         `yaml:"tcp_local_rate_limit_per_sec"`
	TCPLocalRateLimitBurst    int         `yaml:"tcp_local_rate_limit_burst"`
	HealthCheck               HealthCheck `yaml:"health_check"`
}

type HealthCheck struct {
	Timeout            string `yaml:"timeout"`
	Interval           string `yaml:"interval"`
	UnhealthyThreshold int    `yaml:"unhealthy_threshold"`
	HealthyThreshold   int    `yaml:"healthy_threshold"`
}

type Server struct {
	Name            string `yaml:"name"`
	Address         string `yaml:"address"`
	TCPPort         int    `yaml:"tcp_port"`
	UDPPort         int    `yaml:"udp_port"`
	HealthCheckPort int    `yaml:"health_check_port"`
	Enabled         bool   `yaml:"enabled"`
}

type Rule struct {
	Name       string `yaml:"name"`
	Kind       string `yaml:"kind"`
	Server     string `yaml:"server"`
	Protocol   string `yaml:"protocol"`
	ListenPort int    `yaml:"listen_port"`
	Enabled    bool   `yaml:"enabled"`
}

func DefaultPaths(root string) (resources, envoyOut, nftOut string) {
	return filepath.Join(root, "config", "resources.yaml"),
		filepath.Join(root, "gateway", "generated", "envoy.yaml"),
		filepath.Join(root, "deploy", "firewall", "generated", "game-ports.nft")
}

func Load(path string) (*Resources, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read resources: %w", err)
	}
	var r Resources
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse resources: %w", err)
	}
	return &r, nil
}

func (r *Resources) ServerMap() map[string]Server {
	m := make(map[string]Server, len(r.Servers))
	for _, s := range r.Servers {
		m[s.Name] = s
	}
	return m
}

func (r *Resources) EnabledRules() []Rule {
	out := make([]Rule, 0, len(r.Rules))
	for _, rule := range r.Rules {
		if rule.Enabled {
			out = append(out, rule)
		}
	}
	return out
}

func (r *Resources) Validate() error {
	servers := r.ServerMap()
	if len(servers) == 0 {
		return fmt.Errorf("servers 不能为空")
	}
	for name, s := range servers {
		if ip := net.ParseIP(s.Address); ip == nil {
			return fmt.Errorf("%s address 无效: %s", name, s.Address)
		}
		if s.TCPPort < 1 || s.TCPPort > 65535 {
			return fmt.Errorf("%s.tcp_port 端口越界: %d", name, s.TCPPort)
		}
		if s.UDPPort < 1 || s.UDPPort > 65535 {
			return fmt.Errorf("%s.udp_port 端口越界: %d", name, s.UDPPort)
		}
	}

	seen := map[string]string{}
	for _, rule := range r.EnabledRules() {
		proto := strings.ToUpper(rule.Protocol)
		if proto != "TCP" && proto != "UDP" {
			return fmt.Errorf("%s: protocol 必须是 TCP 或 UDP", rule.Name)
		}
		if rule.ListenPort < 1 || rule.ListenPort > 65535 {
			return fmt.Errorf("%s: listen_port 越界: %d", rule.Name, rule.ListenPort)
		}
		key := fmt.Sprintf("%s/%d", proto, rule.ListenPort)
		if other, ok := seen[key]; ok {
			return fmt.Errorf("端口冲突: %s 同时被 %s 与 %s 使用", key, other, rule.Name)
		}
		seen[key] = rule.Name
		s, ok := servers[rule.Server]
		if !ok {
			return fmt.Errorf("%s: 未知 server %s", rule.Name, rule.Server)
		}
		if !s.Enabled {
			return fmt.Errorf("%s: 目标 %s 已禁用", rule.Name, rule.Server)
		}
	}
	if len(r.EnabledRules()) == 0 {
		return fmt.Errorf("没有启用的 rules；至少启用 canary 规则后再渲染")
	}
	return nil
}

func (r *Resources) UpdateServer(name string, address string, tcpPort, udpPort, healthPort int, enabled bool) error {
	for i := range r.Servers {
		if r.Servers[i].Name != name {
			continue
		}
		if address != "" {
			r.Servers[i].Address = address
		}
		if tcpPort > 0 {
			r.Servers[i].TCPPort = tcpPort
		}
		if udpPort > 0 {
			r.Servers[i].UDPPort = udpPort
		}
		if healthPort > 0 {
			r.Servers[i].HealthCheckPort = healthPort
		}
		r.Servers[i].Enabled = enabled
		return nil
	}
	return fmt.Errorf("server not found: %s", name)
}

func (r *Resources) SetRuleEnabled(name string, enabled bool) error {
	for i := range r.Rules {
		if r.Rules[i].Name == name {
			r.Rules[i].Enabled = enabled
			return nil
		}
	}
	return fmt.Errorf("rule not found: %s", name)
}

func (r *Resources) EnableProductionForServer(server string, enabled bool) (changed int, err error) {
	found := false
	for i := range r.Rules {
		if r.Rules[i].Kind != "production" {
			continue
		}
		if server != "" && r.Rules[i].Server != server {
			continue
		}
		found = true
		if r.Rules[i].Enabled != enabled {
			r.Rules[i].Enabled = enabled
			changed++
		}
	}
	if !found {
		return 0, fmt.Errorf("没有匹配的 production 规则")
	}
	return changed, nil
}

// SavePreserveComments updates enabled/address fields via regex where possible for rules,
// and rewrites the full document for server field updates (comments on rules mostly preserved for enable toggles).
func Save(path string, r *Resources) error {
	// Full rewrite is simpler and reliable for panel edits.
	b, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	header := "# 由 gateway-panel / gateway-render 写入\n# 源文件可继续手工编辑后重新 apply\n"
	return os.WriteFile(path, append([]byte(header), b...), 0o644)
}

// PatchRuleEnabledInPlace toggles enabled for a named rule while preserving surrounding comments.
func PatchRuleEnabledInPlace(path, ruleName string, enabled bool) (bool, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	val := "false"
	if enabled {
		val = "true"
	}
	pattern := regexp.MustCompile(
		`(?m)(^[ \t]*- name:\s*` + regexp.QuoteMeta(ruleName) + `\s*\n(?:^[ \t]+.*\n)*?^[ \t]+enabled:\s*)(true|false)`,
	)
	changed := false
	out := pattern.ReplaceAllStringFunc(string(text), func(m string) string {
		sub := pattern.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		if sub[2] != val {
			changed = true
		}
		return sub[1] + val
	})
	if !pattern.MatchString(string(text)) {
		return false, fmt.Errorf("未能定位规则块: %s", ruleName)
	}
	if !changed {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(out), 0o644)
}

func FindRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "config", "resources.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if env := os.Getenv("PANEL_ROOT"); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("cannot locate repo root (config/resources.yaml)")
}

func AbsJoin(root, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, rel)
}

func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func BackupFiles(root, stamp string, files ...string) (string, error) {
	dir := filepath.Join(root, "backups", stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		name := filepath.Base(f)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return "", err
		}
	}
	_ = os.WriteFile(filepath.Join(root, "backups", "latest"), []byte(stamp+"\n"), 0o644)
	return dir, nil
}

func BoolPtr(v bool) *bool { return &v }
