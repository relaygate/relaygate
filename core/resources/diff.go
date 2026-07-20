package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/relaygate/relaygate/core/config"
)

// ChangeSummary describes what changed between two Resources snapshots.
type ChangeSummary struct {
	ServersAdded   []string
	ServersRemoved []string
	ServersChanged []string
	RulesAdded     []string
	RulesRemoved   []string
	RulesToggled   []string
	PortChanges    []string
	DefaultsChanged []string
	ACLChanged     []string
	Note           string
}

func (c ChangeSummary) Empty() bool {
	return len(c.ServersAdded) == 0 && len(c.ServersRemoved) == 0 &&
		len(c.ServersChanged) == 0 && len(c.RulesAdded) == 0 &&
		len(c.RulesRemoved) == 0 && len(c.RulesToggled) == 0 &&
		len(c.PortChanges) == 0 && len(c.DefaultsChanged) == 0 &&
		len(c.ACLChanged) == 0
}

func (c ChangeSummary) String() string {
	var b strings.Builder
	if c.Note != "" {
		fmt.Fprintf(&b, "%s\n", c.Note)
	}
	if c.Empty() {
		b.WriteString("变更摘要: （相对上次备份无 server/rule/defaults/acl 差异）\n")
		return b.String()
	}
	b.WriteString("变更摘要:\n")
	writeList(&b, "  + server", c.ServersAdded)
	writeList(&b, "  - server", c.ServersRemoved)
	writeList(&b, "  ~ server", c.ServersChanged)
	writeList(&b, "  + rule", c.RulesAdded)
	writeList(&b, "  - rule", c.RulesRemoved)
	writeList(&b, "  ~ rule", c.RulesToggled)
	writeList(&b, "  ~ port", c.PortChanges)
	writeList(&b, "  ~ defaults", c.DefaultsChanged)
	writeList(&b, "  ~ acl", c.ACLChanged)
	return b.String()
}

func writeList(b *strings.Builder, prefix string, items []string) {
	for _, item := range items {
		fmt.Fprintf(b, "%s %s\n", prefix, item)
	}
}

