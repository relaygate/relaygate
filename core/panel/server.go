package panel

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/relaygate/relaygate/core/ops"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/resources"
	"github.com/relaygate/relaygate/core/status"
)

const (
	sessionCookie = "panel_session"
	csrfCookie    = "panel_csrf"
	csrfHeader    = "X-CSRF-Token"
	csrfFormField = "csrf_token"
)

type Config struct {
	Root          string
	Bind          string
	AdminPassword string
	EnvoyAdminURL string
	PrometheusURL string
	GrafanaURL    string
	UIDir         string // SPA dist directory (default {Root}/ui/dist)
	SessionTTL    time.Duration
}

type sessionInfo struct {
	expires time.Time
	csrf    string
}

type loginAttempt struct {
	count int
	until time.Time
}

type Server struct {
	cfg          Config
	i18n         *Bundle
	status       *status.Client
	mu           sync.Mutex
	sessions     map[string]sessionInfo
	loginFails   map[string]loginAttempt
	lastApply    string
	grafanaProxy http.Handler
}

func New(cfg Config) (*Server, error) {
	if cfg.Root == "" {
		root, err := resources.FindRoot()
		if err != nil {
			return nil, err
		}
		cfg.Root = root
	}
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1:9000"
	}
	if err := validatePanelBind(cfg.Bind); err != nil {
		return nil, err
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.UIDir == "" {
		cfg.UIDir = filepath.Join(cfg.Root, "ui", "dist")
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

	bundle, err := loadEmbeddedBundle()
	if err != nil {
		return nil, err
	}
	srv := &Server{
		cfg:        cfg,
		i18n:       bundle,
		status:     status.New(cfg.EnvoyAdminURL, cfg.PrometheusURL),
		sessions:   map[string]sessionInfo{},
		loginFails: map[string]loginAttempt{},
	}
	if cfg.GrafanaURL != "" {
		proxy, err := newGrafanaProxy(cfg.GrafanaURL)
		if err != nil {
			return nil, err
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, srv.t(r, "error.grafana_unavailable", err.Error()), http.StatusBadGateway)
		}
		srv.grafanaProxy = proxy
	}
	return srv, nil
}

