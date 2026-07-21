package panel

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/relaygate/relaygate/core/doctor"
	"github.com/relaygate/relaygate/core/ops"
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
	out, err := doctor.RunCapture(doctor.Options{Root: s.cfg.Root})
	writeOpsResult(w, out, err)
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
	out, err := ops.DrainCapture(s.cfg.Root, action)
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
	out, err := ops.SmokeCapture(s.cfg.Root, host)
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
	out, err := ops.CanaryCapture(s.cfg.Root, host)
	writeOpsResult(w, out, err)
}

func (s *Server) apiFirewallCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	out, err := ops.FirewallCapture(s.cfg.Root, false)
	if err != nil && strings.Contains(err.Error(), "需要 root") {
		out = strings.TrimSpace(out) + "\n" + s.t(r, "ops.fw_root_hint")
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
	out := sum.String() + s.t(r, "ops.profile_applied_body")
	s.appendAudit("profile.apply", name)
	writeOpsResult(w, out, nil)
}
