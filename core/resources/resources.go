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

var serverNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
var serverNumRe = regexp.MustCompile(`(?i)^server-(\d+)$`)

type Resources struct {
	Meta     Meta     `yaml:"meta"`
	Gateway  Gateway  `yaml:"gateway"`
	Defaults Defaults `yaml:"defaults"`
	ACL      ACL      `yaml:"acl,omitempty"`
	Servers  []Server `yaml:"servers"`
	Rules    []Rule   `yaml:"rules"`
}

type Meta struct {
	GatewayName  string `yaml:"gateway_name"`
	GameName     string `yaml:"game_name"`
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
	BackendTCPPort          int         `yaml:"backend_tcp_port"`
	BackendUDPPort          int         `yaml:"backend_udp_port"`
	TCPIdleTimeout          string      `yaml:"tcp_idle_timeout"`
	UDPIdleTimeout          string      `yaml:"udp_idle_timeout"`
	MaxConnections          int         `yaml:"max_connections"`
	MaxPendingRequests      int         `yaml:"max_pending_requests"`
	TCPLocalRateLimitPerSec int         `yaml:"tcp_local_rate_limit_per_sec"`
	TCPLocalRateLimitBurst  int         `yaml:"tcp_local_rate_limit_burst"`
	HealthCheck             HealthCheck `yaml:"health_check"`
	Nftables                NftablesDefaults `yaml:"nftables"`
}

// NftablesDefaults drives host nftables per-IP rate limits (same intent as packaging/firewall).
type NftablesDefaults struct {
	TCPNewConnPerIP string `yaml:"tcp_new_conn_per_ip"`
	UDPPPSPerIP     string `yaml:"udp_pps_per_ip"`
	TCPBurst        int    `yaml:"tcp_burst"`
	UDPBurst        int    `yaml:"udp_burst"`
}

