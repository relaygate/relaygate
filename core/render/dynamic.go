package render

import (
	"sort"
	"strings"

	"github.com/relaygate/relaygate/core/resources"
)

// BuildClusterListenerMaps returns Envoy cluster and listener maps for CDS/LDS (no admin/xds static).
func BuildClusterListenerMaps(r *resources.Resources, opt Options) ([]map[string]any, []map[string]any, error) {
	if err := r.Validate(); err != nil {
		return nil, nil, err
	}
	r.Defaults.ApplyNftablesDefaults()
	servers := r.ServerMap()
	rules := r.EnabledRules()
	defaults := r.Defaults
	listenAddress := r.Gateway.ListenAddress

	clusters := map[string]any{}
	var listeners []map[string]any

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

	clusterList := make([]map[string]any, 0, len(clusters))
	names := make([]string, 0, len(clusters))
	for n := range clusters {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		clusterList = append(clusterList, clusters[n].(map[string]any))
	}
	return clusterList, listeners, nil
}

// RenderMergedForValidate builds a full static Envoy config for --mode validate when xDS is on.
// Business clusters/listeners are inlined in static_resources (bootstrap admin + xds_cluster omitted).
func RenderMergedForValidate(r *resources.Resources, opt Options) (map[string]any, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	clusterList, listeners, err := BuildClusterListenerMaps(r, opt)
	if err != nil {
		return nil, err
	}
	adminAddr := strings.TrimSpace(r.Meta.AdminAddress)
	if adminAddr == "" {
		adminAddr = "127.0.0.1"
	}
	adminPort := r.Meta.AdminPort
	if adminPort <= 0 {
		adminPort = 9901
	}
	listenerAny := make([]any, len(listeners))
	for i, l := range listeners {
		listenerAny[i] = l
	}
	clusterAny := make([]any, len(clusterList))
	for i, c := range clusterList {
		clusterAny[i] = c
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
		},
		"static_resources": map[string]any{
			"listeners": listenerAny,
			"clusters":  clusterAny,
		},
	}, nil
}
