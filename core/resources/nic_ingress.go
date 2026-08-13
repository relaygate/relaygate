package resources

import (
	"fmt"
	"strings"
)

// Default NIC ingress police rate (aligned with low-bandwidth host examples ~3 Mbps).
const (
	DefaultNICIngressRate = "3mbit"
)

// NICIngressPoliceParams is the effective overlay for policy nic_ingress_police (tc ingress).
type NICIngressPoliceParams struct {
	Device string // empty → auto-detect default-route iface at apply time
	Rate   string // tc rate token, e.g. 3mbit
}

// EffectiveNICIngressPolice returns police knobs when nic_ingress_police is enabled; nil when off.
// Uses typed Params fields only; Extra is ignored.
func (s *Security) EffectiveNICIngressPolice() *NICIngressPoliceParams {
	s.EnsureSecurityDefaults()
	p := s.PolicyByID(PolicyNICIngressPolice)
	if p == nil || !p.Enabled {
		return nil
	}
	out := &NICIngressPoliceParams{
		Device: strings.TrimSpace(p.Params.Device),
		Rate:   strings.TrimSpace(p.Params.Rate),
	}
	if out.Rate == "" {
		out.Rate = DefaultNICIngressRate
	}
	return out
}

func applyNICIngressDefaults(p *SecurityPolicy, d SecurityPolicy) {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.Params.Rate) == "" {
		p.Params.Rate = d.Params.Rate
		if p.Params.Rate == "" {
			p.Params.Rate = DefaultNICIngressRate
		}
	}
	// Device may stay empty (auto-detect).
	if p.Params.Device == "" && d.Params.Device != "" {
		p.Params.Device = d.Params.Device
	}
}

func normalizeNICIngressParams(pp *PolicyParams) error {
	if pp == nil {
		return nil
	}
	pp.Device = strings.TrimSpace(pp.Device)
	pp.Rate = strings.TrimSpace(pp.Rate)
	if pp.Device != "" && !nicDeviceNameRe.MatchString(pp.Device) {
		return fmt.Errorf("device 无效: %q（须为合法网卡名）", pp.Device)
	}
	if pp.Rate == "" {
		pp.Rate = DefaultNICIngressRate
	}
	if !nicRateRe.MatchString(pp.Rate) {
		return fmt.Errorf("rate 无效: %q（示例: 3mbit、3000kbit）", pp.Rate)
	}
	return nil
}
