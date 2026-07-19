package panel

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robot/proxy/internal/assets"
	"github.com/robot/proxy/internal/envoygen"
	"github.com/robot/proxy/internal/status"
)

type Config struct {
	Root          string
	Bind          string
	AdminPassword string
	EnvoyAdminURL string
	PrometheusURL string
	GrafanaURL    string
	WebDir        string
	SessionTTL    time.Duration
}

type Server struct {
	cfg          Config
	tmpl         *template.Template
	status       *status.Client
	mu           sync.Mutex
	sessions     map[string]time.Time
	lastApply    string
	grafanaProxy http.Handler
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
		cfg.Bind = "127.0.0.1:9000"
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
		if passwordFile := os.Getenv("PANEL_ADMIN_PASSWORD_FILE"); passwordFile != "" {
			password, err := os.ReadFile(passwordFile)
			if err != nil {
				return nil, fmt.Errorf("read PANEL_ADMIN_PASSWORD_FILE: %w", err)
			}
			cfg.AdminPassword = strings.TrimSpace(string(password))
		}
	}
	if cfg.AdminPassword == "" {
		return nil, fmt.Errorf("PANEL_ADMIN_PASSWORD or PANEL_ADMIN_PASSWORD_FILE is required")
	}

	tmpl, err := template.ParseGlob(filepath.Join(cfg.WebDir, "templates", "*.html"))
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	srv := &Server{
		cfg:      cfg,
		tmpl:     tmpl,
		status:   status.New(cfg.EnvoyAdminURL, cfg.PrometheusURL),
		sessions: map[string]time.Time{},
	}
	if cfg.GrafanaURL != "" {
		proxy, err := newGrafanaProxy(cfg.GrafanaURL)
		if err != nil {
			return nil, err
		}
		srv.grafanaProxy = proxy
	}
	return srv, nil
}

// newGrafanaProxy builds a fixed-target reverse proxy for Grafana.
// Path is not stripped (serve_from_sub_path); WebSocket upgrades are handled by httputil.ReverseProxy.
func newGrafanaProxy(rawURL string) (http.Handler, error) {
	target, err := parseGrafanaURL(rawURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1 // streaming / Grafana Live WebSocket
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		fwdHost := req.Host
		director(req)
		// Preserve browser Host (e.g. localhost:9000) so Grafana root_url / redirects stay on Panel.
		if fwdHost != "" {
			req.Host = fwdHost
			if req.Header.Get("X-Forwarded-Host") == "" {
				req.Header.Set("X-Forwarded-Host", fwdHost)
			}
		}
		if req.Header.Get("X-Forwarded-Proto") == "" {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Grafana 暂不可用: "+err.Error(), http.StatusBadGateway)
	}
	return proxy, nil
}

func parseGrafanaURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse GRAFANA_URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("GRAFANA_URL scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("GRAFANA_URL missing host")
	}
	host := u.Hostname()
	if !isLoopbackHost(host) {
		return nil, fmt.Errorf("GRAFANA_URL host must be loopback (127.0.0.1/localhost/[::1]), got %q", host)
	}
	// Dial Grafana at its own root; Panel serves under /grafana/ and forwards that prefix intact.
	return &url.URL{Scheme: u.Scheme, Host: u.Host}, nil
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// GrafanaEnabled reports whether the in-panel Grafana proxy is configured.
func (s *Server) GrafanaEnabled() bool { return s.grafanaProxy != nil }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.cfg.WebDir, "static")))))
	faviconSVG := filepath.Join(s.cfg.WebDir, "static", "favicon.svg")
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, faviconSVG)
	})
	// Browsers still probe /favicon.ico; redirect to SVG.
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/favicon.svg", http.StatusFound)
	})
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/", s.handleOverview)
	mux.HandleFunc("/servers", s.handleServersPage)
	mux.HandleFunc("/rules", s.handleRulesPage)
	mux.HandleFunc("/apply", s.handleApplyPage)
	mux.HandleFunc("/monitoring", s.handleMonitoringPage)
	if s.grafanaProxy != nil {
		mux.HandleFunc("/grafana", func(w http.ResponseWriter, r *http.Request) {
			if !s.authed(r) {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/grafana/", http.StatusFound)
		})
		mux.HandleFunc("/grafana/", func(w http.ResponseWriter, r *http.Request) {
			if !s.authed(r) {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			s.grafanaProxy.ServeHTTP(w, r)
		})
	}

	mux.HandleFunc("/api/servers", s.withAuth(s.apiServers))
	mux.HandleFunc("/api/servers/", s.withAuth(s.apiServerByName))
	mux.HandleFunc("/api/rules", s.withAuth(s.apiRules))
	mux.HandleFunc("/api/rules/", s.withAuth(s.apiRulePatch))
	mux.HandleFunc("/api/apply", s.withAuth(s.apiApply))
	mux.HandleFunc("/api/status/envoy", s.withAuth(s.apiEnvoyStatus))
	mux.HandleFunc("/api/status/traffic", s.withAuth(s.apiTrafficStatus))

	mux.HandleFunc("/hx/servers", s.withAuth(s.hxServers))
	mux.HandleFunc("/hx/servers/", s.withAuth(s.hxServerByName))
	mux.HandleFunc("/hx/rules/", s.withAuth(s.hxRulePatch))
	mux.HandleFunc("/hx/apply", s.withAuth(s.hxApply))
	return mux
}

func (s *Server) ListenAndServe() error {
	log.Printf("relaygate panel listening on http://%s", s.cfg.Bind)
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
		"Nav":       "overview",
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
	_ = s.tmpl.ExecuteTemplate(w, "servers.html", map[string]any{
		"Title":   "Servers",
		"Nav":     "servers",
		"Servers": res.Servers,
	})
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
	_ = s.tmpl.ExecuteTemplate(w, "rules.html", map[string]any{
		"Title": "Rules",
		"Nav":   "rules",
		"Rules": res.Rules,
	})
}

func (s *Server) handleMonitoringPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	_ = s.tmpl.ExecuteTemplate(w, "monitoring.html", map[string]any{
		"Title":          "Monitoring",
		"Nav":            "monitoring",
		"FullBleed":      true,
		"GrafanaEnabled": s.GrafanaEnabled(),
	})
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
	applyBody := s.lastApply
	if applyBody == "" {
		applyBody = "尚无应用记录"
	}
	_ = s.tmpl.ExecuteTemplate(w, "apply.html", map[string]any{
		"Title":      "Apply",
		"Nav":        "apply",
		"Summary":    summary,
		"Message":    msg,
		"LastApply":  s.lastApply,
		"ApplyBody":  applyBody,
		"ApplyError": false,
	})
}

