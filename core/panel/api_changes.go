package panel

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/relaygate/relaygate/core/confirm"
	"github.com/relaygate/relaygate/core/dataplane"
	"github.com/relaygate/relaygate/core/resources"
)

func (s *Server) apiChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := resources.ListChangeSummaries(s.cfg.Root, limit)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	type item struct {
		Stamp   string `json:"stamp"`
		Summary string `json:"summary"`
		Path    string `json:"path"`
	}
	out := make([]item, 0, len(entries))
	for _, e := range entries {
		out = append(out, item{Stamp: e.Stamp, Summary: e.Summary, Path: e.Path})
	}
	writeJSON(w, 200, map[string]any{"entries": out})
}

func (s *Server) apiChangeByStamp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	stamp := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/changes/"), "/")
	if stamp == "" || strings.Contains(stamp, "/") || strings.Contains(stamp, "..") {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.invalid_stamp")})
		return
	}
	entries, err := resources.ListChangeSummaries(s.cfg.Root, 200)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	for _, e := range entries {
		if e.Stamp == stamp {
			writeJSON(w, 200, map[string]any{"stamp": stamp, "summary": e.Summary})
			return
		}
	}
	writeJSON(w, 404, map[string]any{"error": s.t(r, "error.summary_not_found", stamp)})
}

func (s *Server) apiRollbackPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Stamp string `json:"stamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	stamp := strings.TrimSpace(body.Stamp)
	if stamp == "" || strings.Contains(stamp, "/") || strings.Contains(stamp, "..") {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.invalid_stamp")})
		return
	}
	entries, err := resources.ListChangeSummaries(s.cfg.Root, 200)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	summary := s.t(r, "changes.no_summary")
	found := false
	for _, e := range entries {
		if e.Stamp == stamp {
			summary = e.Summary
			found = true
			break
		}
	}
	writeJSON(w, 200, map[string]any{"stamp": stamp, "summary": summary, "found": found})
}

func (s *Server) apiRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Stamp   string `json:"stamp"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	stamp := strings.TrimSpace(body.Stamp)
	if stamp == "" || strings.Contains(stamp, "/") || strings.Contains(stamp, "..") {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.invalid_stamp")})
		return
	}
	if !confirm.Match(body.Confirm) {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.confirm_typed")})
		return
	}
	out, err := dataplane.RollbackCapture(s.cfg.Root, stamp)
	s.appendAudit("rollback", stamp)
	writeOpsResult(w, out, err)
}
