package xds

import (
	"strings"
	"testing"
	"time"
)

func TestPrometheusText(t *testing.T) {
	RecordHotApplyOK("snap-42", 1500*time.Millisecond)
	out := PrometheusText("gw-test")
	for _, needle := range []string{
		`relaygate_xds_snapshot_version{gateway="gw-test"} 42`,
		`relaygate_xds_ack_wait_ms{gateway="gw-test"} 1500`,
		`relaygate_hot_apply_total{gateway="gw-test",result="ok"} 1`,
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("missing %q in:\n%s", needle, out)
		}
	}
	RecordHotApplyFail()
	out = PrometheusText("gw-test")
	if !strings.Contains(out, `result="fail"} 1`) {
		t.Fatalf("fail counter missing: %s", out)
	}
}
