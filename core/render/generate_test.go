package render

import (
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/resources"
)

func testResources() *resources.Resources {
	return &resources.Resources{
		Meta: resources.Meta{
			AdminPort:    9901,
			AdminAddress: "127.0.0.1",
		},
		Gateway: resources.Gateway{ListenAddress: "0.0.0.0"},
		Defaults: resources.Defaults{
			MaxConnections:          1024,
			MaxPendingRequests:      256,
			TCPLocalRateLimitPerSec: 200,
			TCPLocalRateLimitBurst:  400,
			TCPIdleTimeout:          "3600s",
			UDPIdleTimeout:          "120s",
			HealthCheck: resources.HealthCheck{
				Timeout: "2s", Interval: "10s",
				UnhealthyThreshold: 3, HealthyThreshold: 2,
			},
			Nftables: resources.NftablesDefaults{
				TCPNewConnPerIP: "40/second",
				UDPPPSPerIP:     "600/second",
				TCPBurst:        80,
				UDPBurst:        1200,
			},
		},
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCP: resources.ProtoPortOf(7777), UDP: resources.ProtoPortOf(7778), Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "forward-server-01-validation-tcp", Entry: "validation", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-validation-udp", Entry: "validation", Server: "server-01", Protocol: "UDP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-production-tcp", Entry: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
		},
	}
}

func TestEnvoyNaming(t *testing.T) {
	r := testResources()
	cfg, _, err := Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	static := cfg["static_resources"].(map[string]any)
	clusters := static["clusters"].([]any)
	listeners := static["listeners"].([]any)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
	for _, c := range clusters {
		name := c.(map[string]any)["name"].(string)
		if !strings.HasPrefix(name, "upstream-server-01-") {
			t.Fatalf("unexpected cluster name: %s", name)
		}
	}
	for _, l := range listeners {
		name := l.(map[string]any)["name"].(string)
		if !strings.HasPrefix(name, "ingress-forward-server-01-") {
			t.Fatalf("unexpected listener name: %s", name)
		}
	}
}

func TestRenderNFTIncludesPortsAndRateLimits(t *testing.T) {
	r := testResources()
	_, nft, err := Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, needle := range []string{
		"FORWARD_TCP_PORTS = { 11001 }",
		"FORWARD_UDP_PORTS = { 11001 }",
		"FORWARD_TCP_NEW_CONN_RATE = 40",
		"FORWARD_TCP_NEW_CONN_BURST = 80",
		"FORWARD_UDP_PPS_RATE = 600",
		"FORWARD_UDP_PPS_BURST = 1200",
		"ACL_DENY = { 192.0.2.255 }",
		"ACL_ALLOW = { 0.0.0.0/0 }",
		"ACL_ALLOW_STRICT = 0",
	} {
		if !strings.Contains(nft, needle) {
			t.Fatalf("nft missing %q\n%s", needle, nft)
		}
	}
}

func TestRenderNFTIncludesACLSets(t *testing.T) {
	r := testResources()
	r.ACL = resources.ACL{
		Deny:  []string{"1.2.3.4/32", "10.0.0.0/8"},
		Allow: []string{"203.0.113.0/24"},
	}
	_, nft, err := Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, needle := range []string{
		"1.2.3.4/32",
		"10.0.0.0/8",
		"203.0.113.0/24",
		"ACL_ALLOW_STRICT = 1",
	} {
		if !strings.Contains(nft, needle) {
			t.Fatalf("nft missing %q\n%s", needle, nft)
		}
	}
}

func TestRenderNFTAppliesDefaultRateLimits(t *testing.T) {
	r := testResources()
	r.Defaults.Nftables = resources.NftablesDefaults{}
	_, nft, err := Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(nft, "FORWARD_TCP_NEW_CONN_RATE = 30") {
		t.Fatalf("expected default rate, got:\n%s", nft)
	}
}

func TestSummarizeIncludesLifecycle(t *testing.T) {
	text := Summarize(testResources())
	if !strings.Contains(text, "校验通过") || !strings.Contains(text, "入口状态") {
		t.Fatalf("summary: %s", text)
	}
	if !strings.Contains(text, "validation=on") {
		t.Fatalf("expected validation on: %s", text)
	}
}
