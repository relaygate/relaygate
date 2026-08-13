package resources

import (
	"strings"
	"testing"
)

func sampleResources() *Resources {
	return &Resources{
		Security: DefaultSecurity(),
		Upstreams: []Upstream{
			{Name: "server-01", Address: "10.0.0.11", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true},
			{Name: "server-02", Address: "10.0.0.12", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true},
		},
		Forwards: []Forward{
			{Name: "forward-server-01-validation-tcp", Entry: "validation", Upstream: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-validation-udp", Entry: "validation", Upstream: "server-01", Protocol: "UDP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-production-tcp", Entry: "production", Upstream: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
			{Name: "forward-server-01-production-udp", Entry: "production", Upstream: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: false},
			{Name: "forward-server-02-production-tcp", Entry: "production", Upstream: "server-02", Protocol: "TCP", ListenPort: 10002, Enabled: false},
			{Name: "forward-server-02-production-udp", Entry: "production", Upstream: "server-02", Protocol: "UDP", ListenPort: 10002, Enabled: false},
		},
	}
}

func TestForwardName(t *testing.T) {
	got := ForwardName("server-01", "production", "TCP")
	if got != "forward-server-01-production-tcp" {
		t.Fatalf("got %q", got)
	}
	if ForwardName("server-02", "validation", "udp") != "forward-server-02-validation-udp" {
		t.Fatal("validation/udp")
	}
}

func TestAddUpstreamUpstreamOnly(t *testing.T) {
	r := sampleResources()
	rulesBefore := len(r.Forwards)
	err := r.AddUpstream(Upstream{
		Name: "server-11", Address: "10.0.0.21",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddUpstream: %v", err)
	}
	if len(r.Upstreams) != 3 {
		t.Fatalf("upstreams len=%d", len(r.Upstreams))
	}
	if len(r.Forwards) != rulesBefore {
		t.Fatalf("AddUpstream must not create rules; forwards=%d want %d", len(r.Forwards), rulesBefore)
	}
}

func TestAddEntriesValidationAndProduction(t *testing.T) {
	r := sampleResources()
	if err := r.AddUpstream(Upstream{
		Name: "server-14", Address: "10.0.0.24",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := r.AddEntries(AddEntryOptions{
		Upstream: "server-14", Entry: EntryValidation, Protocols: []string{"TCP", "UDP"},
	})
	if err != nil {
		t.Fatalf("AddEntries validation: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 validation forwards, got %d", len(created))
	}
	for _, fwd := range created {
		if fwd.Entry != EntryValidation || fwd.ListenPort != 11014 || !fwd.Enabled {
			t.Fatalf("unexpected validation forward: %+v", fwd)
		}
	}
	prod, err := r.AddEntries(AddEntryOptions{
		Upstream: "server-14", Entry: EntryProduction, Protocols: []string{"TCP", "UDP"},
	})
	if err != nil {
		t.Fatalf("AddEntries production: %v", err)
	}
	if len(prod) != 2 {
		t.Fatalf("expected 2 production, got %d", len(prod))
	}
	for _, fwd := range prod {
		if fwd.Entry != EntryProduction || fwd.ListenPort != 10014 || fwd.Enabled {
			t.Fatalf("unexpected production forward: %+v", fwd)
		}
	}
	// Idempotent
	again, err := r.AddEntries(AddEntryOptions{
		Upstream: "server-14", Entry: EntryValidation, Protocols: []string{"TCP"},
	})
	if err != nil || len(again) != 0 {
		t.Fatalf("idempotent: %+v err=%v", again, err)
	}
}

func TestAddEntriesRejectsBadProtocols(t *testing.T) {
	r := sampleResources()
	_, err := r.AddEntries(AddEntryOptions{
		Upstream: "server-01", Entry: EntryProduction, Protocols: []string{"NOPE"},
	})
	if err == nil || !strings.Contains(err.Error(), "protocols") {
		t.Fatalf("expected protocols error, got %v", err)
	}
}

func TestPortMapAndCSV(t *testing.T) {
	r := sampleResources()
	r.Gateway.PublicIP = "203.0.113.10"
	rows := r.PortMap()
	if len(rows) != 6 {
		t.Fatalf("rows=%d", len(rows))
	}
	csv := FormatPortMapCSV(r)
	if !strings.Contains(csv, "listen_port,protocol,entry") {
		t.Fatalf("header missing: %s", csv)
	}
	if !strings.Contains(csv, "11001,TCP,validation,server-01,10.0.0.11,7777,true,203.0.113.10") {
		t.Fatalf("row missing: %s", csv)
	}
}

func TestAddUpstreamRejectsDuplicateName(t *testing.T) {
	r := sampleResources()
	err := r.AddUpstream(Upstream{
		Name: "server-01", Address: "10.0.0.99",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestAddUpstreamRejectsInvalidFields(t *testing.T) {
	r := sampleResources()
	if err := r.AddUpstream(Upstream{Name: "", Address: "10.0.0.21", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778)}); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := r.AddUpstream(Upstream{Name: "bad name", Address: "10.0.0.21", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778)}); err == nil {
		t.Fatal("expected invalid name error")
	}
	if err := r.AddUpstream(Upstream{Name: "server-99", Address: "not-an-ip", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778)}); err == nil {
		t.Fatal("expected invalid address error")
	}
	if err := r.AddUpstream(Upstream{Name: "server-99", Address: "10.0.0.21", UDP: ProtoPortOf(7778), Enabled: true}); err != nil {
		t.Fatalf("udp-only AddUpstream: %v", err)
	}
	if r.Upstreams[len(r.Upstreams)-1].HasTCP() {
		t.Fatal("expected TCP to stay unset")
	}
	if err := r.AddUpstream(Upstream{Name: "server-98", Address: "10.0.0.22"}); err == nil {
		t.Fatal("expected port error when no protocols enabled")
	}
}

func TestAddEntriesAllocatesNextPortWhenPreferredTaken(t *testing.T) {
	r := sampleResources()
	r.Forwards = append(r.Forwards, Forward{
		Name: "forward-extra", Entry: "production", Upstream: "server-02",
		Protocol: "TCP", ListenPort: 10003, Enabled: false,
	})
	if err := r.AddUpstream(Upstream{
		Name: "server-03", Address: "10.0.0.13",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := r.AddEntries(AddEntryOptions{
		Upstream: "server-03", Entry: EntryProduction, Protocols: []string{"TCP"},
	})
	if err != nil {
		t.Fatalf("AddEntries: %v", err)
	}
	if created[0].ListenPort == 10003 {
		t.Fatal("expected to skip occupied preferred port")
	}
	if created[0].ListenPort < 10001 {
		t.Fatalf("unexpected port %d", created[0].ListenPort)
	}
}

func TestDeleteUpstreamRemovesAssociatedForwards(t *testing.T) {
	r := sampleResources()
	removed, err := r.DeleteUpstream("server-01")
	if err != nil {
		t.Fatalf("DeleteUpstream: %v", err)
	}
	if removed != 4 {
		t.Fatalf("expected 4 removed forwards (2 validation + 2 production), got %d", removed)
	}
	if len(r.Upstreams) != 1 || r.Upstreams[0].Name != "server-02" {
		t.Fatalf("unexpected upstreams: %+v", r.Upstreams)
	}
	for _, fwd := range r.Forwards {
		if fwd.Upstream == "server-01" {
			t.Fatalf("orphan forward left: %+v", fwd)
		}
	}
	if len(r.Forwards) != 2 {
		t.Fatalf("forwards len=%d", len(r.Forwards))
	}
}

func TestDeleteServerRejectsLastAndMissing(t *testing.T) {
	r := sampleResources()
	if _, err := r.DeleteUpstream("server-01"); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	_, err := r.DeleteUpstream("server-02")
	if err == nil || !strings.Contains(err.Error(), "最后一台") {
		t.Fatalf("expected last-server error, got %v", err)
	}
	_, err = r.DeleteUpstream("missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestValidateDetectsDuplicateServerNames(t *testing.T) {
	r := sampleResources()
	r.Upstreams = append(r.Upstreams, Upstream{
		Name: "server-01", Address: "10.0.0.99",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	})
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("expected duplicate validation error, got %v", err)
	}
}

func TestValidatePassesSample(t *testing.T) {
	if err := sampleResources().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejectsUnknownServerOnDisabledRule(t *testing.T) {
	r := sampleResources()
	r.Forwards = append(r.Forwards, Forward{
		Name: "orphan", Entry: "production", Upstream: "missing",
		Protocol: "TCP", ListenPort: 12000, Enabled: false,
	})
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "未知 upstream") {
		t.Fatalf("expected unknown upstream error, got %v", err)
	}
}

func TestValidateRejectsValidationProductionPortOverlap(t *testing.T) {
	r := sampleResources()
	for i := range r.Forwards {
		if r.Forwards[i].Name == "forward-server-01-production-tcp" {
			r.Forwards[i].ListenPort = 11001
		}
	}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "端口冲突") {
		t.Fatalf("expected port conflict, got %v", err)
	}
}

func TestValidateRejectsInvalidEntry(t *testing.T) {
	r := sampleResources()
	r.Forwards[0].Entry = "experimental"
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "entry") {
		t.Fatalf("expected entry error, got %v", err)
	}
}

func TestLifecycleStatus(t *testing.T) {
	r := sampleResources()
	rows := r.LifecycleStatus()
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	var s01 UpstreamLifecycle
	for _, lc := range rows {
		if lc.Name == "server-01" {
			s01 = lc
		}
	}
	if !s01.ValidationEnabled || s01.ProductionEnabled {
		t.Fatalf("unexpected lifecycle: %+v", s01)
	}
	tcp, udp := r.ValidationListenPorts()
	if tcp != 11001 || udp != 11001 {
		t.Fatalf("validation ports tcp=%d udp=%d", tcp, udp)
	}
	ptcp, pudp := r.ProductionListenPorts()
	if ptcp != 0 || pudp != 0 {
		t.Fatalf("expected no enabled production ports, got tcp=%d udp=%d", ptcp, pudp)
	}
	// Enable one production rule and confirm ProductionListenPorts.
	for i := range r.Forwards {
		if r.Forwards[i].Name == "forward-server-01-production-tcp" {
			r.Forwards[i].Enabled = true
		}
		if r.Forwards[i].Name == "forward-server-01-production-udp" {
			r.Forwards[i].Enabled = true
		}
	}
	ptcp, pudp = r.ProductionListenPorts()
	if ptcp != 10001 || pudp != 10001 {
		t.Fatalf("production ports tcp=%d udp=%d", ptcp, pudp)
	}
}

func TestUpdateServerCascadesDisable(t *testing.T) {
	r := sampleResources()
	result, err := r.UpdateUpstream("server-01", "10.0.0.11", ProtoPortOf(7777), ProtoPortOf(7778), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.CascadedForwards < 2 {
		t.Fatalf("expected cascaded forwards, got %d", result.CascadedForwards)
	}
	for _, fwd := range r.Forwards {
		if fwd.Upstream == "server-01" && fwd.Enabled {
			t.Fatalf("forward still enabled: %+v", fwd)
		}
	}
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "没有启用的 forwards") {
		// server-02 production is disabled; all enabled were on server-01
		if err == nil {
			t.Fatal("expected validate fail with no enabled forwards")
		}
	}
}

func TestEnableProductionCreatesWhenMissing(t *testing.T) {
	r := &Resources{
		Upstreams: []Upstream{
			{Name: "server-40", Address: "10.0.0.40", TCP: ProtoPortOf(7777), Enabled: true},
		},
		Forwards: []Forward{
			{Name: "forward-server-40-validation-tcp", Entry: "validation", Upstream: "server-40", Protocol: "TCP", ListenPort: 11040, Enabled: true},
		},
	}
	changed, err := r.EnableProductionForUpstream("server-40", true)
	if err != nil {
		t.Fatal(err)
	}
	if changed < 1 {
		t.Fatalf("expected created+enabled, changed=%d", changed)
	}
	found := false
	for _, fwd := range r.Forwards {
		if fwd.Entry == EntryProduction && fwd.Upstream == "server-40" && fwd.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("production not created/enabled: %+v", r.Forwards)
	}
}

func TestDiffDetectsToggleAndPortChange(t *testing.T) {
	before := sampleResources()
	after := sampleResources()
	for i := range after.Forwards {
		if after.Forwards[i].Name == "forward-server-01-production-tcp" {
			after.Forwards[i].Enabled = true
			after.Forwards[i].ListenPort = 10099
		}
	}
	after.Upstreams = append(after.Upstreams, Upstream{
		Name: "server-03", Address: "10.0.0.13",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	})
	sum := Diff(before, after)
	if len(sum.UpstreamsAdded) != 1 || sum.UpstreamsAdded[0] != "server-03" {
		t.Fatalf("upstreams added: %+v", sum.UpstreamsAdded)
	}
	if len(sum.ForwardsToggled) == 0 && len(sum.PortChanges) == 0 {
		t.Fatalf("expected toggle or port change, got %+v", sum)
	}
	text := sum.String()
	if !strings.Contains(text, "+ upstream") && !strings.Contains(text, "~ forward") && !strings.Contains(text, "~ port") {
		t.Fatalf("summary text: %s", text)
	}
	if strings.Contains(text, "变更摘要") || strings.Contains(text, "相对备份") {
		t.Fatalf("summary should not include redundant labels: %s", text)
	}
}

func TestChangeSummaryStringEmpty(t *testing.T) {
	text := (ChangeSummary{}).String()
	if strings.TrimSpace(text) != "无差异" {
		t.Fatalf("empty summary want 无差异, got %q", text)
	}
	if strings.Contains(text, "相对") || strings.Contains(text, "server/rule") {
		t.Fatalf("empty summary must stay terse: %q", text)
	}
	withNote := ChangeSummary{Note: "（无当前配置）"}.String()
	if strings.TrimSpace(withNote) != "（无当前配置）" {
		t.Fatalf("note preserved: %q", withNote)
	}
}

func TestDiffDefaultsAndSecurity(t *testing.T) {
	before := sampleResources()
	before.Security.PolicyByID(PolicyGatewayNewConnLimit).Params.PerSec = 200
	before.Security.PolicyByID(PolicyFirewallUDPLimit).Params.UDPPPSPerIP = "500/second"
	after := sampleResources()
	after.Security.PolicyByID(PolicyGatewayNewConnLimit).Params.PerSec = 400
	after.Security.PolicyByID(PolicyFirewallUDPLimit).Params.UDPPPSPerIP = "1200/second"
	after.Security.Access.Deny = []string{"1.2.3.4/32"}
	sum := Diff(before, after)
	if len(sum.SecurityChanged) == 0 {
		t.Fatalf("expected security change: %+v", sum)
	}
	text := sum.String()
	if !strings.Contains(text, "security") {
		t.Fatalf("summary: %s", text)
	}
}

func TestAllowlistNormalize(t *testing.T) {
	r := sampleResources()
	a := r.Security.Access
	a.Deny = append(a.Deny, "8.8.8.8")
	a.Allow = append(a.Allow, "10.0.0.0/8")
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if a.Deny[0] != "8.8.8.8/32" {
		t.Fatalf("deny normalized: %+v", a.Deny)
	}
	a.Deny = nil
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeCIDR("not-a-cidr"); err == nil {
		t.Fatal("expected invalid cidr")
	}
}

func TestEnsureSecurityDefaults(t *testing.T) {
	s := Security{}
	s.EnsureSecurityDefaults()
	n := s.EffectiveFirewallRates()
	if n.TCPNewConnPerIP != "30/second" || n.TCPBurst != 60 {
		t.Fatalf("unexpected defaults: %+v", n)
	}
}

func TestEnsureEntriesAddsMissingProtocol(t *testing.T) {
	r := &Resources{
		Upstreams: []Upstream{
			{Name: "server-30", Address: "10.0.0.30", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7777), Enabled: true},
		},
		Forwards: []Forward{
			{Name: "forward-server-30-validation-tcp", Entry: "validation", Upstream: "server-30", Protocol: "TCP", ListenPort: 11030, Enabled: true},
			{Name: "forward-server-30-production-tcp", Entry: "production", Upstream: "server-30", Protocol: "TCP", ListenPort: 10030, Enabled: false},
		},
	}
	created, err := r.EnsureEntries("server-30", EntryValidation, []string{"UDP"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || created[0].Name != "forward-server-30-validation-udp" || created[0].ListenPort != 11030 {
		t.Fatalf("unexpected: %+v", created)
	}
	prod, err := r.EnsureEntries("server-30", EntryProduction, []string{"UDP"}, false)
	if err != nil || len(prod) != 1 {
		t.Fatalf("prod: %+v err=%v", prod, err)
	}
	again, err := r.EnsureEntries("server-30", EntryValidation, []string{"TCP", "UDP"}, false)
	if err != nil || len(again) != 0 {
		t.Fatalf("idempotent: %+v err=%v", again, err)
	}
}

func TestEnsureEntriesNoForwardsCreatesProduction(t *testing.T) {
	r := &Resources{
		Upstreams: []Upstream{
			{Name: "server-31", Address: "10.0.0.31", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true},
		},
	}
	created, err := r.EnsureEntries("server-31", EntryProduction, []string{"UDP"}, true)
	if err != nil {
		t.Fatalf("EnsureEntries: %v", err)
	}
	if len(created) != 1 || created[0].Name != "forward-server-31-production-udp" || !created[0].Enabled {
		t.Fatalf("unexpected: %+v", created)
	}
}
