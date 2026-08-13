package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

func TestPreviewDoesNotWrite(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	data := config.ResolveDataDir(root)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	profDir := filepath.Join(root, "packaging", "profiles")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sec := resources.DefaultSecurity()
	sec.PolicyByID(resources.PolicyGatewayConnLimit).Params.MaxConnections = 100
	res := &resources.Resources{
		Upstreams: []resources.Upstream{
			{Name: "server-01", Address: "10.0.0.1", TCP: resources.ProtoPortOf(7777), UDP: resources.ProtoPortOf(7778), Enabled: true},
		},
		Forwards: []resources.Forward{
			{Name: "forward-server-01-production-tcp", Entry: "production", Upstream: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: true},
		},
		Security: sec,
	}
	if err := resources.Save(filepath.Join(data, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}
	body := `name: demo
description: t
defaults:
  tcp_idle_timeout: 1h
  udp_idle_timeout: 1m
security:
  protections:
    - id: gateway_conn_limit
      type: gateway_conn_limit
      enabled: true
      params:
        max_connections: 999
`
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
	if after.Security.PolicyByID(resources.PolicyGatewayConnLimit).Params.MaxConnections != 100 {
		t.Fatalf("preview wrote security: %d", after.Security.PolicyByID(resources.PolicyGatewayConnLimit).Params.MaxConnections)
	}
}
