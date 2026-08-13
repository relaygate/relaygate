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
	UpstreamsAdded  []string
	UpstreamsRemoved []string
	UpstreamsChanged []string
	ForwardsAdded   []string
	ForwardsRemoved []string
	ForwardsToggled []string
	PortChanges     []string
	DefaultsChanged  []string
	SecurityChanged  []string
	// MetaChanged lists bootstrap-affecting meta field deltas (admin_*, envoy_image).
	// Populated only when Diff has a non-nil before snapshot.
	MetaChanged []string
	Note        string
}

func (c ChangeSummary) Empty() bool {
	return len(c.UpstreamsAdded) == 0 && len(c.UpstreamsRemoved) == 0 &&
		len(c.UpstreamsChanged) == 0 && len(c.ForwardsAdded) == 0 &&
		len(c.ForwardsRemoved) == 0 && len(c.ForwardsToggled) == 0 &&
		len(c.PortChanges) == 0 && len(c.DefaultsChanged) == 0 &&
		len(c.SecurityChanged) == 0 && len(c.MetaChanged) == 0
}

// ApplySurface classifies which product domains a ChangeSummary needs.
// Gateway reload vs firewall apply stay separate. With XDS_ENABLED=0, ops always HardReload.
type ApplySurface struct {
	NeedsReload     bool // upstreams / forwards / Envoy defaults / hard meta
	NeedsFirewall   bool // security policies / enabled listen ports
	CanHotApply     bool // NeedsReload && !NeedsHardReload
	NeedsHardReload bool // meta.admin_* / meta.envoy_image (bootstrap)
}

// Classify returns whether this diff needs gateway reload and/or firewall apply.
// Both may be true (e.g. new listen port). Empty diffs yield all false.
func (c ChangeSummary) Classify() ApplySurface {
	hard := c.needsHardReload()
	reload := c.needsReload() || hard
	return ApplySurface{
		NeedsReload:     reload,
		NeedsFirewall:   c.needsFirewall(),
		NeedsHardReload: hard,
		CanHotApply:     reload && !hard,
	}
}

func (c ChangeSummary) needsHardReload() bool {
	return len(c.MetaChanged) > 0
}

func (c ChangeSummary) needsReload() bool {
	if len(c.UpstreamsAdded) > 0 || len(c.UpstreamsRemoved) > 0 || len(c.UpstreamsChanged) > 0 {
		return true
	}
	if len(c.ForwardsAdded) > 0 || len(c.ForwardsRemoved) > 0 || len(c.ForwardsToggled) > 0 || len(c.PortChanges) > 0 {
		return true
	}
	for _, d := range c.DefaultsChanged {
		if defaultsEntryAffectsEnvoy(d) {
			return true
		}
	}
	for _, d := range c.SecurityChanged {
		if securityEntryAffectsEnvoy(d) {
			return true
		}
	}
	return false
}

func securityEntryAffectsEnvoy(entry string) bool {
	field, _, _ := strings.Cut(entry, " ")
	needsReload, _ := PolicyApplySurfaces(field)
	return needsReload
}

func (c ChangeSummary) needsFirewall() bool {
	if len(c.PortChanges) > 0 {
		return true
	}
	if len(c.ForwardsToggled) > 0 || len(c.ForwardsRemoved) > 0 {
		return true
	}
	for _, r := range c.ForwardsAdded {
		// Diff formats enabled rules as "... , enabled)" — disabled adds do not touch FORWARD_*.
		if strings.Contains(r, ", enabled)") {
			return true
		}
	}
	// Per-entry: kernel / gateway-only policy diffs must not force firewall apply.
	for _, d := range c.SecurityChanged {
		if securityEntryAffectsFirewall(d) {
			return true
		}
	}
	return false
}

func securityEntryAffectsFirewall(entry string) bool {
	field, _, _ := strings.Cut(entry, " ")
	_, needsFirewall := PolicyApplySurfaces(field)
	if needsFirewall {
		return true
	}
	// ACL list deltas (+deny, -allow, etc.)
	if strings.HasPrefix(entry, "+") || strings.HasPrefix(entry, "-") {
		return true
	}
	return false
}

func defaultsEntryAffectsEnvoy(entry string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}
	if strings.Contains(entry, "tcp_idle=") {
		return false
	}
	field, _, _ := strings.Cut(entry, " ")
	return !strings.HasPrefix(field, "security.protections.") && !strings.HasPrefix(field, "security.access")
}

