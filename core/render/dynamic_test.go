package render

import (
	"testing"
)

func TestBuildClusterListenerMaps(t *testing.T) {
	t.Parallel()
	r := testResources()
	clusters, listeners, err := BuildClusterListenerMaps(r, Options{ProxyProtocol: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 2 {
		t.Fatalf("clusters=%d want 2", len(clusters))
	}
	if len(listeners) != 2 {
		t.Fatalf("listeners=%d want 2", len(listeners))
	}
}

func TestNewSnapshotFromResources(t *testing.T) {
	t.Parallel()
	r := testResources()
	snap, err := NewSnapshotFromResources(r, Options{ProxyProtocol: "off"}, "1")
	if err != nil {
		t.Fatal(err)
	}
	if snap == nil {
		t.Fatal("nil snapshot")
	}
}

func TestMapToCluster(t *testing.T) {
	t.Parallel()
	r := testResources()
	clusters, _, err := BuildClusterListenerMaps(r, Options{ProxyProtocol: "off"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := mapToCluster(clusters[0])
	if err != nil {
		t.Fatal(err)
	}
	if c.GetName() == "" {
		t.Fatal("empty cluster name")
	}
}

func TestRenderMergedForValidate(t *testing.T) {
	t.Parallel()
	r := testResources()
	cfg, err := RenderMergedForValidate(r, Options{ProxyProtocol: "off"})
	if err != nil {
		t.Fatal(err)
	}
	static := cfg["static_resources"].(map[string]any)
	if _, ok := static["listeners"]; !ok {
		t.Fatal("missing listeners")
	}
	if _, ok := static["clusters"]; !ok {
		t.Fatal("missing clusters")
	}
	if _, ok := cfg["dynamic_resources"]; ok {
		t.Fatal("validate merge should not include dynamic_resources")
	}
}
