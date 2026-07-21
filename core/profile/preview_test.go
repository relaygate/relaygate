package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

func TestPreviewDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	data := config.ResolveDataDir(root)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	profDir := filepath.Join(root, "packaging", "profiles")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.1", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "forward-server-01-production-tcp", Kind: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: true},
		},
		Defaults: resources.Defaults{MaxConnections: 100},
	}
	res.Defaults.ApplyNftablesDefaults()
	if err := resources.Save(filepath.Join(data, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}
	body := "name: demo\ndescription: t\ndefaults:\n  max_connections: 999\n  tcp_idle_timeout: 1h\n  udp_idle_timeout: 1m\n"
	if err := os.WriteFile(filepath.Join(profDir, "demo.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	sum, err := Preview(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Empty() {
		t.Fatal("expected non-empty diff")
	}
	after, err := resources.Load(filepath.Join(data, "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if after.Defaults.MaxConnections != 100 {
		t.Fatalf("preview wrote defaults: %d", after.Defaults.MaxConnections)
	}
}
