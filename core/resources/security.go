package resources

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// Policy IDs — keys in security.policies[] (resources.yaml). No legacy aliases.
const (
	PolicyKernelSyn            = "kernel_syn"
	PolicyFirewallNewConnLimit = "firewall_new_conn_limit"
	PolicyGatewayNewConnLimit  = "gateway_new_conn_limit"
	PolicyConnLimit            = "conn_limit"
	PolicyAllowlist            = "allowlist"
	PolicyUDPLimit             = "udp_limit"
)

// PolicyType classifies the policy (domain-aligned; no component aliases).
const (
	PolicyTypeKernel               = "kernel"
	PolicyTypeFirewallNewConnLimit = "new_conn_limit_firewall"
	PolicyTypeGatewayNewConnLimit  = "new_conn_limit_gateway"
	PolicyTypeConnLimit            = "conn_limit"
	PolicyTypeAllowlist            = "allowlist"
	PolicyTypeUDPLimit             = "udp_limit"
)

// PolicyLayer is the product security domain (kernel / firewall / nic / gateway).
// Execution components (sysctl, nftables, Envoy) appear only in implementation detail.
type PolicyLayer string

const (
	LayerKernel   PolicyLayer = "kernel"
	LayerFirewall PolicyLayer = "firewall"
	LayerNIC      PolicyLayer = "nic" // reserved; not implemented
	LayerGateway  PolicyLayer = "gateway"
)

// PolicyApplyPath tells operators which apply path a toggle change needs.
type PolicyApplyPath string

const (
	ApplyHostScript     PolicyApplyPath = "host_script"
	ApplyFirewall       PolicyApplyPath = "firewall"
	ApplyReload         PolicyApplyPath = "reload"
	ApplyFirewallReload PolicyApplyPath = "firewall_reload"
)

// Effective values when a policy is disabled (do not rate-limit established TCP).
const (
	DisabledFirewallRatePerIP = "999999/second"
	DisabledFirewallBurst     = 999999
	UnlimitedMaxConnections   = 1048576
)

// Security groups all protection policies (unified model).
type Security struct {
	Policies []SecurityPolicy `yaml:"policies" json:"policies"`
}

// SecurityPolicy is one configurable protection rule.
type SecurityPolicy struct {
	ID         string       `yaml:"id" json:"id"`
	Type       string       `yaml:"type" json:"type"`
	Enabled    bool         `yaml:"enabled" json:"enabled"`
	AttackTags []string     `yaml:"attack_tags,omitempty" json:"attack_tags,omitempty"`
	Params     PolicyParams `yaml:"params,omitempty" json:"params,omitempty"`
}

// PolicyParams holds type-specific parameters.
type PolicyParams struct {
	// allowlist
	Deny  []string `yaml:"deny,omitempty" json:"deny,omitempty"`
	Allow []string `yaml:"allow,omitempty" json:"allow,omitempty"`
	// new_conn_limit_firewall / new_conn_limit_gateway
	TCPPerIP string `yaml:"tcp_per_ip,omitempty" json:"tcp_per_ip,omitempty"`
	PerSec   int    `yaml:"per_sec,omitempty" json:"per_sec,omitempty"`
	Burst    int    `yaml:"burst,omitempty" json:"burst,omitempty"`
	// conn_limit
	MaxConnections int `yaml:"max_connections,omitempty" json:"max_connections,omitempty"`
	// udp_limit
	UDPPPSPerIP string `yaml:"udp_pps_per_ip,omitempty" json:"udp_pps_per_ip,omitempty"`
	UDPBurst    int    `yaml:"udp_burst,omitempty" json:"udp_burst,omitempty"`
	// kernel_syn
	TcpSyncookies      int `yaml:"tcp_syncookies,omitempty" json:"tcp_syncookies,omitempty"`
	TcpMaxSynBacklog   int `yaml:"tcp_max_syn_backlog,omitempty" json:"tcp_max_syn_backlog,omitempty"`
	TcpSynackRetries   int `yaml:"tcp_synack_retries,omitempty" json:"tcp_synack_retries,omitempty"`
	TcpSynRetries      int `yaml:"tcp_syn_retries,omitempty" json:"tcp_syn_retries,omitempty"`
	TcpAbortOnOverflow int `yaml:"tcp_abort_on_overflow,omitempty" json:"tcp_abort_on_overflow,omitempty"`
}

