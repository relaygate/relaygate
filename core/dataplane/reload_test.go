package dataplane

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

func TestApplyMode(t *testing.T) {
	t.Parallel()
	env := Env{XDSEnabled: true}
	summary := resources.ChangeSummary{ServersChanged: []string{"s1"}}
	if got := ApplyMode(env, summary); got != "hot" {
		t.Fatalf("got %q want hot", got)
	}
	summary.MetaChanged = []string{"admin_port 9901→9902"}
	if got := ApplyMode(env, summary); got != "hard" {
		t.Fatalf("got %q want hard", got)
	}
	env.XDSEnabled = false
	if got := ApplyMode(env, summary); got != "hard" {
		t.Fatalf("off flag got %q want hard", got)
	}
}

func TestReloadToXDSOffUsesHardPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	setupMinimalRoot(root, false)
	var buf bytes.Buffer
	err := ReloadTo(root, &buf, &buf, ReloadOptions{})
	if err != nil {
		// docker restart may fail in CI — log should mention Hard path markers
		if !bytes.Contains(buf.Bytes(), []byte("backup")) {
			t.Fatalf("unexpected output: %s err=%v", buf.String(), err)
		}
		return
	}
	if !bytes.Contains(buf.Bytes(), []byte("reloaded")) && !bytes.Contains(buf.Bytes(), []byte("restart")) {
		t.Fatalf("output: %s", buf.String())
	}
}

func setupMinimalRoot(root string, xdsEnabled bool) {
	// RELAYGATE_DATA_DIR cleared in TestMain (t.Parallel cannot use t.Setenv).
	data := config.ResolveDataDir(root)
	_ = os.MkdirAll(filepath.Join(root, "packaging"), 0o755)
	_ = os.MkdirAll(filepath.Join(data, "envoy"), 0o755)
	_ = os.MkdirAll(filepath.Join(data, "firewall"), 0o755)
	_ = os.MkdirAll(filepath.Join(data, "backups"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "packaging", "compose.yaml"), []byte("services: {}"), 0o644)
	res := `# minimal
meta:
  gateway_name: gw-test
  admin_port: 9901
  admin_address: 127.0.0.1
gateway:
  listen_address: 0.0.0.0
defaults:
  max_connections: 100
  tcp_local_rate_limit_per_sec: 10
  tcp_local_rate_limit_burst: 20
servers:
  - name: server-01
    address: 10.0.0.1
    tcp: { port: 7777 }
    udp: { port: 7778 }
    enabled: true
rules:
  - name: forward-server-01-production-tcp
    entry: production
    server: server-01
    protocol: TCP
    listen_port: 10001
    enabled: true
`
	_ = os.WriteFile(filepath.Join(data, "resources.yaml"), []byte(res), 0o644)
	xdsVal := "0"
	if xdsEnabled {
		xdsVal = "1"
	}
	env := "GATEWAY_NAME=gw-test\nENVOY_ADMIN_PORT=9901\nXDS_ENABLED=" + xdsVal + "\n"
	_ = os.WriteFile(filepath.Join(root, ".env"), []byte(env), 0o644)
}
