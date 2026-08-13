package resources

import "testing"

func TestEffectiveNICEgressShapeDefaults(t *testing.T) {
	t.Parallel()
	sec := DefaultSecurity()
	if sec.EffectiveNICEgressShape() != nil {
		t.Fatal("default nic_egress_shape should be disabled")
	}
	p := sec.PolicyByID(PolicyNICEgressShape)
	if p == nil {
		t.Fatal("catalog default missing nic_egress_shape")
	}
	if p.Params.Rate != DefaultNICEgressRate {
		t.Fatalf("rate=%q", p.Params.Rate)
	}
	p.Enabled = true
	p.Params.Device = ""
	p.Params.Rate = ""
	got := sec.EffectiveNICEgressShape()
	if got == nil || got.Rate != DefaultNICEgressRate || got.Device != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeNICEgressRejectsBadDevice(t *testing.T) {
	t.Parallel()
	sec := DefaultSecurity()
	p := sec.PolicyByID(PolicyNICEgressShape)
	p.Params.Device = "eth0;evil"
	if err := sec.NormalizeSecurity(); err == nil {
		t.Fatal("expected error")
	}
}
