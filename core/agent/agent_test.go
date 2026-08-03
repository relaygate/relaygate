package agent

import (
	"os"
	"path/filepath"
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
