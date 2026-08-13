package resources

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/relaygate/relaygate/core/config"

	"gopkg.in/yaml.v3"
)

var upstreamNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
var upstreamNumRe = regexp.MustCompile(`(?i)^server-(\d+)$`)

type Resources struct {
	Meta     Meta     `yaml:"meta"`
	Gateway  Gateway  `yaml:"gateway"`
	Defaults Defaults `yaml:"defaults"`
	Security Security `yaml:"security"`
	Upstreams []Upstream `yaml:"upstreams"`
	Forwards  []Forward  `yaml:"forwards"`
}

type Meta struct {
	ServiceName  string `yaml:"service_name"`
	EnvoyImage   string `yaml:"envoy_image"`
	AdminPort    int    `yaml:"admin_port"`
	AdminAddress string `yaml:"admin_address"`
}

type Gateway struct {
	Name          string `yaml:"name"`
	PublicIP      string `yaml:"public_ip"`
	SSHPort       int    `yaml:"ssh_port"`
	ListenAddress string `yaml:"listen_address"`
}

type Defaults struct {
	DefaultUpstreamTCPPort int         `yaml:"default_upstream_tcp_port"`
	DefaultUpstreamUDPPort int         `yaml:"default_upstream_udp_port"`
	TCPIdleTimeout     string      `yaml:"tcp_idle_timeout"`
	UDPIdleTimeout     string      `yaml:"udp_idle_timeout"`
	MaxPendingRequests int         `yaml:"max_pending_requests"`
	HealthCheck        HealthCheck `yaml:"health_check"`
	// OutlierDetection is thin L4 passive ejection (default off). Single-endpoint clusters
	// eject the only host on failure — keep thresholds conservative.
	OutlierDetection OutlierDetection `yaml:"outlier_detection"`
}

// OutlierDetection maps to Envoy cluster outlier_detection (TCP-oriented fields only).
type OutlierDetection struct {
	Enabled                         bool   `yaml:"enabled"`
	ConsecutiveLocalOriginFailure   int    `yaml:"consecutive_local_origin_failure"`
	Interval                        string `yaml:"interval"`
	BaseEjectionTime                string `yaml:"base_ejection_time"`
}

// ApplyOutlierDefaults fills conservative values when outlier_detection.enabled is true.
// Does not enable outlier when omitted/false.
func (d *Defaults) ApplyOutlierDefaults() {
	if !d.OutlierDetection.Enabled {
		return
	}
	if d.OutlierDetection.ConsecutiveLocalOriginFailure <= 0 {
		d.OutlierDetection.ConsecutiveLocalOriginFailure = 5
	}
	if strings.TrimSpace(d.OutlierDetection.Interval) == "" {
		d.OutlierDetection.Interval = "10s"
	}
	if strings.TrimSpace(d.OutlierDetection.BaseEjectionTime) == "" {
		d.OutlierDetection.BaseEjectionTime = "30s"
	}
}

type HealthCheck struct {
	Timeout            string `yaml:"timeout"`
	Interval           string `yaml:"interval"`
	UnhealthyThreshold int    `yaml:"unhealthy_threshold"`
	HealthyThreshold   int    `yaml:"healthy_threshold"`
}

// ProtoPort is an optional upstream protocol endpoint. Presence with a valid
// port enables that protocol on the server; omit/nil means the protocol is off.
type ProtoPort struct {
	Port int `yaml:"port" json:"port"`
}

// Upstream is an upstream: alias, address, optional TCP/UDP ports, and enabled.
// Stage/listen/rollout live on Forward only. Health check (internal) uses TCP port when TCP is set;
// UDP-only upstreams are not health-checked.
type Upstream struct {
	Name    string     `yaml:"name" json:"name"`
	Address string     `yaml:"address" json:"address"`
	TCP     *ProtoPort `yaml:"tcp,omitempty" json:"tcp,omitempty"`
	UDP     *ProtoPort `yaml:"udp,omitempty" json:"udp,omitempty"`
	Enabled bool       `yaml:"enabled" json:"enabled"`
}

// HasTCP reports whether TCP upstream is enabled.
func (u Upstream) HasTCP() bool {
	return u.TCP != nil && u.TCP.Port >= 1 && u.TCP.Port <= 65535
}

// HasUDP reports whether UDP upstream is enabled.
func (u Upstream) HasUDP() bool {
	return u.UDP != nil && u.UDP.Port >= 1 && u.UDP.Port <= 65535
}

