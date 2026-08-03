package dataplane

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/config"
)

func TestFleetInventory(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	invDir := filepath.Join(config.ResolveDataDir(root), "inventory")
	if err := os.MkdirAll(invDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `GATEWAY_MATRIX=gateway-01,gateway-02
HOST_gateway_01=10.0.0.1
SSH_PORT_gateway_01=2222
HOST_gateway_02=10.0.0.2
`
	if err := os.WriteFile(filepath.Join(invDir, "gateways.env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	path, nodes, err := FleetInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes=%d", len(nodes))
	}
	if nodes[0].Name != "gateway-01" || nodes[0].Host != "10.0.0.1" || nodes[0].SSHPort != "2222" {
		t.Fatalf("node0=%+v", nodes[0])
	}
	if path == "" {
		t.Fatal("empty path")
	}
	hints := FleetCLIHints(root)
	if len(hints) < 2 {
		t.Fatalf("hints=%v", hints)
	}
}

func TestSaveAndRemoveFleetNode(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	if err := SaveFleetNode(root, FleetNode{
		Name:      "gateway-02",
		Host:      "203.0.113.20",
		SSHPort:   "22",
		SSHUser:   "root",
		RemoteDir: "/opt/relaygate",
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveFleetNode(root, FleetNode{
		Name: "gateway-03",
		Host: "203.0.113.30",
	}); err != nil {
		t.Fatal(err)
	}
	_, nodes, err := FleetInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes=%d", len(nodes))
	}
	// upsert
	if err := SaveFleetNode(root, FleetNode{
		Name:    "gateway-02",
		Host:    "203.0.113.21",
		SSHPort: "2222",
	}); err != nil {
		t.Fatal(err)
	}
	_, nodes, err = FleetInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("after upsert nodes=%d", len(nodes))
	}
	found := false
	for _, n := range nodes {
		if n.Name == "gateway-02" {
			found = true
			if n.Host != "203.0.113.21" || n.SSHPort != "2222" {
				t.Fatalf("upserted=%+v", n)
			}
		}
	}
	if !found {
		t.Fatal("gateway-02 missing")
	}
	raw, err := os.ReadFile(filepath.Join(config.ResolveDataDir(root), "inventory", "gateways.env"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "password") || strings.Contains(string(raw), "PRIVATE") {
		t.Fatalf("inventory must not contain secrets: %s", raw)
	}
	if err := RemoveFleetNode(root, "gateway-02"); err != nil {
		t.Fatal(err)
	}
	_, nodes, err = FleetInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Name != "gateway-03" {
		t.Fatalf("after remove=%+v", nodes)
	}
	if err := RemoveFleetNode(root, "gateway-missing"); err == nil {
		t.Fatal("expected error for missing node")
	}
}
