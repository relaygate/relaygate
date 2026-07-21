package resources

import (
	"strings"
	"testing"
)

func sampleResources() *Resources {
	return &Resources{
		Servers: []Server{
			{Name: "server-01", Address: "10.0.0.11", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true},
			{Name: "server-02", Address: "10.0.0.12", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true},
		},
		Rules: []Rule{
			{Name: "forward-server-01-validation-tcp", Entry: "validation", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-validation-udp", Entry: "validation", Server: "server-01", Protocol: "UDP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-production-tcp", Entry: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
			{Name: "forward-server-01-production-udp", Entry: "production", Server: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: false},
			{Name: "forward-server-02-production-tcp", Entry: "production", Server: "server-02", Protocol: "TCP", ListenPort: 10002, Enabled: false},
			{Name: "forward-server-02-production-udp", Entry: "production", Server: "server-02", Protocol: "UDP", ListenPort: 10002, Enabled: false},
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

func TestAddServerUpstreamOnly(t *testing.T) {
	r := sampleResources()
	rulesBefore := len(r.Rules)
	err := r.AddServer(Server{
		Name: "server-11", Address: "10.0.0.21",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if len(r.Servers) != 3 {
		t.Fatalf("servers len=%d", len(r.Servers))
	}
	if len(r.Rules) != rulesBefore {
		t.Fatalf("AddServer must not create rules; rules=%d want %d", len(r.Rules), rulesBefore)
	}
}

func TestAddEntriesValidationAndProduction(t *testing.T) {
	r := sampleResources()
	if err := r.AddServer(Server{
		Name: "server-14", Address: "10.0.0.24",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := r.AddEntries(AddEntryOptions{
		Server: "server-14", Entry: EntryValidation, Protocols: []string{"TCP", "UDP"},
	})
	if err != nil {
		t.Fatalf("AddEntries validation: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 validation rules, got %d", len(created))
	}
	for _, rule := range created {
		if rule.Entry != EntryValidation || rule.ListenPort != 11014 || !rule.Enabled {
			t.Fatalf("unexpected validation rule: %+v", rule)
		}
	}
	prod, err := r.AddEntries(AddEntryOptions{
		Server: "server-14", Entry: EntryProduction, Protocols: []string{"TCP", "UDP"},
	})
	if err != nil {
		t.Fatalf("AddEntries production: %v", err)
	}
	if len(prod) != 2 {
		t.Fatalf("expected 2 production, got %d", len(prod))
	}
	for _, rule := range prod {
		if rule.Entry != EntryProduction || rule.ListenPort != 10014 || rule.Enabled {
			t.Fatalf("unexpected production rule: %+v", rule)
		}
	}
	// Idempotent
	again, err := r.AddEntries(AddEntryOptions{
		Server: "server-14", Entry: EntryValidation, Protocols: []string{"TCP"},
	})
	if err != nil || len(again) != 0 {
		t.Fatalf("idempotent: %+v err=%v", again, err)
	}
}

func TestAddEntriesRejectsBadProtocols(t *testing.T) {
	r := sampleResources()
	_, err := r.AddEntries(AddEntryOptions{
		Server: "server-01", Entry: EntryProduction, Protocols: []string{"NOPE"},
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

func TestAddServerRejectsDuplicateName(t *testing.T) {
	r := sampleResources()
	err := r.AddServer(Server{
		Name: "server-01", Address: "10.0.0.99",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestAddServerRejectsInvalidFields(t *testing.T) {
	r := sampleResources()
	if err := r.AddServer(Server{Name: "", Address: "10.0.0.21", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778)}); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := r.AddServer(Server{Name: "bad name", Address: "10.0.0.21", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778)}); err == nil {
		t.Fatal("expected invalid name error")
	}
	if err := r.AddServer(Server{Name: "server-99", Address: "not-an-ip", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778)}); err == nil {
		t.Fatal("expected invalid address error")
	}
	if err := r.AddServer(Server{Name: "server-99", Address: "10.0.0.21", UDP: ProtoPortOf(7778), Enabled: true}); err != nil {
		t.Fatalf("udp-only AddServer: %v", err)
	}
	if r.Servers[len(r.Servers)-1].HasTCP() {
		t.Fatal("expected TCP to stay unset")
	}
	if err := r.AddServer(Server{Name: "server-98", Address: "10.0.0.22"}); err == nil {
		t.Fatal("expected port error when no protocols enabled")
	}
}

func TestAddEntriesAllocatesNextPortWhenPreferredTaken(t *testing.T) {
	r := sampleResources()
	r.Rules = append(r.Rules, Rule{
		Name: "forward-extra", Entry: "production", Server: "server-02",
		Protocol: "TCP", ListenPort: 10003, Enabled: false,
	})
	if err := r.AddServer(Server{
		Name: "server-03", Address: "10.0.0.13",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := r.AddEntries(AddEntryOptions{
		Server: "server-03", Entry: EntryProduction, Protocols: []string{"TCP"},
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

func TestDeleteServerRemovesAssociatedRules(t *testing.T) {
	r := sampleResources()
	removed, err := r.DeleteServer("server-01")
	if err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if removed != 4 {
		t.Fatalf("expected 4 removed rules (2 validation + 2 production), got %d", removed)
	}
	if len(r.Servers) != 1 || r.Servers[0].Name != "server-02" {
		t.Fatalf("unexpected servers: %+v", r.Servers)
	}
	for _, rule := range r.Rules {
		if rule.Server == "server-01" {
			t.Fatalf("orphan rule left: %+v", rule)
		}
	}
	if len(r.Rules) != 2 {
		t.Fatalf("rules len=%d", len(r.Rules))
	}
}

func TestDeleteServerRejectsLastAndMissing(t *testing.T) {
	r := sampleResources()
	if _, err := r.DeleteServer("server-01"); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	_, err := r.DeleteServer("server-02")
	if err == nil || !strings.Contains(err.Error(), "最后一台") {
		t.Fatalf("expected last-server error, got %v", err)
	}
	_, err = r.DeleteServer("missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestValidateDetectsDuplicateServerNames(t *testing.T) {
	r := sampleResources()
	r.Servers = append(r.Servers, Server{
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
	r.Rules = append(r.Rules, Rule{
		Name: "orphan", Entry: "production", Server: "missing",
		Protocol: "TCP", ListenPort: 12000, Enabled: false,
	})
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "未知 server") {
		t.Fatalf("expected unknown server error, got %v", err)
	}
}

func TestValidateRejectsValidationProductionPortOverlap(t *testing.T) {
	r := sampleResources()
	for i := range r.Rules {
		if r.Rules[i].Name == "forward-server-01-production-tcp" {
			r.Rules[i].ListenPort = 11001
		}
	}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "端口冲突") {
		t.Fatalf("expected port conflict, got %v", err)
	}
}

func TestValidateRejectsInvalidEntry(t *testing.T) {
	r := sampleResources()
	r.Rules[0].Entry = "experimental"
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
	var s01 ServerLifecycle
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
}

func TestUpdateServerCascadesDisable(t *testing.T) {
	r := sampleResources()
	result, err := r.UpdateServer("server-01", "10.0.0.11", ProtoPortOf(7777), ProtoPortOf(7778), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.CascadedRules < 2 {
		t.Fatalf("expected cascaded rules, got %d", result.CascadedRules)
	}
	for _, rule := range r.Rules {
		if rule.Server == "server-01" && rule.Enabled {
			t.Fatalf("rule still enabled: %+v", rule)
		}
	}
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "没有启用的 rules") {
		// server-02 production is disabled; all enabled were on server-01
		if err == nil {
			t.Fatal("expected validate fail with no enabled rules")
		}
	}
}

func TestEnableProductionCreatesWhenMissing(t *testing.T) {
	r := &Resources{
		Servers: []Server{
			{Name: "server-40", Address: "10.0.0.40", TCP: ProtoPortOf(7777), Enabled: true},
		},
		Rules: []Rule{
			{Name: "forward-server-40-validation-tcp", Entry: "validation", Server: "server-40", Protocol: "TCP", ListenPort: 11040, Enabled: true},
		},
	}
	changed, err := r.EnableProductionForServer("server-40", true)
	if err != nil {
		t.Fatal(err)
	}
	if changed < 1 {
		t.Fatalf("expected created+enabled, changed=%d", changed)
	}
	found := false
	for _, rule := range r.Rules {
		if rule.Entry == EntryProduction && rule.Server == "server-40" && rule.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("production not created/enabled: %+v", r.Rules)
	}
}

func TestDiffDetectsToggleAndPortChange(t *testing.T) {
	before := sampleResources()
	after := sampleResources()
	for i := range after.Rules {
		if after.Rules[i].Name == "forward-server-01-production-tcp" {
			after.Rules[i].Enabled = true
			after.Rules[i].ListenPort = 10099
		}
	}
	after.Servers = append(after.Servers, Server{
		Name: "server-03", Address: "10.0.0.13",
		TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true,
	})
	sum := Diff(before, after)
	if len(sum.ServersAdded) != 1 || sum.ServersAdded[0] != "server-03" {
		t.Fatalf("servers added: %+v", sum.ServersAdded)
	}
	if len(sum.RulesToggled) == 0 && len(sum.PortChanges) == 0 {
		t.Fatalf("expected toggle or port change, got %+v", sum)
	}
	text := sum.String()
	if !strings.Contains(text, "+ server") && !strings.Contains(text, "~ rule") && !strings.Contains(text, "~ port") {
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

func TestDiffDefaultsAndACL(t *testing.T) {
	before := sampleResources()
	before.Defaults.TCPLocalRateLimitPerSec = 200
	before.Defaults.Nftables.UDPPPSPerIP = "500/second"
	after := sampleResources()
	after.Defaults = before.Defaults
	after.Defaults.TCPLocalRateLimitPerSec = 400
	after.Defaults.Nftables.UDPPPSPerIP = "1200/second"
	after.ACL.Deny = []string{"1.2.3.4/32"}
	sum := Diff(before, after)
	if len(sum.DefaultsChanged) == 0 {
		t.Fatalf("expected defaults change: %+v", sum)
	}
	if len(sum.ACLChanged) == 0 {
		t.Fatalf("expected acl change: %+v", sum)
	}
	text := sum.String()
	if !strings.Contains(text, "defaults") || !strings.Contains(text, "acl") {
		t.Fatalf("summary: %s", text)
	}
}

func TestACLNormalizeAndCRUD(t *testing.T) {
	r := sampleResources()
	c, err := r.AddACLEntry("deny", "8.8.8.8")
	if err != nil || c != "8.8.8.8/32" {
		t.Fatalf("add deny: %v %q", err, c)
	}
	if _, err := r.AddACLEntry("deny", "8.8.8.8/32"); err == nil {
		t.Fatal("expected duplicate")
	}
	if _, err := r.AddACLEntry("allow", "10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RemoveACLEntry("deny", "8.8.8.8"); err != nil {
		t.Fatal(err)
	}
	if len(r.ACL.Deny) != 0 {
		t.Fatalf("deny should be empty: %+v", r.ACL.Deny)
	}
	if _, err := NormalizeCIDR("not-a-cidr"); err == nil {
		t.Fatal("expected invalid cidr")
	}
}

func TestApplyNftablesDefaults(t *testing.T) {
	d := Defaults{}
	d.ApplyNftablesDefaults()
	if d.Nftables.TCPNewConnPerIP != "30/second" || d.Nftables.TCPBurst != 60 {
		t.Fatalf("unexpected defaults: %+v", d.Nftables)
	}
	d.Nftables.TCPBurst = 99
	d.ApplyNftablesDefaults()
	if d.Nftables.TCPBurst != 99 {
		t.Fatal("should preserve explicit burst")
	}
}

func TestEnsureEntriesAddsMissingProtocol(t *testing.T) {
	r := &Resources{
		Servers: []Server{
			{Name: "server-30", Address: "10.0.0.30", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7777), Enabled: true},
		},
		Rules: []Rule{
			{Name: "forward-server-30-validation-tcp", Entry: "validation", Server: "server-30", Protocol: "TCP", ListenPort: 11030, Enabled: true},
			{Name: "forward-server-30-production-tcp", Entry: "production", Server: "server-30", Protocol: "TCP", ListenPort: 10030, Enabled: false},
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

func TestEnsureEntriesNoRulesCreatesProduction(t *testing.T) {
	r := &Resources{
		Servers: []Server{
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
