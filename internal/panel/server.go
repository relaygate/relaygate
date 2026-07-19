package panel

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/robot/proxy/internal/assets"
	"github.com/robot/proxy/internal/envoygen"
	"github.com/robot/proxy/internal/status"
)

type Config struct {
	Root           string
	Bind           string
	AdminPassword  string
	EnvoyAdminURL  string
	PrometheusURL  string
	WebDir         string
	SessionTTL     time.Duration
}

type Server struct {
	cfg      Config
	tmpl     *template.Template
	status   *status.Client
	mu       sync.Mutex
	sessions map[string]time.Time
	lastApply string
}

func New(cfg Config) (*Server, error) {
	if cfg.Root == "" {
		root, err := assets.FindRoot()
		if err != nil {
			return nil, err
		}
		cfg.Root = root
	}
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1:8080"
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.WebDir == "" {
		cfg.WebDir = filepath.Join(cfg.Root, "web")
	}
	if cfg.AdminPassword == "" {
		cfg.AdminPassword = os.Getenv("PANEL_ADMIN_PASSWORD")
	}
	if cfg.AdminPassword == "" {
		return nil, fmt.Errorf("PANEL_ADMIN_PASSWORD is required")
	}

	tmpl, err := template.ParseGlob(filepath.Join(cfg.WebDir, "templates", "*.html"))
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Server{
		cfg:      cfg,
		tmpl:     tmpl,
		status:   status.New(cfg.EnvoyAdminURL, cfg.PrometheusURL),
		sessions: map[string]time.Time{},
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.cfg.WebDir, "static")))))
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/", s.handleOverview)
	mux.HandleFunc("/servers", s.handleServersPage)
	mux.HandleFunc("/rules", s.handleRulesPage)
	mux.HandleFunc("/apply", s.handleApplyPage)

	mux.HandleFunc("/api/servers", s.withAuth(s.apiServers))
	mux.HandleFunc("/api/servers/", s.withAuth(s.apiServerPut))
	mux.HandleFunc("/api/rules", s.withAuth(s.apiRules))
	mux.HandleFunc("/api/rules/", s.withAuth(s.apiRulePatch))
	mux.HandleFunc("/api/apply", s.withAuth(s.apiApply))
	mux.HandleFunc("/api/status/envoy", s.withAuth(s.apiEnvoyStatus))
	mux.HandleFunc("/api/status/traffic", s.withAuth(s.apiTrafficStatus))
	return mux
}

func (s *Server) ListenAndServe() error {
	log.Printf("gateway-panel listening on http://%s", s.cfg.Bind)
	return http.ListenAndServe(s.cfg.Bind, s.Handler())
}

func (s *Server) resourcesPath() string {
	p, _, _ := assets.DefaultPaths(s.cfg.Root)
	return p
}

func (s *Server) load() (*assets.Resources, error) {
	return assets.Load(s.resourcesPath())
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie("panel_session")
	if err != nil || c.Value == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[c.Value]
	if !ok || time.Now().After(exp) {
		delete(s.sessions, c.Value)
		return false
	}
	return true
}

func (s *Server) requirePageAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authed(r) {
		return true
	}
	http.Redirect(w, r, "/login", http.StatusFound)
	return false
}

func (s *Server) createSession() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(s.cfg.SessionTTL)
	s.mu.Unlock()
	return token
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_ = s.tmpl.ExecuteTemplate(w, "login.html", map[string]any{"Error": ""})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	pass := r.FormValue("password")
	if subtle.ConstantTimeCompare([]byte(pass), []byte(s.cfg.AdminPassword)) != 1 {
		_ = s.tmpl.ExecuteTemplate(w, "login.html", map[string]any{"Error": "密码错误"})
		return
	}
	token := s.createSession()
	http.SetCookie(w, &http.Cookie{
		Name:     "panel_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("panel_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "panel_session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !s.requirePageAuth(w, r) {
		return
	}
	res, _ := s.load()
	env := s.status.Envoy()
	traf := s.status.Traffic()
	_ = s.tmpl.ExecuteTemplate(w, "overview.html", map[string]any{
		"Title":     "Overview",
		"Resources": res,
		"Envoy":     env,
		"Traffic":   traf,
		"LastApply": s.lastApply,
	})
}

