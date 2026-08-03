package render

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/relaygate/relaygate/core/resources"
	"gopkg.in/yaml.v3"
)

// Options controls Envoy knobs from host .env (not resources.yaml).
//
// PROXY_PROTOCOL defaults to off: product primary path is public direct exposure
// (no cloud L4 LB in front). Multiple upstreams ≠ need PROXY — PROXY only matters
// when a LB (or similar) prepends PROXY headers before the gateway.
//
// Never auto-enable PROXY/compat on public listeners: clients can forge PROXY
// headers and spoof access-log / ACL source IPs. Compat is only for LB-trusted CIDRs.
type Options struct {
	// ProxyProtocol: off | v1 | v2. Empty / unset → off.
	ProxyProtocol string
	// ProxyProtocolAllowWithout: Envoy allow_requests_without_proxy_protocol
	// (compat: no header → TCP peer; with header → header IP). Unsafe on public ports.
	ProxyProtocolAllowWithout bool
}

// OptionsFromEnv reads PROXY_PROTOCOL / PROXY_PROTOCOL_ALLOW_WITHOUT.
//
//	off (default)     — no PROXY filter; downstream = TCP peer (直连 / preserve)
//	v1 | v2 | on(=v2) — require PROXY header (LB only; ports must not be public)
//	v2-compat|compat  — v2 + allow_without (same as v2 + PROXY_PROTOCOL_ALLOW_WITHOUT=1)
//
// Unknown values → off (fail closed for public exposure).
func OptionsFromEnv() Options {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("PROXY_PROTOCOL")))
	allowEnv := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROXY_PROTOCOL_ALLOW_WITHOUT"))) {
	case "1", "true", "yes", "on":
		allowEnv = true
	}

	mode := "off"
	allow := allowEnv
	switch raw {
	case "", "0", "off", "false", "no", "none":
		mode = "off"
		allow = false
	case "1", "on", "true", "yes", "proxy", "v2":
		mode = "v2"
	case "v1":
		mode = "v1"
	case "v2-compat", "compat", "v2_compat":
		mode = "v2"
		allow = true
	default:
		mode = "off"
		allow = false
	}
	if mode == "off" {
		allow = false
	}
	return Options{ProxyProtocol: mode, ProxyProtocolAllowWithout: allow}
}

// UpstreamClusterName is the Envoy upstream cluster for an upstream server/protocol pair.
// Format: upstream-{server}-{proto} — shared by all forwards for that pair.
// Product display: Upstream; Envoy resource remains cluster.
func UpstreamClusterName(server, protocol string) string {
	return fmt.Sprintf("upstream-%s-%s", server, strings.ToLower(protocol))
}

// IngressListenerName is the Envoy ingress listener for one forwarding rule (1:1).
// Format: ingress-{forwardName} so it stays aligned with resources.yaml rules[].name (forward-*).
func IngressListenerName(rule resources.Rule) string {
	return "ingress-" + rule.Name
}

// rateLimitStatPrefix is the Envoy local_rate_limit stat_prefix (metric name), not a forward name.
// Format: rl_{forwardName with - → _} → e.g. rl_forward_server_01_validation_tcp.
func rateLimitStatPrefix(rule resources.Rule) string {
	return "rl_" + strings.ReplaceAll(rule.Name, "-", "_")
}

func proxyStatPrefix(rule resources.Rule, proto string) string {
	return strings.ToLower(proto) + "_" + strings.ReplaceAll(rule.Name, "-", "_")
}

// Render builds Envoy static config + nft defines using OptionsFromEnv().
func Render(r *resources.Resources) (map[string]any, string, error) {
	return RenderWith(r, OptionsFromEnv())
}

