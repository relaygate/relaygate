package panel

import (
	"crypto/rand"
	"crypto/sha256"
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
	"path/filepath"
	"strconv"
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
	FrontendDir   string
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
	tmpl         *template.Template
	i18n         *Bundle
	status       *status.Client
	mu           sync.Mutex
	sessions     map[string]sessionInfo
	loginFails   map[string]loginAttempt
	lastApply    string
	grafanaProxy http.Handler
	renderMu     sync.Mutex
	renderLang   string
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
	if cfg.FrontendDir == "" {
		cfg.FrontendDir = filepath.Join(cfg.Root, "frontend")
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

	bundle, err := loadBundle(cfg.FrontendDir)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		cfg:        cfg,
		i18n:       bundle,
		status:     status.New(cfg.EnvoyAdminURL, cfg.PrometheusURL),
		sessions:   map[string]sessionInfo{},
		loginFails: map[string]loginAttempt{},
		renderLang: langDefault,
	}
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"urlquery": url.QueryEscape,
		"T": func(key string, args ...any) string {
			return srv.i18n.T(srv.renderLang, key, args...)
		},
	}).ParseGlob(filepath.Join(cfg.FrontendDir, "templates", "*.html"))
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	srv.tmpl = tmpl
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

// newGrafanaProxy builds a fixed-target reverse proxy for Grafana.
// Path is not stripped (serve_from_sub_path); WebSocket upgrades are handled by httputil.ReverseProxy.
func newGrafanaProxy(rawURL string) (*httputil.ReverseProxy, error) {
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

// validatePanelBind requires loopback only — Panel must not be exposed on 0.0.0.0/public.
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
	// Hash first so ConstantTimeCompare is length-independent.
	sumGot := sha256.Sum256([]byte(got))
	sumWant := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(sumGot[:], sumWant[:]) == 1
}

// GrafanaEnabled reports whether the in-panel Grafana proxy is configured.
func (s *Server) GrafanaEnabled() bool { return s.grafanaProxy != nil }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.cfg.FrontendDir, "static")))))
	faviconSVG := filepath.Join(s.cfg.FrontendDir, "static", "favicon.svg")
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, faviconSVG)
	})
	// Browsers still probe /favicon.ico; redirect to SVG.
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/favicon.svg", http.StatusFound)
	})
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/lang", s.handleLang)
	mux.HandleFunc("/", s.handleOverview)
	mux.HandleFunc("/servers", s.handleServersPage)
	mux.HandleFunc("/rules", s.handleRulesPage)
	mux.HandleFunc("/acl", s.handleACLPage)
	mux.HandleFunc("/apply", s.handleApplyPage)
	mux.HandleFunc("/ops", s.handleOpsPage)
	mux.HandleFunc("/changes", s.handleChangesPage)
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
	mux.HandleFunc("/api/acl", s.withAuth(s.apiACL))
	mux.HandleFunc("/api/apply", s.withAuth(s.apiApply))
	mux.HandleFunc("/api/status/envoy", s.withAuth(s.apiEnvoyStatus))
	mux.HandleFunc("/api/status/traffic", s.withAuth(s.apiTrafficStatus))

	mux.HandleFunc("/hx/servers", s.withAuth(s.hxServers))
	mux.HandleFunc("/hx/servers/", s.withAuth(s.hxServerByName))
	mux.HandleFunc("/hx/rules/", s.withAuth(s.hxRulePatch))
	mux.HandleFunc("/hx/acl", s.withAuth(s.hxACLAdd))
	mux.HandleFunc("/hx/acl/remove", s.withAuth(s.hxACLRemove))
	mux.HandleFunc("/hx/apply", s.withAuth(s.hxApply))
	mux.HandleFunc("/hx/ops/doctor", s.withAuthReadonly(s.hxDoctor))
	mux.HandleFunc("/hx/ops/drain", s.withAuthReadonly(s.hxDrain))
	mux.HandleFunc("/hx/ops/smoke", s.withAuthReadonly(s.hxSmoke))
	mux.HandleFunc("/hx/ops/canary", s.withAuthReadonly(s.hxCanary))
	mux.HandleFunc("/hx/ops/firewall-check", s.withAuthReadonly(s.hxFirewallCheck))
	mux.HandleFunc("/hx/ops/profile-preview", s.withAuthReadonly(s.hxProfilePreview))
	mux.HandleFunc("/hx/ops/profile-apply", s.withAuth(s.hxProfileApply))
	mux.HandleFunc("/hx/changes/", s.withAuthReadonly(s.hxChangeSummary))
	mux.HandleFunc("/hx/rollback/preview", s.withAuthReadonly(s.hxRollbackPreview))
	mux.HandleFunc("/hx/rollback", s.withAuth(s.hxRollback))
	return mux
}

