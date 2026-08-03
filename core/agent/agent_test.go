package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishJoinLeaveStatus(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "resources.yaml"), []byte("version: 1\nservers: []\nrules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pub, err := Publish(root)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if pub.Version == "" {
		t.Fatal("empty version")
	}
	ver, err := CurrentVersion(root)
	if err != nil || ver != pub.Version {
		t.Fatalf("current=%q err=%v want=%q", ver, err, pub.Version)
	}

	join, err := JoinNode(root, "gateway-02", "http://203.0.113.10:9000")
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if join.Token == "" {
		t.Fatal("empty token")
	}
	if join.JoinCommand == "" || !strings.Contains(join.JoinCommand, "--token ") {
		t.Fatalf("join_command=%q", join.JoinCommand)
	}
	if !strings.Contains(join.JoinCommand, "--control 'http://203.0.113.10:9000'") {
		t.Fatalf("missing control url in command: %q", join.JoinCommand)
	}
	if !strings.Contains(join.JoinCommand, " bash -s -- node ") {
		t.Fatalf("join command missing node subcommand: %q", join.JoinCommand)
	}
	if !strings.Contains(join.JoinCommand, "--name 'gateway-02'") {
		t.Fatalf("join command missing name: %q", join.JoinCommand)
	}
	if strings.Contains(join.JoinCommand, "PANEL_ADMIN") {
		t.Fatal("join command must not include panel admin secrets")
	}
	if strings.Contains(join.JoinCommand, "ENABLE_PANEL=") || strings.Contains(join.JoinCommand, "PRIMARY_URL") || strings.Contains(join.JoinCommand, "NONINTERACTIVE=") {
		t.Fatalf("join command should use short syntax only: %q", join.JoinCommand)
	}
	ctrl := FormatControlInstallCommand()
	if !strings.Contains(ctrl, " bash -s -- control") || strings.Contains(ctrl, "ENABLE_PANEL=") {
		t.Fatalf("control install: %q", ctrl)
	}
	up := FormatUpgradeCommand()
	if !strings.Contains(up, " bash -s -- upgrade") || !strings.Contains(up, "install.sh") {
		t.Fatalf("upgrade: %q", up)
	}
	n, err := LookupByToken(root, join.Token)
	if err != nil || n.Name != "gateway-02" {
		t.Fatalf("lookup: %+v err=%v", n, err)
	}
	if err := RecordHeartbeat(root, "gateway-02", pub.Version); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	published, nodes, err := BuildStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if published != pub.Version {
		t.Fatalf("published=%s", published)
	}
	if len(nodes) != 1 || nodes[0].Status != StatusAligned {
		t.Fatalf("nodes=%+v", nodes)
	}

	if _, err := LeaveNode(root, "gateway-02"); err != nil {
		t.Fatalf("leave: %v", err)
	}
	reg, err := LoadRegistry(root)
	if err != nil || len(reg.Nodes) != 0 {
		t.Fatalf("registry after leave: %+v err=%v", reg, err)
	}
}

func TestJoinRejectsControlLabel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := JoinNode(root, "control", "http://203.0.113.10:9000"); err == nil {
		t.Fatal("expected join control label to fail")
	}
}

func TestLeaveRejectsControlRole(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := &Registry{Nodes: []Node{{Name: "gateway-01", Role: RoleControl}}}
	if err := saveRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	if _, err := LeaveNode(root, "gateway-01"); err == nil {
		t.Fatal("expected leave control role to fail")
	}
}
