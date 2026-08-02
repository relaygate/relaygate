package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

func TestPromoteEnablesProduction(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/servers/server-01/promote", nil)
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
		if rule.Entry == "production" && rule.Server == "server-01" && !rule.Enabled {
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
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"action": "fail", "confirm": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/ops/drain", bytes.NewReader(body))
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
	srv, token, csrf := setupPanel(t)
	t.Setenv("PANEL_ROLE", "standby")
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/ops/doctor", nil)
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "standby") {
		t.Fatalf("doctor should be allowed on standby: %s", rec.Body.String())
	}

	body, _ := json.Marshal(map[string]string{"action": "fail", "confirm": "DRAIN_FAIL"})
	req = httptest.NewRequest(http.MethodPost, "/api/ops/drain", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("drain fail status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestChangesListAndRollbackConfirm(t *testing.T) {
	srv, token, csrf := setupPanel(t)
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
	req := httptest.NewRequest(http.MethodGet, "/api/changes?limit=50", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("changes status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), stamp) {
		t.Fatalf("missing stamp in response: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/changes/"+stamp, nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "servers: +1") {
		t.Fatalf("summary status=%d body=%s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]string{"stamp": stamp, "confirm": "no"})
	req = httptest.NewRequest(http.MethodPost, "/api/rollback", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("rollback without confirm status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestApplyPreviewClassify(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/apply/preview", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("preview status=%d body=%s", rec.Code, rec.Body.String())
	}
	var preview map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if _, ok := preview["needs_reload"]; !ok {
		t.Fatalf("missing needs_reload: %s", rec.Body.String())
	}
	if _, ok := preview["needs_firewall"]; !ok {
		t.Fatalf("missing needs_firewall: %s", rec.Body.String())
	}

	// ACL-only write should flip needs_firewall after backup exists from prior apply path.
	// Seed a "before" by writing a backup snapshot via resources path used by Diff.
	aclBody, _ := json.Marshal(map[string]string{"list": "deny", "cidr": "203.0.113.10/32"})
	req = httptest.NewRequest(http.MethodPost, "/api/acl", bytes.NewReader(aclBody))
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("acl status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFirewallApplyRequiresConfirm(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"confirm": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/firewall/apply", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("firewall apply without confirm status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "YES_FLUSH_NFTABLES") {
		t.Fatalf("expected confirm hint: %s", rec.Body.String())
	}
}

func TestApplyRequiresConfirm(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"confirm": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/apply", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("apply without confirm status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "RELOAD_ENVOY") {
		t.Fatalf("expected confirm hint: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/apply", bytes.NewReader(nil))
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("apply empty body status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPILang(t *testing.T) {
	srv, _, _ := setupPanel(t)
	h := srv.Handler()
	body, _ := json.Marshal(map[string]string{"lang": "en"})
	req := httptest.NewRequest(http.MethodPost, "/api/lang", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var gotLang string
	for _, c := range rec.Result().Cookies() {
		if c.Name == langCookie {
			gotLang = c.Value
		}
	}
	if gotLang != langEnglish {
		t.Fatalf("lang cookie=%q", gotLang)
	}
}
