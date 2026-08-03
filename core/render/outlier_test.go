package render

import (
	"testing"

	"github.com/relaygate/relaygate/core/resources"
)

func TestOutlierDetectionOffByDefault(t *testing.T) {
	t.Parallel()
	r := testResources()
	cfg, _, err := RenderWith(r, Options{ProxyProtocol: "off"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cfg["static_resources"].(map[string]any)["clusters"].([]any) {
		m := c.(map[string]any)
		if _, ok := m["outlier_detection"]; ok {
			t.Fatalf("cluster %v should not have outlier_detection when disabled", m["name"])
		}
	}
}

func TestOutlierDetectionEnabledOnTCP(t *testing.T) {
	t.Parallel()
	r := testResources()
	r.Defaults.OutlierDetection = resources.OutlierDetection{
		Enabled:                       true,
		ConsecutiveLocalOriginFailure: 5,
		Interval:                      "10s",
		BaseEjectionTime:              "30s",
	}
	cfg, _, err := RenderWith(r, Options{ProxyProtocol: "off"})
	if err != nil {
		t.Fatal(err)
	}
	var tcp map[string]any
	for _, c := range cfg["static_resources"].(map[string]any)["clusters"].([]any) {
		m := c.(map[string]any)
		if m["name"] == "upstream-server-01-tcp" {
			tcp = m
			break
		}
	}
	if tcp == nil {
		t.Fatal("missing tcp cluster")
	}
	od, ok := tcp["outlier_detection"].(map[string]any)
	if !ok {
		t.Fatal("expected outlier_detection on TCP cluster")
	}
	if od["consecutive_local_origin_failure"] != 5 {
		t.Fatalf("got %#v", od["consecutive_local_origin_failure"])
	}
	if od["split_external_local_origin_errors"] != true {
		t.Fatal("want split_external_local_origin_errors")
	}
}
