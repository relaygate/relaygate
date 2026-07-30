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
	cfg, _, err := RenderWith(r, Options{ProxyProtocol: "off"})
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

func TestTCPAccessLogHasConnID(t *testing.T) {
	cfg, _, err := RenderWith(testResources(), Options{ProxyProtocol: "off"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	listeners := cfg["static_resources"].(map[string]any)["listeners"].([]any)
	found := false
	for _, raw := range listeners {
		l := raw.(map[string]any)
		if !strings.Contains(l["name"].(string), "-tcp") {
			continue
		}
		if _, ok := l["listener_filters"]; ok {
			t.Fatalf("PROXY off must not emit listener_filters")
		}
		fc := l["filter_chains"].([]any)[0].(map[string]any)
		filters := fc["filters"].([]any)
		tcp := filters[len(filters)-1].(map[string]any)
		typed := tcp["typed_config"].(map[string]any)
		al := typed["access_log"].([]any)[0].(map[string]any)
		fmt := al["typed_config"].(map[string]any)["log_format"].(map[string]any)
		jf := fmt["json_format"].(map[string]any)
		if jf["conn_id"] != "%CONNECTION_ID%" {
			t.Fatalf("conn_id=%v", jf["conn_id"])
		}
		if jf["downstream"] != "%DOWNSTREAM_REMOTE_ADDRESS%" {
			t.Fatalf("downstream=%v", jf["downstream"])
		}
		found = true
	}
	if !found {
		t.Fatal("no TCP listener")
	}
}

func TestProxyProtocolV2ListenerFilter(t *testing.T) {
	cfg, _, err := RenderWith(testResources(), Options{ProxyProtocol: "v2"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	listeners := cfg["static_resources"].(map[string]any)["listeners"].([]any)
	tcpOK, udpOK := false, false
	for _, raw := range listeners {
		l := raw.(map[string]any)
		name := l["name"].(string)
		if strings.Contains(name, "-udp") {
			if _, ok := l["listener_filters"]; ok {
				// UDP already has udp_proxy filter; must not add PROXY
				lfs := l["listener_filters"].([]any)
				for _, f := range lfs {
					if f.(map[string]any)["name"] == "envoy.filters.listener.proxy_protocol" {
						t.Fatal("UDP must not get PROXY filter")
					}
				}
			}
			udpOK = true
			continue
		}
		lfs, ok := l["listener_filters"].([]any)
		if !ok || len(lfs) != 1 {
			t.Fatalf("TCP expected 1 PROXY listener_filter, got %#v", l["listener_filters"])
		}
		typed := lfs[0].(map[string]any)["typed_config"].(map[string]any)
		if typed["disallowed_versions"].([]any)[0] != "V1" {
			t.Fatalf("v2 should disallow V1: %#v", typed)
		}
		if _, has := typed["allow_requests_without_proxy_protocol"]; has {
			t.Fatal("allow_without must be unset by default")
		}
		tcpOK = true
	}
	if !tcpOK || !udpOK {
		t.Fatalf("tcpOK=%v udpOK=%v", tcpOK, udpOK)
	}
}

func TestOptionsFromEnvDefaultsOff(t *testing.T) {
	t.Setenv("PROXY_PROTOCOL", "")
	t.Setenv("PROXY_PROTOCOL_ALLOW_WITHOUT", "")
	opt := OptionsFromEnv()
	if opt.ProxyProtocol != "off" || opt.ProxyProtocolAllowWithout {
		t.Fatalf("unset default: %+v", opt)
	}
	t.Setenv("PROXY_PROTOCOL", "off")
	t.Setenv("PROXY_PROTOCOL_ALLOW_WITHOUT", "1")
	opt = OptionsFromEnv()
	if opt.ProxyProtocol != "off" || opt.ProxyProtocolAllowWithout {
		t.Fatalf("off ignores allow_without: %+v", opt)
	}
}

func TestOptionsFromEnvV2Compat(t *testing.T) {
	t.Setenv("PROXY_PROTOCOL_ALLOW_WITHOUT", "")
	t.Setenv("PROXY_PROTOCOL", "v2-compat")
	opt := OptionsFromEnv()
	if opt.ProxyProtocol != "v2" || !opt.ProxyProtocolAllowWithout {
		t.Fatalf("v2-compat: %+v", opt)
	}
	t.Setenv("PROXY_PROTOCOL", "v2")
	t.Setenv("PROXY_PROTOCOL_ALLOW_WITHOUT", "1")
	opt = OptionsFromEnv()
	if opt.ProxyProtocol != "v2" || !opt.ProxyProtocolAllowWithout {
		t.Fatalf("v2+allow: %+v", opt)
	}
}

func TestProxyProtocolCompatEmitsAllowWithout(t *testing.T) {
	cfg, _, err := RenderWith(testResources(), Options{ProxyProtocol: "v2", ProxyProtocolAllowWithout: true})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	listeners := cfg["static_resources"].(map[string]any)["listeners"].([]any)
	found := false
	for _, raw := range listeners {
		l := raw.(map[string]any)
		if !strings.Contains(l["name"].(string), "-tcp") {
			continue
		}
		lfs := l["listener_filters"].([]any)
		typed := lfs[0].(map[string]any)["typed_config"].(map[string]any)
		if typed["allow_requests_without_proxy_protocol"] != true {
			t.Fatalf("expected allow_without: %#v", typed)
		}
		found = true
	}
	if !found {
		t.Fatal("no TCP listener")
	}
}
