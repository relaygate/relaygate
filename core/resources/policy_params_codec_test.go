package resources

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPolicyParamsExtraYAMLRoundTrip(t *testing.T) {
	t.Parallel()
	in := `
tcp_per_ip: 40/second
burst: 80
custom_vendor_knob: acme
future_flag: true
`
	var p PolicyParams
	if err := yaml.Unmarshal([]byte(in), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.TCPPerIP != "40/second" || p.Burst != 80 {
		t.Fatalf("known fields: %+v", p)
	}
	if p.Extra["custom_vendor_knob"] != "acme" {
		t.Fatalf("extra custom_vendor_knob=%v", p.Extra["custom_vendor_knob"])
	}
	if p.Extra["future_flag"] != true {
		t.Fatalf("extra future_flag=%v", p.Extra["future_flag"])
	}

	out, err := yaml.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	for _, want := range []string{"tcp_per_ip: 40/second", "burst: 80", "custom_vendor_knob: acme", "future_flag: true"} {
		if !strings.Contains(s, want) {
			t.Fatalf("marshaled missing %q:\n%s", want, s)
		}
	}

	var again PolicyParams
	if err := yaml.Unmarshal(out, &again); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if again.TCPPerIP != "40/second" || again.Burst != 80 {
		t.Fatalf("re-known: %+v", again)
	}
	if again.Extra["custom_vendor_knob"] != "acme" || again.Extra["future_flag"] != true {
		t.Fatalf("re-extra: %+v", again.Extra)
	}
}

func TestPolicyParamsExtraJSONRoundTrip(t *testing.T) {
	t.Parallel()
	raw := `{"per_sec":10,"burst":20,"adapter_hint":"x"}`
	var p PolicyParams
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.PerSec != 10 || p.Burst != 20 {
		t.Fatalf("known: %+v", p)
	}
	if p.Extra["adapter_hint"] != "x" {
		t.Fatalf("extra=%v", p.Extra)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("map: %v", err)
	}
	if m["per_sec"] != float64(10) || m["burst"] != float64(20) || m["adapter_hint"] != "x" {
		t.Fatalf("got %v", m)
	}
}

func TestPolicyParamsKnownOverridesExtraOnMarshal(t *testing.T) {
	t.Parallel()
	p := PolicyParams{
		TCPPerIP: "30/second",
		Extra: map[string]any{
			"tcp_per_ip":         "999/second", // must not override known
			"custom_vendor_knob": 1,
		},
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "999/second") {
		t.Fatalf("Extra overrode known key:\n%s", s)
	}
	if !strings.Contains(s, "tcp_per_ip: 30/second") || !strings.Contains(s, "custom_vendor_knob: 1") {
		t.Fatalf("unexpected:\n%s", s)
	}
}

func TestPolicyParamsExtraIgnoredByEffective(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	p := s.PolicyByID(PolicyFirewallNewConnLimit)
	p.Params.TCPPerIP = "55/second"
	p.Params.Burst = 110
	p.Params.Extra = map[string]any{"tcp_per_ip": "1/second", "noise": true}
	n := s.EffectiveFirewallRates()
	if n.TCPNewConnPerIP != "55/second" || n.TCPBurst != 110 {
		t.Fatalf("Effective used Extra? got %+v", n)
	}
}

func TestMergePolicyParamsExtra(t *testing.T) {
	t.Parallel()
	dst := PolicyParams{
		TCPPerIP: "30/second",
		Extra:    map[string]any{"keep": "a", "overwrite": 1},
	}
	src := PolicyParams{
		Extra: map[string]any{"overwrite": 2, "new": "b"},
	}
	MergePolicyParamsExtra(&dst, src)
	if dst.Extra["keep"] != "a" || dst.Extra["overwrite"] != 2 || dst.Extra["new"] != "b" {
		t.Fatalf("extra=%v", dst.Extra)
	}
	if dst.TCPPerIP != "30/second" {
		t.Fatalf("typed field changed: %s", dst.TCPPerIP)
	}
}

func TestSecurityPolicyParamsExtraLoadSave(t *testing.T) {
	t.Parallel()
	in := `
id: firewall_new_conn_limit
type: firewall_new_conn_limit
enabled: true
params:
  tcp_per_ip: 40/second
  burst: 80
  custom_vendor_knob: keep-me
`
	var pol SecurityPolicy
	if err := yaml.Unmarshal([]byte(in), &pol); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pol.Params.TCPPerIP != "40/second" || pol.Params.Burst != 80 {
		t.Fatalf("known: %+v", pol.Params)
	}
	if pol.Params.Extra["custom_vendor_knob"] != "keep-me" {
		t.Fatalf("extra lost on load: %+v", pol.Params.Extra)
	}
	out, err := yaml.Marshal(pol)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), "custom_vendor_knob: keep-me") {
		t.Fatalf("extra lost on save:\n%s", out)
	}
	if !strings.Contains(string(out), "tcp_per_ip: 40/second") {
		t.Fatalf("known lost on save:\n%s", out)
	}
}