// FirewallRateDefaults drives host nftables per-IP rate limits (effective values).
type FirewallRateDefaults struct {
	TCPNewConnPerIP string
	UDPPPSPerIP     string
	TCPBurst        int
	UDPBurst        int
}

// PolicyMeta is catalog metadata (not persisted in YAML).
type PolicyMeta struct {
	ID            string
	Type          string
	Threats       []string
	Layer         PolicyLayer
	ApplyPath     PolicyApplyPath
	NameZH        string
	NameEN        string
	DescriptionZH string
	DescriptionEN string
}

// SecurityPolicyCatalog is the product catalog of configurable protections.
var SecurityPolicyCatalog = []PolicyMeta{
	{
		ID: PolicyKernelSyn, Type: PolicyTypeKernel, Threats: []string{"T1"},
		Layer: LayerKernel, ApplyPath: ApplyHostScript,
		NameZH: "SYN 洪泛加固", NameEN: "SYN flood hardening",
		DescriptionZH: "内核 SYN cookies 与握手队列（仅影响新建握手，不伤 established 长连接）。",
		DescriptionEN: "Kernel SYN cookies and handshake queues (new handshakes only; established sessions unaffected).",
	},
	{
		ID: PolicyFirewallNewConnLimit, Type: PolicyTypeFirewallNewConnLimit, Threats: []string{"T1", "T4"},
		Layer: LayerFirewall, ApplyPath: ApplyFirewallReload,
		NameZH: "防火墙新建连接限速", NameEN: "Firewall new-connection rate limit",
		DescriptionZH: "主机防火墙对 ct state new 每 IP 限速；已建立会话先放行，不做 PPS 限速。",
		DescriptionEN: "Host firewall per-IP ct state new limits; established sessions accepted first — no PPS throttling.",
	},
	{
		ID: PolicyGatewayNewConnLimit, Type: PolicyTypeGatewayNewConnLimit, Threats: []string{"T1", "T4"},
		Layer: LayerGateway, ApplyPath: ApplyReload,
		NameZH: "网关新建连接限速", NameEN: "Gateway new-connection rate limit",
		DescriptionZH: "网关 listener 本地新建连接令牌桶。",
		DescriptionEN: "Gateway listener local new-connection token bucket.",
	},
	{
		ID: PolicyConnLimit, Type: PolicyTypeConnLimit, Threats: []string{"T2"},
		Layer: LayerGateway, ApplyPath: ApplyReload,
		NameZH: "并发连接上限", NameEN: "Connection limit",
		DescriptionZH: "网关 circuit breaker max_connections，防连接耗尽占满转发槽位。",
		DescriptionEN: "Gateway circuit breaker max_connections to mitigate connection exhaustion.",
	},
	{
		ID: PolicyAllowlist, Type: PolicyTypeAllowlist, Threats: []string{"T5"},
		Layer: LayerFirewall, ApplyPath: ApplyFirewall,
		NameZH: "来源访问控制", NameEN: "Source allowlist",
		DescriptionZH: "防火墙来源 allowlist/denylist；默认丢弃未放行来源。",
		DescriptionEN: "Firewall source allowlist/denylist; drops non-allowed sources when configured.",
	},
	{
		ID: PolicyUDPLimit, Type: PolicyTypeUDPLimit, Threats: []string{"T6"},
		Layer: LayerFirewall, ApplyPath: ApplyFirewall,
		NameZH: "UDP 包速限制", NameEN: "UDP PPS limit",
		DescriptionZH: "每源 IP UDP 包速率上限，缓解反射/扫描噪声（非抗带宽型 DDoS）。",
		DescriptionEN: "Per-source UDP packet rate cap; mitigates reflection/scan noise (not volumetric DDoS).",
	},
}

