package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
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
	node, err := agent.LookupByToken(s.cfg.Root, token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未授权的代理令牌"})
		return
	}
	ver, data, err := agent.ReadPublishedResources(s.cfg.Root, "")
	if err != nil {
		writeJSON(w, 404, map[string]any{"error": err.Error()})
		return
	}
	// Successful config fetch clears a pending per-node sync mark.
	_ = agent.ClearSyncRequest(s.cfg.Root, node.Name)
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
	pullNow, _ := agent.HasSyncRequest(s.cfg.Root, node.Name)
	writeJSON(w, 200, map[string]any{"ok": true, "pull_now": pullNow})
}

// apiAgentMetricsWrite accepts Prometheus remote_write from a joined node and
// forwards it to the control-plane Prometheus (loopback receiver).
func (s *Server) apiAgentMetricsWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if _, err := agent.LookupByToken(s.cfg.Root, bearerToken(r)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "未授权的代理令牌"})
		return
	}
	prom := strings.TrimSpace(s.cfg.PrometheusURL)
	if prom == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "主控未配置指标接收地址"})
		return
	}
	base, err := url.Parse(prom)
	if err != nil || base.Host == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "主控指标接收地址无效"})
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(base)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "无法写入主控指标库"})
	}
	r.URL.Path = "/api/v1/write"
	r.URL.RawQuery = ""
	r.Header.Del("Authorization")
	r.Header.Del("X-Agent-Token")
	proxy.ServeHTTP(w, r)
}
