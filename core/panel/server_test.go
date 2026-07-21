package panel

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

func writeMinimalUI(t *testing.T, uiDir string, withFavicon bool) {
	t.Helper()
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte(`<!doctype html><html><body>RelayGate SPA</body></html>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if withFavicon {
		if err := os.WriteFile(filepath.Join(uiDir, "favicon.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func setupPanel(t *testing.T) (*Server, string, string) {
	t.Helper()
	root := t.TempDir()
	cfgDir := config.ResolveDataDir(root)
	uiDir := filepath.Join(root, "ui", "dist")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalUI(t, uiDir, false)
	res := &resources.Resources{
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
			{Name: "server-02", Address: "10.0.0.12", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "forward-server-01-canary-tcp", Kind: "canary", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-production-tcp", Kind: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
			{Name: "forward-server-01-production-udp", Kind: "production", Server: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: false},
			{Name: "forward-server-02-production-tcp", Kind: "production", Server: "server-02", Protocol: "TCP", ListenPort: 10002, Enabled: false},
			{Name: "forward-server-02-production-udp", Kind: "production", Server: "server-02", Protocol: "UDP", ListenPort: 10002, Enabled: false},
		},
	}
	if err := resources.Save(filepath.Join(cfgDir, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{
		Root:          root,
		UIDir:         uiDir,
		AdminPassword: "test-pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, csrf, err := srv.createSession()
	if err != nil {
		t.Fatal(err)
	}
	return srv, token, csrf
}

func setupPanelWithGrafana(t *testing.T) (*Server, string, string) {
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
	cfgDir := config.ResolveDataDir(root)
	uiDir := filepath.Join(root, "ui", "dist")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalUI(t, uiDir, true)
	res := &resources.Resources{
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
	}
	if err := resources.Save(filepath.Join(cfgDir, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{
		Root:          root,
		UIDir:         uiDir,
		AdminPassword: "test-pass",
		GrafanaURL:    ts.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	token, csrf, err := srv.createSession()
	if err != nil {
		t.Fatal(err)
	}
	return srv, token, csrf
}

func authed(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: "panel_session", Value: token})
}

func authedCSRF(req *http.Request, token, csrf string) {
	authed(req, token)
	req.Header.Set("X-CSRF-Token", csrf)
}

func TestNewReadsAdminPasswordFile(t *testing.T) {
	root := t.TempDir()
	uiDir := filepath.Join(root, "ui", "dist")
	writeMinimalUI(t, uiDir, false)
	passwordFile := filepath.Join(root, "panel-password")
	if err := os.WriteFile(passwordFile, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PANEL_ADMIN_PASSWORD", "")
	t.Setenv("PANEL_ADMIN_PASSWORD_FILE", passwordFile)

	srv, err := New(Config{Root: root, UIDir: uiDir})
	if err != nil {
		t.Fatal(err)
	}
	if srv.cfg.AdminPassword != "file-secret" {
		t.Fatalf("password=%q", srv.cfg.AdminPassword)
	}
}

func TestAPIServerCreateAndDelete(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{
		"name": "server-11", "address": "10.0.0.21",
		"tcp_port": 7777, "udp_port": 7778, "health_check_port": 7777, "enabled": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
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

	res, err := resources.Load(filepath.Join(config.ResolveDataDir(srv.cfg.Root), "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Servers) != 3 {
		t.Fatalf("servers=%d", len(res.Servers))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 409 {
		t.Fatalf("duplicate status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/servers/server-11", nil)
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/servers/server-01", nil)
	authedCSRF(req, token, csrf)
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

	res, err = resources.Load(filepath.Join(config.ResolveDataDir(srv.cfg.Root), "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range res.Rules {
		if rule.Server == "server-01" {
			t.Fatalf("orphan rule: %+v", rule)
		}
	}
}

func TestAPIServersIncludesLifecycle(t *testing.T) {
	srv, token, _ := setupPanel(t)
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["servers"]; !ok {
		t.Fatalf("missing servers: %s", rec.Body.String())
	}
	if _, ok := payload["lifecycle"]; !ok {
		t.Fatalf("missing lifecycle: %s", rec.Body.String())
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
	srv, token, _ := setupPanelWithGrafana(t)
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
		t.Fatalf("upstream host=%q", got)
	}
}

func TestFaviconRedirect(t *testing.T) {
	srv, _, _ := setupPanelWithGrafana(t)
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
	srv, _, _ := setupPanel(t)
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

func TestSPAFallback(t *testing.T) {
	srv, _, _ := setupPanel(t)
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/servers", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "RelayGate SPA") {
		t.Fatalf("expected index.html body: %s", rec.Body.String())
	}
}

func TestAPILoginLogoutSession(t *testing.T) {
	srv, _, _ := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"password": "test-pass"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	var loginResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &loginResp)
	if loginResp["ok"] != true {
		t.Fatalf("login resp=%#v", loginResp)
	}
	var sessionToken, csrfToken string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "panel_session" {
			sessionToken = c.Value
		}
		if c.Name == "panel_csrf" {
			csrfToken = c.Value
		}
	}
	if sessionToken == "" || csrfToken == "" {
		t.Fatal("missing cookies")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/session", nil)
	authed(req, sessionToken)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("session status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	authedCSRF(req, sessionToken, csrfToken)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("logout status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCSRFRejectsMutatingWithoutToken(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{
		"name": "server-csrf", "address": "10.0.0.99",
		"tcp_port": 7777, "udp_port": 7778, "health_check_port": 7777,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewReader(body))
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/apply", nil)
	authed(req, token)
	req.Header.Set("X-CSRF-Token", "wrong-token")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad csrf status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status/envoy", nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("get status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("with csrf status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStandbyRejectsWrites(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	t.Setenv("PANEL_ROLE", "standby")
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{
		"name": "server-standby", "address": "10.0.0.55",
		"tcp_port": 7777, "udp_port": 7778, "health_check_port": 7777,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/servers", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("standby api status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "standby") && !strings.Contains(rec.Body.String(), "备用") {
		t.Fatalf("expected standby message: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/apply", nil)
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("standby apply status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("standby get status=%d", rec.Code)
	}
}

func TestValidatePanelBind(t *testing.T) {
	cases := []struct {
		bind string
		ok   bool
	}{
		{"127.0.0.1:9000", true},
		{"localhost:9000", true},
		{"[::1]:9000", true},
		{"0.0.0.0:9000", false},
		{":9000", false},
		{"192.168.1.1:9000", false},
		{"bad", false},
	}
	for _, tc := range cases {
		err := validatePanelBind(tc.bind)
		if tc.ok && err != nil {
			t.Fatalf("bind %q: unexpected err %v", tc.bind, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("bind %q: expected error", tc.bind)
		}
	}
}

func TestNewRejectsNonLoopbackBind(t *testing.T) {
	root := t.TempDir()
	uiDir := filepath.Join(root, "ui", "dist")
	writeMinimalUI(t, uiDir, false)
	_, err := New(Config{Root: root, UIDir: uiDir, AdminPassword: "x", Bind: "0.0.0.0:9000"})
	if err == nil {
		t.Fatal("expected error for 0.0.0.0 bind")
	}
}

func TestPasswordMatchConstantLength(t *testing.T) {
	if !passwordMatch("secret", "secret") {
		t.Fatal("equal passwords should match")
	}
	if passwordMatch("short", "much-longer-password") {
		t.Fatal("different passwords must not match")
	}
}