func newGrafanaProxy(rawURL string) (*httputil.ReverseProxy, error) {
	target, err := parseGrafanaURL(rawURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		fwdHost := req.Host
		director(req)
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

func validatePanelBind(bind string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return fmt.Errorf("PANEL_BIND 无效 %q: %w（期望 127.0.0.1:9000）", bind, err)
	}
	if port == "" {
		return fmt.Errorf("PANEL_BIND 缺少端口: %q", bind)
	}
	if host == "" {
		return fmt.Errorf("PANEL_BIND 不能省略主机（禁止 :%s 监听全接口）", port)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("PANEL_BIND 必须是 loopback（127.0.0.1 / ::1 / localhost），当前: %s", bind)
	}
	return nil
}

func passwordMatch(got, want string) bool {
	sumGot := sha256.Sum256([]byte(got))
	sumWant := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(sumGot[:], sumWant[:]) == 1
}

func (s *Server) GrafanaEnabled() bool { return s.grafanaProxy != nil }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Auth / session (JSON)
	mux.HandleFunc("/api/login", s.apiLogin)
	mux.HandleFunc("/api/logout", s.withAuthReadonly(s.apiLogout))
	mux.HandleFunc("/api/session", s.apiSession)
	mux.HandleFunc("/api/lang", s.apiLang)

	// Resource APIs
	mux.HandleFunc("/api/servers", s.withAuth(s.apiServers))
	mux.HandleFunc("/api/servers/", s.withAuth(s.apiServersPath))
	mux.HandleFunc("/api/rules", s.withAuth(s.apiRules))
	mux.HandleFunc("/api/rules/", s.withAuth(s.apiRulePatch))
	mux.HandleFunc("/api/acl", s.withAuth(s.apiACL))
	mux.HandleFunc("/api/apply", s.withAuth(s.apiApply))
	mux.HandleFunc("/api/apply/preview", s.withAuthReadonly(s.apiApplyPreview))
	mux.HandleFunc("/api/firewall/apply", s.withAuth(s.apiFirewallApply))
	mux.HandleFunc("/api/status/envoy", s.withAuthReadonly(s.apiEnvoyStatus))
	mux.HandleFunc("/api/status/traffic", s.withAuthReadonly(s.apiTrafficStatus))

	// Ops / changes
	mux.HandleFunc("/api/profiles", s.withAuthReadonly(s.apiProfiles))
	mux.HandleFunc("/api/ops/doctor", s.withAuthReadonly(s.apiDoctor))
	mux.HandleFunc("/api/ops/drain", s.withAuthReadonly(s.apiDrain))
	mux.HandleFunc("/api/ops/smoke", s.withAuthReadonly(s.apiSmoke))
	mux.HandleFunc("/api/ops/canary", s.withAuthReadonly(s.apiCanary))
	mux.HandleFunc("/api/ops/firewall-check", s.withAuthReadonly(s.apiFirewallCheck))
	mux.HandleFunc("/api/ops/profile-preview", s.withAuthReadonly(s.apiProfilePreview))
	mux.HandleFunc("/api/ops/profile-apply", s.withAuth(s.apiProfileApply))
	mux.HandleFunc("/api/changes", s.withAuthReadonly(s.apiChanges))
	mux.HandleFunc("/api/changes/", s.withAuthReadonly(s.apiChangeByStamp))
	mux.HandleFunc("/api/rollback/preview", s.withAuthReadonly(s.apiRollbackPreview))
	mux.HandleFunc("/api/rollback", s.withAuth(s.apiRollback))

	// Config YAML (resources.yaml mirror + export; validate/put for P1)
	mux.HandleFunc("/api/config/resources", s.withAuthOpts(s.apiConfigResources, true))
	mux.HandleFunc("/api/config/resources/validate", s.withAuthReadonly(s.apiConfigResourcesValidate))
	mux.HandleFunc("/api/config/export", s.withAuthReadonly(s.apiConfigExport))

	// Legacy lang redirect (optional bookmark)
	mux.HandleFunc("/lang", s.handleLangRedirect)

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

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/favicon.svg", http.StatusFound)
	})

	// SPA static + index.html fallback for client routes
	mux.Handle("/", s.spaHandler())

	return mux
}

func (s *Server) ListenAndServe() error {
	log.Printf("relaygate panel listening on http://%s (ui=%s)", s.cfg.Bind, s.cfg.UIDir)
	return http.ListenAndServe(s.cfg.Bind, s.Handler())
}

func (s *Server) resourcesPath() string {
	p, _, _ := resources.DefaultPaths(s.cfg.Root)
	return p
}

func (s *Server) load() (*resources.Resources, error) {
	return resources.Load(s.resourcesPath())
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return s.withAuthOpts(next, true)
}

func (s *Server) withAuthReadonly(next http.HandlerFunc) http.HandlerFunc {
	return s.withAuthOpts(next, false)
}

func (s *Server) withAuthOpts(next http.HandlerFunc, refuseStandbyWrite bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if isMutatingMethod(r.Method) {
			if !s.validCSRF(r) {
				http.Error(w, "csrf token missing or invalid", http.StatusForbidden)
				return
			}
			if refuseStandbyWrite && isStandbyRole() {
				s.refuseStandbyWrite(w, r)
				return
			}
		}
		next(w, r)
	}
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func panelRole() string {
	role := strings.ToLower(strings.TrimSpace(os.Getenv("PANEL_ROLE")))
	if role == "" {
		return "primary"
	}
	return role
}

func isStandbyRole() bool {
	return panelRole() == "standby"
}

func (s *Server) refuseStandbyWrite(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusForbidden, map[string]any{"error": s.t(r, "error.standby")})
}

func (s *Server) authed(r *http.Request) bool {
	_, ok := s.lookupSession(r)
	return ok
}