// DefaultSecurity returns the product-default policy set.
func DefaultSecurity() Security {
	return Security{Policies: []SecurityPolicy{
		{
			ID: PolicyKernelSyn, Type: PolicyTypeKernel, Enabled: true, AttackTags: []string{"T1"},
			Params: PolicyParams{
				TcpSyncookies:      DefaultTcpSyncookies,
				TcpMaxSynBacklog:   DefaultTcpMaxSynBacklog,
				TcpSynackRetries:   DefaultTcpSynackRetries,
				TcpSynRetries:      DefaultTcpSynRetries,
				TcpAbortOnOverflow: DefaultTcpAbortOnOverflow,
			},
		},
		{
			ID: PolicyFirewallNewConnLimit, Type: PolicyTypeFirewallNewConnLimit, Enabled: true, AttackTags: []string{"T1", "T4"},
			Params: PolicyParams{TCPPerIP: "30/second", Burst: 60},
		},
		{
			ID: PolicyGatewayNewConnLimit, Type: PolicyTypeGatewayNewConnLimit, Enabled: true, AttackTags: []string{"T1", "T4"},
			Params: PolicyParams{PerSec: 200, Burst: 400},
		},
		{
			ID: PolicyConnLimit, Type: PolicyTypeConnLimit, Enabled: true, AttackTags: []string{"T2"},
			Params: PolicyParams{MaxConnections: 1024},
		},
		{
			ID: PolicyAllowlist, Type: PolicyTypeAllowlist, Enabled: true, AttackTags: []string{"T5"},
			Params: PolicyParams{Deny: []string{}, Allow: []string{}},
		},
		{
			ID: PolicyUDPLimit, Type: PolicyTypeUDPLimit, Enabled: true, AttackTags: []string{"T6"},
			Params: PolicyParams{UDPPPSPerIP: "500/second", UDPBurst: 1000},
		},
	}}
}

// EnsureSecurityDefaults fills missing built-in policies and applies param defaults.
func (s *Security) EnsureSecurityDefaults() {
	if s == nil {
		return
	}
	def := DefaultSecurity()
	byID := map[string]int{}
	for i, p := range s.Policies {
		byID[p.ID] = i
	}
	for _, d := range def.Policies {
		if idx, ok := byID[d.ID]; ok {
			s.Policies[idx].applyParamDefaults(d)
			if s.Policies[idx].Type == "" {
				s.Policies[idx].Type = d.Type
			}
			if len(s.Policies[idx].AttackTags) == 0 {
				s.Policies[idx].AttackTags = d.AttackTags
			}
		} else {
			s.Policies = append(s.Policies, d)
		}
	}
	sort.Slice(s.Policies, func(i, j int) bool {
		return policyOrder(s.Policies[i].ID) < policyOrder(s.Policies[j].ID)
	})
}

func policyOrder(id string) int {
	for i, m := range SecurityPolicyCatalog {
		if m.ID == id {
			return i
		}
	}
	return 99
}

func (p *SecurityPolicy) applyParamDefaults(d SecurityPolicy) {
	switch p.ID {
	case PolicyFirewallNewConnLimit:
		if strings.TrimSpace(p.Params.TCPPerIP) == "" {
			p.Params.TCPPerIP = d.Params.TCPPerIP
		}
		if p.Params.Burst <= 0 {
			p.Params.Burst = d.Params.Burst
		}
	case PolicyGatewayNewConnLimit:
		if p.Params.PerSec <= 0 {
			p.Params.PerSec = d.Params.PerSec
		}
		if p.Params.Burst <= 0 {
			p.Params.Burst = d.Params.Burst
		}
	case PolicyConnLimit:
		if p.Params.MaxConnections <= 0 {
			p.Params.MaxConnections = d.Params.MaxConnections
		}
	case PolicyUDPLimit:
		if strings.TrimSpace(p.Params.UDPPPSPerIP) == "" {
			p.Params.UDPPPSPerIP = d.Params.UDPPPSPerIP
		}
		if p.Params.UDPBurst <= 0 {
			p.Params.UDPBurst = d.Params.UDPBurst
		}
	case PolicyKernelSyn:
		applyKernelSynDefaults(p, d)
	}
}

// PolicyByID returns a policy pointer or nil.
func (s *Security) PolicyByID(id string) *SecurityPolicy {
	if s == nil {
		return nil
	}
	for i := range s.Policies {
		if s.Policies[i].ID == id {
			return &s.Policies[i]
		}
	}
	return nil
}

// PolicyEnabled reports whether policy id is on (unknown id → true).
func (s *Security) PolicyEnabled(id string) bool {
	p := s.PolicyByID(id)
	if p == nil {
		return true
	}
	return p.Enabled
}

