package resources

import "testing"

func TestPolicyEnabledDefaultsOn(t *testing.T) {
	t.Parallel()
	var s Security
	s.EnsureSecurityDefaults()
	for _, id := range []string{
		PolicyKernelSyn,
		PolicyFirewallNewConnLimit,
		PolicyGatewayNewConnLimit,
		PolicyConnLimit,
		PolicyAllowlist,
		PolicyUDPLimit,
	} {
		if !s.PolicyEnabled(id) {
			t.Fatalf("%s should default enabled", id)
		}
	}
}

func TestEffectiveFirewallRatesDisabledPolicies(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	for i := range s.Policies {
		if s.Policies[i].ID == PolicyFirewallNewConnLimit {
			s.Policies[i].Enabled = false
		}
		if s.Policies[i].ID == PolicyUDPLimit {
			s.Policies[i].Enabled = false
		}
	}
	n := s.EffectiveFirewallRates()
	if n.TCPNewConnPerIP != DisabledFirewallRatePerIP || n.TCPBurst != DisabledFirewallBurst {
		t.Fatalf("tcp: %+v", n)
	}
	if n.UDPPPSPerIP != DisabledFirewallRatePerIP || n.UDPBurst != DisabledFirewallBurst {
		t.Fatalf("udp: %+v", n)
	}
}

func TestEffectiveMaxConnectionsPolicy(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	p := s.PolicyByID(PolicyConnLimit)
	p.Params.MaxConnections = 512
	if s.EffectiveMaxConnections() != 512 {
		t.Fatalf("want 512")
	}
	p.Enabled = false
	if s.EffectiveMaxConnections() != UnlimitedMaxConnections {
		t.Fatalf("want unlimited when disabled")
	}
}

func TestEffectiveTCPLocalRateLimitPolicy(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	p := s.PolicyByID(PolicyGatewayNewConnLimit)
	p.Params.PerSec = 100
	p.Params.Burst = 200
	per, burst := s.EffectiveTCPLocalRateLimit()
	if per != 100 || burst != 200 {
		t.Fatalf("got %d/%d", per, burst)
	}
	p.Enabled = false
	per, burst = s.EffectiveTCPLocalRateLimit()
	if per != 0 || burst != 0 {
		t.Fatalf("want 0/0 when disabled, got %d/%d", per, burst)
	}
}

func TestDiffSecurityPolicies(t *testing.T) {
	t.Parallel()
	before := DefaultSecurity()
	after := DefaultSecurity()
	for i := range after.Policies {
		if after.Policies[i].ID == PolicyGatewayNewConnLimit {
			after.Policies[i].Enabled = false
		}
	}
	parts := DiffSecurityPolicies(before, after)
	if len(parts) != 1 || parts[0] != "security.policies.gateway_new_conn_limit.enabled true→false" {
		t.Fatalf("parts=%v", parts)
	}
}

func TestPolicyApplySurfaces(t *testing.T) {
	t.Parallel()
	cases := []struct {
		field            string
		reload, firewall bool
	}{
		{"security.policies.firewall_new_conn_limit", false, true},
		{"security.policies.gateway_new_conn_limit", true, false},
		{"security.policies.conn_limit", true, false},
		{"security.policies.allowlist", false, true},
		{"security.policies.kernel_syn", false, false},
	}
	for _, c := range cases {
		r, f := PolicyApplySurfaces(c.field)
		if r != c.reload || f != c.firewall {
			t.Fatalf("%s: got reload=%v firewall=%v", c.field, r, f)
		}
	}
}
