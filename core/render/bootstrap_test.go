package render

import (
	"testing"

	"github.com/relaygate/relaygate/core/resources"
)

func TestRenderBootstrapMinimal(t *testing.T) {
	t.Parallel()
	r := testResources()
	r.Gateway.Name = "gateway-01"

	cfg, err := RenderBootstrap(r, BootstrapOptions{XDSPort: 18000})
	if err != nil {
		t.Fatal(err)
	}

	node := cfg["node"].(map[string]any)
	if node["id"] != "gateway-01" || node["cluster"] != "gateway-01" {
		t.Fatalf("node=%v", node)
	}

	static := cfg["static_resources"].(map[string]any)
	if _, hasListeners := static["listeners"]; hasListeners {
		t.Fatal("bootstrap must not embed business listeners")
	}
	clusters := static["clusters"].([]any)
	if len(clusters) != 1 {
		t.Fatalf("want only xds_cluster, got %d", len(clusters))
	}
	c0 := clusters[0].(map[string]any)
	if c0["name"] != "xds_cluster" {
		t.Fatalf("cluster name=%v", c0["name"])
	}

	dyn := cfg["dynamic_resources"].(map[string]any)
	if _, ok := dyn["ads_config"]; !ok {
		t.Fatal("missing ads_config")
	}
	if _, ok := dyn["cds_config"]; !ok {
		t.Fatal("missing cds_config")
	}
	if _, ok := dyn["lds_config"]; !ok {
		t.Fatal("missing lds_config")
	}

	admin := cfg["admin"].(map[string]any)
	addr := admin["address"].(map[string]any)["socket_address"].(map[string]any)
	if addr["port_value"] != 9901 || addr["address"] != "127.0.0.1" {
		t.Fatalf("admin=%v", addr)
	}
}

func TestBootstrapOptionsFromEnvPrefersLocalName(t *testing.T) {
	t.Parallel()
	r := testResources()
	r.Gateway.Name = "gateway-01"
	opt := BootstrapOptionsFromEnv("gateway-03", "18000", r)
	if opt.NodeID != "gateway-03" || opt.NodeCluster != "gateway-03" {
		t.Fatalf("local GATEWAY_NAME must win over fleet gateway.name: %+v", opt)
	}
	opt2 := BootstrapOptionsFromEnv("", "18000", r)
	if opt2.NodeID != "gateway-01" {
		t.Fatalf("empty env should fall back to gateway.name: %+v", opt2)
	}
}

func TestParseXDSPort(t *testing.T) {
	t.Parallel()
	if ParseXDSPort("") != DefaultXDSPort {
		t.Fatal("empty")
	}
	if ParseXDSPort("18001") != 18001 {
		t.Fatal("custom")
	}
	if ParseXDSPort("nope") != DefaultXDSPort {
		t.Fatal("invalid")
	}
}

func TestRenderBootstrapRejectsInvalid(t *testing.T) {
	t.Parallel()
	r := &resources.Resources{}
	if _, err := RenderBootstrap(r, BootstrapOptions{}); err == nil {
		t.Fatal("expected validate error")
	}
}
