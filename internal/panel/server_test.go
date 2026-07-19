package panel

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robot/proxy/internal/assets"
)

func setupPanel(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	webDir := filepath.Join(root, "web")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(webDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(webDir, "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := &assets.Resources{
		Servers: []assets.Server{
			{Name: "server-01", Address: "10.0.0.11", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
			{Name: "server-02", Address: "10.0.0.12", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
		Rules: []assets.Rule{
			{Name: "rule-canary-server-01-tcp", Kind: "canary", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "rule-server-01-tcp-game", Kind: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
			{Name: "rule-server-01-udp-game", Kind: "production", Server: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: false},
			{Name: "rule-server-02-tcp-game", Kind: "production", Server: "server-02", Protocol: "TCP", ListenPort: 10002, Enabled: false},
			{Name: "rule-server-02-udp-game", Kind: "production", Server: "server-02", Protocol: "UDP", ListenPort: 10002, Enabled: false},
		},
	}
	if err := assets.Save(filepath.Join(cfgDir, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}
	// Minimal templates so New() can parse.
	for _, name := range []string{"login.html", "overview.html", "servers.html", "rules.html", "apply.html", "monitoring.html"} {
		path := filepath.Join(webDir, "templates", name)
		if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// layout nav referenced by pages is optional for API tests.
	srv, err := New(Config{
		Root:          root,
		WebDir:        webDir,
		AdminPassword: "test-pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	token := srv.createSession()
	return srv, token
}

func setupPanelWithGrafana(t *testing.T) (*Server, string) {
	t.Helper()
	upstream := http.NewServeMux()
	upstream.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Path", r.URL.Path)
		w.Header().Set("X-Upstream-Host", r.Host)
		_, _ = io.WriteString(w, "grafana:"+r.URL.Path)
	})
	ts := httptest.NewServer(upstream)
	t.Cleanup(ts.Close)

	root := t.TempDir()
	webDir := filepath.Join(root, "web")
	if err := os.MkdirAll(filepath.Join(webDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(webDir, "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"login.html", "overview.html", "servers.html", "rules.html", "apply.html", "monitoring.html"} {
		if err := os.WriteFile(filepath.Join(webDir, "templates", name), []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(webDir, "static", "favicon.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{
		Root:          root,
		WebDir:        webDir,
		AdminPassword: "test-pass",
		GrafanaURL:    ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, srv.createSession()
}

func authed(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: "panel_session", Value: token})
}

func TestNewReadsAdminPasswordFile(t *testing.T) {
	root := t.TempDir()
	webDir := filepath.Join(root, "web")
	templatesDir := filepath.Join(webDir, "templates")
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "login.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	passwordFile := filepath.Join(root, "panel-password")
	if err := os.WriteFile(passwordFile, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PANEL_ADMIN_PASSWORD", "")
	t.Setenv("PANEL_ADMIN_PASSWORD_FILE", passwordFile)

	srv, err := New(Config{Root: root, WebDir: webDir})
	if err != nil {
		t.Fatal(err)
	}
	if srv.cfg.AdminPassword != "file-secret" {
		t.Fatalf("password=%q", srv.cfg.AdminPassword)
	}
}

func TestAPIServerCreateAndDelete(t *testing.T) {
	srv, token := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{
		"name": "server-11", "address": "10.0.0.21",
		"tcp_port": 7777, "udp_port": 7778, "health_check_port": 7777, "enabled": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewReader(body))
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rules, _ := created["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules in response, got %#v", created["rules"])
	}

	res, err := assets.Load(filepath.Join(srv.cfg.Root, "config", "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Servers) != 3 {
		t.Fatalf("servers=%d", len(res.Servers))
	}

	// duplicate
	req = httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewReader(body))
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 409 {
		t.Fatalf("duplicate status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/servers/server-11", nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/servers/server-01", nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("delete server-01 status=%d body=%s", rec.Code, rec.Body.String())
	}
	var deleted map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &deleted)
	if int(deleted["removed_rules"].(float64)) < 2 {
		t.Fatalf("expected cascade remove, got %#v", deleted)
	}

	res, err = assets.Load(filepath.Join(srv.cfg.Root, "config", "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range res.Rules {
		if rule.Server == "server-01" {
			t.Fatalf("orphan rule: %+v", rule)
		}
	}
}

func TestParseGrafanaURL(t *testing.T) {
	ok, err := parseGrafanaURL("http://127.0.0.1:3000")
	if err != nil {
		t.Fatal(err)
	}
	if ok.String() != "http://127.0.0.1:3000" {
		t.Fatalf("got %s", ok)
	}
	if _, err := parseGrafanaURL("http://localhost:3000/ignored"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseGrafanaURL("ftp://127.0.0.1:3000"); err == nil {
		t.Fatal("expected scheme error")
	}
	if _, err := parseGrafanaURL("http://example.com:3000"); err == nil {
		t.Fatal("expected loopback error")
	}
	if _, err := parseGrafanaURL("not a url"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestGrafanaProxyRequiresAuth(t *testing.T) {
	srv, token := setupPanelWithGrafana(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/grafana/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("unauth status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("location=%q", loc)
	}

	req = httptest.NewRequest(http.MethodGet, "/grafana", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("unauth /grafana status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/grafana/d/foo", nil)
	authed(req, token)
	req.Host = "localhost:9000"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("auth status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Upstream-Path"); got != "/grafana/d/foo" {
		t.Fatalf("upstream path=%q", got)
	}
	if got := rec.Header().Get("X-Upstream-Host"); got != "localhost:9000" {
		t.Fatalf("upstream host=%q (want preserved Panel host)", got)
	}
	if !strings.Contains(rec.Body.String(), "grafana:/grafana/d/foo") {
		t.Fatalf("body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/grafana", nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/grafana/" {
		t.Fatalf("auth /grafana redirect status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestFaviconRedirect(t *testing.T) {
	srv, _ := setupPanelWithGrafana(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/favicon.svg" {
		t.Fatalf("status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("svg status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<svg") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestGrafanaDisabledWithoutURL(t *testing.T) {
	srv, _ := setupPanel(t)
	if srv.GrafanaEnabled() {
		t.Fatal("expected grafana disabled")
	}
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/grafana/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestParseRepoTemplates(t *testing.T) {
	root, err := assets.FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	webDir := filepath.Join(root, "web")
	tmpl, err := template.ParseGlob(filepath.Join(webDir, "templates", "*.html"))
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	for _, name := range []string{
		"login.html", "overview.html", "servers.html", "rules.html", "apply.html", "monitoring.html",
		"servers-table", "rules-table", "apply-result", "page-start", "page-end", "sidebar",
	} {
		if tmpl.Lookup(name) == nil {
			t.Fatalf("missing template %q", name)
		}
	}
}

func TestHXServerCreateAndRulePatch(t *testing.T) {
	srv, token := setupPanel(t)
	// Use real templates so fragment execution works.
	root, err := assets.FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.ParseGlob(filepath.Join(root, "web", "templates", "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	srv.tmpl = tmpl
	h := srv.Handler()

	form := strings.NewReader("name=server-11&address=10.0.0.21&tcp_port=7777&udp_port=7778&health_check_port=7777&enabled=on")
	req := httptest.NewRequest(http.MethodPost, "/hx/servers", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "server-11") {
		t.Fatalf("table missing server-11: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("HX-Trigger"), "show-toast") {
		t.Fatalf("missing HX-Trigger: %s", rec.Header().Get("HX-Trigger"))
	}

	req = httptest.NewRequest(http.MethodPatch, "/hx/rules/rule-server-02-tcp-game", strings.NewReader("enabled=on"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "id=\"rules-table\"") {
		t.Fatalf("expected rules table fragment: %s", rec.Body.String())
	}
}
