package panel

import (
	"net/http"
	"strings"

	"github.com/relaygate/relaygate/core/ops"
	"github.com/relaygate/relaygate/core/resources"
)

func (s *Server) handleChangesPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	entries, err := resources.ListChangeSummaries(s.cfg.Root, 50)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_ = s.executeTemplate(w, r, "changes.html", map[string]any{
		"Title":   s.t(r, "changes.title"),
		"Nav":     "changes",
		"Entries": entries,
		"Message": msg,
	})
}

func (s *Server) hxChangeSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	stamp := strings.TrimPrefix(r.URL.Path, "/hx/changes/")
	stamp = strings.Trim(stamp, "/")
	if stamp == "" || strings.Contains(stamp, "/") || strings.Contains(stamp, "..") {
		hxError(w, 400, s.t(r, "error.invalid_stamp"))
		return
	}
	entries, err := resources.ListChangeSummaries(s.cfg.Root, 200)
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	var summary string
	found := false
	for _, e := range entries {
		if e.Stamp == stamp {
			summary = e.Summary
			found = true
			break
		}
	}
	if !found {
		hxError(w, 404, s.t(r, "error.summary_not_found", stamp))
		return
	}
	_ = s.executeTemplate(w, r, "change-summary", map[string]any{
		"Stamp":   stamp,
		"Summary": summary,
	})
}

func (s *Server) hxRollbackPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	stamp := strings.TrimSpace(r.FormValue("stamp"))
	if stamp == "" || strings.Contains(stamp, "/") || strings.Contains(stamp, "..") {
		hxError(w, 400, s.t(r, "error.invalid_stamp"))
		return
	}
	entries, err := resources.ListChangeSummaries(s.cfg.Root, 200)
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	var summary string
	found := false
	for _, e := range entries {
		if e.Stamp == stamp {
			summary = e.Summary
			found = true
			break
		}
	}
	if !found {
		// Allow rollback to backup dirs that may lack change-summary.
		summary = s.t(r, "changes.no_summary")
	}
	_ = s.executeTemplate(w, r, "rollback-preview", map[string]any{
		"Stamp":   stamp,
		"Summary": summary,
		"Found":   found,
	})
}

func (s *Server) hxRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	stamp := strings.TrimSpace(r.FormValue("stamp"))
	confirm := strings.TrimSpace(r.FormValue("confirm"))
	if stamp == "" || strings.Contains(stamp, "/") || strings.Contains(stamp, "..") {
		hxError(w, 400, s.t(r, "error.invalid_stamp"))
		return
	}
	if confirm != "ROLLBACK" {
		hxError(w, 400, s.t(r, "error.confirm_typed", "ROLLBACK"))
		return
	}
	out, err := ops.RollbackCapture(s.cfg.Root, stamp)
	s.appendAudit("rollback", stamp)
	isErr := err != nil
	body := out
	if isErr {
		if body == "" {
			body = err.Error()
		} else {
			body = strings.TrimRight(body, "\n") + "\n" + err.Error()
		}
		triggerToast(w, s.t(r, "changes.rollback_err"), "error")
	} else {
		triggerToast(w, s.t(r, "changes.rollback_ok"), "ok")
	}
	if body == "" {
		body = s.t(r, "error.no_output")
	}
	_ = s.executeTemplate(w, r, "rollback-result", map[string]any{
		"Body":  body,
		"Error": isErr,
		"Stamp": stamp,
	})
}