// EffectiveFirewallRates returns nftables rate limits with disabled policies → permissive.
// Enforcement order on the host: INPUT established,related accept → ACL deny → ACL allow strict
// → new TCP / UDP rate limits (this function's values). See SecurityExecutionOrder().
func (s *Security) EffectiveFirewallRates() FirewallRateDefaults {
	s.EnsureSecurityDefaults()
	n := FirewallRateDefaults{
		TCPNewConnPerIP: "30/second",
		TCPBurst:        60,
		UDPPPSPerIP:     "500/second",
		UDPBurst:        1000,
	}
	if p := s.PolicyByID(PolicyFirewallNewConnLimit); p != nil && p.Enabled {
		if v := strings.TrimSpace(p.Params.TCPPerIP); v != "" {
			n.TCPNewConnPerIP = v
		}
		if p.Params.Burst > 0 {
			n.TCPBurst = p.Params.Burst
		}
	} else {
		n.TCPNewConnPerIP = DisabledFirewallRatePerIP
		n.TCPBurst = DisabledFirewallBurst
	}
	if p := s.PolicyByID(PolicyUDPLimit); p != nil && p.Enabled {
		if v := strings.TrimSpace(p.Params.UDPPPSPerIP); v != "" {
			n.UDPPPSPerIP = v
		}
		if p.Params.UDPBurst > 0 {
			n.UDPBurst = p.Params.UDPBurst
		}
	} else {
		n.UDPPPSPerIP = DisabledFirewallRatePerIP
		n.UDPBurst = DisabledFirewallBurst
	}
	return n
}

// EffectiveMaxConnections returns max_connections honoring conn_limit policy.
func (s *Security) EffectiveMaxConnections() int {
	s.EnsureSecurityDefaults()
	p := s.PolicyByID(PolicyConnLimit)
	if p == nil || !p.Enabled {
		return UnlimitedMaxConnections
	}
	if p.Params.MaxConnections > 0 {
		return p.Params.MaxConnections
	}
	return 1024
}

// EffectiveTCPLocalRateLimit returns per-sec/burst for Envoy local_ratelimit (0,0 = omit filter).
// Runs after host nft INPUT limits; both layers must allow the connection (see PolicySurfaces).
func (s *Security) EffectiveTCPLocalRateLimit() (perSec, burst int) {
	s.EnsureSecurityDefaults()
	p := s.PolicyByID(PolicyGatewayNewConnLimit)
	if p == nil || !p.Enabled {
		return 0, 0
	}
	return p.Params.PerSec, p.Params.Burst
}

// AllowlistEnforced reports whether allowlist policy is active.
func (s *Security) AllowlistEnforced() bool {
	return s.PolicyEnabled(PolicyAllowlist)
}

// EffectiveAllowlist returns deny/allow CIDR lists for nft rendering.
func (s *Security) EffectiveAllowlist() (deny, allow []string) {
	s.EnsureSecurityDefaults()
	p := s.PolicyByID(PolicyAllowlist)
	if p == nil {
		return nil, nil
	}
	return p.Params.Deny, p.Params.Allow
}

// NormalizeSecurity validates and normalizes all policy params.
func (s *Security) NormalizeSecurity() error {
	if s == nil {
		return nil
	}
	s.EnsureSecurityDefaults()
	for i := range s.Policies {
		if s.Policies[i].ID == PolicyKernelSyn {
			if err := normalizeKernelSynParams(&s.Policies[i].Params); err != nil {
				return fmt.Errorf("security.policies[kernel_syn].params: %w", err)
			}
		}
		if s.Policies[i].ID == PolicyAllowlist {
			deny, err := normalizeCIDRList(s.Policies[i].Params.Deny)
			if err != nil {
				return fmt.Errorf("security.policies[allowlist].params.deny: %w", err)
			}
			allow, err := normalizeCIDRList(s.Policies[i].Params.Allow)
			if err != nil {
				return fmt.Errorf("security.policies[allowlist].params.allow: %w", err)
			}
			s.Policies[i].Params.Deny = deny
			s.Policies[i].Params.Allow = allow
		}
	}
	return nil
}

// NormalizeCIDR accepts "1.2.3.4" or "1.2.3.4/32" and returns canonical CIDR form.
func NormalizeCIDR(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil {
			return "", fmt.Errorf("无效地址: %s", raw)
		}
		if v4 := ip.To4(); v4 != nil {
			return v4.String() + "/32", nil
		}
		return ip.String() + "/128", nil
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return "", fmt.Errorf("无效 CIDR: %s", raw)
	}
	ones, _ := n.Mask.Size()
	return fmt.Sprintf("%s/%d", n.IP.String(), ones), nil
}

func normalizeCIDRList(in []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		cid, err := NormalizeCIDR(raw)
		if err != nil {
			return nil, err
		}
		if cid == "" {
			continue
		}
		if _, ok := seen[cid]; ok {
			continue
		}
		seen[cid] = struct{}{}
		out = append(out, cid)
	}
	sort.Strings(out)
	return out, nil
}

