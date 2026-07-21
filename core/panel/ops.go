package panel

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/relaygate/relaygate/core/doctor"
	"github.com/relaygate/relaygate/core/ops"
	"github.com/relaygate/relaygate/core/profile"
	"github.com/relaygate/relaygate/core/resources"
)

func (s *Server) handleOpsPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	profiles, _ := profile.List(s.cfg.Root)
	type profileItem struct {
		Name        string
		Description string
	}
	items := make([]profileItem, 0, len(profiles))
	for _, name := range profiles {
		desc := ""
		if p, err := profile.Load(s.cfg.Root, name); err == nil {
			desc = p.Description
		}
		items = append(items, profileItem{Name: name, Description: desc})
	}
	_ = s.executeTemplate(w, r, "ops.html", map[string]any{
		"Title":      s.t(r, "ops.title"),
		"Nav":        "ops",
		"Profiles":   items,
		"FWCheckCmd": "sudo relaygate firewall check",
		"FWApplyCmd": "sudo APPLY_FIREWALL=1 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES relaygate firewall apply",
	})
}

func (s *Server) hxDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	out, err := doctor.RunCapture(doctor.Options{Root: s.cfg.Root})
	s.renderOpsResult(w, r, "doctor-result", out, err, s.t(r, "ops.toast_doctor_ok"), s.t(r, "ops.toast_doctor_err"))
}

func (s *Server) hxDrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	action := strings.TrimSpace(r.FormValue("action"))
	switch action {
	case "status", "fail", "ok":
	default:
		hxError(w, 400, s.t(r, "error.drain_action"))
		return
	}
	if action == "fail" || action == "ok" {
		if isStandbyRole() {
			s.refuseStandbyWrite(w, r)
			return
		}
		confirm := strings.TrimSpace(r.FormValue("confirm"))
		want := "DRAIN_" + strings.ToUpper(action)
		if confirm != want {
			hxError(w, 400, s.t(r, "error.confirm_typed", want))
			return
		}
	}
	out, err := ops.DrainCapture(s.cfg.Root, action)
	s.appendAudit("drain."+action, strings.TrimSpace(out))
	s.renderOpsResult(w, r, "drain-result", out, err, s.t(r, "ops.toast_drain_ok", action), s.t(r, "ops.toast_drain_err", action))
}

func (s *Server) hxSmoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	host := "127.0.0.1"
	if err := r.ParseForm(); err == nil {
		if h := strings.TrimSpace(r.FormValue("host")); h != "" {
			host = h
		}
	}
	out, err := ops.SmokeCapture(s.cfg.Root, host)
	s.renderOpsResult(w, r, "smoke-result", out, err, s.t(r, "ops.toast_smoke_ok"), s.t(r, "ops.toast_smoke_err"))
}

func (s *Server) hxCanary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	host := "127.0.0.1"
	if err := r.ParseForm(); err == nil {
		if h := strings.TrimSpace(r.FormValue("host")); h != "" {
			host = h
		}
	}
	out, err := ops.CanaryCapture(s.cfg.Root, host)
	s.renderOpsResult(w, r, "canary-result", out, err, s.t(r, "ops.toast_canary_ok"), s.t(r, "ops.toast_canary_err"))
}

func (s *Server) hxFirewallCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	out, err := ops.FirewallCapture(s.cfg.Root, false)
	if err != nil && strings.Contains(err.Error(), "需要 root") {
		out = strings.TrimSpace(out) + "\n" + s.t(r, "ops.fw_root_hint")
		s.renderOpsResult(w, r, "firewall-result", out, err, "", s.t(r, "ops.toast_fw_root"))
		return
	}
	s.renderOpsResult(w, r, "firewall-result", out, err, s.t(r, "ops.toast_fw_ok"), s.t(r, "ops.toast_fw_err"))
}

func (s *Server) hxProfilePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	sum, err := profile.Preview(s.cfg.Root, name)
	if err != nil {
		s.renderOpsResult(w, r, "profile-result", err.Error(), err, "", s.t(r, "ops.toast_preview_err"))
		return
	}
	body := sum.String()
	if p, err := profile.Load(s.cfg.Root, name); err == nil {
		body = profile.FormatShow(p) + "\n--- diff ---\n" + body
	}
	triggerToast(w, s.t(r, "ops.toast_preview_ok"), "ok")
	_ = s.executeTemplate(w, r, "profile-result", map[string]any{
		"Body":  body,
		"Error": false,
		"Name":  name,
	})
}

func (s *Server) hxProfileApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	confirm := strings.TrimSpace(r.FormValue("confirm"))
	if confirm != "APPLY_PROFILE" {
		hxError(w, 400, s.t(r, "error.confirm_typed", "APPLY_PROFILE"))
		return
	}
	sum, err := profile.Apply(s.cfg.Root, name)
	if err != nil {
		s.renderOpsResult(w, r, "profile-result", err.Error(), err, "", s.t(r, "ops.toast_profile_err"))
		return
	}
	body := sum.String() + s.t(r, "ops.profile_applied_body")
	s.appendAudit("profile.apply", name)
	s.renderOpsResult(w, r, "profile-result", body, nil, s.t(r, "ops.toast_profile_ok"), "")
}

func (s *Server) renderOpsResult(w http.ResponseWriter, r *http.Request, tmpl, body string, err error, toastOK, toastErr string) {
	isErr := err != nil
	if isErr {
		if body == "" {
			body = err.Error()
		} else {
			body = strings.TrimRight(body, "\n") + "\n" + err.Error()
		}
		if toastErr != "" {
			triggerToast(w, toastErr, "error")
		}
	} else if toastOK != "" {
		triggerToast(w, toastOK, "ok")
	}
	if body == "" {
		body = s.t(r, "error.no_output")
	}
	_ = s.executeTemplate(w, r, tmpl, map[string]any{"Body": body, "Error": isErr})
}

func (s *Server) hxPromoteServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	// path: /hx/servers/<name>/promote
	path := strings.TrimPrefix(r.URL.Path, "/hx/servers/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "promote" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	res, err := s.load()
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	changed, err := res.EnableProductionForServer(name, true)
	if err != nil {
		hxError(w, 400, err.Error())
		return
	}
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		hxError(w, 500, err.Error())
		return
	}
	s.appendAudit("server.promote", fmt.Sprintf("%s changed=%d", name, changed))
	s.renderServersTable(w, r, res, s.t(r, "servers.toast_promoted", name, changed), "ok")
}
