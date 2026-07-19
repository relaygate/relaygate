package assets

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
			{Name: "rule-server-01-tcp-game", Kind: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
			{Name: "rule-server-01-udp-game", Kind: "production", Server: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: false},
			{Name: "rule-server-02-tcp-game", Kind: "production", Server: "server-02", Protocol: "TCP", ListenPort: 10002, Enabled: false},
			{Name: "rule-server-02-udp-game", Kind: "production", Server: "server-02", Protocol: "UDP", ListenPort: 10002, Enabled: false},
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
	if created[0].Name != "rule-server-11-tcp-game" || created[1].Name != "rule-server-11-udp-game" {
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