// AllowlistView is the API projection of security.policies[allowlist] params.
type AllowlistView struct {
	Deny  []string `json:"deny"`
	Allow []string `json:"allow"`
}

// AllowlistView returns current deny/allow CIDR lists from the allowlist policy.
func (r *Resources) AllowlistView() AllowlistView {
	r.Security.EnsureSecurityDefaults()
	p := r.Security.PolicyByID(PolicyAllowlist)
	if p == nil {
		return AllowlistView{}
	}
	return AllowlistView{Deny: append([]string(nil), p.Params.Deny...), Allow: append([]string(nil), p.Params.Allow...)}
}

func (r *Resources) allowlistPolicy() *SecurityPolicy {
	r.Security.EnsureSecurityDefaults()
	return r.Security.PolicyByID(PolicyAllowlist)
}

// AddAllowlistEntry appends a CIDR to allowlist deny or allow ("deny"|"allow").
func (r *Resources) AddAllowlistEntry(list, cidr string) (string, error) {
	canonical, err := NormalizeCIDR(cidr)
	if err != nil {
		return "", err
	}
	if canonical == "" {
		return "", fmt.Errorf("CIDR 不能为空")
	}
	p := r.allowlistPolicy()
	if p == nil {
		return "", fmt.Errorf("缺少 allowlist 策略")
	}
	switch strings.ToLower(strings.TrimSpace(list)) {
	case "deny":
		for _, e := range p.Params.Deny {
			if e == canonical {
				return canonical, fmt.Errorf("已存在: %s", canonical)
			}
		}
		p.Params.Deny = append(p.Params.Deny, canonical)
		sort.Strings(p.Params.Deny)
	case "allow":
		for _, e := range p.Params.Allow {
			if e == canonical {
				return canonical, fmt.Errorf("已存在: %s", canonical)
			}
		}
		p.Params.Allow = append(p.Params.Allow, canonical)
		sort.Strings(p.Params.Allow)
	default:
		return "", fmt.Errorf("名单须为 deny 或 allow，当前: %s", list)
	}
	return canonical, nil
}

// RemoveAllowlistEntry removes a CIDR from allowlist deny or allow.
func (r *Resources) RemoveAllowlistEntry(list, cidr string) (string, error) {
	canonical, err := NormalizeCIDR(cidr)
	if err != nil {
		return "", err
	}
	if canonical == "" {
		return "", fmt.Errorf("CIDR 不能为空")
	}
	p := r.allowlistPolicy()
	if p == nil {
		return "", fmt.Errorf("缺少 allowlist 策略")
	}
	switch strings.ToLower(strings.TrimSpace(list)) {
	case "deny":
		next, ok := removeFromList(p.Params.Deny, canonical)
		if !ok {
			return "", fmt.Errorf("未找到: %s", canonical)
		}
		p.Params.Deny = next
	case "allow":
		next, ok := removeFromList(p.Params.Allow, canonical)
		if !ok {
			return "", fmt.Errorf("未找到: %s", canonical)
		}
		p.Params.Allow = next
	default:
		return "", fmt.Errorf("名单须为 deny 或 allow，当前: %s", list)
	}
	return canonical, nil
}

func removeFromList(in []string, want string) ([]string, bool) {
	out := make([]string, 0, len(in))
	found := false
	for _, e := range in {
		if e == want {
			found = true
			continue
		}
		out = append(out, e)
	}
	return out, found
}

// PolicyApplySurfaces maps a security policy diff entry to reload/firewall needs.
func PolicyApplySurfaces(field string) (needsReload, needsFirewall bool) {
	field = strings.TrimSpace(field)
	switch {
	case strings.HasPrefix(field, "security.policies."):
		id := strings.TrimPrefix(field, "security.policies.")
		if i := strings.IndexByte(id, '.'); i >= 0 {
			id = id[:i]
		}
		switch id {
		case PolicyFirewallNewConnLimit:
			return false, true
		case PolicyGatewayNewConnLimit:
			return true, false
		case PolicyConnLimit:
			return true, false
		case PolicyAllowlist, PolicyUDPLimit:
			return false, true
		case PolicyKernelSyn:
			return false, false
		}
	}
	return false, false
}