// TCPPort returns the TCP upstream port, or 0 if TCP is not enabled.
func (u Upstream) TCPPort() int {
	if u.HasTCP() {
		return u.TCP.Port
	}
	return 0
}

// UDPPort returns the UDP upstream port, or 0 if UDP is not enabled.
func (u Upstream) UDPPort() int {
	if u.HasUDP() {
		return u.UDP.Port
	}
	return 0
}

// HealthCheckPort returns the probe port for Envoy TCP health checks (same as TCP port).
// Returns 0 when TCP is not enabled (UDP-only: no health check).
func (u Upstream) HealthCheckPort() int {
	return u.TCPPort()
}

// EnabledProtocols lists protocols with a configured upstream port.
func (u Upstream) EnabledProtocols() []string {
	out := make([]string, 0, 2)
	if u.HasTCP() {
		out = append(out, "TCP")
	}
	if u.HasUDP() {
		out = append(out, "UDP")
	}
	return out
}

// ProtoPortOf builds a ProtoPort pointer, or nil when port is unset.
func ProtoPortOf(port int) *ProtoPort {
	if port < 1 {
		return nil
	}
	return &ProtoPort{Port: port}
}

// Entry types for Forward.Entry (入口类型：验证 / 正式).
const (
	EntryValidation = "validation"
	EntryProduction = "production"
)

// Forward is an ingress forward: entry type, protocol, listen port, switch, upstream ref.
// Naming: forward-{server}-{entry}-{proto}.
type Forward struct {
	Name       string `yaml:"name" json:"name"`
	Entry      string `yaml:"entry" json:"entry"` // validation | production
	Upstream   string `yaml:"upstream" json:"upstream"`
	Protocol   string `yaml:"protocol" json:"protocol"`
	ListenPort int    `yaml:"listen_port" json:"listen_port"`
	Enabled    bool   `yaml:"enabled" json:"enabled"`
}

// NormalizeEntry returns canonical entry id, or "" if invalid.
func NormalizeEntry(entry string) string {
	switch strings.ToLower(strings.TrimSpace(entry)) {
	case EntryValidation:
		return EntryValidation
	case EntryProduction:
		return EntryProduction
	default:
		return ""
	}
}

// ValidEntry reports whether entry is validation or production.
func ValidEntry(entry string) bool {
	e := strings.ToLower(strings.TrimSpace(entry))
	return e == EntryValidation || e == EntryProduction
}

func DefaultPaths(root string) (resourcesPath, envoyOut, nftOut string) {
	p := config.ResolvePaths(root)
	return p.Resources, p.EnvoyYAML, p.ForwardPorts
}

func Load(path string) (*Resources, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read resources: %w", err)
	}
	var r Resources
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse resources: %w", err)
	}
	return &r, nil
}

func (r *Resources) UpstreamMap() map[string]Upstream {
	m := make(map[string]Upstream, len(r.Upstreams))
	for _, s := range r.Upstreams {
		m[s.Name] = s
	}
	return m
}

func (r *Resources) EnabledForwards() []Forward {
	out := make([]Forward, 0, len(r.Forwards))
	for _, fwd := range r.Forwards {
		if fwd.Enabled {
			out = append(out, fwd)
		}
	}
	return out
}