// Diff compares before→after. before may be nil (首次).
func Diff(before, after *Resources) ChangeSummary {
	if after == nil {
		return ChangeSummary{Note: "变更摘要: （无当前配置）"}
	}
	if before == nil {
		var c ChangeSummary
		c.Note = "变更摘要: 首次/无上次备份对比"
		for _, s := range after.Servers {
			c.ServersAdded = append(c.ServersAdded, s.Name)
		}
		for _, rule := range after.Rules {
			state := "disabled"
			if rule.Enabled {
				state = "enabled"
			}
			c.RulesAdded = append(c.RulesAdded, fmt.Sprintf("%s (%s %s/%d → %s, %s)",
				rule.Name, rule.Kind, strings.ToUpper(rule.Protocol), rule.ListenPort, rule.Server, state))
		}
		c.DefaultsChanged = append(c.DefaultsChanged, summarizeDefaults(after.Defaults)...)
		c.ACLChanged = append(c.ACLChanged, summarizeACL(after.ACL)...)
		return c
	}

	var c ChangeSummary
	beforeServers := before.ServerMap()
	afterServers := after.ServerMap()

	for name, s := range afterServers {
		old, ok := beforeServers[name]
		if !ok {
			c.ServersAdded = append(c.ServersAdded, name)
			continue
		}
		var parts []string
		if old.Address != s.Address {
			parts = append(parts, fmt.Sprintf("address %s→%s", old.Address, s.Address))
		}
		if old.TCPPort != s.TCPPort {
			parts = append(parts, fmt.Sprintf("tcp_port %d→%d", old.TCPPort, s.TCPPort))
		}
		if old.UDPPort != s.UDPPort {
			parts = append(parts, fmt.Sprintf("udp_port %d→%d", old.UDPPort, s.UDPPort))
		}
		if old.HealthCheckPort != s.HealthCheckPort {
			parts = append(parts, fmt.Sprintf("health %d→%d", old.HealthCheckPort, s.HealthCheckPort))
		}
		if old.Enabled != s.Enabled {
			parts = append(parts, fmt.Sprintf("enabled %v→%v", old.Enabled, s.Enabled))
		}
		if len(parts) > 0 {
			c.ServersChanged = append(c.ServersChanged, name+": "+strings.Join(parts, ", "))
		}
	}
	for name := range beforeServers {
		if _, ok := afterServers[name]; !ok {
			c.ServersRemoved = append(c.ServersRemoved, name)
		}
	}

	beforeRules := ruleMap(before)
	afterRules := ruleMap(after)
	for name, rule := range afterRules {
		old, ok := beforeRules[name]
		if !ok {
			state := "disabled"
			if rule.Enabled {
				state = "enabled"
			}
			c.RulesAdded = append(c.RulesAdded, fmt.Sprintf("%s (%s %s/%d → %s, %s)",
				rule.Name, rule.Kind, strings.ToUpper(rule.Protocol), rule.ListenPort, rule.Server, state))
			continue
		}
		if old.Enabled != rule.Enabled {
			from, to := "off", "on"
			if !rule.Enabled {
				from, to = "on", "off"
			}
			c.RulesToggled = append(c.RulesToggled, fmt.Sprintf("%s %s→%s", name, from, to))
		}
		if old.ListenPort != rule.ListenPort || !strings.EqualFold(old.Protocol, rule.Protocol) {
			c.PortChanges = append(c.PortChanges, fmt.Sprintf("%s %s/%d→%s/%d",
				name,
				strings.ToUpper(old.Protocol), old.ListenPort,
				strings.ToUpper(rule.Protocol), rule.ListenPort))
		} else if old.Server != rule.Server || !strings.EqualFold(old.Kind, rule.Kind) {
			c.RulesToggled = append(c.RulesToggled, fmt.Sprintf("%s target/kind changed", name))
		}
	}
	for name := range beforeRules {
		if _, ok := afterRules[name]; !ok {
			c.RulesRemoved = append(c.RulesRemoved, name)
		}
	}

	c.DefaultsChanged = diffDefaults(before.Defaults, after.Defaults)
	c.ACLChanged = diffACL(before.ACL, after.ACL)

	sort.Strings(c.ServersAdded)
	sort.Strings(c.ServersRemoved)
	sort.Strings(c.ServersChanged)
	sort.Strings(c.RulesAdded)
	sort.Strings(c.RulesRemoved)
	sort.Strings(c.RulesToggled)
	sort.Strings(c.PortChanges)
	sort.Strings(c.DefaultsChanged)
	sort.Strings(c.ACLChanged)
	return c
}

func diffDefaults(before, after Defaults) []string {
	var parts []string
	add := func(field string, a, b any) {
		as, bs := fmt.Sprint(a), fmt.Sprint(b)
		if as != bs {
			parts = append(parts, fmt.Sprintf("%s %s→%s", field, as, bs))
		}
	}
	add("backend_tcp_port", before.BackendTCPPort, after.BackendTCPPort)
	add("backend_udp_port", before.BackendUDPPort, after.BackendUDPPort)
	add("tcp_idle_timeout", before.TCPIdleTimeout, after.TCPIdleTimeout)
	add("udp_idle_timeout", before.UDPIdleTimeout, after.UDPIdleTimeout)
	add("max_connections", before.MaxConnections, after.MaxConnections)
	add("max_pending_requests", before.MaxPendingRequests, after.MaxPendingRequests)
	add("tcp_local_rate_limit_per_sec", before.TCPLocalRateLimitPerSec, after.TCPLocalRateLimitPerSec)
	add("tcp_local_rate_limit_burst", before.TCPLocalRateLimitBurst, after.TCPLocalRateLimitBurst)
	add("health.timeout", before.HealthCheck.Timeout, after.HealthCheck.Timeout)
	add("health.interval", before.HealthCheck.Interval, after.HealthCheck.Interval)
	add("health.unhealthy_threshold", before.HealthCheck.UnhealthyThreshold, after.HealthCheck.UnhealthyThreshold)
	add("health.healthy_threshold", before.HealthCheck.HealthyThreshold, after.HealthCheck.HealthyThreshold)
	add("nft.tcp_new_conn_per_ip", before.Nft.TCPNewConnPerIP, after.Nft.TCPNewConnPerIP)
	add("nft.udp_pps_per_ip", before.Nft.UDPPPSPerIP, after.Nft.UDPPPSPerIP)
	add("nft.tcp_burst", before.Nft.TCPBurst, after.Nft.TCPBurst)
	add("nft.udp_burst", before.Nft.UDPBurst, after.Nft.UDPBurst)
	return parts
}

