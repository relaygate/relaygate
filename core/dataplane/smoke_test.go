package dataplane

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

func TestResolveCanaryPortsPrefersValidationThenProduction(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	dataDir := config.ResolveDataDir(root)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Gateway: resources.Gateway{Name: "gw"},
		Upstreams: []resources.Upstream{
			{Name: "server-01", Address: "10.0.0.11", TCP: resources.ProtoPortOf(7777), Enabled: true},
		},
		Forwards: []resources.Forward{
			{Name: "forward-server-01-production-tcp", Entry: "production", Upstream: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: true},
			{Name: "forward-server-01-production-udp", Entry: "production", Upstream: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: true},
		},
	}
	writeResources(t, filepath.Join(dataDir, "resources.yaml"), res)

	tcp, udp, source, err := resolveCanaryPorts(root, "11001", "11001")
	if err != nil {
		t.Fatal(err)
	}
	if tcp != "10001" || udp != "10001" || source != "resources production" {
		t.Fatalf("got tcp=%s udp=%s source=%s", tcp, udp, source)
	}

	res.Forwards = append(res.Forwards,
		resources.Forward{Name: "forward-server-01-validation-tcp", Entry: "validation", Upstream: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
		resources.Forward{Name: "forward-server-01-validation-udp", Entry: "validation", Upstream: "server-01", Protocol: "UDP", ListenPort: 11001, Enabled: true},
	)
	writeResources(t, filepath.Join(dataDir, "resources.yaml"), res)

	tcp, udp, source, err = resolveCanaryPorts(root, "11001", "11001")
	if err != nil {
		t.Fatal(err)
	}
	if tcp != "11001" || udp != "11001" || source != "resources validation" {
		t.Fatalf("got tcp=%s udp=%s source=%s", tcp, udp, source)
	}
}

func TestResolveCanaryPortsNoEnabledEntries(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	dataDir := config.ResolveDataDir(root)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Gateway: resources.Gateway{Name: "gw"},
		Upstreams: []resources.Upstream{
			{Name: "server-01", Address: "10.0.0.11", TCP: resources.ProtoPortOf(7777), Enabled: true},
		},
		Forwards: []resources.Forward{
			{Name: "forward-server-01-production-tcp", Entry: "production", Upstream: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
		},
	}
	writeResources(t, filepath.Join(dataDir, "resources.yaml"), res)

	_, _, _, err := resolveCanaryPorts(root, "11001", "11001")
	if err == nil {
		t.Fatal("expected error when no enabled entry ports")
	}
}

func writeResources(t *testing.T, path string, res *resources.Resources) {
	t.Helper()
	b, err := yaml.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