func (c ChangeSummary) String() string {
	var b strings.Builder
	if c.Empty() {
		if c.Note != "" {
			fmt.Fprintf(&b, "%s\n", c.Note)
		} else {
			// Keep terse: no "相对备份" / "相对上次备份…" boilerplate.
			b.WriteString("无差异\n")
		}
		return b.String()
	}
	// Non-empty: list +/-/~ only (callers must not prepend backup stamp headers).
	if c.Note != "" {
		fmt.Fprintf(&b, "%s\n", c.Note)
	}
	writeList(&b, "  + upstream", c.UpstreamsAdded)
	writeList(&b, "  - upstream", c.UpstreamsRemoved)
	writeList(&b, "  ~ upstream", c.UpstreamsChanged)
	writeList(&b, "  + forward", c.ForwardsAdded)
	writeList(&b, "  - forward", c.ForwardsRemoved)
	writeList(&b, "  ~ forward", c.ForwardsToggled)
	writeList(&b, "  ~ port", c.PortChanges)
	writeList(&b, "  ~ defaults", c.DefaultsChanged)
	writeList(&b, "  ~ security", c.SecurityChanged)
	writeList(&b, "  ~ meta", c.MetaChanged)
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
		return ChangeSummary{Note: "（无当前配置）"}
	}
	if before == nil {
		var c ChangeSummary
		c.Note = "首次/无上次备份对比"
		for _, s := range after.Upstreams {
			c.UpstreamsAdded = append(c.UpstreamsAdded, s.Name)
		}
		for _, fwd := range after.Forwards {
			state := "disabled"
			if fwd.Enabled {
				state = "enabled"
			}
			c.ForwardsAdded = append(c.ForwardsAdded, fmt.Sprintf("%s (%s %s/%d → %s, %s)",
				fwd.Name, fwd.Entry, strings.ToUpper(fwd.Protocol), fwd.ListenPort, fwd.Upstream, state))
		}
		c.DefaultsChanged = append(c.DefaultsChanged, summarizeDefaults(after.Defaults)...)
		c.SecurityChanged = append(c.SecurityChanged, summarizeSecurity(after.Security)...)
		return c
	}

	var c ChangeSummary
	beforeUpstreams := before.UpstreamMap()
	afterUpstreams := after.UpstreamMap()

	for name, s := range afterUpstreams {
		old, ok := beforeUpstreams[name]
		if !ok {
			c.UpstreamsAdded = append(c.UpstreamsAdded, name)
			continue
		}
		var parts []string
		if old.Address != s.Address {
			parts = append(parts, fmt.Sprintf("address %s→%s", old.Address, s.Address))
		}
		if old.TCPPort() != s.TCPPort() {
			parts = append(parts, fmt.Sprintf("tcp %d→%d", old.TCPPort(), s.TCPPort()))
		}
		if old.UDPPort() != s.UDPPort() {
			parts = append(parts, fmt.Sprintf("udp %d→%d", old.UDPPort(), s.UDPPort()))
		}
		if old.Enabled != s.Enabled {
			parts = append(parts, fmt.Sprintf("enabled %v→%v", old.Enabled, s.Enabled))
		}
		if len(parts) > 0 {
			c.UpstreamsChanged = append(c.UpstreamsChanged, name+": "+strings.Join(parts, ", "))
		}
	}
	for name := range beforeUpstreams {
		if _, ok := afterUpstreams[name]; !ok {
			c.UpstreamsRemoved = append(c.UpstreamsRemoved, name)
		}
	}

	beforeForwards := forwardMap(before)
	afterForwards := forwardMap(after)
	for name, fwd := range afterForwards {
		old, ok := beforeForwards[name]
		if !ok {
			state := "disabled"
			if fwd.Enabled {
				state = "enabled"
			}
			c.ForwardsAdded = append(c.ForwardsAdded, fmt.Sprintf("%s (%s %s/%d → %s, %s)",
				fwd.Name, fwd.Entry, strings.ToUpper(fwd.Protocol), fwd.ListenPort, fwd.Upstream, state))
			continue
		}
		if old.Enabled != fwd.Enabled {
			from, to := "off", "on"
			if !fwd.Enabled {
				from, to = "on", "off"
			}
			c.ForwardsToggled = append(c.ForwardsToggled, fmt.Sprintf("%s %s→%s", name, from, to))
		}
		if old.ListenPort != fwd.ListenPort || !strings.EqualFold(old.Protocol, fwd.Protocol) {
			c.PortChanges = append(c.PortChanges, fmt.Sprintf("%s %s/%d→%s/%d",
				name,
				strings.ToUpper(old.Protocol), old.ListenPort,
				strings.ToUpper(fwd.Protocol), fwd.ListenPort))
		} else if old.Upstream != fwd.Upstream || !strings.EqualFold(old.Entry, fwd.Entry) {
			c.ForwardsToggled = append(c.ForwardsToggled, fmt.Sprintf("%s target/kind changed", name))
		}
	}
	for name := range beforeForwards {
		if _, ok := afterForwards[name]; !ok {
			c.ForwardsRemoved = append(c.ForwardsRemoved, name)
		}
	}

	c.DefaultsChanged = diffDefaults(before.Defaults, after.Defaults)
	c.SecurityChanged = DiffSecurityPolicies(before.Security, after.Security)
	c.MetaChanged = diffMetaHardReload(before.Meta, after.Meta)

	sort.Strings(c.UpstreamsAdded)
	sort.Strings(c.UpstreamsRemoved)
	sort.Strings(c.UpstreamsChanged)
	sort.Strings(c.ForwardsAdded)
	sort.Strings(c.ForwardsRemoved)
	sort.Strings(c.ForwardsToggled)
	sort.Strings(c.PortChanges)
	sort.Strings(c.DefaultsChanged)
	sort.Strings(c.SecurityChanged)
	sort.Strings(c.MetaChanged)
	return c
}

