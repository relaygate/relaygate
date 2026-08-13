package resources

import (
	"fmt"
	"strings"
)

// ExecutionStep is one ordered enforcement stage along the product domain path
// (kernel → nic → firewall → gateway). Component holds execution detail only.
type ExecutionStep struct {
	Order     int      `json:"order"`
	Layer     string   `json:"layer"`     // product domain: kernel | nic | firewall | gateway
	Component string   `json:"component"` // execution detail (sysctl, nft INPUT, Envoy listener, …)
	Action    string   `json:"action"`
	Policies  []string `json:"policies,omitempty"`
}

// PolicySurface maps a catalog policy to enforcement domains and apply path.
type PolicySurface struct {
	PolicyID    string   `json:"policy_id"`
	Layers      []string `json:"layers"`
	ApplyPath   string   `json:"apply_path"`
	OverlapNote string   `json:"overlap_note,omitempty"`
}

// KernelPreviewSection is the effective kernel overlay when kernel_syn is on.
type KernelPreviewSection struct {
	Enabled     bool   `json:"enabled"`
	ApplyScript string `json:"apply_script"`
	Content     string `json:"content"`
}

// NICPreviewSection summarizes nic_egress_shape / nic_ingress_police when enabled.
type NICPreviewSection struct {
	Enabled        bool   `json:"enabled"` // true when any nic_* policy is on
	ApplyScript    string `json:"apply_script"`
	Device         string `json:"device,omitempty"` // egress device (or auto)
	Rate           string `json:"rate,omitempty"`   // egress rate
	EgressEnabled  bool   `json:"egress_enabled"`
	IngressEnabled bool   `json:"ingress_enabled"`
	IngressDevice  string `json:"ingress_device,omitempty"` // ingress device (or auto)
	IngressRate    string `json:"ingress_rate,omitempty"`
}

// FirewallPreviewSection holds rendered firewall rule fragments.
type FirewallPreviewSection struct {
	ForwardPorts   string `json:"forward_ports"`
	GatewayExcerpt string `json:"gateway_excerpt"`
}

// GatewayPreviewSection summarizes gateway knobs derived from security.protections.
type GatewayPreviewSection struct {
	MaxConnections         int  `json:"max_connections"`
	LocalRateLimitPerSec   int  `json:"local_ratelimit_per_sec"`
	LocalRateLimitBurst    int  `json:"local_ratelimit_burst"`
	ListenersWithRateLimit int  `json:"listeners_with_rate_limit"`
	EnabledTCPForwards     int  `json:"enabled_tcp_forwards"`
	RateLimitEnabled       bool `json:"rate_limit_enabled"`
	ConnLimitEnabled       bool `json:"gateway_conn_limit_enabled"`
}

// SecurityPreview is the API projection of effective security materialization.
// JSON keys are product domains only (no component aliases).
type SecurityPreview struct {
	ExecutionOrder []ExecutionStep         `json:"execution_order"`
	Surfaces       []PolicySurface         `json:"surfaces"`
	Kernel         *KernelPreviewSection   `json:"kernel,omitempty"`
	NIC            *NICPreviewSection      `json:"nic,omitempty"`
	Firewall       *FirewallPreviewSection `json:"firewall,omitempty"`
	Gateway        *GatewayPreviewSection  `json:"gateway,omitempty"`
	Notes          []string                `json:"notes"`
}

// SecurityExecutionOrder documents the product enforcement pipeline.
// Apply/verify order: kernel → nic → firewall → gateway.
// Inbound path places firewall downstream of NIC.
// firewall_new_conn_limit and gateway_new_conn_limit are independent policies; both may apply.
func SecurityExecutionOrder() []ExecutionStep {
	return []ExecutionStep{
		{Order: 1, Layer: string(LayerKernel), Component: "sysctl", Action: "SYN cookies / backlog (new handshakes only)", Policies: []string{PolicyKernelSyn}},
		{Order: 2, Layer: string(LayerNIC), Component: "tc", Action: "egress shape + ingress police on service NIC (optional)", Policies: []string{PolicyNICEgressShape, PolicyNICIngressPolice}},
		{Order: 3, Layer: string(LayerFirewall), Component: "nft INPUT", Action: "accept established,related (long-lived sessions bypass rate limits)", Policies: nil},
		{Order: 4, Layer: string(LayerFirewall), Component: "nft INPUT", Action: "ACL deny (drop listed sources first)", Policies: []string{"access"}},
		{Order: 5, Layer: string(LayerFirewall), Component: "nft INPUT", Action: "ACL allow strict (drop non-listed when allow non-empty)", Policies: []string{"access"}},
		{Order: 6, Layer: string(LayerFirewall), Component: "nft INPUT", Action: "new TCP conn per-IP rate", Policies: []string{PolicyFirewallNewConnLimit}},
		{Order: 7, Layer: string(LayerFirewall), Component: "nft INPUT", Action: "UDP per-IP PPS", Policies: []string{PolicyFirewallUDPLimit}},
		{Order: 8, Layer: string(LayerGateway), Component: "Envoy listener", Action: "local_ratelimit token bucket (new connections)", Policies: []string{PolicyGatewayNewConnLimit}},
		{Order: 9, Layer: string(LayerGateway), Component: "Envoy cluster", Action: "circuit_breaker max_connections", Policies: []string{PolicyGatewayConnLimit}},
	}
}

