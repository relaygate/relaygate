package resources

import (
	"fmt"
	"sort"
	"strings"
)

// ServerLifecycle summarizes canary/production enablement for one server.
type ServerLifecycle struct {
	Name                string
	ServerEnabled       bool
	CanaryEnabled       bool
	ProductionEnabled   bool
	CanaryPorts         []string // e.g. "TCP/11001"
	ProductionPorts     []string
	CanaryRuleCount     int
	ProductionRuleCount int
}

// LifecycleStatus returns per-server canary→production visibility.
func (r *Resources) LifecycleStatus() []ServerLifecycle {
	byName := make(map[string]*ServerLifecycle, len(r.Servers))
	order := make([]string, 0, len(r.Servers))
	for _, s := range r.Servers {
		order = append(order, s.Name)
		byName[s.Name] = &ServerLifecycle{
			Name:          s.Name,
			ServerEnabled: s.Enabled,
		}
	}
	for _, rule := range r.Rules {
		lc, ok := byName[rule.Server]
		if !ok {
			continue
		}
		port := fmt.Sprintf("%s/%d", strings.ToUpper(rule.Protocol), rule.ListenPort)
		switch strings.ToLower(rule.Kind) {
		case "canary":
			lc.CanaryRuleCount++
			if rule.Enabled {
				lc.CanaryEnabled = true
				lc.CanaryPorts = append(lc.CanaryPorts, port)
			}
		case "production":
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
		sort.Strings(lc.CanaryPorts)
		sort.Strings(lc.ProductionPorts)
		out = append(out, *lc)
	}
	return out
}

// FormatLifecycle prints a human-readable canary/production matrix.
func FormatLifecycle(r *Resources) string {
	rows := r.LifecycleStatus()
	var b strings.Builder
	fmt.Fprintf(&b, "生命周期状态: %d 台服务器\n", len(rows))
	for _, lc := range rows {
		srv := "off"
		if lc.ServerEnabled {
			srv = "on"
		}
		canary := "—"
		if lc.CanaryRuleCount > 0 {
			if lc.CanaryEnabled {
				canary = "on " + strings.Join(lc.CanaryPorts, ",")
			} else {
				canary = "off"
			}
		}
		prod := "—"
		if lc.ProductionRuleCount > 0 {
			if lc.ProductionEnabled {
				prod = "on " + strings.Join(lc.ProductionPorts, ",")
			} else {
				prod = "off（可 enable 后 reload）"
			}
		}
		fmt.Fprintf(&b, "  - %s server=%s canary=%s production=%s\n", lc.Name, srv, canary, prod)
	}
	return b.String()
}

// CanaryListenPorts returns enabled canary TCP/UDP listen ports (0 if none).
func (r *Resources) CanaryListenPorts() (tcpPort, udpPort int) {
	for _, rule := range r.EnabledRules() {
		if strings.ToLower(rule.Kind) != "canary" {
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
