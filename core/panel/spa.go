package panel

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spaHandler serves ui/dist assets and falls back to index.html for SPA routes.
func (s *Server) spaHandler() http.HandlerFunc {
	uiDir := s.cfg.UIDir
	indexPath := filepath.Join(uiDir, "index.html")
	fileServer := http.FileServer(http.Dir(uiDir))

	return func(w http.ResponseWriter, r *http.Request) {
		// Never let SPA swallow API or grafana paths (defensive).
		// Check before method gate: otherwise an unregistered POST /api/... looks like
		// "method not allowed" (405) instead of a missing route (404).
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/grafana") || strings.HasPrefix(p, "/public/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, err := os.Stat(indexPath); err != nil {
			http.Error(w, "panel UI not built (missing ui/dist); run: make ui", http.StatusServiceUnavailable)
			return
		}
		// Clean path and try static file first.
		clean := path.Clean("/" + strings.TrimPrefix(p, "/"))
		if clean == "/" {
			http.ServeFile(w, r, indexPath)
			return
		}
		fsPath := filepath.Join(uiDir, filepath.FromSlash(clean))
		// Prevent path escape
		if !strings.HasPrefix(fsPath, filepath.Clean(uiDir)+string(os.PathSeparator)) && fsPath != filepath.Clean(uiDir) {
			http.NotFound(w, r)
			return
		}
		if st, err := os.Stat(fsPath); err == nil && !st.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Client-side route → index.html
		http.ServeFile(w, r, indexPath)
	}
}

func (s *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	ip := clientIP(r)
	if ok, wait := s.loginAllowed(ip); !ok {
		writeJSON(w, 429, map[string]any{"error": s.t(r, "login.error_rate_limit", wait.Seconds())})
		return
	}
	if !passwordMatch(body.Password, s.cfg.AdminPassword) {
		s.recordLoginFailure(ip)
		writeJSON(w, 401, map[string]any{"error": s.t(r, "login.error_password")})
		return
	}
	token, csrf, err := s.createSession()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": s.t(r, "login.error_session")})
		return
	}
	s.clearLoginFailures(ip)
	s.setSessionCookies(w, token, csrf)
	writeJSON(w, 200, map[string]any{
		"ok":              true,
		"csrf":            csrf,
		"lang":            resolveLang(r),
		"role":            panelRole(),
		"grafana_enabled": s.GrafanaEnabled(),
	})
}

func (s *Server) apiLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	s.clearSessionCookies(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) apiSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.authed(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, 200, map[string]any{
		"authenticated":   true,
		"csrf":            s.sessionCSRF(r),
		"lang":            resolveLang(r),
		"role":            panelRole(),
		"standby":         isStandbyRole(),
		"grafana_enabled": s.GrafanaEnabled(),
	})
}

func (s *Server) apiLang(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Lang string `json:"lang"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	lang := normalizeLang(body.Lang)
	if lang == "" {
		writeJSON(w, 400, map[string]any{"error": "invalid lang"})
		return
	}
	setLangCookie(w, lang)
	writeJSON(w, 200, map[string]any{"ok": true, "lang": lang})
}
