package resources

import (
	"fmt"
	"sort"
	"strings"
)

// ServerLifecycle summarizes validation/production entry enablement for one upstream.
type UpstreamLifecycle struct {
	Name                   string   `json:"name"`
	UpstreamEnabled        bool     `json:"upstream_enabled"`
	ValidationEnabled      bool     `json:"validation_enabled"`
	ProductionEnabled      bool     `json:"production_enabled"`
	ValidationPorts        []string `json:"validation_ports"` // e.g. "TCP/11001"
	ProductionPorts        []string `json:"production_ports"`
	ValidationForwardCount int      `json:"validation_forward_count"`
	ProductionForwardCount int      `json:"production_forward_count"`
	Protocols              []string `json:"protocols"`
}

// LifecycleStatus returns per-server validation→production visibility (from Forwards).
// Protocols come from the Upstream's enabled upstreams (tcp/udp ports).
func (r *Resources) LifecycleStatus() []UpstreamLifecycle {
	byName := make(map[string]*UpstreamLifecycle, len(r.Upstreams))
	order := make([]string, 0, len(r.Upstreams))
	for _, s := range r.Upstreams {
		order = append(order, s.Name)
		byName[s.Name] = &UpstreamLifecycle{
			Name:          s.Name,
			UpstreamEnabled: s.Enabled,
			Protocols:     s.EnabledProtocols(),
		}
	}
	for _, fwd := range r.Forwards {
		lc, ok := byName[fwd.Upstream]
		if !ok {
			continue
		}
		proto := strings.ToUpper(strings.TrimSpace(fwd.Protocol))
		port := fmt.Sprintf("%s/%d", proto, fwd.ListenPort)
		switch strings.ToLower(strings.TrimSpace(fwd.Entry)) {
		case EntryValidation:
			lc.ValidationForwardCount++
			if fwd.Enabled {
				lc.ValidationEnabled = true
				lc.ValidationPorts = append(lc.ValidationPorts, port)
			}
		case EntryProduction:
			lc.ProductionForwardCount++
			if fwd.Enabled {
				lc.ProductionEnabled = true
				lc.ProductionPorts = append(lc.ProductionPorts, port)
			}
		}
	}
	out := make([]UpstreamLifecycle, 0, len(order))
	for _, name := range order {
		lc := byName[name]
		sort.Strings(lc.ValidationPorts)
		sort.Strings(lc.ProductionPorts)
		out = append(out, *lc)
	}
	return out
}

// FormatLifecycle prints a human-readable validation/production entry matrix.
// Panel OpsLogView treats this block as informational (never error-colored).
func FormatLifecycle(r *Resources) string {
	rows := r.LifecycleStatus()
	var b strings.Builder
	fmt.Fprintf(&b, "## 入口状态: %d 台上游\n", len(rows))
	for _, lc := range rows {
		srv := "off"
		if lc.UpstreamEnabled {
			srv = "on"
		}
		validation := "—"
		if lc.ValidationForwardCount > 0 {
			if lc.ValidationEnabled {
				validation = "on " + strings.Join(lc.ValidationPorts, ",")
			} else {
				validation = "off"
			}
		}
		prod := "—"
		if lc.ProductionForwardCount > 0 {
			if lc.ProductionEnabled {
				prod = "on " + strings.Join(lc.ProductionPorts, ",")
			} else {
				prod = "off（可启用正式入口后 reload）"
			}
		}
		// Use "·" (not "-") so change-summary coloring does not treat rows as
		// `  - upstream <removed>` when names are server-*.
		fmt.Fprintf(&b, "  · %s upstream=%s validation=%s production=%s\n", lc.Name, srv, validation, prod)
	}
	return b.String()
}

// EntryListenPorts returns the first enabled TCP/UDP listen ports for entry (0 if none).
func (r *Resources) EntryListenPorts(entry string) (tcpPort, udpPort int) {
	want := strings.ToLower(strings.TrimSpace(entry))
	for _, fwd := range r.EnabledForwards() {
		if strings.ToLower(strings.TrimSpace(fwd.Entry)) != want {
			continue
		}
		switch strings.ToUpper(fwd.Protocol) {
		case "TCP":
			if tcpPort == 0 {
				tcpPort = fwd.ListenPort
			}
		case "UDP":
			if udpPort == 0 {
				udpPort = fwd.ListenPort
			}
		}
	}
	return tcpPort, udpPort
}

// ValidationListenPorts returns enabled validation TCP/UDP listen ports (0 if none).
func (r *Resources) ValidationListenPorts() (tcpPort, udpPort int) {
	return r.EntryListenPorts(EntryValidation)
}

// ProductionListenPorts returns enabled production TCP/UDP listen ports (0 if none).
func (r *Resources) ProductionListenPorts() (tcpPort, udpPort int) {
	return r.EntryListenPorts(EntryProduction)
}
