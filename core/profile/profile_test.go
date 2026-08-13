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
	if err := os.MkdirAll(filepath.Join(root, "core", "cmd", "relaygate"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := []byte(`name: default-l4
defaults:
  tcp_idle_timeout: 3600s
  udp_idle_timeout: 120s
  max_pending_requests: 256
  health_check:
    timeout: 2s
    interval: 10s
    unhealthy_threshold: 3
    healthy_threshold: 2
security:
  protections:
    - id: gateway_new_conn_limit
      type: gateway_new_conn_limit
      enabled: true
      params:
        per_sec: 200
        burst: 400
    - id: gateway_conn_limit
      type: gateway_conn_limit
      enabled: true
      params:
        max_connections: 1024
`)
	if err := os.WriteFile(filepath.Join(pack, "default-l4.yaml"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	sec := resources.DefaultSecurity()
	sec.PolicyByID(resources.PolicyGatewayNewConnLimit).Params.PerSec = 50
	sec.PolicyByID(resources.PolicyGatewayConnLimit).Params.MaxConnections = 100
	res := &resources.Resources{
		Defaults: resources.Defaults{
			TCPIdleTimeout: "100s",
			UDPIdleTimeout: "30s",
		},
		Security: sec,
		Upstreams: []resources.Upstream{
			{Name: "server-01", Address: "10.0.0.11", TCP: resources.ProtoPortOf(7777), UDP: resources.ProtoPortOf(7778), Enabled: true},
		},
		Forwards: []resources.Forward{
			{Name: "server-01-validation-tcp", Entry: "validation", Upstream: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
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
	sum, err := Apply(root, "default-l4")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum == nil || (len(sum.DefaultsChanged) == 0 && len(sum.SecurityChanged) == 0) {
		t.Fatalf("expected diff, got %+v", sum)
	}
	after, err := resources.Load(filepath.Join(data, "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if after.Security.PolicyByID(resources.PolicyGatewayNewConnLimit).Params.PerSec != 200 {
		t.Fatalf("expected profile rate 200, got %d", after.Security.PolicyByID(resources.PolicyGatewayNewConnLimit).Params.PerSec)
	}
	p, err := Load(root, "default-l4")
	if err != nil {
		t.Fatal(err)
	}
	text := FormatShow(p)
	if !strings.Contains(text, "default-l4") {
		t.Fatalf("show: %s", text)
	}
}
