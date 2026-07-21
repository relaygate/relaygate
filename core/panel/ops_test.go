package panel

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

func setupPanelRealTemplates(t *testing.T) (*Server, string, string) {
	t.Helper()
	root := t.TempDir()
	cfgDir := config.ResolveDataDir(root)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{
		Servers: []resources.Server{
			{Name: "server-01", Address: "10.0.0.11", TCPPort: 7777, UDPPort: 7778, HealthCheckPort: 7777, Enabled: true},
		},
		Rules: []resources.Rule{
			{Name: "forward-server-01-canary-tcp", Kind: "canary", Server: "server-01", Protocol: "TCP", ListenPort: 11001, Enabled: true},
			{Name: "forward-server-01-production-tcp", Kind: "production", Server: "server-01", Protocol: "TCP", ListenPort: 10001, Enabled: false},
			{Name: "forward-server-01-production-udp", Kind: "production", Server: "server-01", Protocol: "UDP", ListenPort: 10001, Enabled: false},
		},
	}
	if err := resources.Save(filepath.Join(cfgDir, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}
	// Point FrontendDir at repo frontend (templates must parse).
	repoRoot, err := resources.FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	front := filepath.Join(repoRoot, "frontend")
	srv, err := New(Config{
		Root:          root,
		FrontendDir:   front,
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

func TestPromoteEnablesProduction(t *testing.T) {
	srv, token, csrf := setupPanelRealTemplates(t)
	h := srv.Handler()

	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/hx/servers/server-01/promote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	res, err := resources.Load(filepath.Join(config.ResolveDataDir(srv.cfg.Root), "resources.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range res.Rules {
		if rule.Kind == "production" && rule.Server == "server-01" && !rule.Enabled {
			t.Fatalf("production rule still disabled: %s", rule.Name)
		}
	}
	audit := filepath.Join(config.ResolveDataDir(srv.cfg.Root), "panel-audit.log")
	b, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "server.promote") {
		t.Fatalf("audit missing promote: %s", b)
	}
}

func TestDrainRequiresConfirm(t *testing.T) {
	srv, token, csrf := setupPanelRealTemplates(t)
	h := srv.Handler()

	form := url.Values{"action": {"fail"}, "confirm": {"nope"}}
	req := httptest.NewRequest(http.MethodPost, "/hx/ops/drain", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DRAIN_FAIL") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestStandbyAllowsDoctorBlocksDrainFail(t *testing.T) {
	srv, token, csrf := setupPanelRealTemplates(t)
	t.Setenv("PANEL_ROLE", "standby")
	h := srv.Handler()

	// doctor (readonly) allowed
	req := httptest.NewRequest(http.MethodPost, "/hx/ops/doctor", nil)
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "standby") {
		t.Fatalf("doctor should be allowed on standby: %s", rec.Body.String())
	}

	form := url.Values{"action": {"fail"}, "confirm": {"DRAIN_FAIL"}}
	req = httptest.NewRequest(http.MethodPost, "/hx/ops/drain", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("drain fail status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestChangesListAndRollbackConfirm(t *testing.T) {
	srv, token, csrf := setupPanelRealTemplates(t)
	root := srv.cfg.Root
	data := config.ResolveDataDir(root)
	stamp := "20260101-120000"
	dir := filepath.Join(data, "backups", stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "change-summary.txt"), []byte("servers: +1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/changes", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("changes page status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), stamp) {
		t.Fatalf("missing stamp in page")
	}

	req = httptest.NewRequest(http.MethodGet, "/hx/changes/"+stamp, nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || (!strings.Contains(rec.Body.String(), "servers: +1") && !strings.Contains(rec.Body.String(), "servers: &#43;1")) {
		t.Fatalf("summary status=%d body=%s", rec.Code, rec.Body.String())
	}

	form := url.Values{"stamp": {stamp}, "confirm": {"no"}}
	req = httptest.NewRequest(http.MethodPost, "/hx/rollback", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("rollback without confirm status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpsAndChangesNavPages(t *testing.T) {
	srv, token, _ := setupPanelRealTemplates(t)
	h := srv.Handler()
	for _, path := range []string{"/ops", "/changes"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		authed(req, token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "运维") && path == "/ops" {
			t.Fatalf("ops page missing title")
		}
		if !strings.Contains(rec.Body.String(), "变更") && path == "/changes" {
			t.Fatalf("changes page missing title")
		}
	}
}
