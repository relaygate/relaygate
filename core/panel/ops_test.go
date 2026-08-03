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
	if !strings.Contains(rec.Body.String(), "Confirm") && !strings.Contains(rec.Body.String(), "确认") {
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

	body, _ := json.Marshal(map[string]string{"action": "fail", "confirm": "Confirm"})
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
	if !strings.Contains(rec.Body.String(), "Confirm") && !strings.Contains(rec.Body.String(), "确认") {
		t.Fatalf("expected confirm hint: %s", rec.Body.String())
	}
}

func TestApplyRequiresConfirm(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	// Seed ADS bootstrap so XDS hot path is eligible (unmigrated → RELOAD_ENVOY).
	envoyDir := filepath.Join(config.ResolveDataDir(srv.cfg.Root), "envoy")
	if err := os.MkdirAll(envoyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	boot := []byte("dynamic_resources:\n  cds_config: {}\nstatic_resources:\n  clusters:\n    - name: xds_cluster\n")
	if err := os.WriteFile(filepath.Join(envoyDir, "envoy.yaml"), boot, 0o644); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{"confirm": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/apply", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("apply without confirm status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Confirm") && !strings.Contains(rec.Body.String(), "确认") {
		t.Fatalf("expected confirm hint (XDS default on + migrated): %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/apply", bytes.NewReader(nil))
	authedCSRF(req, token, csrf)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("apply empty body status=%d body=%s", rec.Code, rec.Body.String())
	}
}


func TestFleetPublishRequiresConfirm(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"confirm": "nope"})
	req := httptest.NewRequest(http.MethodPost, "/api/ops/fleet/publish", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Confirm") && !strings.Contains(rec.Body.String(), "确认") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestStandbyBlocksFleetPublish(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	t.Setenv("PANEL_ROLE", "standby")
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"confirm": "Confirm"})
	req := httptest.NewRequest(http.MethodPost, "/api/ops/fleet/publish", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("publish on standby status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFleetJoinIssuesCommand(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	// GET must hit the join handler (405), not SPA fallback.
	getReq := httptest.NewRequest(http.MethodGet, "/api/ops/fleet/join", nil)
	authed(getReq, token)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET join status=%d body=%s", getRec.Code, getRec.Body.String())
	}

	body, _ := json.Marshal(map[string]string{
		"name": "gateway-02",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ops/fleet/join", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "join_command") {
		t.Fatalf("expected join_command in body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "--token ") {
		t.Fatalf("expected --token in join command body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), " bash -s -- node ") {
		t.Fatalf("expected node subcommand in join command body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "PANEL_ADMIN_PASSWORD") {
		t.Fatalf("must not embed panel password: %s", rec.Body.String())
	}
}

func TestUnknownAPIPathNotSPAMethodNotAllowed(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	// Unregistered /api/* must be 404, not SPA's misleading 405.
	req := httptest.NewRequest(http.MethodPost, "/api/ops/fleet-sync", bytes.NewReader([]byte(`{}`)))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown API, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStandbyBlocksFleetJoin(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	t.Setenv("PANEL_ROLE", "standby")
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{
		"name": "gateway-02",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ops/fleet/join", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("join on standby status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFleetLeaveRequiresConfirm(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{
		"confirm": "nope",
		"name":    "gateway-02",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ops/fleet/leave", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Confirm") && !strings.Contains(rec.Body.String(), "确认") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestStandbyBlocksFleetLeave(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	t.Setenv("PANEL_ROLE", "standby")
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{
		"confirm": "Confirm",
		"name":    "gateway-02",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/ops/fleet/leave", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("leave on standby status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFleetStatusEndpoint(t *testing.T) {
	srv, token, _ := setupPanel(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/ops/fleet/status", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["nodes"]; !ok {
		t.Fatalf("missing nodes: %s", rec.Body.String())
	}
	if _, ok := body["published_version"]; !ok {
		t.Fatalf("missing published_version: %s", rec.Body.String())
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