func (s *Server) handleServersPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	res, err := s.load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.tmpl.ExecuteTemplate(w, "servers.html", map[string]any{"Title": "Servers", "Servers": res.Servers})
}

func (s *Server) handleRulesPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	res, err := s.load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.tmpl.ExecuteTemplate(w, "rules.html", map[string]any{"Title": "Rules", "Rules": res.Rules})
}

func (s *Server) handleApplyPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	res, err := s.load()
	msg := ""
	summary := ""
	if err == nil {
		summary = envoygen.Summarize(res)
	} else {
		msg = err.Error()
	}
	_ = s.tmpl.ExecuteTemplate(w, "apply.html", map[string]any{
		"Title":     "Apply",
		"Summary":   summary,
		"Message":   msg,
		"LastApply": s.lastApply,
	})
}

func (s *Server) apiServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res.Servers)
}

func (s *Server) apiServerPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", 405)
		return
	}
	name := filepath.Base(r.URL.Path)
	var body struct {
		Address         string `json:"address"`
		TCPPort         int    `json:"tcp_port"`
		UDPPort         int    `json:"udp_port"`
		HealthCheckPort int    `json:"health_check_port"`
		Enabled         bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	if err := res.UpdateServer(name, body.Address, body.TCPPort, body.UDPPort, body.HealthCheckPort, body.Enabled); err != nil {
		writeJSON(w, 404, map[string]any{"error": err.Error()})
		return
	}
	if err := assets.Save(s.resourcesPath(), res); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) apiRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res.Rules)
}

func (s *Server) apiRulePatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", 405)
		return
	}
	name := filepath.Base(r.URL.Path)
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	changed, err := assets.PatchRuleEnabledInPlace(s.resourcesPath(), name, body.Enabled)
	if err != nil {
		// fallback full rewrite
		res, loadErr := s.load()
		if loadErr != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		if setErr := res.SetRuleEnabled(name, body.Enabled); setErr != nil {
			writeJSON(w, 404, map[string]any{"error": setErr.Error()})
			return
		}
		if saveErr := assets.Save(s.resourcesPath(), res); saveErr != nil {
			writeJSON(w, 500, map[string]any{"error": saveErr.Error()})
			return
		}
		changed = true
	}
	writeJSON(w, 200, map[string]any{"ok": true, "changed": changed})
}

func (s *Server) apiApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	if err := res.Validate(); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	resourcesPath, envoyOut, nftOut := assets.DefaultPaths(s.cfg.Root)
	stamp := time.Now().Format("20060102-150405")
	if _, err := assets.BackupFiles(s.cfg.Root, stamp, resourcesPath, envoyOut); err != nil {
		writeJSON(w, 500, map[string]any{"error": "backup failed: " + err.Error()})
		return
	}
	if err := envoygen.Write(envoyOut, nftOut, res); err != nil {
		writeJSON(w, 500, map[string]any{"error": "render failed: " + err.Error()})
		return
	}
	reload := filepath.Join(s.cfg.Root, "scripts", "reload_envoy.sh")
	cmd := exec.Command("bash", reload)
	cmd.Dir = s.cfg.Root
	out, err := cmd.CombinedOutput()
	msg := string(out)
	if err != nil {
		s.lastApply = time.Now().Format(time.RFC3339) + " FAIL: " + err.Error() + "\n" + msg
		writeJSON(w, 500, map[string]any{"error": err.Error(), "output": msg})
		return
	}
	s.lastApply = time.Now().Format(time.RFC3339) + " OK\n" + msg
	writeJSON(w, 200, map[string]any{"ok": true, "output": msg, "summary": envoygen.Summarize(res)})
}

func (s *Server) apiEnvoyStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.status.Envoy())
}

func (s *Server) apiTrafficStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.status.Traffic())
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
