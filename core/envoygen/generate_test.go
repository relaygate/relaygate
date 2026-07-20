package envoygen

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
			Nft: resources.NftDefaults{
				TCPNewConnPerIP: "40/second",
				UDPPPSPerIP:     "600/second",
				TCPBurst:        80,
				UDPBurst:        1200,
			},
		},
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "rule-canary-tcp", Kind: "canary", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "rule-canary-udp", Kind: "canary", Server: "server-01", Protocol: "UDP", ListenPort: 11001, Enabled: true},
			{Name: "rule-prod-tcp", Kind: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
		},
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
		"FORWARD_TCP_NEW_CONN_RATE = 40/second",
		"FORWARD_TCP_NEW_CONN_BURST = 80",
		"FORWARD_UDP_PPS_RATE = 600/second",
		"FORWARD_UDP_PPS_BURST = 1200",
	} {
		if !strings.Contains(nft, needle) {
			t.Fatalf("nft missing %q\n%s", needle, nft)
		}
	}
}

func TestRenderNFTAppliesDefaultRateLimits(t *testing.T) {
	r := testResources()
	r.Defaults.Nft = resources.NftDefaults{}
	_, nft, err := Render(r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(nft, "FORWARD_TCP_NEW_CONN_RATE = 30/second") {
		t.Fatalf("expected default rate, got:\n%s", nft)
	}
}

func TestSummarizeIncludesLifecycle(t *testing.T) {
	text := Summarize(testResources())
	if !strings.Contains(text, "校验通过") || !strings.Contains(text, "生命周期状态") {
		t.Fatalf("summary: %s", text)
	}
	if !strings.Contains(text, "canary=on") {
		t.Fatalf("expected canary on: %s", text)
	}
}
