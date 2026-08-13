package resources

import "strings"

// StripFleetNodeIdentity clears per-node identity fields from a fleet package.
// Fleet intent syncs business/security config only; node name must come from
// the node's local install (.env GATEWAY_NAME / Envoy --service-node).
func StripFleetNodeIdentity(r *Resources) {
	if r == nil {
		return
	}
	r.Meta.GatewayName = ""
	r.Gateway.Name = ""
	r.Gateway.PublicIP = ""
}

// ApplyLocalNodeIdentity stamps the node's install identity onto resources so
// local tools (doctor/smoke/bootstrap) match .env / Envoy --service-node.
// publicIP may be empty when only the name is known.
func ApplyLocalNodeIdentity(r *Resources, gatewayName, publicIP string) {
	if r == nil {
		return
	}
	if name := strings.TrimSpace(gatewayName); name != "" {
		r.Meta.GatewayName = name
		r.Gateway.Name = name
	}
	if ip := strings.TrimSpace(publicIP); ip != "" {
		r.Gateway.PublicIP = ip
	}
}
