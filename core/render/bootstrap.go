package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/relaygate/relaygate/core/resources"
)

// DefaultXDSPort is the loopback ADS port when XDS_PORT is unset.
const DefaultXDSPort = 18000

// BootstrapOptions controls xDS bootstrap rendering.
type BootstrapOptions struct {
	// XDSPort is the host loopback port for the in-process ADS (default 18000).
	XDSPort int
	// NodeID / NodeCluster become Envoy node.id / node.cluster (usually GATEWAY_NAME).
	NodeID      string
	NodeCluster string
}

// RenderBootstrap builds the minimal Envoy bootstrap: admin + xds_cluster + dynamic_resources.
// Business listeners/clusters must NOT appear in static_resources (they come via ADS CDS/LDS).
func RenderBootstrap(r *resources.Resources, opt BootstrapOptions) (map[string]any, error) {
	if r == nil {
		return nil, fmt.Errorf("resources is nil")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	port := opt.XDSPort
	if port <= 0 {
		port = DefaultXDSPort
	}
	adminAddr := strings.TrimSpace(r.Meta.AdminAddress)
	if adminAddr == "" {
		adminAddr = "127.0.0.1"
	}
	adminPort := r.Meta.AdminPort
	if adminPort <= 0 {
		adminPort = 9901
	}
	nodeID := strings.TrimSpace(opt.NodeID)
	if nodeID == "" {
		nodeID = strings.TrimSpace(r.Gateway.Name)
	}
	if nodeID == "" {
		nodeID = "gateway"
	}
	nodeCluster := strings.TrimSpace(opt.NodeCluster)
	if nodeCluster == "" {
		nodeCluster = nodeID
	}

	return map[string]any{
		"admin": map[string]any{
			"address": map[string]any{
				"socket_address": map[string]any{
					"protocol":   "TCP",
					"address":    adminAddr,
					"port_value": adminPort,
				},
			},
			"access_log": []any{
				map[string]any{
					"name": "envoy.access_loggers.file",
					"typed_config": map[string]any{
						"@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
						"path":  "/var/log/envoy/admin-access.log",
					},
				},
			},
		},
		"node": map[string]any{
			"id":      nodeID,
			"cluster": nodeCluster,
		},
		"static_resources": map[string]any{
			"clusters": []any{
				xdsCluster(port),
			},
		},
		"dynamic_resources": map[string]any{
			"ads_config": map[string]any{
				"api_type":              "GRPC",
				"transport_api_version": "V3",
				"grpc_services": []any{
					map[string]any{
						"envoy_grpc": map[string]any{
							"cluster_name": "xds_cluster",
						},
					},
				},
			},
			"cds_config": map[string]any{
				"ads":                  map[string]any{},
				"resource_api_version": "V3",
			},
			"lds_config": map[string]any{
				"ads":                  map[string]any{},
				"resource_api_version": "V3",
			},
		},
	}, nil
}

func xdsCluster(port int) map[string]any {
	return map[string]any{
		"name":            "xds_cluster",
		"type":            "STATIC",
		"connect_timeout": "1s",
		"load_assignment": map[string]any{
			"cluster_name": "xds_cluster",
			"endpoints": []any{
				map[string]any{
					"lb_endpoints": []any{
						map[string]any{
							"endpoint": map[string]any{
								"address": map[string]any{
									"socket_address": map[string]any{
										"address":    "127.0.0.1",
										"port_value": port,
									},
								},
							},
						},
					},
				},
			},
		},
		"typed_extension_protocol_options": map[string]any{
			"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": map[string]any{
				"@type": "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions",
				"explicit_http_config": map[string]any{
					"http2_protocol_options": map[string]any{},
				},
			},
		},
	}
}

// ParseXDSPort converts an env/port string to int; empty or invalid → DefaultXDSPort.
func ParseXDSPort(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultXDSPort
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 65535 {
		return DefaultXDSPort
	}
	return n
}
