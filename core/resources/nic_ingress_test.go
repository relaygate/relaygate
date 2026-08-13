package resources

import "testing"

func TestEffectiveNICIngressPoliceDefaults(t *testing.T) {
	t.Parallel()
	sec := DefaultSecurity()
	if sec.EffectiveNICIngressPolice() != nil {
		t.Fatal("default nic_ingress_police should be disabled")
	}
	p := sec.PolicyByID(PolicyNICIngressPolice)
	if p == nil {
		t.Fatal("catalog default missing nic_ingress_police")
	}
	if p.Params.Rate != DefaultNICIngressRate {
		t.Fatalf("rate=%q", p.Params.Rate)
	}
	p.Enabled = true
	p.Params.Device = ""
	p.Params.Rate = ""
	got := sec.EffectiveNICIngressPolice()
	if got == nil || got.Rate != DefaultNICIngressRate || got.Device != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeNICIngressRejectsBadDevice(t *testing.T) {
	t.Parallel()
	sec := DefaultSecurity()
	p := sec.PolicyByID(PolicyNICIngressPolice)
	p.Params.Device = "eth0;evil"
	if err := sec.NormalizeSecurity(); err == nil {
		t.Fatal("expected error")
	}
}
