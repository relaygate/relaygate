package resources

import "testing"

func TestStripAndApplyLocalNodeIdentity(t *testing.T) {
	t.Parallel()
	r := &Resources{
		Meta:    Meta{GatewayName: "gateway-01", ServiceName: "relay"},
		Gateway: Gateway{Name: "gateway-01", PublicIP: "203.0.113.10", ListenAddress: "0.0.0.0", SSHPort: 22},
	}
	StripFleetNodeIdentity(r)
	if r.Meta.GatewayName != "" || r.Gateway.Name != "" || r.Gateway.PublicIP != "" {
		t.Fatalf("strip incomplete: meta=%q gateway=%q ip=%q", r.Meta.GatewayName, r.Gateway.Name, r.Gateway.PublicIP)
	}
	if r.Meta.ServiceName != "relay" || r.Gateway.ListenAddress != "0.0.0.0" || r.Gateway.SSHPort != 22 {
		t.Fatalf("strip removed shared fields: %+v %+v", r.Meta, r.Gateway)
	}
	ApplyLocalNodeIdentity(r, "gateway-03", "203.0.113.33")
	if r.Meta.GatewayName != "gateway-03" || r.Gateway.Name != "gateway-03" {
		t.Fatalf("apply local: meta=%q gateway=%q", r.Meta.GatewayName, r.Gateway.Name)
	}
	if r.Gateway.PublicIP != "203.0.113.33" {
		t.Fatalf("apply local public_ip=%q", r.Gateway.PublicIP)
	}
}
