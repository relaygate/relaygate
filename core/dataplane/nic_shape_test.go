package dataplane

import (
	"testing"

	"github.com/relaygate/relaygate/core/resources"
)

func TestParseDefaultRouteDevice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"default via 203.0.113.1 dev eth0 proto dhcp metric 100", "eth0"},
		{"default via 203.0.113.1 dev ens3", "ens3"},
		{"1.2.3.4/32 via 10.0.0.1 dev eth1\ndefault via 10.0.0.1 dev eth1", "eth1"},
		{"", ""},
		{"unreachable default", ""},
	}
	for _, tc := range cases {
		if got := parseDefaultRouteDevice(tc.in); got != tc.want {
			t.Fatalf("parseDefaultRouteDevice(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeRateToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"3mbit", "3mbit"},
		{"3Mbit", "3mbit"},
		{"3000kbit", "3000kbit"},
		{"3mbps", "3mbit"},
		{"bad", ""},
	}
	for _, tc := range cases {
		if got := normalizeRateToken(tc.in); got != tc.want {
			t.Fatalf("normalizeRateToken(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestNICQdiscMatches(t *testing.T) {
	t.Parallel()
	out := "qdisc tbf 1: root refcnt 2 rate 3Mbit burst 32Kb lat 400.0ms"
	if !nicQdiscMatches(out, "3mbit") {
		t.Fatal("expected match for 3mbit")
	}
	if nicQdiscMatches(out, "10mbit") {
		t.Fatal("should not match 10mbit")
	}
	if nicQdiscMatches("qdisc fq_codel 0: root", "3mbit") {
		t.Fatal("fq_codel should not match")
	}
}

func TestNICPoliceMatches(t *testing.T) {
	t.Parallel()
	out := "filter protocol all pref 1 u32 police 0x1 rate 3Mbit burst 4Kb mtu 2Kb action drop/pipe"
	if !nicPoliceMatches(out, "3mbit") {
		t.Fatal("expected police match for 3mbit")
	}
	if nicPoliceMatches(out, "10mbit") {
		t.Fatal("should not match 10mbit")
	}
	if nicPoliceMatches("filter protocol all pref 1 u32", "3mbit") {
		t.Fatal("no police should not match")
	}
	if !nicIngressQdiscPresent("qdisc ingress ffff: parent ffff:fff1") {
		t.Fatal("expected ingress present")
	}
}

func TestEffectiveNICEgressShape(t *testing.T) {
	t.Parallel()
	sec := resources.DefaultSecurity()
	if sec.EffectiveNICEgressShape() != nil {
		t.Fatal("default nic should be off")
	}
	p := sec.PolicyByID(resources.PolicyNICEgressShape)
	if p == nil {
		t.Fatal("missing nic_egress_shape in defaults")
	}
	p.Enabled = true
	p.Params.Rate = "3mbit"
	p.Params.Device = "eth0"
	got := sec.EffectiveNICEgressShape()
	if got == nil || got.Rate != "3mbit" || got.Device != "eth0" {
		t.Fatalf("got %+v", got)
	}
}

func TestEffectiveNICIngressPolice(t *testing.T) {
	t.Parallel()
	sec := resources.DefaultSecurity()
	if sec.EffectiveNICIngressPolice() != nil {
		t.Fatal("default nic ingress should be off")
	}
	p := sec.PolicyByID(resources.PolicyNICIngressPolice)
	if p == nil {
		t.Fatal("missing nic_ingress_police in defaults")
	}
	p.Enabled = true
	p.Params.Rate = "3mbit"
	p.Params.Device = "eth0"
	got := sec.EffectiveNICIngressPolice()
	if got == nil || got.Rate != "3mbit" || got.Device != "eth0" {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeNICEgressParams(t *testing.T) {
	t.Parallel()
	sec := resources.DefaultSecurity()
	p := sec.PolicyByID(resources.PolicyNICEgressShape)
	p.Enabled = true
	p.Params.Device = "eth0;rm -rf /"
	if err := sec.NormalizeSecurity(); err == nil {
		t.Fatal("expected invalid device")
	}
	p.Params.Device = "eth0"
	p.Params.Rate = "3mbit"
	if err := sec.NormalizeSecurity(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNICHelpers(t *testing.T) {
	t.Parallel()
	if !resources.ValidateNICDeviceName("eth0") || resources.ValidateNICDeviceName("eth0 bad") {
		t.Fatal("device validation")
	}
	if !resources.ValidateNICRate("3mbit") || resources.ValidateNICRate("fast") {
		t.Fatal("rate validation")
	}
}