// diffMetaHardReload lists meta fields that force HardReload (bootstrap / image).
// gateway_name / service_name are labels only and do not flip NeedsHardReload.
func diffMetaHardReload(before, after Meta) []string {
	var parts []string
	add := func(field string, a, b any) {
		as, bs := fmt.Sprint(a), fmt.Sprint(b)
		if as != bs {
			parts = append(parts, fmt.Sprintf("%s %s→%s", field, as, bs))
		}
	}
	add("admin_port", before.AdminPort, after.AdminPort)
	add("admin_address", before.AdminAddress, after.AdminAddress)
	add("envoy_image", before.EnvoyImage, after.EnvoyImage)
	return parts
}

func diffDefaults(before, after Defaults) []string {
	var parts []string
	add := func(field string, a, b any) {
		as, bs := fmt.Sprint(a), fmt.Sprint(b)
		if as != bs {
			parts = append(parts, fmt.Sprintf("%s %s→%s", field, as, bs))
		}
	}
	add("default_upstream_tcp_port", before.DefaultUpstreamTCPPort, after.DefaultUpstreamTCPPort)
	add("default_upstream_udp_port", before.DefaultUpstreamUDPPort, after.DefaultUpstreamUDPPort)
	add("tcp_idle_timeout", before.TCPIdleTimeout, after.TCPIdleTimeout)
	add("udp_idle_timeout", before.UDPIdleTimeout, after.UDPIdleTimeout)
	add("max_pending_requests", before.MaxPendingRequests, after.MaxPendingRequests)
	add("health.timeout", before.HealthCheck.Timeout, after.HealthCheck.Timeout)
	add("health.interval", before.HealthCheck.Interval, after.HealthCheck.Interval)
	add("health.unhealthy_threshold", before.HealthCheck.UnhealthyThreshold, after.HealthCheck.UnhealthyThreshold)
	add("health.healthy_threshold", before.HealthCheck.HealthyThreshold, after.HealthCheck.HealthyThreshold)
	add("outlier.enabled", before.OutlierDetection.Enabled, after.OutlierDetection.Enabled)
	add("outlier.consecutive_local_origin_failure", before.OutlierDetection.ConsecutiveLocalOriginFailure, after.OutlierDetection.ConsecutiveLocalOriginFailure)
	add("outlier.interval", before.OutlierDetection.Interval, after.OutlierDetection.Interval)
	add("outlier.base_ejection_time", before.OutlierDetection.BaseEjectionTime, after.OutlierDetection.BaseEjectionTime)
	return parts
}

func summarizeDefaults(d Defaults) []string {
	return []string{
		fmt.Sprintf("tcp_idle=%s udp_idle=%s",
			d.TCPIdleTimeout, d.UDPIdleTimeout),
	}
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

func forwardMap(r *Resources) map[string]Forward {
	m := make(map[string]Forward, len(r.Forwards))
	for _, fwd := range r.Forwards {
		m[fwd.Name] = fwd
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
