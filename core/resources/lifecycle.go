package resources

import (
	"fmt"
	"sort"
	"strings"
)

// ServerLifecycle summarizes validation/production entry enablement for one upstream.
type ServerLifecycle struct {
	Name                   string   `json:"name"`
	ServerEnabled          bool     `json:"server_enabled"`
	ValidationEnabled      bool     `json:"validation_enabled"`
	ProductionEnabled      bool     `json:"production_enabled"`
	ValidationPorts        []string `json:"validation_ports"` // e.g. "TCP/11001"
	ProductionPorts        []string `json:"production_ports"`
	ValidationRuleCount    int      `json:"validation_rule_count"`
	ProductionRuleCount    int      `json:"production_rule_count"`
	Protocols              []string `json:"protocols"`
}

// LifecycleStatus returns per-server validation→production visibility (from Rules).
// Protocols come from the Server's enabled upstreams (tcp/udp ports).
func (r *Resources) LifecycleStatus() []ServerLifecycle {
	byName := make(map[string]*ServerLifecycle, len(r.Servers))
	order := make([]string, 0, len(r.Servers))
	for _, s := range r.Servers {
		order = append(order, s.Name)
		byName[s.Name] = &ServerLifecycle{
			Name:          s.Name,
			ServerEnabled: s.Enabled,
			Protocols:     s.EnabledProtocols(),
		}
	}
	for _, rule := range r.Rules {
		lc, ok := byName[rule.Server]
		if !ok {
			continue
		}
		proto := strings.ToUpper(strings.TrimSpace(rule.Protocol))
		port := fmt.Sprintf("%s/%d", proto, rule.ListenPort)
		switch strings.ToLower(strings.TrimSpace(rule.Entry)) {
		case EntryValidation:
			lc.ValidationRuleCount++
			if rule.Enabled {
				lc.ValidationEnabled = true
				lc.ValidationPorts = append(lc.ValidationPorts, port)
			}
		case EntryProduction:
			lc.ProductionRuleCount++
			if rule.Enabled {
				lc.ProductionEnabled = true
				lc.ProductionPorts = append(lc.ProductionPorts, port)
			}
		}
	}
	out := make([]ServerLifecycle, 0, len(order))
	for _, name := range order {
		lc := byName[name]
		sort.Strings(lc.ValidationPorts)
		sort.Strings(lc.ProductionPorts)
		out = append(out, *lc)
	}
	return out
}

// FormatLifecycle prints a human-readable validation/production entry matrix.
func FormatLifecycle(r *Resources) string {
	rows := r.LifecycleStatus()
	var b strings.Builder
	fmt.Fprintf(&b, "入口状态: %d 台上游\n", len(rows))
	for _, lc := range rows {
		srv := "off"
		if lc.ServerEnabled {
			srv = "on"
		}
		validation := "—"
		if lc.ValidationRuleCount > 0 {
			if lc.ValidationEnabled {
				validation = "on " + strings.Join(lc.ValidationPorts, ",")
			} else {
				validation = "off"
			}
		}
		prod := "—"
		if lc.ProductionRuleCount > 0 {
			if lc.ProductionEnabled {
				prod = "on " + strings.Join(lc.ProductionPorts, ",")
			} else {
				prod = "off（可启用正式入口后 reload）"
			}
		}
		fmt.Fprintf(&b, "  - %s server=%s validation=%s production=%s\n", lc.Name, srv, validation, prod)
	}
	return b.String()
}

// ValidationListenPorts returns enabled validation TCP/UDP listen ports (0 if none).
func (r *Resources) ValidationListenPorts() (tcpPort, udpPort int) {
	for _, rule := range r.EnabledRules() {
		if strings.ToLower(strings.TrimSpace(rule.Entry)) != EntryValidation {
			continue
		}
		switch strings.ToUpper(rule.Protocol) {
		case "TCP":
			if tcpPort == 0 {
				tcpPort = rule.ListenPort
			}
		case "UDP":
			if udpPort == 0 {
				udpPort = rule.ListenPort
			}
		}
	}
	return tcpPort, udpPort
}
