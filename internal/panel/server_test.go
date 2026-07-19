package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	for _, name := range []string{"login.html", "overview.html", "servers.html", "rules.html", "apply.html"} {
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

func authed(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: "panel_session", Value: token})
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