func (s *Server) lookupSession(r *http.Request) (sessionInfo, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return sessionInfo{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	if !ok || time.Now().After(sess.expires) {
		delete(s.sessions, c.Value)
		return sessionInfo{}, false
	}
	return sess, true
}

func (s *Server) sessionCSRF(r *http.Request) string {
	sess, ok := s.lookupSession(r)
	if !ok {
		return ""
	}
	return sess.csrf
}

func (s *Server) requestCSRFToken(r *http.Request) string {
	if t := r.Header.Get(csrfHeader); t != "" {
		return t
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") || strings.HasPrefix(ct, "multipart/form-data") {
		_ = r.ParseForm()
		return r.FormValue(csrfFormField)
	}
	return ""
}

func (s *Server) validCSRF(r *http.Request) bool {
	sess, ok := s.lookupSession(r)
	if !ok || sess.csrf == "" {
		return false
	}
	got := s.requestCSRFToken(r)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(sess.csrf)) == 1
}

func (s *Server) createSession() (sessionToken, csrfToken string, err error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("session token: %w", err)
	}
	sessionToken = hex.EncodeToString(b)
	b2 := make([]byte, 16)
	if _, err := rand.Read(b2); err != nil {
		return "", "", fmt.Errorf("csrf token: %w", err)
	}
	csrfToken = hex.EncodeToString(b2)
	s.mu.Lock()
	s.sessions[sessionToken] = sessionInfo{
		expires: time.Now().Add(s.cfg.SessionTTL),
		csrf:    csrfToken,
	}
	s.mu.Unlock()
	return sessionToken, csrfToken, nil
}

func (s *Server) loginAllowed(ip string) (ok bool, retryAfter time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, exists := s.loginFails[ip]
	if !exists {
		return true, 0
	}
	now := time.Now()
	if now.Before(a.until) {
		return false, a.until.Sub(now)
	}
	if a.count >= 5 && !now.Before(a.until) {
		delete(s.loginFails, ip)
	}
	return true, 0
}

func (s *Server) recordLoginFailure(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.loginFails[ip]
	a.count++
	if a.count >= 5 {
		a.until = time.Now().Add(60 * time.Second)
	}
	s.loginFails[ip] = a
}

func (s *Server) clearLoginFailures(ip string) {
	s.mu.Lock()
	delete(s.loginFails, ip)
	s.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) setSessionCookies(w http.ResponseWriter, sessionToken, csrfToken string) {
	maxAge := int(s.cfg.SessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func (s *Server) clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: "", Path: "/", MaxAge: -1})
}