// DiffSecurityPolicies compares policy sets.
func DiffSecurityPolicies(before, after Security) []string {
	before.EnsureSecurityDefaults()
	after.EnsureSecurityDefaults()
	bm := map[string]SecurityPolicy{}
	am := map[string]SecurityPolicy{}
	for _, p := range before.Policies {
		bm[p.ID] = p
	}
	for _, p := range after.Policies {
		am[p.ID] = p
	}
	var parts []string
	seen := map[string]struct{}{}
	for _, meta := range SecurityPolicyCatalog {
		id := meta.ID
		seen[id] = struct{}{}
		bp, bok := bm[id]
		ap, aok := am[id]
		if !bok && !aok {
			continue
		}
		if !bok {
			parts = append(parts, fmt.Sprintf("security.policies.%s added enabled=%t", id, ap.Enabled))
			continue
		}
		if !aok {
			parts = append(parts, fmt.Sprintf("security.policies.%s removed", id))
			continue
		}
		if bp.Enabled != ap.Enabled {
			parts = append(parts, fmt.Sprintf("security.policies.%s.enabled %t→%t", id, bp.Enabled, ap.Enabled))
		}
		parts = append(parts, diffPolicyParams(id, bp.Params, ap.Params)...)
	}
	return parts
}

func diffPolicyParams(id string, before, after PolicyParams) []string {
	var parts []string
	add := func(field string, a, b any) {
		as, bs := fmt.Sprint(a), fmt.Sprint(b)
		if as != bs {
			parts = append(parts, fmt.Sprintf("security.policies.%s.params.%s %s→%s", id, field, as, bs))
		}
	}
	switch id {
	case PolicyAllowlist:
		added, removed := listDelta(before.Deny, after.Deny)
		for _, e := range added {
			parts = append(parts, "+deny "+e)
		}
		for _, e := range removed {
			parts = append(parts, "-deny "+e)
		}
		added, removed = listDelta(before.Allow, after.Allow)
		for _, e := range added {
			parts = append(parts, "+allow "+e)
		}
		for _, e := range removed {
			parts = append(parts, "-allow "+e)
		}
	case PolicyFirewallNewConnLimit:
		add("tcp_per_ip", before.TCPPerIP, after.TCPPerIP)
		add("burst", before.Burst, after.Burst)
	case PolicyGatewayNewConnLimit:
		add("per_sec", before.PerSec, after.PerSec)
		add("burst", before.Burst, after.Burst)
	case PolicyConnLimit:
		add("max_connections", before.MaxConnections, after.MaxConnections)
	case PolicyUDPLimit:
		add("udp_pps_per_ip", before.UDPPPSPerIP, after.UDPPPSPerIP)
		add("udp_burst", before.UDPBurst, after.UDPBurst)
	case PolicyKernelSyn:
		add("tcp_syncookies", before.TcpSyncookies, after.TcpSyncookies)
		add("tcp_max_syn_backlog", before.TcpMaxSynBacklog, after.TcpMaxSynBacklog)
		add("tcp_synack_retries", before.TcpSynackRetries, after.TcpSynackRetries)
		add("tcp_syn_retries", before.TcpSynRetries, after.TcpSynRetries)
		add("tcp_abort_on_overflow", before.TcpAbortOnOverflow, after.TcpAbortOnOverflow)
	}
	return parts
}

func summarizeSecurity(s Security) []string {
	s.EnsureSecurityDefaults()
	var parts []string
	for _, p := range s.Policies {
		if !p.Enabled {
			parts = append(parts, fmt.Sprintf("%s=off", p.ID))
			continue
		}
		switch p.ID {
		case PolicyAllowlist:
			if len(p.Params.Deny) > 0 {
				parts = append(parts, fmt.Sprintf("deny=%s", strings.Join(p.Params.Deny, ",")))
			}
			if len(p.Params.Allow) > 0 {
				parts = append(parts, fmt.Sprintf("allow=%s (strict)", strings.Join(p.Params.Allow, ",")))
			}
		case PolicyFirewallNewConnLimit:
			parts = append(parts, fmt.Sprintf("firewall_new_conn=%s/%d", p.Params.TCPPerIP, p.Params.Burst))
		case PolicyGatewayNewConnLimit:
			parts = append(parts, fmt.Sprintf("gateway_new_conn=%d/%d", p.Params.PerSec, p.Params.Burst))
		case PolicyConnLimit:
			parts = append(parts, fmt.Sprintf("max_conn=%d", p.Params.MaxConnections))
		case PolicyKernelSyn:
			parts = append(parts, fmt.Sprintf("kernel_syn=%d/%d", p.Params.TcpSyncookies, p.Params.TcpMaxSynBacklog))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return parts
}
