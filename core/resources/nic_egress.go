package resources

import (
	"fmt"
	"regexp"
	"strings"
)

// Default NIC egress shaping (aligned with low-bandwidth host examples ~3 Mbps).
const (
	DefaultNICEgressRate = "3mbit"
)

var (
	nicDeviceNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,14}$`)
	nicRateRe       = regexp.MustCompile(`(?i)^\d+(\.\d+)?(bit|kbit|mbit|gbit|bps|kbps|mbps|gbps)$`)
)

// NICEgressShapeParams is the effective overlay for policy nic_egress_shape (tc egress).
type NICEgressShapeParams struct {
	Device string // empty → auto-detect default-route iface at apply time
	Rate   string // tc rate token, e.g. 3mbit
}

// EffectiveNICEgressShape returns shaping knobs when nic_egress_shape is enabled; nil when off.
// Uses typed Params fields only; Extra is ignored.
func (s *Security) EffectiveNICEgressShape() *NICEgressShapeParams {
	s.EnsureSecurityDefaults()
	p := s.PolicyByID(PolicyNICEgressShape)
	if p == nil || !p.Enabled {
		return nil
	}
	out := &NICEgressShapeParams{
		Device: strings.TrimSpace(p.Params.Device),
		Rate:   strings.TrimSpace(p.Params.Rate),
	}
	if out.Rate == "" {
		out.Rate = DefaultNICEgressRate
	}
	return out
}

func applyNICEgressDefaults(p *SecurityPolicy, d SecurityPolicy) {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.Params.Rate) == "" {
		p.Params.Rate = d.Params.Rate
		if p.Params.Rate == "" {
			p.Params.Rate = DefaultNICEgressRate
		}
	}
	// Device may stay empty (auto-detect).
	if p.Params.Device == "" && d.Params.Device != "" {
		p.Params.Device = d.Params.Device
	}
}

func normalizeNICEgressParams(pp *PolicyParams) error {
	if pp == nil {
		return nil
	}
	pp.Device = strings.TrimSpace(pp.Device)
	pp.Rate = strings.TrimSpace(pp.Rate)
	if pp.Device != "" && !nicDeviceNameRe.MatchString(pp.Device) {
		return fmt.Errorf("device 无效: %q（须为合法网卡名）", pp.Device)
	}
	if pp.Rate == "" {
		pp.Rate = DefaultNICEgressRate
	}
	if !nicRateRe.MatchString(pp.Rate) {
		return fmt.Errorf("rate 无效: %q（示例: 3mbit、3000kbit）", pp.Rate)
	}
	return nil
}

// ValidateNICDeviceName reports whether name is safe to pass to tc/ip.
func ValidateNICDeviceName(name string) bool {
	return nicDeviceNameRe.MatchString(strings.TrimSpace(name))
}

// ValidateNICRate reports whether rate is a tc-style bandwidth token.
func ValidateNICRate(rate string) bool {
	return nicRateRe.MatchString(strings.TrimSpace(rate))
}