func (s *Server) ListenAndServe() error {
	log.Printf("relaygate panel listening on http://%s", s.cfg.Bind)
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

// withAuthReadonly allows POST on standby (doctor / smoke / status 等只读运维)。
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
	msg := s.t(r, "error.standby")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": msg})
		return
	}
	hxError(w, http.StatusForbidden, msg)
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

func (s *Server) withPageData(r *http.Request, data map[string]any) map[string]any {
	if data == nil {
		data = map[string]any{}
	}
	lang := resolveLang(r)
	data["CSRFToken"] = s.sessionCSRF(r)
	data["Lang"] = lang
	if _, ok := data["LangSwitchNext"]; !ok {
		next := "/"
		if r != nil && r.URL != nil {
			if uri := r.URL.RequestURI(); uri != "" && uri != "/lang" {
				next = safeNextPath(uri)
			}
		}
		data["LangSwitchNext"] = next
	}
	return data
}

func (s *Server) executeTemplate(w http.ResponseWriter, r *http.Request, name string, data map[string]any) error {
	data = s.withPageData(r, data)
	s.renderMu.Lock()
	defer s.renderMu.Unlock()
	prev := s.renderLang
	s.renderLang = resolveLang(r)
	err := s.tmpl.ExecuteTemplate(w, name, data)
	s.renderLang = prev
	return err
}

