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

// Profile is a named preset under packaging/profiles/*.yaml.
type Profile struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Scenario    string             `yaml:"scenario,omitempty"`
	Defaults    resources.Defaults `yaml:"defaults"`
	Security    resources.Security `yaml:"security,omitempty"`
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
		p.Security.EnsureSecurityDefaults()
		return &p, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("profile 不存在: %s (%v)", name, lastErr)
	}
	return nil, fmt.Errorf("profile 不存在: %s", name)
}

// MergeSecurityInto overlays profile security policies onto dst (Panel scenario picker).
func MergeSecurityInto(dst *resources.Security, src resources.Security) {
	mergeProfileSecurity(dst, src)
}

func mergeProfileSecurity(dst *resources.Security, src resources.Security) {
	dst.EnsureSecurityDefaults()
	if len(src.Policies) == 0 {
		return
	}
	byID := map[string]int{}
	for i, p := range dst.Policies {
		byID[p.ID] = i
	}
	for _, sp := range src.Policies {
		if idx, ok := byID[sp.ID]; ok {
			if sp.Enabled != dst.Policies[idx].Enabled {
				dst.Policies[idx].Enabled = sp.Enabled
			}
			mergePolicyParams(&dst.Policies[idx].Params, sp.Params, sp.ID)
		}
	}
}

func mergePolicyParams(dst *resources.PolicyParams, src resources.PolicyParams, id string) {
	switch id {
	case resources.PolicyFirewallNewConnLimit:
		if src.TCPPerIP != "" {
			dst.TCPPerIP = src.TCPPerIP
		}
		if src.Burst > 0 {
			dst.Burst = src.Burst
		}
	case resources.PolicyGatewayNewConnLimit:
		if src.PerSec > 0 {
			dst.PerSec = src.PerSec
		}
		if src.Burst > 0 {
			dst.Burst = src.Burst
		}
	case resources.PolicyConnLimit:
		if src.MaxConnections > 0 {
			dst.MaxConnections = src.MaxConnections
		}
	case resources.PolicyUDPLimit:
		if src.UDPPPSPerIP != "" {
			dst.UDPPPSPerIP = src.UDPPPSPerIP
		}
		if src.UDPBurst > 0 {
			dst.UDPBurst = src.UDPBurst
		}
	}
}

// Preview diffs profile against current resources without writing.
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
	mergeProfileSecurity(&after.Security, p.Security)
	if err := after.Validate(); err != nil {
		return nil, fmt.Errorf("preview 校验失败: %w", err)
	}
	sum := resources.Diff(&before, &after)
	sum.Note = fmt.Sprintf("profile preview %s", p.Name)
	return &sum, nil
}

// Apply merges profile into resources.yaml.
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
	mergeProfileSecurity(&res.Security, p.Security)
	if err := res.Validate(); err != nil {
		return nil, fmt.Errorf("apply 后校验失败: %w", err)
	}
	if err := resources.Save(resPath, res); err != nil {
		return nil, err
	}
	sum := resources.Diff(&before, res)
	sum.Note = fmt.Sprintf("profile apply %s", p.Name)
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
	fmt.Fprintf(&b, "  max_pending_requests: %d\n", d.MaxPendingRequests)
	p.Security.EnsureSecurityDefaults()
	for _, pol := range p.Security.Policies {
		fmt.Fprintf(&b, "security.policies.%s: enabled=%t\n", pol.ID, pol.Enabled)
	}
	return b.String()
}
