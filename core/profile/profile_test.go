package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/resources"
)

func TestListAndApply(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "packaging", "profiles")
	if err := os.MkdirAll(pack, 0o755); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, ".runtime")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	// Minimal source-tree marker for ResolveDataDir
	if err := os.MkdirAll(filepath.Join(root, "core", "cmd", "relaygate"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("..", "..", "..", "packaging", "profiles", "default-safe.yaml")
	b, err := os.ReadFile(src)
	if err != nil {
		// fallback inline
		b = []byte(`name: default-safe
defaults:
  tcp_idle_timeout: 3600s
  udp_idle_timeout: 120s
  max_connections: 1024
  max_pending_requests: 256
  tcp_local_rate_limit_per_sec: 200
  tcp_local_rate_limit_burst: 400
  health_check:
    timeout: 2s
    interval: 10s
    unhealthy_threshold: 3
    healthy_threshold: 2
  nft:
    tcp_new_conn_per_ip: 30/second
    udp_pps_per_ip: 500/second
    tcp_burst: 60
    udp_burst: 1000
`)
	}
	if err := os.WriteFile(filepath.Join(pack, "default-safe.yaml"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Defaults: resources.Defaults{
			TCPLocalRateLimitPerSec: 50,
			TCPLocalRateLimitBurst:  100,
			MaxConnections:          100,
			TCPIdleTimeout:          "100s",
			UDPIdleTimeout:          "30s",
		},
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "server-01-canary-tcp", Kind: "canary", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
		},
	}
	if err := resources.Save(filepath.Join(data, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELAYGATE_DATA_DIR", data)

	names, err := List(root)
	if err != nil || len(names) == 0 {
		t.Fatalf("List: %v %v", names, err)
	}
	sum, err := Apply(root, "default-safe")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum == nil || len(sum.DefaultsChanged) == 0 {
		t.Fatalf("expected defaults diff, got %+v", sum)
	}
	after, err := resources.Load(filepath.Join(data, "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if after.Defaults.TCPLocalRateLimitPerSec != 200 {
		t.Fatalf("expected profile rate 200, got %d", after.Defaults.TCPLocalRateLimitPerSec)
	}
	p, err := Load(root, "default-safe")
	if err != nil {
		t.Fatal(err)
	}
	text := FormatShow(p)
	if !strings.Contains(text, "default-safe") {
		t.Fatalf("show: %s", text)
	}
}
