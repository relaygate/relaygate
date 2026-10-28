package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/relaygate/relaygate/core/resources"
)

const identityFixture = `meta:
    service_name: main
gateway:
    name: gateway-01
    public_ip: 203.0.113.10
    ssh_port: 22
    listen_address: 0.0.0.0
`

func writeIdentityFixture(t *testing.T, root, body string) string {
	t.Helper()
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "resources.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStampSeededGatewayIdentityReplacesTemplate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := writeIdentityFixture(t, root, identityFixture)

	opt := Options{Root: root, GatewayName: "gateway-07", PublicIP: "198.51.100.7", SSHPort: "2222"}
	if err := stampSeededGatewayIdentity(opt); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	res, err := resources.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Gateway.Name != "gateway-07" {
		t.Fatalf("name=%q", res.Gateway.Name)
	}
	if res.Gateway.PublicIP != "198.51.100.7" {
		t.Fatalf("public_ip=%q", res.Gateway.PublicIP)
	}
	if res.Gateway.SSHPort != 2222 {
		t.Fatalf("ssh_port=%d", res.Gateway.SSHPort)
	}
}

func TestStampSeededGatewayIdentityKeepsCustomIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	custom := `meta:
    service_name: main
gateway:
    name: edge-a
    public_ip: 198.51.100.99
    ssh_port: 2200
    listen_address: 0.0.0.0
`
	path := writeIdentityFixture(t, root, custom)

	opt := Options{Root: root, GatewayName: "gateway-07", PublicIP: "198.51.100.7", SSHPort: "2222"}
	if err := stampSeededGatewayIdentity(opt); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	res, err := resources.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if res.Gateway.Name != "edge-a" || res.Gateway.PublicIP != "198.51.100.99" || res.Gateway.SSHPort != 2200 {
		t.Fatalf("custom identity overwritten: %+v", res.Gateway)
	}
}
