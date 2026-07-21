package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
	"gopkg.in/yaml.v3"
)

// Profile is a named defaults preset under packaging/profiles/*.yaml.
type Profile struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Defaults    resources.Defaults  `yaml:"defaults"`
}

// Dir returns packaging/profiles absolute path.
func Dir(root string) string {
	return filepath.Join(config.ResolvePaths(root).Packaging, "profiles")
}

// List returns sorted profile basenames (without .yaml).
func List(root string) ([]string, error) {
	dir := Dir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("缺少 profiles 目录: %s", dir)
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		names = append(names, base)
	}
	sort.Strings(names)
	return names, nil
}

// Load reads a profile by name (basename without extension).
func Load(root, name string) (*Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("profile 名称不能为空")
	}
	dir := Dir(root)
	candidates := []string{
		filepath.Join(dir, name+".yaml"),
		filepath.Join(dir, name+".yml"),
	}
	var lastErr error
	for _, path := range candidates {
		b, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		var p Profile
		if err := yaml.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if strings.TrimSpace(p.Name) == "" {
			p.Name = name
		}
		p.Defaults.ApplyNftablesDefaults()
		return &p, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("profile 不存在: %s (%v)", name, lastErr)
	}
	return nil, fmt.Errorf("profile 不存在: %s", name)
}

// Preview diffs profile defaults against current resources without writing.
func Preview(root, name string) (*resources.ChangeSummary, error) {
	p, err := Load(root, name)
	if err != nil {
		return nil, err
	}
	resPath := config.ResolvePaths(root).Resources
	res, err := resources.Load(resPath)
	if err != nil {
		return nil, err
	}
	before := *res
	after := *res
	after.Defaults = p.Defaults
	after.Defaults.ApplyNftablesDefaults()
	if err := after.Validate(); err != nil {
		return nil, fmt.Errorf("preview 校验失败: %w", err)
	}
	sum := resources.Diff(&before, &after)
	sum.Note = fmt.Sprintf("profile preview %s → defaults", p.Name)
	return &sum, nil
}

// Apply merges profile defaults into resources.yaml (replace defaults section fields from profile).
func Apply(root, name string) (*resources.ChangeSummary, error) {
	p, err := Load(root, name)
	if err != nil {
		return nil, err
	}
	resPath := config.ResolvePaths(root).Resources
	res, err := resources.Load(resPath)
	if err != nil {
		return nil, err
	}
	before := *res
	res.Defaults = p.Defaults
	res.Defaults.ApplyNftablesDefaults()
	if err := res.Validate(); err != nil {
		return nil, fmt.Errorf("apply 后校验失败: %w", err)
	}
	if err := resources.Save(resPath, res); err != nil {
		return nil, err
	}
	sum := resources.Diff(&before, res)
	sum.Note = fmt.Sprintf("profile apply %s → defaults", p.Name)
	return &sum, nil
}

// FormatShow returns human-readable profile details.
func FormatShow(p *Profile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n", p.Name)
	if p.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", p.Description)
	}
	d := p.Defaults
	fmt.Fprintf(&b, "defaults:\n")
	fmt.Fprintf(&b, "  tcp_idle_timeout: %s\n", d.TCPIdleTimeout)
	fmt.Fprintf(&b, "  udp_idle_timeout: %s\n", d.UDPIdleTimeout)
	fmt.Fprintf(&b, "  max_connections: %d\n", d.MaxConnections)
	fmt.Fprintf(&b, "  max_pending_requests: %d\n", d.MaxPendingRequests)
	fmt.Fprintf(&b, "  tcp_local_rate_limit_per_sec: %d\n", d.TCPLocalRateLimitPerSec)
	fmt.Fprintf(&b, "  tcp_local_rate_limit_burst: %d\n", d.TCPLocalRateLimitBurst)
	fmt.Fprintf(&b, "  nftables.tcp_new_conn_per_ip: %s\n", d.Nftables.TCPNewConnPerIP)
	fmt.Fprintf(&b, "  nftables.udp_pps_per_ip: %s\n", d.Nftables.UDPPPSPerIP)
	fmt.Fprintf(&b, "  nftables.tcp_burst: %d\n", d.Nftables.TCPBurst)
	fmt.Fprintf(&b, "  nftables.udp_burst: %d\n", d.Nftables.UDPBurst)
	return b.String()
}
