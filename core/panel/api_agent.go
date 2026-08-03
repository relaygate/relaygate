package panel

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/relaygate/relaygate/core/agent"
)

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-Agent-Token"))
}

func (s *Server) apiAgentConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	token := bearerToken(r)
	if _, err := agent.LookupByToken(s.cfg.Root, token); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未授权的代理令牌"})
		return
	}
	ver, data, err := agent.ReadPublishedResources(s.cfg.Root, "")
	if err != nil {
		writeJSON(w, 404, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"version":        ver,
		"resources_yaml": string(data),
	})
}

func (s *Server) apiAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	token := bearerToken(r)
	node, err := agent.LookupByToken(s.cfg.Root, token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未授权的代理令牌"})
		return
	}
	var body struct {
		AppliedVersion string `json:"applied_version"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := agent.RecordHeartbeat(s.cfg.Root, node.Name, body.AppliedVersion); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