// ApplyNftablesDefaults fills empty nftables fields with product defaults (单一来源).
func (d *Defaults) ApplyNftablesDefaults() {
	if strings.TrimSpace(d.Nftables.TCPNewConnPerIP) == "" {
		d.Nftables.TCPNewConnPerIP = "30/second"
	}
	if strings.TrimSpace(d.Nftables.UDPPPSPerIP) == "" {
		d.Nftables.UDPPPSPerIP = "500/second"
	}
	if d.Nftables.TCPBurst <= 0 {
		d.Nftables.TCPBurst = 60
	}
	if d.Nftables.UDPBurst <= 0 {
		d.Nftables.UDPBurst = 1000
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

// Server is an upstream (game server): alias, address, optional TCP/UDP ports, and enabled.
// Product display name: Upstream; YAML key remains "servers".
// Stage/listen/rollout live on Rule only. Health check (internal) uses TCP port when TCP is set;
// UDP-only servers are not health-checked.
type Server struct {
	Name    string     `yaml:"name" json:"name"`
	Address string     `yaml:"address" json:"address"`
	TCP     *ProtoPort `yaml:"tcp,omitempty" json:"tcp,omitempty"`
	UDP     *ProtoPort `yaml:"udp,omitempty" json:"udp,omitempty"`
	Enabled bool       `yaml:"enabled" json:"enabled"`
}

// HasTCP reports whether TCP upstream is enabled.
func (s Server) HasTCP() bool {
	return s.TCP != nil && s.TCP.Port >= 1 && s.TCP.Port <= 65535
}

// HasUDP reports whether UDP upstream is enabled.
func (s Server) HasUDP() bool {
	return s.UDP != nil && s.UDP.Port >= 1 && s.UDP.Port <= 65535
}

// TCPPort returns the TCP upstream port, or 0 if TCP is not enabled.
func (s Server) TCPPort() int {
	if s.HasTCP() {
		return s.TCP.Port
	}
	return 0
}

// UDPPort returns the UDP upstream port, or 0 if UDP is not enabled.
func (s Server) UDPPort() int {
	if s.HasUDP() {
		return s.UDP.Port
	}
	return 0
}

// HealthCheckPort returns the probe port for Envoy TCP health checks (same as TCP port).
// Returns 0 when TCP is not enabled (UDP-only: no health check).
func (s Server) HealthCheckPort() int {
	return s.TCPPort()
}

// EnabledProtocols lists protocols with a configured upstream port.
func (s Server) EnabledProtocols() []string {
	out := make([]string, 0, 2)
	if s.HasTCP() {
		out = append(out, "TCP")
	}
	if s.HasUDP() {
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

// Entry types for Rule.Entry (入口类型：验证 / 正式).
const (
	EntryValidation = "validation"
	EntryProduction = "production"
)

// Rule is an ingress forward: entry type, protocol, listen port, switch, upstream ref.
// Naming: forward-{server}-{entry}-{proto}.
type Rule struct {
	Name       string `yaml:"name" json:"name"`
	Entry      string `yaml:"entry" json:"entry"` // validation | production
	Server     string `yaml:"server" json:"server"`
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

func (r *Resources) ServerMap() map[string]Server {
	m := make(map[string]Server, len(r.Servers))
	for _, s := range r.Servers {
		m[s.Name] = s
	}
	return m
}

func (r *Resources) EnabledRules() []Rule {
	out := make([]Rule, 0, len(r.Rules))
	for _, rule := range r.Rules {
		if rule.Enabled {
			out = append(out, rule)
		}
	}
	return out
}

func (r *Resources) Validate() error {
	if err := r.ACL.NormalizeACL(); err != nil {
		return err
	}
	if len(r.Servers) == 0 {
		return fmt.Errorf("servers 不能为空")
	}
	seenNames := map[string]struct{}{}
	for _, s := range r.Servers {
		if err := validateServerFields(s); err != nil {
			return err
		}
		if _, ok := seenNames[s.Name]; ok {
			return fmt.Errorf("server 名称重复: %s", s.Name)
		}
		seenNames[s.Name] = struct{}{}
	}
	servers := r.ServerMap()

	// All rules: entry/protocol/port/server refs (规划期也要能发现错误)
	allPorts := map[string]string{} // proto/port -> rule name
	for _, rule := range r.Rules {
		entry := strings.ToLower(strings.TrimSpace(rule.Entry))
		if !ValidEntry(entry) {
			return fmt.Errorf("%s: entry 必须是 validation 或 production（当前 %q）", rule.Name, rule.Entry)
		}
		proto := strings.ToUpper(rule.Protocol)
		if proto != "TCP" && proto != "UDP" {
			return fmt.Errorf("%s: protocol 必须是 TCP 或 UDP", rule.Name)
		}
		if rule.ListenPort < 1 || rule.ListenPort > 65535 {
			return fmt.Errorf("%s: listen_port 越界: %d", rule.Name, rule.ListenPort)
		}
		srv, ok := servers[rule.Server]
		if !ok {
			return fmt.Errorf("%s: 未知 server %s", rule.Name, rule.Server)
		}
		if proto == "TCP" && !srv.HasTCP() {
			return fmt.Errorf("%s: server %s 未启用 TCP（缺少 tcp.port）", rule.Name, rule.Server)
		}
		if proto == "UDP" && !srv.HasUDP() {
			return fmt.Errorf("%s: server %s 未启用 UDP（缺少 udp.port）", rule.Name, rule.Server)
		}
		key := fmt.Sprintf("%s/%d", proto, rule.ListenPort)
		if other, ok := allPorts[key]; ok {
			return fmt.Errorf("端口冲突: %s 同时被 %s 与 %s 使用（含未启用转发；验证与正式入口也不可重叠）", key, other, rule.Name)
		}
		allPorts[key] = rule.Name
	}

	for _, rule := range r.EnabledRules() {
		s := servers[rule.Server]
		if !s.Enabled {
			return fmt.Errorf("%s: 目标 %s 已禁用", rule.Name, rule.Server)
		}
	}
	if len(r.EnabledRules()) == 0 {
		return fmt.Errorf("没有启用的转发（rules）；至少启用一条转发后再渲染")
	}
	return nil
}

func validateServerFields(s Server) error {
	name := strings.TrimSpace(s.Name)
	if name == "" {
		return fmt.Errorf("server name 不能为空")
	}
	if !serverNameRe.MatchString(name) {
		return fmt.Errorf("server name 无效: %s（仅允许字母数字、_、-，且不能以符号开头）", name)
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

// normalizeServerProtocols clears invalid/empty protocol blocks so unused
// protocols stay truly absent (no mirror placeholders).
func normalizeServerProtocols(s *Server) {
	if s.TCP != nil && (s.TCP.Port < 1 || s.TCP.Port > 65535) {
		s.TCP = nil
	}
	if s.UDP != nil && (s.UDP.Port < 1 || s.UDP.Port > 65535) {
		s.UDP = nil
	}
}

// AddServer appends an upstream only — no rules are created.
// Create entries separately via AddEntries / EnsureEntries.
func (r *Resources) AddServer(s Server) error {
	s.Name = strings.TrimSpace(s.Name)
	s.Address = strings.TrimSpace(s.Address)
	normalizeServerProtocols(&s)
	if err := validateServerFields(s); err != nil {
		return err
	}
	for _, existing := range r.Servers {
		if existing.Name == s.Name {
			return fmt.Errorf("server 已存在: %s", s.Name)
		}
	}
	r.Servers = append(r.Servers, s)
	return nil
}

// AddEntryOptions controls creating ingress rules for an existing upstream.
type AddEntryOptions struct {
	Server    string
	Entry     string   // validation | production
	Protocols []string // empty = all enabled protocols on the server
	// Enabled overrides default: validation→true, production→false. Nil keeps default.
	Enabled *bool
	// ListenPort reuses/forces listen port; 0 = allocate (or reuse existing entry port).
	ListenPort int
}

// AddEntries creates missing entry×protocol rules for a server (idempotent by ForwardName).
// Existing rules with the same name are skipped (not modified).
func (r *Resources) AddEntries(opts AddEntryOptions) (created []Rule, err error) {
	server := strings.TrimSpace(opts.Server)
	if server == "" {
		return nil, fmt.Errorf("server 不能为空")
	}
	entry := NormalizeEntry(opts.Entry)
	if entry == "" {
		return nil, fmt.Errorf("entry 必须是 validation 或 production")
	}
	srv, ok := r.ServerMap()[server]
	if !ok {
		return nil, fmt.Errorf("server not found: %s", server)
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
		if existing := r.entryListenPort(server, entry); existing > 0 {
			listenPort = existing
		} else {
			listenPort, err = r.allocateListenPort(server, entry)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, proto := range protos {
		name := ForwardName(server, entry, proto)
		if r.ruleNameConflict(name) != "" {
			continue // idempotent skip
		}
		rule := Rule{
			Name:       name,
			Entry:      entry,
			Server:     server,
			Protocol:   proto,
			ListenPort: listenPort,
			Enabled:    enable,
		}
		created = append(created, rule)
	}
	r.Rules = append(r.Rules, created...)
	return created, nil
}

// EnsureEntries creates missing forwards for the given entry type + protocols.
// Unlike filling only existing entries, this may create a brand-new entry type.
func (r *Resources) EnsureEntries(server, entry string, protocols []string, enable bool) (created []Rule, err error) {
	return r.AddEntries(AddEntryOptions{
		Server:    server,
		Entry:     entry,
		Protocols: protocols,
		Enabled:   BoolPtr(enable),
	})
}

func (r *Resources) entryListenPort(server, entry string) int {
	entry = strings.ToLower(strings.TrimSpace(entry))
	for _, rule := range r.Rules {
		if rule.Server != server {
			continue
		}
		if strings.ToLower(strings.TrimSpace(rule.Entry)) != entry {
			continue
		}
		if rule.ListenPort > 0 {
			return rule.ListenPort
		}
	}
	return 0
}

// resolveAddProtocols picks rule protocols from explicit opts or from server ports.
func resolveAddProtocols(s Server, protocols []string) ([]string, error) {
	if len(protocols) == 0 {
		out := s.EnabledProtocols()
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
			if !s.HasTCP() {
				return nil, fmt.Errorf("已选 TCP 但未设置 tcp.port")
			}
		case "UDP":
			if !s.HasUDP() {
				return nil, fmt.Errorf("已选 UDP 但未设置 udp.port")
			}
		}
	}
	return protos, nil
}

// DeleteServer removes a server and all rules that reference it.
func (r *Resources) DeleteServer(name string) (removedRules int, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("server name 不能为空")
	}
	idx := -1
	for i, s := range r.Servers {
		if s.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, fmt.Errorf("server not found: %s", name)
	}
	if len(r.Servers) == 1 {
		return 0, fmt.Errorf("不能删除最后一台 server")
	}
	r.Servers = append(r.Servers[:idx], r.Servers[idx+1:]...)
	kept := r.Rules[:0]
	for _, rule := range r.Rules {
		if rule.Server == name {
			removedRules++
			continue
		}
		kept = append(kept, rule)
	}
	r.Rules = kept
	return removedRules, nil
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

// (ruleTemplates removed — rules are built via AddEntries)

func (r *Resources) ruleNameConflict(name string) string {
	for _, rule := range r.Rules {
		if rule.Name == name {
			return name
		}
	}
	return ""
}

func (r *Resources) allocateListenPort(serverName, entry string, extraUsed ...int) (int, error) {
	used := map[int]struct{}{}
	for _, rule := range r.Rules {
		if rule.ListenPort > 0 {
			used[rule.ListenPort] = struct{}{}
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
	m := serverNumRe.FindStringSubmatch(serverName)
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
// JSON fields backend_* are historical; product layer calls them upstream address/port.
type PortMapRow struct {
	Server         string `json:"server"`
	Entry          string `json:"entry"`
	Protocol       string `json:"protocol"`
	ListenPort     int    `json:"listen_port"`
	BackendAddress string `json:"backend_address"`
	BackendPort    int    `json:"backend_port"`
	Enabled        bool   `json:"enabled"`
	RuleName       string `json:"rule_name"`
}

// PortMap returns ingress listen ports mapped to backend address/port for clients.
func (r *Resources) PortMap() []PortMapRow {
	servers := r.ServerMap()
	rows := make([]PortMapRow, 0, len(r.Rules))
	for _, rule := range r.Rules {
		srv, ok := servers[rule.Server]
		if !ok {
			continue
		}
		backendPort := 0
		if strings.EqualFold(rule.Protocol, "UDP") {
			backendPort = srv.UDPPort()
		} else {
			backendPort = srv.TCPPort()
		}
		rows = append(rows, PortMapRow{
			Server:         rule.Server,
			Entry:          strings.ToLower(strings.TrimSpace(rule.Entry)),
			Protocol:       strings.ToUpper(strings.TrimSpace(rule.Protocol)),
			ListenPort:     rule.ListenPort,
			BackendAddress: srv.Address,
			BackendPort:    backendPort,
			Enabled:        rule.Enabled,
			RuleName:       rule.Name,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ListenPort != rows[j].ListenPort {
			return rows[i].ListenPort < rows[j].ListenPort
		}
		if rows[i].Protocol != rows[j].Protocol {
			return rows[i].Protocol < rows[j].Protocol
		}
		return rows[i].RuleName < rows[j].RuleName
	})
	return rows
}

// FormatPortMapCSV builds a client-facing port mapping table (not YAML).
func FormatPortMapCSV(r *Resources) string {
	var b strings.Builder
	b.WriteString("listen_port,protocol,entry,server,backend_address,backend_port,enabled,gateway_public_ip\n")
	ip := strings.TrimSpace(r.Gateway.PublicIP)
	for _, row := range r.PortMap() {
		enabled := "false"
		if row.Enabled {
			enabled = "true"
		}
		fmt.Fprintf(&b, "%d,%s,%s,%s,%s,%d,%s,%s\n",
			row.ListenPort, row.Protocol, row.Entry, row.Server,
			row.BackendAddress, row.BackendPort, enabled, ip)
	}
	return b.String()
}

// UpdateServerResult captures side effects of a server update.
type UpdateServerResult struct {
	CascadedRules int // rules disabled because the server was turned off
}

func (r *Resources) UpdateServer(name string, address string, tcp, udp *ProtoPort, enabled bool) (UpdateServerResult, error) {
	name = strings.TrimSpace(name)
	idx := -1
	for i := range r.Servers {
		if r.Servers[i].Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return UpdateServerResult{}, fmt.Errorf("server not found: %s", name)
	}
	wasEnabled := r.Servers[idx].Enabled
	if address != "" {
		r.Servers[idx].Address = address
	}
	r.Servers[idx].TCP = tcp
	r.Servers[idx].UDP = udp
	r.Servers[idx].Enabled = enabled
	normalizeServerProtocols(&r.Servers[idx])
	if err := validateServerFields(r.Servers[idx]); err != nil {
		return UpdateServerResult{}, err
	}
	var out UpdateServerResult
	if wasEnabled && !enabled {
		for i := range r.Rules {
			if r.Rules[i].Server == name && r.Rules[i].Enabled {
				r.Rules[i].Enabled = false
				out.CascadedRules++
			}
		}
	}
	return out, nil
}

func (r *Resources) SetRuleEnabled(name string, enabled bool) error {
	for i := range r.Rules {
		if r.Rules[i].Name == name {
			r.Rules[i].Enabled = enabled
			return nil
		}
	}
	return fmt.Errorf("rule not found: %s", name)
}

// EnableProductionForServer enables production entry forwards for a server.
// If none exist, creates them (all server protocols, enabled) then returns.
func (r *Resources) EnableProductionForServer(server string, enabled bool) (changed int, err error) {
	server = strings.TrimSpace(server)
	found := false
	for i := range r.Rules {
		if strings.ToLower(strings.TrimSpace(r.Rules[i].Entry)) != EntryProduction {
			continue
		}
		if server != "" && r.Rules[i].Server != server {
			continue
		}
		found = true
		if r.Rules[i].Enabled != enabled {
			r.Rules[i].Enabled = enabled
			changed++
		}
	}
	if found {
		return changed, nil
	}
	if server == "" || !enabled {
		return 0, fmt.Errorf("没有匹配的正式转发（production）")
	}
	// Missing production entry: create then enable (product: 禁止有上游缺入口却无法补)
	created, err := r.EnsureEntries(server, EntryProduction, nil, true)
	if err != nil {
		return 0, err
	}
	if len(created) == 0 {
		return 0, fmt.Errorf("没有匹配的正式转发（production）")
	}
	return len(created), nil
}

// SavePreserveComments updates enabled/address fields via regex where possible for rules,
// and rewrites the full document for server field updates (comments on rules mostly preserved for enable toggles).
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

// PatchRuleEnabledInPlace toggles enabled for a named rule while preserving surrounding comments.
func PatchRuleEnabledInPlace(path, ruleName string, enabled bool) (bool, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	val := "false"
	if enabled {
		val = "true"
	}
	pattern := regexp.MustCompile(
		`(?m)(^[ \t]*- name:\s*` + regexp.QuoteMeta(ruleName) + `\s*\n(?:^[ \t]+.*\n)*?^[ \t]+enabled:\s*)(true|false)`,
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
		return false, fmt.Errorf("未能定位转发配置块: %s", ruleName)
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
