package xds

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var (
	lastSnapshotVersion atomic.Uint64
	lastACKWaitMs       atomic.Uint64
	hotApplyOK          atomic.Uint64
	hotApplyFail        atomic.Uint64
)

// RecordHotApplyOK updates in-process counters after a successful HotApply ACK.
func RecordHotApplyOK(version string, ackWait time.Duration) {
	if v, err := parseVersionCounter(version); err == nil {
		lastSnapshotVersion.Store(v)
	}
	lastACKWaitMs.Store(uint64(ackWait.Milliseconds()))
	hotApplyOK.Add(1)
}

// RecordHotApplyFail increments the failed HotApply counter.
func RecordHotApplyFail() {
	hotApplyFail.Add(1)
}

func parseVersionCounter(version string) (uint64, error) {
	v := strings.TrimSpace(version)
	if v == "" {
		return 0, fmt.Errorf("empty version")
	}
	if i := strings.LastIndex(v, "-"); i >= 0 && i < len(v)-1 {
		v = v[i+1:]
	}
	return strconv.ParseUint(v, 10, 64)
}

// PrometheusText renders exposition-format metrics for scrape or diag.
func PrometheusText(gateway string) string {
	if gateway == "" {
		gateway = "unknown"
	}
	gateway = strings.ReplaceAll(gateway, `"`, `'`)
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP relaygate_xds_snapshot_version Last published xDS snapshot version counter.\n")
	fmt.Fprintf(&b, "# TYPE relaygate_xds_snapshot_version gauge\n")
	fmt.Fprintf(&b, "relaygate_xds_snapshot_version{gateway=%q} %d\n", gateway, lastSnapshotVersion.Load())
	fmt.Fprintf(&b, "# HELP relaygate_xds_ack_wait_ms Milliseconds waited for Envoy ACK on last HotApply.\n")
	fmt.Fprintf(&b, "# TYPE relaygate_xds_ack_wait_ms gauge\n")
	fmt.Fprintf(&b, "relaygate_xds_ack_wait_ms{gateway=%q} %d\n", gateway, lastACKWaitMs.Load())
	fmt.Fprintf(&b, "# HELP relaygate_hot_apply_total HotApply attempts by result.\n")
	fmt.Fprintf(&b, "# TYPE relaygate_hot_apply_total counter\n")
	fmt.Fprintf(&b, "relaygate_hot_apply_total{gateway=%q,result=\"ok\"} %d\n", gateway, hotApplyOK.Load())
	fmt.Fprintf(&b, "relaygate_hot_apply_total{gateway=%q,result=\"fail\"} %d\n", gateway, hotApplyFail.Load())
	return b.String()
}
