package dataplane

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/relaygate/relaygate/core/resources"
	"github.com/relaygate/relaygate/core/xds"
)

func TestEnsureGatewayADSStartsAndPublishes(t *testing.T) {
	xds.Global().Stop()
	t.Cleanup(func() { xds.Global().Stop() })

	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Meta: resources.Meta{
			ServiceName:  "relay",
			EnvoyImage:   "envoyproxy/envoy:v1.39.0",
			AdminPort:    9901,
			AdminAddress: "127.0.0.1",
		},
		Gateway: resources.Gateway{
			Name:          "gateway-01",
			PublicIP:      "203.0.113.10",
			ListenAddress: "0.0.0.0",
		},
		Defaults: resources.Defaults{
			DefaultUpstreamTCPPort: 4000,
			DefaultUpstreamUDPPort: 4000,
			TCPIdleTimeout:         "3600s",
			UDPIdleTimeout:         "300s",
			MaxPendingRequests:     1024,
			HealthCheck: resources.HealthCheck{
				Timeout:            "2s",
				Interval:           "10s",
				UnhealthyThreshold: 3,
				HealthyThreshold:   2,
			},
		},
		Upstreams: []resources.Upstream{{
			Name:    "server-01",
			Address: "203.0.113.20",
			TCP:     resources.ProtoPortOf(4000),
			Enabled: true,
		}},
		Forwards: []resources.Forward{{
			Name:       "tcp-4000",
			Entry:      "production",
			Protocol:   "TCP",
			ListenPort: 4000,
			Upstream:   "server-01",
			Enabled:    true,
		}},
	}
	if err := resources.Save(filepath.Join(data, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	env := Env{
		XDSEnabled:  true,
		XDSPort:     strconv.Itoa(port),
		GatewayName: "gateway-01",
	}
	if err := EnsureGatewayADS(root, env); err != nil {
		t.Fatalf("EnsureGatewayADS: %v", err)
	}
	srv := xds.Global().Server()
	if srv == nil || !srv.Running() {
		t.Fatal("expected local ADS running")
	}
	if !xds.PortListening(port) {
		t.Fatalf("ADS not listening on %d", port)
	}
	if got := srv.Publisher.LastVersion("gateway-01"); got == "" {
		t.Fatal("expected published snapshot version")
	}
}

func TestEnsureGatewayADSNoopWhenXDSDisabled(t *testing.T) {
	xds.Global().Stop()
	t.Cleanup(func() { xds.Global().Stop() })

	err := EnsureGatewayADS(t.TempDir(), Env{XDSEnabled: false})
	if err != nil {
		t.Fatalf("expected nil: %v", err)
	}
	if xds.Global().Server() != nil {
		t.Fatal("ADS should not start when XDS disabled")
	}
}
