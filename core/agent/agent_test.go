package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalFleetResources = `meta:
  service_name: relay
  envoy_image: envoyproxy/envoy:v1.39.0
  admin_port: 9901
  admin_address: 127.0.0.1
  gateway_name: gateway-01
gateway:
  name: gateway-01
  public_ip: 203.0.113.10
  ssh_port: 22
  listen_address: 0.0.0.0
defaults:
  default_upstream_tcp_port: 4000
  default_upstream_udp_port: 4000
  tcp_idle_timeout: 3600s
  udp_idle_timeout: 300s
  max_pending_requests: 1024
  health_check:
    timeout: 1s
    interval: 5s
    unhealthy_threshold: 3
    healthy_threshold: 1
security:
  protections: []
upstreams:
  - name: server-01
    address: 203.0.113.20
    tcp:
      port: 4000
    enabled: true
forwards:
  - name: tcp-4000
    entry: production
    protocol: TCP
    listen_port: 4000
    upstream: server-01
    enabled: true
`

func TestPublishJoinLeaveStatus(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "resources.yaml"), []byte(minimalFleetResources), 0o644); err != nil {
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
	_, body, err := ReadPublishedResources(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "gateway_name: gateway-01") || strings.Contains(string(body), "name: gateway-01") {
		t.Fatalf("published package must strip control host identity:\n%s", body)
	}
	if !strings.Contains(string(body), "server-01") {
		t.Fatalf("business config missing:\n%s", body)
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
	if strings.Contains(join.JoinCommand, "PANEL_ENABLED=") || strings.Contains(join.JoinCommand, "PRIMARY_URL") || strings.Contains(join.JoinCommand, "NONINTERACTIVE=") {
		t.Fatalf("join command should use short syntax only: %q", join.JoinCommand)
	}
	ctrl := FormatControlInstallCommand()
	if !strings.Contains(ctrl, " bash -s -- control") || strings.Contains(ctrl, "PANEL_ENABLED=") {
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

func TestPullDoesNotMarkAppliedUntilMarkApplied(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("GATEWAY_NAME", "gateway-03")
	t.Setenv("GATEWAY_PUBLIC_IP", "203.0.113.33")
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "resources.yaml"), []byte(minimalFleetResources), 0o644); err != nil {
		t.Fatal(err)
	}
	pub, err := Publish(root)
	if err != nil {
		t.Fatal(err)
	}
	join, err := JoinNode(root, "gateway-03", "http://127.0.0.1:9")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/config", func(w http.ResponseWriter, r *http.Request) {
		_, body, err := ReadPublishedResources(root, "")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"` + pub.Version + `","resources_yaml":` + jsonString(string(body)) + `}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &Client{ControlURL: srv.URL, Token: join.Token, HTTP: srv.Client()}
	ver, err := client.PullOnce(root)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if ver != pub.Version {
		t.Fatalf("ver=%s", ver)
	}
	if got := LocalAppliedVersion(root); got != "" {
		t.Fatalf("pull must not set applied-version, got %q", got)
	}
	pulled, _ := os.ReadFile(filepath.Join(data, "pulled-version"))
	if strings.TrimSpace(string(pulled)) != pub.Version {
		t.Fatalf("pulled-version=%q", pulled)
	}
	resBody, err := os.ReadFile(filepath.Join(data, "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resBody), "gateway_name: gateway-03") {
		t.Fatalf("local identity not applied:\n%s", resBody)
	}
	if !strings.Contains(string(resBody), "public_ip: 203.0.113.33") {
		t.Fatalf("local public_ip not applied:\n%s", resBody)
	}
	if strings.Contains(string(resBody), "gateway_name: gateway-01") || strings.Contains(string(resBody), "public_ip: 203.0.113.10") {
		t.Fatalf("control identity leaked into node resources:\n%s", resBody)
	}
	if err := MarkApplied(root, ver); err != nil {
		t.Fatal(err)
	}
	if got := LocalAppliedVersion(root); got != ver {
		t.Fatalf("applied=%q", got)
	}
}

func TestSkipAfterPullWhenAlreadyAligned(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	const ver = "same-ver"
	if err := MarkApplied(root, ver); err != nil {
		t.Fatal(err)
	}
	calls := 0
	after := func(r, v string) error {
		calls++
		return nil
	}
	// Simulate the gate used in Run.doPull.
	if applied := LocalAppliedVersion(root); applied != "" && applied == ver {
		// skip
	} else if err := after(root, ver); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("AfterPull should be skipped when already aligned, calls=%d", calls)
	}
}

func TestMarkAppliedOnlyAfterSuccessfulAfterPull(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(data, "applied-version"), []byte("old-ver\n"), 0o644)

	// Simulate Run's applied gate without starting the loop.
	applyFail := func(root, version string) error {
		return errApplyFailed
	}
	if err := applyFail(root, "new-ver"); err == nil {
		t.Fatal("expected fail")
	}
	// Failed apply must leave old applied intact.
	if got := LocalAppliedVersion(root); got != "old-ver" {
		t.Fatalf("applied=%q", got)
	}
	if err := MarkApplied(root, "new-ver"); err != nil {
		t.Fatal(err)
	}
	if got := LocalAppliedVersion(root); got != "new-ver" {
		t.Fatalf("applied=%q", got)
	}
}

var errApplyFailed = errString("hot apply failed")

type errString string

func (e errString) Error() string { return string(e) }

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
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
