package ops

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/relaygate/relaygate/core/resources"
)

func TestResolveCanaryPortsPrefersValidationThenProduction(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Meta: resources.Meta{GatewayName: "gw"},
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCP: resources.ProtoPortOf(7777), Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "forward-server-01-production-tcp", Entry: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: true},
			{Name: "forward-server-01-production-udp", Entry: "production", Server: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: true},
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

	res.Rules = append(res.Rules,
		resources.Rule{Name: "forward-server-01-validation-tcp", Entry: "validation", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
		resources.Rule{Name: "forward-server-01-validation-udp", Entry: "validation", Server: "server-01", Protocol: "UDP", ListenPort: 11001, Enabled: true},
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
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Meta: resources.Meta{GatewayName: "gw"},
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCP: resources.ProtoPortOf(7777), Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "forward-server-01-production-tcp", Entry: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
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