func summarizeDefaults(d Defaults) []string {
	return []string{
		fmt.Sprintf("tcp_rl=%d/%d max_conn=%d nft.tcp=%s nft.udp=%s",
			d.TCPLocalRateLimitPerSec, d.TCPLocalRateLimitBurst, d.MaxConnections,
			d.Nft.TCPNewConnPerIP, d.Nft.UDPPPSPerIP),
	}
}

func diffACL(before, after ACL) []string {
	var parts []string
	added, removed := listDelta(before.Deny, after.Deny)
	for _, e := range added {
		parts = append(parts, "+deny "+e)
	}
	for _, e := range removed {
		parts = append(parts, "-deny "+e)
	}
	added, removed = listDelta(before.Allow, after.Allow)
	for _, e := range added {
		parts = append(parts, "+allow "+e)
	}
	for _, e := range removed {
		parts = append(parts, "-allow "+e)
	}
	return parts
}

func summarizeACL(a ACL) []string {
	var parts []string
	if len(a.Deny) > 0 {
		parts = append(parts, fmt.Sprintf("deny=%s", strings.Join(a.Deny, ",")))
	}
	if len(a.Allow) > 0 {
		parts = append(parts, fmt.Sprintf("allow=%s (strict)", strings.Join(a.Allow, ",")))
	}
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func listDelta(before, after []string) (added, removed []string) {
	bm := map[string]struct{}{}
	am := map[string]struct{}{}
	for _, e := range before {
		bm[e] = struct{}{}
	}
	for _, e := range after {
		am[e] = struct{}{}
	}
	for e := range am {
		if _, ok := bm[e]; !ok {
			added = append(added, e)
		}
	}
	for e := range bm {
		if _, ok := am[e]; !ok {
			removed = append(removed, e)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func ruleMap(r *Resources) map[string]Rule {
	m := make(map[string]Rule, len(r.Rules))
	for _, rule := range r.Rules {
		m[rule.Name] = rule
	}
	return m
}

// LoadPreviousBackupResources loads resources.yaml from the stamp in backups/latest.
// Returns (nil, "", nil) when no previous backup exists.
func LoadPreviousBackupResources(root string) (*Resources, string, error) {
	p := config.ResolvePaths(root)
	latestPath := filepath.Join(p.Backups, "latest")
	b, err := os.ReadFile(latestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	stamp := strings.TrimSpace(string(b))
	if stamp == "" {
		return nil, "", nil
	}
	resPath := filepath.Join(p.Backups, stamp, "resources.yaml")
	res, err := Load(resPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, stamp, nil
		}
		return nil, stamp, err
	}
	return res, stamp, nil
}

// WriteChangeSummaryFile writes summary text into backupDir/change-summary.txt.
func WriteChangeSummaryFile(backupDir string, summary ChangeSummary) error {
	if backupDir == "" {
		return nil
	}
	return os.WriteFile(filepath.Join(backupDir, "change-summary.txt"), []byte(summary.String()), 0o644)
}

// ChangeEntry is one backup stamp with its change-summary.txt body.
type ChangeEntry struct {
	Stamp   string
	Summary string
	Path    string
}

// ListChangeSummaries reads DataDir/backups/*/change-summary.txt newest first.
func ListChangeSummaries(root string, limit int) ([]ChangeEntry, error) {
	p := config.ResolvePaths(root)
	entries, err := os.ReadDir(p.Backups)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var stamps []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		sumPath := filepath.Join(p.Backups, name, "change-summary.txt")
		if _, err := os.Stat(sumPath); err != nil {
			continue
		}
		stamps = append(stamps, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(stamps)))
	if limit > 0 && len(stamps) > limit {
		stamps = stamps[:limit]
	}
	out := make([]ChangeEntry, 0, len(stamps))
	for _, stamp := range stamps {
		path := filepath.Join(p.Backups, stamp, "change-summary.txt")
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, ChangeEntry{Stamp: stamp, Summary: string(b), Path: path})
	}
	return out, nil
}