func (r *Resources) Validate() error {
	r.Security.EnsureSecurityDefaults()
	if err := r.Security.NormalizeSecurity(); err != nil {
		return err
	}
	if len(r.Upstreams) == 0 {
		return fmt.Errorf("upstreams 不能为空")
	}
	seenNames := map[string]struct{}{}
	for _, s := range r.Upstreams {
		if err := validateUpstreamFields(s); err != nil {
			return err
		}
		if _, ok := seenNames[s.Name]; ok {
			return fmt.Errorf("upstream 名称重复: %s", s.Name)
		}
		seenNames[s.Name] = struct{}{}
	}
	upstreams := r.UpstreamMap()

	// All forwards: entry/protocol/port/upstream refs (规划期也要能发现错误)
	allPorts := map[string]string{} // proto/port -> rule name
	for _, fwd := range r.Forwards {
		entry := strings.ToLower(strings.TrimSpace(fwd.Entry))
		if !ValidEntry(entry) {
			return fmt.Errorf("%s: entry 必须是 validation 或 production（当前 %q）", fwd.Name, fwd.Entry)
		}
		proto := strings.ToUpper(fwd.Protocol)
		if proto != "TCP" && proto != "UDP" {
			return fmt.Errorf("%s: protocol 必须是 TCP 或 UDP", fwd.Name)
		}
		if fwd.ListenPort < 1 || fwd.ListenPort > 65535 {
			return fmt.Errorf("%s: listen_port 越界: %d", fwd.Name, fwd.ListenPort)
		}
		srv, ok := upstreams[fwd.Upstream]
		if !ok {
			return fmt.Errorf("%s: 未知 upstream %s", fwd.Name, fwd.Upstream)
		}
		if proto == "TCP" && !srv.HasTCP() {
			return fmt.Errorf("%s: upstream %s 未启用 TCP（缺少 tcp.port）", fwd.Name, fwd.Upstream)
		}
		if proto == "UDP" && !srv.HasUDP() {
			return fmt.Errorf("%s: upstream %s 未启用 UDP（缺少 udp.port）", fwd.Name, fwd.Upstream)
		}
		key := fmt.Sprintf("%s/%d", proto, fwd.ListenPort)
		if other, ok := allPorts[key]; ok {
			return fmt.Errorf("端口冲突: %s 同时被 %s 与 %s 使用（含未启用转发；验证与正式入口也不可重叠）", key, other, fwd.Name)
		}
		allPorts[key] = fwd.Name
	}

	for _, fwd := range r.EnabledForwards() {
		s := upstreams[fwd.Upstream]
		if !s.Enabled {
			return fmt.Errorf("%s: 目标 %s 已禁用", fwd.Name, fwd.Upstream)
		}
	}
	if len(r.EnabledForwards()) == 0 {
		return fmt.Errorf("没有启用的转发（forwards）；至少启用一条转发后再渲染")
	}
	return nil
}

func validateUpstreamFields(s Upstream) error {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		return fmt.Errorf("upstream name 不能为空")
	}
	if !upstreamNameRe.MatchString(name) {
		return fmt.Errorf("upstream name 无效: %s（仅允许字母数字、_、-，且不能以符号开头）", name)
	}
	if ip := net.ParseIP(strings.TrimSpace(s.Address)); ip == nil {
		return fmt.Errorf("%s address 无效: %s", name, s.Address)
	}
	if s.TCP != nil {
		if s.TCP.Port < 1 || s.TCP.Port > 65535 {
			return fmt.Errorf("%s.tcp.port 端口越界: %d", name, s.TCP.Port)
		}
	}
	if s.UDP != nil {
		if s.UDP.Port < 1 || s.UDP.Port > 65535 {
			return fmt.Errorf("%s.udp.port 端口越界: %d", name, s.UDP.Port)
		}
	}
	if !s.HasTCP() && !s.HasUDP() {
		return fmt.Errorf("%s: 至少启用 TCP 或 UDP 之一（设置 tcp.port / udp.port）", name)
	}
	return nil
}

// normalizeUpstreamProtocols clears invalid/empty protocol blocks so unused
// protocols stay truly absent (no mirror placeholders).
func normalizeUpstreamProtocols(s *Upstream) {
	if s.TCP != nil && (s.TCP.Port < 1 || s.TCP.Port > 65535) {
		s.TCP = nil
	}
	if s.UDP != nil && (s.UDP.Port < 1 || s.UDP.Port > 65535) {
		s.UDP = nil
	}
}

// AddUpstream appends an upstream only — no forwards are created.
// Create entries separately via AddEntries / EnsureEntries.
func (r *Resources) AddUpstream(s Upstream) error {
	s.Name = strings.TrimSpace(s.Name)
	s.Address = strings.TrimSpace(s.Address)
	normalizeUpstreamProtocols(&s)
	if err := validateUpstreamFields(s); err != nil {
		return err
	}
	for _, existing := range r.Upstreams {
		if existing.Name == s.Name {
			return fmt.Errorf("upstream 已存在: %s", s.Name)
		}
	}
	r.Upstreams = append(r.Upstreams, s)
	return nil
}

// AddEntryOptions controls creating ingress forwards for an existing upstream.
type AddEntryOptions struct {
	Upstream  string
	Entry     string   // validation | production
	Protocols []string // empty = all enabled protocols on the upstream
	// Enabled overrides default: validation→true, production→false. Nil keeps default.
	Enabled *bool
	// ListenPort reuses/forces listen port; 0 = allocate (or reuse existing entry port).
	ListenPort int
}