func (s *Server) handleLangRedirect(w http.ResponseWriter, r *http.Request) {
	lang := normalizeLang(r.URL.Query().Get("set"))
	if lang == "" {
		http.Redirect(w, r, safeNextPath(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	setLangCookie(w, lang)
	http.Redirect(w, r, safeNextPath(r.URL.Query().Get("next")), http.StatusFound)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func lifecycleByName(res *resources.Resources) map[string]resources.ServerLifecycle {
	out := make(map[string]resources.ServerLifecycle, len(res.Servers))
	for _, lc := range res.LifecycleStatus() {
		out[lc.Name] = lc
	}
	return out
}

// --- Resource JSON APIs ---

func (s *Server) apiServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		res, err := s.load()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{
			"servers":   res.Servers,
			"lifecycle": lifecycleByName(res),
		})
	case http.MethodPost:
		s.apiServerCreate(w, r)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) apiServersPath(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/servers/"), "/")
	parts := strings.Split(rel, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	if len(parts) == 2 && parts[1] == "promote" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		s.apiServerPromote(w, r, name)
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.apiServerPut(w, r, name)
	case http.MethodDelete:
		s.apiServerDelete(w, r, name)
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
	created, err := res.AddServer(resources.Server{
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
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.appendAudit("server.create", body.Name)
	writeJSON(w, 201, map[string]any{"ok": true, "rules": created})
}

func (s *Server) apiServerPut(w http.ResponseWriter, r *http.Request, name string) {
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
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.appendAudit("server.update", name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) apiServerDelete(w http.ResponseWriter, r *http.Request, name string) {
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	removed, err := res.DeleteServer(name)
	if err != nil {
		writeJSON(w, 404, map[string]any{"error": err.Error()})
		return
	}
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.appendAudit("server.delete", name)
	writeJSON(w, 200, map[string]any{"ok": true, "removed_rules": removed})
}

func (s *Server) apiServerPromote(w http.ResponseWriter, r *http.Request, name string) {
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	changed, err := res.EnableProductionForServer(name, true)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.appendAudit("server.promote", fmt.Sprintf("%s changed=%d", name, changed))
	writeJSON(w, 200, map[string]any{"ok": true, "changed": changed})
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

func (s *Server) apiACL(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		res, err := s.load()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		_ = res.ACL.NormalizeACL()
		writeJSON(w, 200, res.ACL)
	case http.MethodPost:
		var body struct {
			List string `json:"list"`
			CIDR string `json:"cidr"`
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
		canonical, err := res.AddACLEntry(body.List, body.CIDR)
		if err != nil {
			code := 400
			if strings.Contains(err.Error(), "已存在") {
				code = 409
			}
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		if err := resources.Save(s.resourcesPath(), res); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 201, map[string]any{"ok": true, "cidr": canonical, "acl": res.ACL})
	case http.MethodDelete:
		var body struct {
			List string `json:"list"`
			CIDR string `json:"cidr"`
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
		canonical, err := res.RemoveACLEntry(body.List, body.CIDR)
		if err != nil {
			writeJSON(w, 404, map[string]any{"error": err.Error()})
			return
		}
		if err := resources.Save(s.resourcesPath(), res); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "cidr": canonical, "acl": res.ACL})
	default:
		http.Error(w, "method not allowed", 405)
	}
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
	changed, err := resources.PatchRuleEnabledInPlace(s.resourcesPath(), name, body.Enabled)
	if err != nil {
		res, loadErr := s.load()
		if loadErr != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		if setErr := res.SetRuleEnabled(name, body.Enabled); setErr != nil {
			writeJSON(w, 404, map[string]any{"error": setErr.Error()})
			return
		}
		if saveErr := resources.Save(s.resourcesPath(), res); saveErr != nil {
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
	msg, err := ops.ReloadCapture(s.cfg.Root)
	if err != nil {
		s.lastApply = time.Now().Format(time.RFC3339) + " FAIL: " + err.Error() + "\n" + msg
		writeJSON(w, 500, map[string]any{"error": err.Error(), "output": msg, "ok": false})
		return
	}
	s.lastApply = time.Now().Format(time.RFC3339) + " OK\n" + msg
	s.appendAudit("apply", "ok")
	writeJSON(w, 200, map[string]any{"ok": true, "output": msg, "summary": render.Summarize(res)})
}

func (s *Server) apiApplyPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	res, err := s.load()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	var b strings.Builder
	before, prevStamp, _ := resources.LoadPreviousBackupResources(s.cfg.Root)
	diff := resources.Diff(before, res)
	if prevStamp != "" && before != nil {
		diff.Note = s.t(r, "apply.diff_note", prevStamp)
	}
	plan := diff.Classify()
	b.WriteString(diff.String())
	b.WriteString(render.Summarize(res))
	last := s.lastApply
	if last == "" {
		last = s.t(r, "apply.none")
	}
	writeJSON(w, 200, map[string]any{
		"summary":        b.String(),
		"last_apply":     last,
		"needs_reload":   plan.NeedsReload,
		"needs_firewall": plan.NeedsFirewall,
	})
}

func (s *Server) apiFirewallApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	const phrase = "YES_FLUSH_NFTABLES"
	if strings.TrimSpace(body.Confirm) != phrase {
		writeJSON(w, 400, map[string]any{"error": s.t(r, "error.confirm_typed", phrase)})
		return
	}
	out, err := ops.FirewallApplyCapture(s.cfg.Root)
	if err != nil {
		if strings.Contains(err.Error(), "需要 root") {
			out = strings.TrimSpace(out) + "\n" + s.t(r, "ops.fw_root_hint")
		}
		s.lastApply = time.Now().Format(time.RFC3339) + " FIREWALL FAIL: " + err.Error() + "\n" + out
		writeOpsResult(w, out, err)
		return
	}
	s.lastApply = time.Now().Format(time.RFC3339) + " FIREWALL OK\n" + out
	s.appendAudit("firewall.apply", "ok")
	writeOpsResult(w, out, nil)
}

func (s *Server) apiEnvoyStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.status.Envoy())
}

func (s *Server) apiTrafficStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.status.Traffic())
}