// RenderWith is like Render but with explicit options (tests / callers that already loaded env).
func RenderWith(r *resources.Resources, opt Options) (map[string]any, string, error) {
	if err := r.Validate(); err != nil {
		return nil, "", err
	}
	r.Defaults.ApplyNftablesDefaults()
	servers := r.ServerMap()
	rules := r.EnabledRules()
	defaults := r.Defaults
	listenAddress := r.Gateway.ListenAddress

	clusters := map[string]any{}
	var listeners []any

	for _, rule := range rules {
		server := servers[rule.Server]
		proto := strings.ToUpper(rule.Protocol)
		cname := UpstreamClusterName(server.Name, proto)
		if _, ok := clusters[cname]; !ok {
			if proto == "TCP" {
				clusters[cname] = renderTCPCluster(server, defaults)
			} else {
				clusters[cname] = renderUDPCluster(server, defaults)
			}
		}
		if proto == "TCP" {
			listeners = append(listeners, renderTCPListener(rule, listenAddress, defaults, opt))
		} else {
			listeners = append(listeners, renderUDPListener(rule, listenAddress, defaults))
		}
	}

	clusterList := make([]any, 0, len(clusters))
	names := make([]string, 0, len(clusters))
	for n := range clusters {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		clusterList = append(clusterList, clusters[n])
	}

	cfg := map[string]any{
		"admin": map[string]any{
			"address": map[string]any{
				"socket_address": map[string]any{
					"protocol":   "TCP",
					"address":    r.Meta.AdminAddress,
					"port_value": r.Meta.AdminPort,
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
		"static_resources": map[string]any{
			"listeners": listeners,
			"clusters":  clusterList,
		},
	}
	return cfg, renderNFT(rules, defaults.Nftables, r.ACL), nil
}

func Write(envoyPath, nftPath string, r *resources.Resources) error {
	return WriteWith(envoyPath, nftPath, r, OptionsFromEnv())
}

func WriteWith(envoyPath, nftPath string, r *resources.Resources, opt Options) error {
	cfg, nft, err := RenderWith(r, opt)
	if err != nil {
		return err
	}
	if err := resources.EnsureDir(envoyPath); err != nil {
		return err
	}
	if err := resources.EnsureDir(nftPath); err != nil {
		return err
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# Generated by relaygate. Do not edit.\n# Source: DataDir/resources.yaml\n"
	if err := os.WriteFile(envoyPath, append([]byte(header), body...), 0o644); err != nil {
		return err
	}
	// 显式 chmod：调用方若 umask 077，WriteFile(0644) 会变成 0600，Envoy 容器读失败
	if err := os.Chmod(envoyPath, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(nftPath, []byte(nft), 0o644); err != nil {
		return err
	}
	return os.Chmod(nftPath, 0o644)
}

func renderTCPCluster(server resources.Server, d resources.Defaults) map[string]any {
	d.ApplyOutlierDefaults()
	name := UpstreamClusterName(server.Name, "TCP")
	tcpPort := server.TCPPort()
	hcPort := server.HealthCheckPort()
	endpoint := map[string]any{
		"address": map[string]any{
			"socket_address": map[string]any{
				"address":    server.Address,
				"port_value": tcpPort,
			},
		},
	}
	if hcPort > 0 {
		endpoint["health_check_config"] = map[string]any{
			"port_value": hcPort,
		}
	}
	cluster := map[string]any{
		"name":            name,
		"type":            "STATIC",
		"connect_timeout": "2s",
		"lb_policy":       "ROUND_ROBIN",
		"circuit_breakers": map[string]any{
			"thresholds": []any{
				map[string]any{
					"priority":             "DEFAULT",
					"max_connections":      d.MaxConnections,
					"max_pending_requests": d.MaxPendingRequests,
					"max_requests":         d.MaxConnections,
				},
			},
		},
		"health_checks": []any{
			map[string]any{
				"timeout":             d.HealthCheck.Timeout,
				"interval":            d.HealthCheck.Interval,
				"unhealthy_threshold": d.HealthCheck.UnhealthyThreshold,
				"healthy_threshold":   d.HealthCheck.HealthyThreshold,
				"tcp_health_check":    map[string]any{},
			},
		},
		"load_assignment": map[string]any{
			"cluster_name": name,
			"endpoints": []any{
				map[string]any{
					"lb_endpoints": []any{
						map[string]any{
							"endpoint": endpoint,
						},
					},
				},
			},
		},
	}
	if od := outlierDetectionConfig(d); od != nil {
		cluster["outlier_detection"] = od
	}
	return cluster
}

// outlierDetectionConfig returns Envoy outlier_detection for TCP when enabled.
// Uses local-origin failure counting (connect failures); default off.
func outlierDetectionConfig(d resources.Defaults) map[string]any {
	if !d.OutlierDetection.Enabled {
		return nil
	}
	return map[string]any{
		"split_external_local_origin_errors":     true,
		"consecutive_local_origin_failure":       d.OutlierDetection.ConsecutiveLocalOriginFailure,
		"interval":                               d.OutlierDetection.Interval,
		"base_ejection_time":                     d.OutlierDetection.BaseEjectionTime,
		"max_ejection_percent":                   100,
		"enforcing_consecutive_local_origin_failure": 100,
	}
}

func renderUDPCluster(server resources.Server, d resources.Defaults) map[string]any {
	name := UpstreamClusterName(server.Name, "UDP")
	return map[string]any{
		"name":            name,
		"type":            "STATIC",
		"connect_timeout": "2s",
		"lb_policy":       "ROUND_ROBIN",
		"circuit_breakers": map[string]any{
			"thresholds": []any{
				map[string]any{
					"priority":        "DEFAULT",
					"max_connections": d.MaxConnections,
				},
			},
		},
		"load_assignment": map[string]any{
			"cluster_name": name,
			"endpoints": []any{
				map[string]any{
					"lb_endpoints": []any{
						map[string]any{
							"endpoint": map[string]any{
								"address": map[string]any{
									"socket_address": map[string]any{
										"address":    server.Address,
										"port_value": server.UDPPort(),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func renderTCPListener(rule resources.Rule, listenAddress string, d resources.Defaults, opt Options) map[string]any {
	cname := UpstreamClusterName(rule.Server, "TCP")
	listener := map[string]any{
		"name": IngressListenerName(rule),
		"address": map[string]any{
			"socket_address": map[string]any{
				"protocol":   "TCP",
				"address":    listenAddress,
				"port_value": rule.ListenPort,
			},
		},
		"filter_chains": []any{
			map[string]any{
				"filters": []any{
					map[string]any{
						"name": "envoy.filters.network.local_ratelimit",
						"typed_config": map[string]any{
							"@type":       "type.googleapis.com/envoy.extensions.filters.network.local_ratelimit.v3.LocalRateLimit",
							"stat_prefix": rateLimitStatPrefix(rule),
							"token_bucket": map[string]any{
								"max_tokens":      d.TCPLocalRateLimitBurst,
								"tokens_per_fill": d.TCPLocalRateLimitPerSec,
								"fill_interval":   "1s",
							},
						},
					},
					map[string]any{
						"name": "envoy.filters.network.tcp_proxy",
						"typed_config": map[string]any{
							"@type":        "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy",
							"stat_prefix":  proxyStatPrefix(rule, "TCP"),
							"cluster":      cname,
							"idle_timeout": d.TCPIdleTimeout,
							"access_log": []any{
								map[string]any{
									"name": "envoy.access_loggers.file",
									"typed_config": map[string]any{
										"@type": "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog",
										"path":  "/var/log/envoy/tcp-access.json",
										"log_format": map[string]any{
											"json_format": map[string]any{
												"ts":          "%START_TIME%",
												"rule":        rule.Name,
												"protocol":    "TCP",
												"downstream":  "%DOWNSTREAM_REMOTE_ADDRESS%",
												"upstream":    "%UPSTREAM_HOST%",
												"bytes_rx":    "%BYTES_RECEIVED%",
												"bytes_tx":    "%BYTES_SENT%",
												"duration_ms": "%DURATION%",
												"flags":       "%RESPONSE_FLAGS%",
												"conn_id":     "%CONNECTION_ID%",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if filter := proxyProtocolListenerFilter(opt); filter != nil {
		listener["listener_filters"] = []any{filter}
	}
	return listener
}

// proxyProtocolListenerFilter returns Envoy PROXY listener filter, or nil when off.
// Security: Envoy does not CIDR-scope who may send PROXY; restrict peers via SG/nft to LB only.
func proxyProtocolListenerFilter(opt Options) map[string]any {
	mode := strings.ToLower(strings.TrimSpace(opt.ProxyProtocol))
	if mode == "" || mode == "off" {
		return nil
	}
	typed := map[string]any{
		"@type": "type.googleapis.com/envoy.extensions.filters.listener.proxy_protocol.v3.ProxyProtocol",
	}
	if opt.ProxyProtocolAllowWithout {
		typed["allow_requests_without_proxy_protocol"] = true
	}
	switch mode {
	case "v1":
		typed["disallowed_versions"] = []any{"V2"}
	case "v2":
		typed["disallowed_versions"] = []any{"V1"}
	default:
		return nil
	}
	return map[string]any{
		"name":         "envoy.filters.listener.proxy_protocol",
		"typed_config": typed,
	}
}

func renderUDPListener(rule resources.Rule, listenAddress string, d resources.Defaults) map[string]any {
	cname := UpstreamClusterName(rule.Server, "UDP")
	return map[string]any{
		"name": IngressListenerName(rule),
		"address": map[string]any{
			"socket_address": map[string]any{
				"protocol":   "UDP",
				"address":    listenAddress,
				"port_value": rule.ListenPort,
			},
		},
		"udp_listener_config": map[string]any{
			"downstream_socket_config": map[string]any{
				"max_rx_datagram_size": 1500,
			},
		},
		"listener_filters": []any{
			map[string]any{
				"name": "envoy.filters.udp_listener.udp_proxy",
				"typed_config": map[string]any{
					"@type":        "type.googleapis.com/envoy.extensions.filters.udp.udp_proxy.v3.UdpProxyConfig",
					"stat_prefix":  proxyStatPrefix(rule, "UDP"),
					"idle_timeout": d.UDPIdleTimeout,
					"matcher": map[string]any{
						"on_no_match": map[string]any{
							"action": map[string]any{
								"name": "route",
								"typed_config": map[string]any{
									"@type":   "type.googleapis.com/envoy.extensions.filters.udp.udp_proxy.v3.Route",
									"cluster": cname,
								},
							},
						},
					},
					"upstream_socket_config": map[string]any{
						"max_rx_datagram_size": 1500,
					},
				},
			},
		},
	}
}

func renderNFT(rules []resources.Rule, nft resources.NftablesDefaults, acl resources.ACL) string {
	tcpSet := map[int]struct{}{}
	udpSet := map[int]struct{}{}
	for _, r := range rules {
		switch strings.ToUpper(r.Protocol) {
		case "TCP":
			tcpSet[r.ListenPort] = struct{}{}
		case "UDP":
			udpSet[r.ListenPort] = struct{}{}
		}
	}
	tcp := sortedPorts(tcpSet)
	udp := sortedPorts(udpSet)
	if len(tcp) == 0 {
		tcp = []int{10001}
	}
	if len(udp) == 0 {
		udp = []int{10001}
	}
	// Empty deny: TEST-NET-1 host that should never be a real client source.
	denySet := acl.Deny
	if len(denySet) == 0 {
		denySet = []string{"192.0.2.255"}
	}
	// Empty allow: 0.0.0.0/0 so `saddr != $ACL_ALLOW` never matches (no strict mode).
	allowSet := acl.Allow
	allowStrict := 0
	if len(allowSet) == 0 {
		allowSet = []string{"0.0.0.0/0"}
	} else {
		allowStrict = 1
	}
	return fmt.Sprintf(`# Generated by relaygate. Do not edit.
# Source: DataDir/resources.yaml (defaults.nftables + enabled rules + acl)
# TCP forward (listen) ports from enabled rules
define FORWARD_TCP_PORTS = { %s }
# UDP forward (listen) ports from enabled rules
define FORWARD_UDP_PORTS = { %s }
# Per-IP rate limits (nftables) — numeric defines for nft 1.0.x (rate unit inlined in gateway.nft)
define FORWARD_TCP_NEW_CONN_RATE = %d
define FORWARD_TCP_NEW_CONN_BURST = %d
define FORWARD_UDP_PPS_RATE = %d
define FORWARD_UDP_PPS_BURST = %d
# ACL (nftables truth): deny always applied; allow strict when ACL_ALLOW_STRICT=1
define ACL_DENY = { %s }
define ACL_ALLOW = { %s }
define ACL_ALLOW_STRICT = %d
`, joinInts(tcp), joinInts(udp),
		nftRateNumber(nft.TCPNewConnPerIP, 30), nft.TCPBurst,
		nftRateNumber(nft.UDPPPSPerIP, 500), nft.UDPBurst,
		strings.Join(denySet, ", "),
		strings.Join(allowSet, ", "),
		allowStrict)
}

func sortedPorts(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// nftRateNumber extracts the numeric part of a rate like "30/second" for nft define.
// Old nft (e.g. 1.0.2) rejects string defines; gateway.nft appends "/second" inline.
func nftRateNumber(rate string, fallback int) int {
	rate = strings.TrimSpace(rate)
	if rate == "" {
		return fallback
	}
	if i := strings.IndexByte(rate, '/'); i > 0 {
		rate = rate[:i]
	}
	v, err := strconv.Atoi(strings.TrimSpace(rate))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func joinInts(ports []int) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = fmt.Sprintf("%d", p)
	}
	return strings.Join(parts, ", ")
}

func Summarize(r *resources.Resources) string {
	servers := r.ServerMap()
	rules := r.EnabledRules()
	var b strings.Builder
	fmt.Fprintf(&b, "校验通过: %d 台上游, %d 条启用转发\n", len(servers), len(rules))
	for _, rule := range rules {
		fmt.Fprintf(&b, "  - %s: %s/%d -> %s (%s)\n",
			rule.Name, strings.ToUpper(rule.Protocol), rule.ListenPort, rule.Server, rule.Entry)
	}
	b.WriteString(resources.FormatLifecycle(r))
	return b.String()
}