// AddEntries creates missing entry×protocol forwards for an upstream (idempotent by ForwardName).
// Existing forwards with the same name are skipped (not modified).
func (r *Resources) AddEntries(opts AddEntryOptions) (created []Forward, err error) {
	upstream := strings.TrimSpace(opts.Upstream)
	if upstream == "" {
		return nil, fmt.Errorf("upstream 不能为空")
	}
	entry := NormalizeEntry(opts.Entry)
	if entry == "" {
		return nil, fmt.Errorf("entry 必须是 validation 或 production")
	}
	srv, ok := r.UpstreamMap()[upstream]
	if !ok {
		return nil, fmt.Errorf("upstream not found: %s", upstream)
	}
	protos, err := resolveAddProtocols(srv, opts.Protocols)
	if err != nil {
		return nil, err
	}
	enable := entry == EntryValidation
	if opts.Enabled != nil {
		enable = *opts.Enabled
	}
	listenPort := opts.ListenPort
	if listenPort <= 0 {
		if existing := r.entryListenPort(upstream, entry); existing > 0 {
			listenPort = existing
		} else {
			listenPort, err = r.allocateListenPort(upstream, entry)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, proto := range protos {
		name := ForwardName(upstream, entry, proto)
		if r.forwardNameConflict(name) != "" {
			continue // idempotent skip
		}
		fwd := Forward{
			Name:       name,
			Entry:      entry,
			Upstream:   upstream,
			Protocol:   proto,
			ListenPort: listenPort,
			Enabled:    enable,
		}
		created = append(created, fwd)
	}
	r.Forwards = append(r.Forwards, created...)
	return created, nil
}

// EnsureEntries creates missing forwards for the given entry type + protocols.
// Unlike filling only existing entries, this may create a brand-new entry type.
func (r *Resources) EnsureEntries(upstreamName, entry string, protocols []string, enable bool) (created []Forward, err error) {
	return r.AddEntries(AddEntryOptions{
		Upstream:  upstreamName,
		Entry:     entry,
		Protocols: protocols,
		Enabled:   BoolPtr(enable),
	})
}

func (r *Resources) entryListenPort(server, entry string) int {
	entry = strings.ToLower(strings.TrimSpace(entry))
	for _, fwd := range r.Forwards {
		if fwd.Upstream != server {
			continue
		}
		if strings.ToLower(strings.TrimSpace(fwd.Entry)) != entry {
			continue
		}
		if fwd.ListenPort > 0 {
			return fwd.ListenPort
		}
	}
	return 0
}

// resolveAddProtocols picks forward protocols from explicit opts or from upstream ports.
func resolveAddProtocols(u Upstream, protocols []string) ([]string, error) {
	if len(protocols) == 0 {
		out := u.EnabledProtocols()
		if len(out) == 0 {
			return nil, fmt.Errorf("protocols 至少选择 TCP 或 UDP")
		}
		return out, nil
	}
	protos, err := normalizeProtocols(protocols)
	if err != nil {
		return nil, err
	}
	for _, p := range protos {
		switch p {
		case "TCP":
			if !u.HasTCP() {
				return nil, fmt.Errorf("已选 TCP 但未设置 tcp.port")
			}
		case "UDP":
			if !u.HasUDP() {
				return nil, fmt.Errorf("已选 UDP 但未设置 udp.port")
			}
		}
	}
	return protos, nil
}

// DeleteUpstream removes an upstream and all forwards that reference it.
func (r *Resources) DeleteUpstream(name string) (removedForwards int, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("upstream name 不能为空")
	}
	idx := -1
	for i, s := range r.Upstreams {
		if s.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, fmt.Errorf("upstream not found: %s", name)
	}
	if len(r.Upstreams) == 1 {
		return 0, fmt.Errorf("不能删除最后一台上游")
	}
	r.Upstreams = append(r.Upstreams[:idx], r.Upstreams[idx+1:]...)
	kept := r.Forwards[:0]
	for _, fwd := range r.Forwards {
		if fwd.Upstream == name {
			removedForwards++
			continue
		}
		kept = append(kept, fwd)
	}
	r.Forwards = kept
	return removedForwards, nil
}

// ForwardName is the canonical forwarding-rule identifier: forward-{server}-{entry}-{proto}.
// Example: forward-server-01-production-tcp, forward-server-01-validation-udp.
func ForwardName(server, entry, protocol string) string {
	return fmt.Sprintf("forward-%s-%s-%s",
		server, strings.ToLower(strings.TrimSpace(entry)), strings.ToLower(strings.TrimSpace(protocol)))
}

func normalizeProtocols(protocols []string) ([]string, error) {
	if len(protocols) == 0 {
		return []string{"TCP", "UDP"}, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	for _, p := range protocols {
		switch strings.ToUpper(strings.TrimSpace(p)) {
		case "TCP":
			if !seen["TCP"] {
				out = append(out, "TCP")
				seen["TCP"] = true
			}
		case "UDP":
			if !seen["UDP"] {
				out = append(out, "UDP")
				seen["UDP"] = true
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("protocols 至少选择 TCP 或 UDP")
	}
	return out, nil
}

// Forwards are built via AddEntries.

func (r *Resources) forwardNameConflict(name string) string {
	for _, fwd := range r.Forwards {
		if fwd.Name == name {
			return name
		}
	}
	return ""
}

func (r *Resources) allocateListenPort(serverName, entry string, extraUsed ...int) (int, error) {
	used := map[int]struct{}{}
	for _, fwd := range r.Forwards {
		if fwd.ListenPort > 0 {
			used[fwd.ListenPort] = struct{}{}
		}
	}
	for _, p := range extraUsed {
		if p > 0 {
			used[p] = struct{}{}
		}
	}
	base := 10000
	if strings.ToLower(entry) == EntryValidation {
		base = 11000
	}
	if preferred, ok := preferredListenPort(serverName, base); ok {
		if _, taken := used[preferred]; !taken {
			return preferred, nil
		}
	}
	start := base + 1
	for port := start; port <= 65535; port++ {
		if _, taken := used[port]; !taken {
			return port, nil
		}
	}
	return 0, fmt.Errorf("无法分配 %s listen_port", entry)
}

func preferredListenPort(serverName string, base int) (int, bool) {
	m := upstreamNumRe.FindStringSubmatch(serverName)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 || n > 55535 {
		return 0, false
	}
	return base + n, true
}

// PortMapRow is one client-facing entry (listen) → upstream mapping.
type PortMapRow struct {
	Upstream        string `json:"upstream"`
	Entry           string `json:"entry"`
	Protocol        string `json:"protocol"`
	ListenPort      int    `json:"listen_port"`
	UpstreamAddress string `json:"upstream_address"`
	UpstreamPort    int    `json:"upstream_port"`
	Enabled         bool   `json:"enabled"`
	ForwardName     string `json:"forward_name"`
}

// PortMap returns ingress listen ports mapped to backend address/port for clients.
func (r *Resources) PortMap() []PortMapRow {
	upstreams := r.UpstreamMap()
	rows := make([]PortMapRow, 0, len(r.Forwards))
	for _, fwd := range r.Forwards {
		srv, ok := upstreams[fwd.Upstream]
		if !ok {
			continue
		}
		backendPort := 0
		if strings.EqualFold(fwd.Protocol, "UDP") {
			backendPort = srv.UDPPort()
		} else {
			backendPort = srv.TCPPort()
		}
		rows = append(rows, PortMapRow{
			Upstream:       fwd.Upstream,
			Entry:          strings.ToLower(strings.TrimSpace(fwd.Entry)),
			Protocol:       strings.ToUpper(strings.TrimSpace(fwd.Protocol)),
			ListenPort:     fwd.ListenPort,
			UpstreamAddress: srv.Address,
			UpstreamPort:    backendPort,
			Enabled:        fwd.Enabled,
			ForwardName:       fwd.Name,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ListenPort != rows[j].ListenPort {
			return rows[i].ListenPort < rows[j].ListenPort
		}
		if rows[i].Protocol != rows[j].Protocol {
			return rows[i].Protocol < rows[j].Protocol
		}
		return rows[i].ForwardName < rows[j].ForwardName
	})
	return rows
}

// FormatPortMapCSV builds a client-facing port mapping table (not YAML).
func FormatPortMapCSV(r *Resources) string {
	var b strings.Builder
	b.WriteString("listen_port,protocol,entry,upstream,upstream_address,upstream_port,enabled,gateway_public_ip\n")
	ip := strings.TrimSpace(r.Gateway.PublicIP)
	for _, row := range r.PortMap() {
		enabled := "false"
		if row.Enabled {
			enabled = "true"
		}
		fmt.Fprintf(&b, "%d,%s,%s,%s,%s,%d,%s,%s\n",
			row.ListenPort, row.Protocol, row.Entry, row.Upstream,
			row.UpstreamAddress, row.UpstreamPort, enabled, ip)
	}
	return b.String()
}

// UpdateUpstreamResult captures side effects of an upstream update.
type UpdateUpstreamResult struct {
	CascadedForwards int // forwards disabled because the upstream was turned off
}

func (r *Resources) UpdateUpstream(name string, address string, tcp, udp *ProtoPort, enabled bool) (UpdateUpstreamResult, error) {
	name = strings.TrimSpace(name)
	idx := -1
	for i := range r.Upstreams {
		if r.Upstreams[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return UpdateUpstreamResult{}, fmt.Errorf("upstream not found: %s", name)
	}
	wasEnabled := r.Upstreams[idx].Enabled
	if address != "" {
		r.Upstreams[idx].Address = address
	}
	r.Upstreams[idx].TCP = tcp
	r.Upstreams[idx].UDP = udp
	r.Upstreams[idx].Enabled = enabled
	normalizeUpstreamProtocols(&r.Upstreams[idx])
	if err := validateUpstreamFields(r.Upstreams[idx]); err != nil {
		return UpdateUpstreamResult{}, err
	}
	var out UpdateUpstreamResult
	if wasEnabled && !enabled {
		for i := range r.Forwards {
			if r.Forwards[i].Upstream == name && r.Forwards[i].Enabled {
				r.Forwards[i].Enabled = false
				out.CascadedForwards++
			}
		}
	}
	return out, nil
}

func (r *Resources) SetForwardEnabled(name string, enabled bool) error {
	for i := range r.Forwards {
		if r.Forwards[i].Name == name {
			r.Forwards[i].Enabled = enabled
			return nil
		}
	}
	return fmt.Errorf("forward not found: %s", name)
}

// EnableProductionForUpstream enables production entry forwards for an upstream.
// If none exist, creates them (all upstream protocols, enabled) then returns.
func (r *Resources) EnableProductionForUpstream(upstreamName string, enabled bool) (changed int, err error) {
	upstreamName = strings.TrimSpace(upstreamName)
	found := false
	for i := range r.Forwards {
		if strings.ToLower(strings.TrimSpace(r.Forwards[i].Entry)) != EntryProduction {
			continue
		}
		if upstreamName != "" && r.Forwards[i].Upstream != upstreamName {
			continue
		}
		found = true
		if r.Forwards[i].Enabled != enabled {
			r.Forwards[i].Enabled = enabled
			changed++
		}
	}
	if found {
		return changed, nil
	}
	if upstreamName == "" || !enabled {
		return 0, fmt.Errorf("没有匹配的正式转发（production）")
	}
	// Missing production entry: create then enable (product: 禁止有上游缺入口却无法补)
	created, err := r.EnsureEntries(upstreamName, EntryProduction, nil, true)
	if err != nil {
		return 0, err
	}
	if len(created) == 0 {
		return 0, fmt.Errorf("没有匹配的正式转发（production）")
	}
	return len(created), nil
}

func Save(path string, r *Resources) error {
	// Full rewrite is simpler and reliable for panel edits.
	b, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	header := "# 由 relaygate 写入\n# 源文件可继续手工编辑后重新 apply\n"
	if err := os.WriteFile(path, append([]byte(header), b...), 0o660); err != nil {
		return err
	}
	// Panel UMask=0077 会把 WriteFile(0660) 收成 0600；显式 chmod 保留组可写。
	return os.Chmod(path, 0o660)
}

// PatchForwardEnabledInPlace toggles enabled for a named forward while preserving surrounding comments.
func PatchForwardEnabledInPlace(path, forwardName string, enabled bool) (bool, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	val := "false"
	if enabled {
		val = "true"
	}
	pattern := regexp.MustCompile(
		`(?m)(^[ \t]*- name:\s*` + regexp.QuoteMeta(forwardName) + `\s*\n(?:^[ \t]+.*\n)*?^[ \t]+enabled:\s*)(true|false)`,
	)
	changed := false
	out := pattern.ReplaceAllStringFunc(string(text), func(m string) string {
		sub := pattern.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		if sub[2] != val {
			changed = true
		}
		return sub[1] + val
	})
	if !pattern.MatchString(string(text)) {
		return false, fmt.Errorf("未能定位转发配置块: %s", forwardName)
	}
	if !changed {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(out), 0o644)
}

func FindRoot() (string, error) {
	return config.FindRoot()
}

func AbsJoin(root, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, rel)
}

func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func BackupFiles(root, stamp string, files ...string) (string, error) {
	dir := filepath.Join(config.ResolvePaths(root).Backups, stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		name := filepath.Base(f)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return "", err
		}
	}
	_ = os.WriteFile(filepath.Join(config.ResolvePaths(root).Backups, "latest"), []byte(stamp+"\n"), 0o644)
	return dir, nil
}

func BoolPtr(v bool) *bool { return &v }