// PolicySurfaces returns catalog policies with domain / apply-path metadata for preview UI.
func PolicySurfaces() []PolicySurface {
	out := make([]PolicySurface, 0, len(SecurityPolicyCatalog))
	for _, m := range SecurityPolicyCatalog {
		s := PolicySurface{
			PolicyID:  m.ID,
			Layers:    []string{string(m.Layer)},
			ApplyPath: string(m.ApplyPath),
		}
		out = append(out, s)
	}
	return out
}

// BuildSecurityPreview assembles kernel / firewall / gateway summaries for r.
// packagingRoot is reserved for callers that already resolve the install packaging dir
// (firewall excerpts are passed in pre-rendered).
func BuildSecurityPreview(r *Resources, packagingRoot, firewallForwardPorts, firewallGatewayExcerpt string) (*SecurityPreview, error) {
	if r == nil {
		return nil, fmt.Errorf("resources 不能为空")
	}
	r.Security.EnsureSecurityDefaults()
	if err := r.Security.NormalizeSecurity(); err != nil {
		return nil, err
	}

	prev := &SecurityPreview{
		ExecutionOrder: SecurityExecutionOrder(),
		Surfaces:       PolicySurfaces(),
		Notes: []string{
			"「应用安全策略」仅落地防火墙。",
			"「本机应用」/ reload 写入网关转发限速与并发上限。",
			"节点 agent 拉取后可按 SECURITY_AUTO_APPLY 按序落地：内核 → 网卡 → 防火墙 → 网关；主控（PANEL_ENABLED=1）默认不自动应用主机侧（含网卡）。",
			"也可手动：relaygate security apply-kernel --verify（内核）；relaygate security apply-nic --verify（网卡）；sudo relaygate firewall apply（防火墙）。",
		},
	}

	if r.Security.PolicyEnabled(PolicyKernelSyn) {
		prev.Kernel = &KernelPreviewSection{
			Enabled:     true,
			ApplyScript: "relaygate security apply-kernel --verify",
			Content:     RenderKernelHardenConf(&r.Security),
		}
	} else {
		prev.Kernel = &KernelPreviewSection{Enabled: false}
	}

	egress := r.Security.EffectiveNICEgressShape()
	ingress := r.Security.EffectiveNICIngressPolice()
	if egress != nil || ingress != nil {
		sec := &NICPreviewSection{
			Enabled:        true,
			ApplyScript:    "relaygate security apply-nic --verify",
			EgressEnabled:  egress != nil,
			IngressEnabled: ingress != nil,
		}
		if egress != nil {
			dev := egress.Device
			if dev == "" {
				dev = "auto"
			}
			sec.Device = dev
			sec.Rate = egress.Rate
		}
		if ingress != nil {
			dev := ingress.Device
			if dev == "" {
				dev = "auto"
			}
			sec.IngressDevice = dev
			sec.IngressRate = ingress.Rate
		}
		prev.NIC = sec
	} else {
		prev.NIC = &NICPreviewSection{Enabled: false}
	}

	prev.Firewall = &FirewallPreviewSection{
		ForwardPorts:   strings.TrimSpace(firewallForwardPorts),
		GatewayExcerpt: strings.TrimSpace(firewallGatewayExcerpt),
	}

	tcpForwards := 0
	for _, fwd := range r.EnabledForwards() {
		if strings.EqualFold(fwd.Protocol, "TCP") {
			tcpForwards++
		}
	}
	perSec, burst := r.Security.EffectiveTCPLocalRateLimit()
	connP := r.Security.PolicyByID(PolicyGatewayConnLimit)
	newP := r.Security.PolicyByID(PolicyGatewayNewConnLimit)
	prev.Gateway = &GatewayPreviewSection{
		MaxConnections:         r.Security.EffectiveMaxConnections(),
		LocalRateLimitPerSec:   perSec,
		LocalRateLimitBurst:    burst,
		ListenersWithRateLimit: 0,
		EnabledTCPForwards:     tcpForwards,
		RateLimitEnabled:       newP != nil && newP.Enabled && perSec > 0 && burst > 0,
		ConnLimitEnabled:       connP != nil && connP.Enabled,
	}
	if prev.Gateway.RateLimitEnabled {
		prev.Gateway.ListenersWithRateLimit = tcpForwards
	}

	return prev, nil
}
