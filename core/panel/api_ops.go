package panel

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/relaygate/relaygate/core/agent"
	"github.com/relaygate/relaygate/core/diag"
	"github.com/relaygate/relaygate/core/dataplane"
	"github.com/relaygate/relaygate/core/profile"
)

func writeOpsResult(w http.ResponseWriter, body string, err error) {
	if err != nil {
		if body == "" {
			body = err.Error()
		} else {
			body = strings.TrimRight(body, "\n") + "\n" + err.Error()
		}
		writeJSON(w, 500, map[string]any{"ok": false, "output": body, "error": err.Error()})
		return
	}
	if body == "" {
		body = "(no output)"
	}
	writeJSON(w, 200, map[string]any{"ok": true, "output": body})
}

func (s *Server) apiProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	names, _ := profile.List(s.cfg.Root)
	type item struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	items := make([]item, 0, len(names))
	for _, name := range names {
		desc := ""
		if p, err := profile.Load(s.cfg.Root, name); err == nil {
			desc = p.Description
		}
		items = append(items, item{Name: name, Description: desc})
	}
	writeJSON(w, 200, map[string]any{"profiles": items})
}

func (s *Server) apiDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	out, err := diag.RunCapture(diag.Options{Root: s.cfg.Root})
	writeOpsResult(w, out, err)
}

func (s *Server) apiFleet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	reg, err := agent.LoadRegistry(s.cfg.Root)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	meta, _ := agent.CurrentMeta(s.cfg.Root)
	writeJSON(w, 200, map[string]any{
		"nodes":            reg.Nodes,
		"published":        meta,
		"registry":         agent.NodesPath(s.cfg.Root),
		"hints":            fleetProductHints(),
	})
}

func fleetProductHints() []string {
	return []string{
		"relaygate fleet status",
		"relaygate fleet publish   # 确认 PUBLISH_FLEET",
		"relaygate fleet join <name>   # 生成一句话安装命令",
		"relaygate fleet leave <name> # 确认 FLEET_LEAVE",
		"# 安装主控: " + agent.FormatControlInstallCommand(),
		"# 升级（主控/节点）: " + agent.FormatUpgradeCommand(),
		"# 节点接入: 粘贴 join 输出的一行，或以 PRIMARY_URL+AGENT_TOKEN 跑 install.sh",
	}
}

func (s *Server) apiFleetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	published, nodes, err := agent.BuildStatus(s.cfg.Root)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	meta, _ := agent.CurrentMeta(s.cfg.Root)
	writeJSON(w, 200, map[string]any{
		"published_version": published,
		"published":         meta,
		"nodes":             nodes,
	})
}

func (s *Server) apiFleetPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Confirm) != "PUBLISH_FLEET" {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.confirm_typed", "PUBLISH_FLEET")})
		return
	}
	res, err := agent.Publish(s.cfg.Root)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.appendAudit("fleet.publish", res.Version)
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"version": res.Version,
		"path":    res.Path,
		"output":  "已发布配置版本 " + res.Version + "。网关节点将自行拉取并对齐。",
	})
}

func (s *Server) apiFleetJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Name       string `json:"name"`
		PrimaryURL string `json:"primary_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	// Join 只写名册与一次性令牌，不发布版本、不热更新/硬重启既有节点，故无需确认词。
	primary := agent.ResolvePrimaryURL(body.PrimaryURL)
	res, err := agent.JoinNode(s.cfg.Root, body.Name, primary)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.appendAudit("fleet.join", res.Name)
	writeJSON(w, 200, map[string]any{
		"ok":               true,
		"name":             res.Name,
		"token":            res.Token,
		"token_file_hint":  res.TokenFileHint,
		"bootstrap_hint":   res.BootstrapHint,
		"join_command":     res.JoinCommand,
		"primary_url_hint": res.PrimaryURLHint,
		"manual_hints": []string{
			"在目标主机以 root 执行 join_command 一行即可安装并连接主控。",
			"若前置了可选云 L4 入口：请在云控制台/Terraform 将新节点注册到目标组（本产品不调用云 API）。公网直连网关可跳过。",
		},
	})
}

func (s *Server) apiFleetLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Confirm) != "FLEET_LEAVE" {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.confirm_typed", "FLEET_LEAVE")})
		return
	}
	res, err := agent.LeaveNode(s.cfg.Root, body.Name)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.appendAudit("fleet.leave", res.Name)
	writeJSON(w, 200, map[string]any{
		"ok":           true,
		"name":         res.Name,
		"manual_hints": res.ManualHints,
	})
}

func (s *Server) apiDrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Action  string `json:"action"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	action := strings.TrimSpace(body.Action)
	switch action {
	case "status", "fail", "ok":
	default:
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.drain_action")})
		return
	}
	if action == "fail" || action == "ok" {
		if isStandbyRole() {
			s.refuseStandbyWrite(w, r)
			return
		}
		want := "DRAIN_" + strings.ToUpper(action)
		if strings.TrimSpace(body.Confirm) != want {
			writeJSON(w, 400, map[string]any{"error": s.t(r, "error.confirm_typed", want)})
			return
		}
	}
	out, err := dataplane.DrainCapture(s.cfg.Root, action)
	s.appendAudit("drain."+action, strings.TrimSpace(out))
	writeOpsResult(w, out, err)
}

func (s *Server) apiSmoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Host string `json:"host"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	host := strings.TrimSpace(body.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	out, err := dataplane.SmokeCapture(s.cfg.Root, host)
	writeOpsResult(w, out, err)
}

func (s *Server) apiCanary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Host string `json:"host"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	host := strings.TrimSpace(body.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	out, err := dataplane.CanaryCapture(s.cfg.Root, host)
	writeOpsResult(w, out, err)
}

func (s *Server) apiFirewallCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	out, err := dataplane.FirewallCapture(s.cfg.Root, false)
	if err != nil && strings.Contains(err.Error(), "需要 root") {
		out = strings.TrimSpace(out) + "\n" + s.t(r, "dataplane.fw_root_hint")
		writeOpsResult(w, out, err)
		return
	}
	writeOpsResult(w, out, err)
}

func (s *Server) apiProfilePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(body.Name)
	sum, err := profile.Preview(s.cfg.Root, name)
	if err != nil {
		writeOpsResult(w, err.Error(), err)
		return
	}
	out := sum.String()
	if p, err := profile.Load(s.cfg.Root, name); err == nil {
		out = profile.FormatShow(p) + "\n--- diff ---\n" + out
	}
	writeOpsResult(w, out, nil)
}

func (s *Server) apiProfileApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Name    string `json:"name"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Confirm) != "APPLY_PROFILE" {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.confirm_typed", "APPLY_PROFILE")})
		return
	}
	name := strings.TrimSpace(body.Name)
	sum, err := profile.Apply(s.cfg.Root, name)
	if err != nil {
		writeOpsResult(w, err.Error(), err)
		return
	}
	out := sum.String() + s.t(r, "dataplane.profile_applied_body")
	s.appendAudit("profile.apply", name)
	writeOpsResult(w, out, nil)
}