func (s *Server) apiServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		res, err := s.load()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, res.Servers)
	case http.MethodPost:
		s.apiServerCreate(w, r)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) apiServerByName(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		s.apiServerPut(w, r)
	case http.MethodDelete:
		s.apiServerDelete(w, r)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) apiServerCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		Address         string `json:"address"`
		TCPPort         int    `json:"tcp_port"`
		UDPPort         int    `json:"udp_port"`
		HealthCheckPort int    `json:"health_check_port"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	created, err := res.AddServer(assets.Server{
		Name:            body.Name,
		Address:         body.Address,
		TCPPort:         body.TCPPort,
		UDPPort:         body.UDPPort,
		HealthCheckPort: body.HealthCheckPort,
		Enabled:         enabled,
	})
	if err != nil {
		code := 400
		if strings.Contains(err.Error(), "已存在") {
			code = 409
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	if err := assets.Save(s.resourcesPath(), res); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true, "rules": created})
}

func (s *Server) apiServerPut(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) apiServerDelete(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	removed, err := res.DeleteServer(name)
	if err != nil {
		code := 400
		if strings.Contains(err.Error(), "not found") {
			code = 404
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	if err := assets.Save(s.resourcesPath(), res); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "removed_rules": removed})
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
	resourcesPath, envoyOut, _ := assets.DefaultPaths(s.cfg.Root)
	stamp := time.Now().Format("20060102-150405")
	if _, err := assets.BackupFiles(s.cfg.Root, stamp, resourcesPath, envoyOut); err != nil {
		writeJSON(w, 500, map[string]any{"error": "backup failed: " + err.Error()})
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

func (s *Server) hxServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	tcpPort, _ := strconv.Atoi(r.FormValue("tcp_port"))
	udpPort, _ := strconv.Atoi(r.FormValue("udp_port"))
	healthPort, _ := strconv.Atoi(r.FormValue("health_check_port"))
	enabled := formEnabled(r)
	res, err := s.load()
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	created, err := res.AddServer(assets.Server{
		Name:            strings.TrimSpace(r.FormValue("name")),
		Address:         strings.TrimSpace(r.FormValue("address")),
		TCPPort:         tcpPort,
		UDPPort:         udpPort,
		HealthCheckPort: healthPort,
		Enabled:         enabled,
	})
	if err != nil {
		code := 400
		if strings.Contains(err.Error(), "已存在") {
			code = 409
		}
		hxError(w, code, err.Error())
		return
	}
	if err := assets.Save(s.resourcesPath(), res); err != nil {
		hxError(w, 500, err.Error())
		return
	}
	names := make([]string, 0, len(created))
	for _, rule := range created {
		names = append(names, rule.Name)
	}
	msg := "已添加 " + strings.TrimSpace(r.FormValue("name"))
	if len(names) > 0 {
		msg += "，生成规则: " + strings.Join(names, ", ")
	}
	msg += "（尚未 Apply）"
	s.renderServersTable(w, res, msg, "ok")
}

func (s *Server) hxServerByName(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	switch r.Method {
	case http.MethodPut:
		if err := r.ParseForm(); err != nil {
			hxError(w, 400, err.Error())
			return
		}
		tcpPort, _ := strconv.Atoi(r.FormValue("tcp_port"))
		udpPort, _ := strconv.Atoi(r.FormValue("udp_port"))
		healthPort, _ := strconv.Atoi(r.FormValue("health_check_port"))
		res, err := s.load()
		if err != nil {
			hxError(w, 500, err.Error())
			return
		}
		if err := res.UpdateServer(name, strings.TrimSpace(r.FormValue("address")), tcpPort, udpPort, healthPort, formEnabled(r)); err != nil {
			hxError(w, 404, err.Error())
			return
		}
		if err := assets.Save(s.resourcesPath(), res); err != nil {
			hxError(w, 500, err.Error())
			return
		}
		s.renderServersTable(w, res, "已保存 "+name+"（尚未 Apply）", "ok")
	case http.MethodDelete:
		res, err := s.load()
		if err != nil {
			hxError(w, 500, err.Error())
			return
		}
		removed, err := res.DeleteServer(name)
		if err != nil {
			code := 400
			if strings.Contains(err.Error(), "not found") {
				code = 404
			}
			hxError(w, code, err.Error())
			return
		}
		if err := assets.Save(s.resourcesPath(), res); err != nil {
			hxError(w, 500, err.Error())
			return
		}
		s.renderServersTable(w, res, fmt.Sprintf("已删除 %s（移除规则 %d 条，尚未 Apply）", name, removed), "ok")
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) hxRulePatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	name := filepath.Base(r.URL.Path)
	enabled := formEnabled(r)
	_, err := assets.PatchRuleEnabledInPlace(s.resourcesPath(), name, enabled)
	if err != nil {
		res, loadErr := s.load()
		if loadErr != nil {
			hxError(w, 500, err.Error())
			return
		}
		if setErr := res.SetRuleEnabled(name, enabled); setErr != nil {
			hxError(w, 404, setErr.Error())
			return
		}
		if saveErr := assets.Save(s.resourcesPath(), res); saveErr != nil {
			hxError(w, 500, saveErr.Error())
			return
		}
	}
	res, err := s.load()
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	state := "已禁用 "
	if enabled {
		state = "已启用 "
	}
	triggerToast(w, state+name+"（尚未 Apply）", "ok")
	_ = s.tmpl.ExecuteTemplate(w, "rules-table", map[string]any{"Rules": res.Rules})
}

func (s *Server) hxApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	res, err := s.load()
	if err != nil {
		s.renderApplyResult(w, err.Error(), true, "加载配置失败", "error")
		return
	}
	if err := res.Validate(); err != nil {
		s.renderApplyResult(w, err.Error(), true, "校验失败", "error")
		return
	}
	resourcesPath, envoyOut, _ := assets.DefaultPaths(s.cfg.Root)
	stamp := time.Now().Format("20060102-150405")
	if _, err := assets.BackupFiles(s.cfg.Root, stamp, resourcesPath, envoyOut); err != nil {
		s.renderApplyResult(w, "backup failed: "+err.Error(), true, "备份失败", "error")
		return
	}
	reload := filepath.Join(s.cfg.Root, "scripts", "reload_envoy.sh")
	cmd := exec.Command("bash", reload)
	cmd.Dir = s.cfg.Root
	out, err := cmd.CombinedOutput()
	msg := string(out)
	if err != nil {
		body := time.Now().Format(time.RFC3339) + " FAIL: " + err.Error() + "\n" + msg
		s.lastApply = body
		s.renderApplyResult(w, body, true, "Apply 失败", "error")
		return
	}
	body := time.Now().Format(time.RFC3339) + " OK\n" + envoygen.Summarize(res) + "\n" + msg
	s.lastApply = body
	s.renderApplyResult(w, body, false, "Apply 成功", "ok")
}

func (s *Server) renderServersTable(w http.ResponseWriter, res *assets.Resources, message, kind string) {
	triggerToast(w, message, kind)
	_ = s.tmpl.ExecuteTemplate(w, "servers-table", map[string]any{"Servers": res.Servers})
}

func (s *Server) renderApplyResult(w http.ResponseWriter, body string, isErr bool, toastMsg, toastKind string) {
	triggerToast(w, toastMsg, toastKind)
	_ = s.tmpl.ExecuteTemplate(w, "apply-result", map[string]any{
		"ApplyBody":  body,
		"ApplyError": isErr,
	})
}

func formEnabled(r *http.Request) bool {
	v := strings.ToLower(strings.TrimSpace(r.FormValue("enabled")))
	return v == "on" || v == "true" || v == "1" || v == "yes"
}

func triggerToast(w http.ResponseWriter, message, kind string) {
	payload, err := json.Marshal(map[string]any{
		"show-toast": map[string]string{"message": message, "kind": kind},
	})
	if err != nil {
		return
	}
	w.Header().Set("HX-Trigger", string(payload))
}

func hxError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, message)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