func (s *Server) handleLang(w http.ResponseWriter, r *http.Request) {
	lang := normalizeLang(r.URL.Query().Get("set"))
	if lang == "" {
		http.Redirect(w, r, safeNextPath(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	setLangCookie(w, lang)
	http.Redirect(w, r, safeNextPath(r.URL.Query().Get("next")), http.StatusFound)
}

func (s *Server) requirePageAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authed(r) {
		return true
	}
	http.Redirect(w, r, "/login", http.StatusFound)
	return false
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
		// lockout expired — reset
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
		HttpOnly: false, // htmx / alpine may read via meta; cookie aids double-submit clients
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func (s *Server) clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: "", Path: "/", MaxAge: -1})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_ = s.executeTemplate(w, r, "login.html", map[string]any{
			"Error":          "",
			"LangSwitchNext": "/login",
		})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ip := clientIP(r)
	if ok, wait := s.loginAllowed(ip); !ok {
		_ = s.executeTemplate(w, r, "login.html", map[string]any{
			"Error":          s.t(r, "login.error_rate_limit", wait.Seconds()),
			"LangSwitchNext": "/login",
		})
		return
	}
	pass := r.FormValue("password")
	if !passwordMatch(pass, s.cfg.AdminPassword) {
		s.recordLoginFailure(ip)
		_ = s.executeTemplate(w, r, "login.html", map[string]any{
			"Error":          s.t(r, "login.error_password"),
			"LangSwitchNext": "/login",
		})
		return
	}
	token, csrf, err := s.createSession()
	if err != nil {
		http.Error(w, s.t(r, "login.error_session"), http.StatusInternalServerError)
		return
	}
	s.clearLoginFailures(ip)
	s.setSessionCookies(w, token, csrf)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	s.clearSessionCookies(w)
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
	_ = s.executeTemplate(w, r, "overview.html", map[string]any{
		"Title":     s.t(r, "overview.title"),
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
	_ = s.executeTemplate(w, r, "servers.html", map[string]any{
		"Title":     s.t(r, "servers.title"),
		"Nav":       "servers",
		"Servers":   res.Servers,
		"Lifecycle": lifecycleByName(res),
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
	_ = s.executeTemplate(w, r, "rules.html", map[string]any{
		"Title": s.t(r, "rules.title"),
		"Nav":   "rules",
		"Rules": res.Rules,
	})
}

func (s *Server) handleACLPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	res, err := s.load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = res.ACL.NormalizeACL()
	_ = s.executeTemplate(w, r, "acl.html", map[string]any{
		"Title": s.t(r, "acl.title"),
		"Nav":   "acl",
		"Deny":  res.ACL.Deny,
		"Allow": res.ACL.Allow,
	})
}

func (s *Server) handleMonitoringPage(w http.ResponseWriter, r *http.Request) {
	if !s.requirePageAuth(w, r) {
		return
	}
	_ = s.executeTemplate(w, r, "monitoring.html", map[string]any{
		"Title":          s.t(r, "monitoring.title"),
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
		var b strings.Builder
		before, prevStamp, _ := resources.LoadPreviousBackupResources(s.cfg.Root)
		diff := resources.Diff(before, res)
		if prevStamp != "" && before != nil {
			diff.Note = s.t(r, "apply.diff_note", prevStamp)
		}
		b.WriteString(diff.String())
		b.WriteString(render.Summarize(res))
		summary = b.String()
	} else {
		msg = err.Error()
	}
	applyBody := s.lastApply
	if applyBody == "" {
		applyBody = s.t(r, "apply.none")
	}
	_ = s.executeTemplate(w, r, "apply.html", map[string]any{
		"Title":      s.t(r, "apply.title"),
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
	if err := resources.Save(s.resourcesPath(), res); err != nil {
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
	if err := resources.Save(s.resourcesPath(), res); err != nil {
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
	// ReloadTo 内含 backup + change-summary + 分阶段 drain/restart
	msg, err := ops.ReloadCapture(s.cfg.Root)
	if err != nil {
		s.lastApply = time.Now().Format(time.RFC3339) + " FAIL: " + err.Error() + "\n" + msg
		writeJSON(w, 500, map[string]any{"error": err.Error(), "output": msg})
		return
	}
	s.lastApply = time.Now().Format(time.RFC3339) + " OK\n" + msg
	s.appendAudit("apply", "ok")
	writeJSON(w, 200, map[string]any{"ok": true, "output": msg, "summary": render.Summarize(res)})
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
	created, err := res.AddServer(resources.Server{
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
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		hxError(w, 500, err.Error())
		return
	}
	names := make([]string, 0, len(created))
	for _, rule := range created {
		names = append(names, rule.Name)
	}
	serverName := strings.TrimSpace(r.FormValue("name"))
	var msg string
	if len(names) > 0 {
		msg = s.t(r, "servers.toast_added_rules", serverName, strings.Join(names, ", "))
	} else {
		msg = s.t(r, "servers.toast_added", serverName)
	}
	s.renderServersTable(w, r, res, msg, "ok")
}

func (s *Server) hxServerByName(w http.ResponseWriter, r *http.Request) {
	rel := strings.Trim(strings.TrimPrefix(r.URL.Path, "/hx/servers/"), "/")
	parts := strings.Split(rel, "/")
	if len(parts) == 2 && parts[1] == "promote" {
		s.hxPromoteServer(w, r)
		return
	}
	name := parts[0]
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
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
		if err := resources.Save(s.resourcesPath(), res); err != nil {
			hxError(w, 500, err.Error())
			return
		}
		s.renderServersTable(w, r, res, s.t(r, "servers.toast_saved", name), "ok")
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
		if err := resources.Save(s.resourcesPath(), res); err != nil {
			hxError(w, 500, err.Error())
			return
		}
		s.renderServersTable(w, r, res, s.t(r, "servers.toast_deleted", name, removed), "ok")
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
	_, err := resources.PatchRuleEnabledInPlace(s.resourcesPath(), name, enabled)
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
		if saveErr := resources.Save(s.resourcesPath(), res); saveErr != nil {
			hxError(w, 500, saveErr.Error())
			return
		}
	}
	res, err := s.load()
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	key := "rules.toast_disabled"
	if enabled {
		key = "rules.toast_enabled"
	}
	triggerToast(w, s.t(r, key, name), "ok")
	_ = s.executeTemplate(w, r, "rules-table", map[string]any{"Rules": res.Rules})
}

func (s *Server) hxACLAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	res, err := s.load()
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	canonical, err := res.AddACLEntry(r.FormValue("list"), r.FormValue("cidr"))
	if err != nil {
		code := 400
		if strings.Contains(err.Error(), "已存在") {
			code = 409
		}
		hxError(w, code, err.Error())
		return
	}
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		hxError(w, 500, err.Error())
		return
	}
	s.renderACLTable(w, r, res, s.t(r, "acl.toast_added", canonical), "ok")
}

func (s *Server) hxACLRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := r.ParseForm(); err != nil {
		hxError(w, 400, err.Error())
		return
	}
	res, err := s.load()
	if err != nil {
		hxError(w, 500, err.Error())
		return
	}
	canonical, err := res.RemoveACLEntry(r.FormValue("list"), r.FormValue("cidr"))
	if err != nil {
		hxError(w, 404, err.Error())
		return
	}
	if err := resources.Save(s.resourcesPath(), res); err != nil {
		hxError(w, 500, err.Error())
		return
	}
	s.renderACLTable(w, r, res, s.t(r, "acl.toast_removed", canonical), "ok")
}

func (s *Server) renderACLTable(w http.ResponseWriter, r *http.Request, res *resources.Resources, message, kind string) {
	_ = res.ACL.NormalizeACL()
	triggerToast(w, message, kind)
	_ = s.executeTemplate(w, r, "acl-table", map[string]any{
		"Deny":  res.ACL.Deny,
		"Allow": res.ACL.Allow,
	})
}

func (s *Server) hxApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	res, err := s.load()
	if err != nil {
		s.renderApplyResult(w, r, err.Error(), true, s.t(r, "apply.toast_load_fail"), "error")
		return
	}
	if err := res.Validate(); err != nil {
		s.renderApplyResult(w, r, err.Error(), true, s.t(r, "apply.toast_validate_fail"), "error")
		return
	}
	msg, err := ops.ReloadCapture(s.cfg.Root)
	if err != nil {
		body := time.Now().Format(time.RFC3339) + " FAIL: " + err.Error() + "\n" + msg
		s.lastApply = body
		s.renderApplyResult(w, r, body, true, s.t(r, "apply.toast_fail"), "error")
		return
	}
	body := time.Now().Format(time.RFC3339) + " OK\n" + msg
	s.lastApply = body
	s.appendAudit("apply", "ok")
	s.renderApplyResult(w, r, body, false, s.t(r, "apply.toast_ok"), "ok")
}

func (s *Server) renderServersTable(w http.ResponseWriter, r *http.Request, res *resources.Resources, message, kind string) {
	triggerToast(w, message, kind)
	_ = s.executeTemplate(w, r, "servers-table", map[string]any{
		"Servers":   res.Servers,
		"Lifecycle": lifecycleByName(res),
	})
}

func lifecycleByName(res *resources.Resources) map[string]resources.ServerLifecycle {
	out := make(map[string]resources.ServerLifecycle, len(res.Servers))
	for _, lc := range res.LifecycleStatus() {
		out[lc.Name] = lc
	}
	return out
}

func (s *Server) renderApplyResult(w http.ResponseWriter, r *http.Request, body string, isErr bool, toastMsg, toastKind string) {
	triggerToast(w, toastMsg, toastKind)
	_ = s.executeTemplate(w, r, "apply-result", map[string]any{
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
