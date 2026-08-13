package resources

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSecurityPreview(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	packaging := filepath.Join(root, "packaging")

	res := &Resources{
		Upstreams: []Upstream{
			{Name: "server-01", Address: "10.0.0.1", TCP: ProtoPortOf(7777), UDP: ProtoPortOf(7778), Enabled: true},
		},
		Forwards: []Forward{
			{Name: "forward-server-01-production-tcp", Entry: "production", Upstream: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: true},
		},
		Security: DefaultSecurity(),
	}
	fwd := "# define FORWARD_TCP_PORTS = { 10001 }\n"
	gatewayExcerpt := "chain input { policy drop; }"
	prev, err := BuildSecurityPreview(res, packaging, fwd, gatewayExcerpt)
	if err != nil {
		t.Fatal(err)
	}
	if prev.Kernel == nil || !prev.Kernel.Enabled {
		t.Fatal("kernel should be enabled by default")
	}
	if !strings.Contains(prev.Kernel.Content, "tcp_syncookies") {
		t.Fatal("kernel content missing")
	}
	if prev.NIC == nil || prev.NIC.Enabled {
		t.Fatal("nic should be present and disabled by default")
	}
	if prev.Firewall == nil || !strings.Contains(prev.Firewall.ForwardPorts, "FORWARD_TCP_PORTS") {
		t.Fatal("firewall forward ports missing")
	}
	if prev.Gateway == nil || prev.Gateway.MaxConnections != 1024 {
		t.Fatalf("gateway max_conn=%d", prev.Gateway.MaxConnections)
	}
	if len(prev.ExecutionOrder) < 5 {
		t.Fatalf("execution order=%d", len(prev.ExecutionOrder))
	}

	nic := res.Security.PolicyByID(PolicyNICEgressShape)
	if nic == nil {
		t.Fatal("missing nic_egress_shape")
	}
	nic.Enabled = true
	nic.Params.Device = "eth0"
	nic.Params.Rate = "3mbit"
	ing := res.Security.PolicyByID(PolicyNICIngressPolice)
	if ing == nil {
		t.Fatal("missing nic_ingress_police")
	}
	ing.Enabled = true
	ing.Params.Device = "eth0"
	ing.Params.Rate = "3mbit"
	prev2, err := BuildSecurityPreview(res, packaging, fwd, gatewayExcerpt)
	if err != nil {
		t.Fatal(err)
	}
	if prev2.NIC == nil || !prev2.NIC.Enabled || !prev2.NIC.EgressEnabled || prev2.NIC.Device != "eth0" || prev2.NIC.Rate != "3mbit" {
		t.Fatalf("nic egress preview: %+v", prev2.NIC)
	}
	if !prev2.NIC.IngressEnabled || prev2.NIC.IngressDevice != "eth0" || prev2.NIC.IngressRate != "3mbit" {
		t.Fatalf("nic ingress preview: %+v", prev2.NIC)
	}
	if prev2.NIC.ApplyScript == "" || !strings.Contains(prev2.NIC.ApplyScript, "apply-nic") {
		t.Fatalf("nic apply_script=%q", prev2.NIC.ApplyScript)
	}
}

func TestPolicySurfacesSeparateNewConnLimits(t *testing.T) {
	t.Parallel()
	surfaces := PolicySurfaces()
	var fwFound, gwFound, nicFound, ingressFound bool
	for _, s := range surfaces {
		switch s.PolicyID {
		case PolicyFirewallNewConnLimit:
			fwFound = true
			if len(s.Layers) != 1 || s.Layers[0] != string(LayerFirewall) || s.OverlapNote != "" {
				t.Fatalf("firewall_new_conn_limit surface: %+v", s)
			}
		case PolicyGatewayNewConnLimit:
			gwFound = true
			if len(s.Layers) != 1 || s.Layers[0] != string(LayerGateway) || s.OverlapNote != "" {
				t.Fatalf("gateway_new_conn_limit surface: %+v", s)
			}
		case PolicyNICEgressShape:
			nicFound = true
			if len(s.Layers) != 1 || s.Layers[0] != string(LayerNIC) || s.ApplyPath != string(ApplyHostScript) {
				t.Fatalf("nic_egress_shape surface: %+v", s)
			}
		case PolicyNICIngressPolice:
			ingressFound = true
			if len(s.Layers) != 1 || s.Layers[0] != string(LayerNIC) || s.ApplyPath != string(ApplyHostScript) {
				t.Fatalf("nic_ingress_police surface: %+v", s)
			}
		}
	}
	if !fwFound || !gwFound || !nicFound || !ingressFound {
		t.Fatalf("missing surfaces: firewall=%t gateway=%t nic=%t ingress=%t", fwFound, gwFound, nicFound, ingressFound)
	}
}

func TestSecurityExecutionOrderDomains(t *testing.T) {
	t.Parallel()
	order := SecurityExecutionOrder()
	if len(order) < 4 {
		t.Fatalf("order too short: %d", len(order))
	}
	want := []string{string(LayerKernel), string(LayerNIC), string(LayerFirewall)}
	for i, layer := range want {
		if order[i].Layer != layer {
			t.Fatalf("step %d layer=%s want %s", i+1, order[i].Layer, layer)
		}
	}
	last := order[len(order)-1]
	if last.Layer != string(LayerGateway) {
		t.Fatalf("last layer=%s want gateway", last.Layer)
	}
}
