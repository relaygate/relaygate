package resources

import (
	"strings"
	"testing"
)

func sampleResources() *Resources {
	return &Resources{
		Servers: []Server{
			{Name: "server-01", Address: "10.0.0.11", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
			{Name: "server-02", Address: "10.0.0.12", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
		Rules: []Rule{
			{Name: "rule-canary-server-01-tcp", Kind: "canary", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "rule-canary-server-01-udp", Kind: "canary", Server: "server-01", Protocol: "UDP", ListenPort: 11001, Enabled: true},
			{Name: "rule-server-01-tcp", Kind: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
			{Name: "rule-server-01-udp", Kind: "production", Server: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: false},
			{Name: "rule-server-02-tcp", Kind: "production", Server: "server-02", Protocol: "TCP", ListenPort: 10002, Enabled: false},
			{Name: "rule-server-02-udp", Kind: "production", Server: "server-02", Protocol: "UDP", ListenPort: 10002, Enabled: false},
		},
	}
}

func TestAddServerCreatesProductionRules(t *testing.T) {
	r := sampleResources()
	created, err := r.AddServer(Server{
		Name: "server-11", Address: "10.0.0.21",
		TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(created))
	}
	if created[0].Name != "rule-server-11-tcp" || created[1].Name != "rule-server-11-udp" {
		t.Fatalf("unexpected rule names: %+v", created)
	}
	for _, rule := range created {
		if rule.Kind != "production" || rule.Server != "server-11" || rule.ListenPort != 10011 || rule.Enabled {
			t.Fatalf("unexpected rule: %+v", rule)
		}
	}
	if len(r.Servers) != 3 {
		t.Fatalf("servers len=%d", len(r.Servers))
	}
	if len(r.Rules) != 8 {
		t.Fatalf("rules len=%d", len(r.Rules))
	}
}

func TestAddServerRejectsDuplicateName(t *testing.T) {
	r := sampleResources()
	_, err := r.AddServer(Server{
		Name: "server-01", Address: "10.0.0.99",
		TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestAddServerRejectsInvalidFields(t *testing.T) {
	r := sampleResources()
	_, err := r.AddServer(Server{Name: "", Address: "10.0.0.21", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777})
	if err == nil {
		t.Fatal("expected empty name error")
	}
	_, err = r.AddServer(Server{Name: "bad name", Address: "10.0.0.21", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777})
	if err == nil {
		t.Fatal("expected invalid name error")
	}
	_, err = r.AddServer(Server{Name: "server-99", Address: "not-an-ip", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777})
	if err == nil {
		t.Fatal("expected invalid address error")
	}
	_, err = r.AddServer(Server{Name: "server-99", Address: "10.0.0.21", TCPPort: 0, UDPPort: 7778, HealthCheckPort: 7777})
	if err == nil {
		t.Fatal("expected port error")
	}
}

func TestAddServerAllocatesNextPortWhenPreferredTaken(t *testing.T) {
	r := sampleResources()
	// Prefer 10003 for server-03, but occupy it with an unrelated rule first.
	r.Rules = append(r.Rules, Rule{
		Name: "rule-extra", Kind: "production", Server: "server-02",
		Protocol: "TCP", ListenPort: 10003, Enabled: false,
	})
	created, err := r.AddServer(Server{
		Name: "server-03", Address: "10.0.0.13",
		TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true,
	})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
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
		t.Fatalf("expected 4 removed rules (2 canary + 2 production), got %d", removed)
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
		TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true,
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
		Name: "orphan", Kind: "production", Server: "missing",
		Protocol: "TCP", ListenPort: 12000, Enabled: false,
	})
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "未知 server") {
		t.Fatalf("expected unknown server error, got %v", err)
	}
}

func TestValidateRejectsCanaryProductionPortOverlap(t *testing.T) {
	r := sampleResources()
	// Point a disabled production rule at the canary port → planning conflict
	for i := range r.Rules {
		if r.Rules[i].Name == "rule-server-01-tcp" {
			r.Rules[i].ListenPort = 11001
		}
	}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "端口冲突") {
		t.Fatalf("expected port conflict, got %v", err)
	}
}

func TestValidateRejectsInvalidKind(t *testing.T) {
	r := sampleResources()
	r.Rules[0].Kind = "experimental"
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind error, got %v", err)
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
	if !s01.CanaryEnabled || s01.ProductionEnabled {
		t.Fatalf("unexpected lifecycle: %+v", s01)
	}
	tcp, udp := r.CanaryListenPorts()
	if tcp != 11001 || udp != 11001 {
		t.Fatalf("canary ports tcp=%d udp=%d", tcp, udp)
	}
}

func TestDiffDetectsToggleAndPortChange(t *testing.T) {
	before := sampleResources()
	after := sampleResources()
	for i := range after.Rules {
		if after.Rules[i].Name == "rule-server-01-tcp" {
			after.Rules[i].Enabled = true
			after.Rules[i].ListenPort = 10099
		}
	}
	after.Servers = append(after.Servers, Server{
		Name: "server-03", Address: "10.0.0.13",
		TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true,
	})
	sum := Diff(before, after)
	if len(sum.ServersAdded) != 1 || sum.ServersAdded[0] != "server-03" {
		t.Fatalf("servers added: %+v", sum.ServersAdded)
	}
	if len(sum.RulesToggled) == 0 && len(sum.PortChanges) == 0 {
		t.Fatalf("expected toggle or port change, got %+v", sum)
	}
	text := sum.String()
	if !strings.Contains(text, "变更摘要") {
		t.Fatalf("summary text: %s", text)
	}
}

func TestDiffDefaultsAndACL(t *testing.T) {
	before := sampleResources()
	before.Defaults.TCPLocalRateLimitPerSec = 200
	before.Defaults.Nft.UDPPPSPerIP = "500/second"
	after := sampleResources()
	after.Defaults = before.Defaults
	after.Defaults.TCPLocalRateLimitPerSec = 400
	after.Defaults.Nft.UDPPPSPerIP = "1200/second"
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

func TestApplyNftDefaults(t *testing.T) {
	d := Defaults{}
	d.ApplyNftDefaults()
	if d.Nft.TCPNewConnPerIP != "30/second" || d.Nft.TCPBurst != 60 {
		t.Fatalf("unexpected defaults: %+v", d.Nft)
	}
	d.Nft.TCPBurst = 99
	d.ApplyNftDefaults()
	if d.Nft.TCPBurst != 99 {
		t.Fatal("should preserve explicit burst")
	}
}

