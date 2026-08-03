package render

import (
	"encoding/json"
	"fmt"

	"github.com/relaygate/relaygate/core/resources"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/local_ratelimit/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/udp/udp_proxy/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

var protoUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

func mapToCluster(m map[string]any) (*cluster.Cluster, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	c := &cluster.Cluster{}
	if err := protoUnmarshal.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("cluster %v: %w", m["name"], err)
	}
	return c, nil
}

func mapToListener(m map[string]any) (*listener.Listener, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	l := &listener.Listener{}
	if err := protoUnmarshal.Unmarshal(b, l); err != nil {
		return nil, fmt.Errorf("listener %v: %w", m["name"], err)
	}
	return l, nil
}

// BuildXDSResources converts cluster/listener maps to go-control-plane resource slices.
func BuildXDSResources(clusterMaps []map[string]any, listenerMaps []map[string]any) ([]types.Resource, []types.Resource, error) {
	clusters := make([]types.Resource, 0, len(clusterMaps))
	for _, m := range clusterMaps {
		c, err := mapToCluster(m)
		if err != nil {
			return nil, nil, err
		}
		clusters = append(clusters, c)
	}
	listeners := make([]types.Resource, 0, len(listenerMaps))
	for _, m := range listenerMaps {
		l, err := mapToListener(m)
		if err != nil {
			return nil, nil, err
		}
		listeners = append(listeners, l)
	}
	return clusters, listeners, nil
}

// NewSnapshotFromResources builds a versioned CDS+LDS snapshot from resources.yaml intent.
func NewSnapshotFromResources(r *resources.Resources, opt Options, version string) (*cache.Snapshot, error) {
	clusterMaps, listenerMaps, err := BuildClusterListenerMaps(r, opt)
	if err != nil {
		return nil, err
	}
	clusters, listeners, err := BuildXDSResources(clusterMaps, listenerMaps)
	if err != nil {
		return nil, err
	}
	return cache.NewSnapshot(version, map[resource.Type][]types.Resource{
		resource.ClusterType:  clusters,
		resource.ListenerType: listeners,
	})
}
