package panel

import (
	"archive/zip"
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
)

func TestConfigResourcesGetAndExport(t *testing.T) {
	srv, token, _ := setupPanel(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/config/resources", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	content, _ := got["content"].(string)
	mtime, _ := got["mtime"].(string)
	if !strings.Contains(content, "server-01") || mtime == "" {
		t.Fatalf("unexpected payload: %+v", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/export", nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("export status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "resources.yaml") {
		t.Fatalf("disposition=%s", rec.Header().Get("Content-Disposition"))
	}
	if !strings.Contains(rec.Body.String(), "server-01") {
		t.Fatalf("export body missing server: %s", rec.Body.String())
	}
}

func TestConfigExportZipOptionalNFT(t *testing.T) {
	srv, token, _ := setupPanel(t)
	h := srv.Handler()

	// No nft yet — should still succeed.
	req := httptest.NewRequest(http.MethodGet, "/api/config/export?pack=zip", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("zip status=%d body=%s", rec.Code, rec.Body.String())
	}
	names := zipNames(t, rec.Body.Bytes())
	if !names["resources.yaml"] || !names["SUMMARY.txt"] {
		t.Fatalf("zip names=%v", names)
	}
	if names["forward-ports.nft"] {
		t.Fatalf("expected nft skipped when missing")
	}

	data := config.ResolveDataDir(srv.cfg.Root)
	nftDir := filepath.Join(data, "firewall")
	if err := os.MkdirAll(nftDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nftDir, "forward-ports.nft"), []byte("# nft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/export?pack=zip", nil)
	authed(req, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("zip2 status=%d body=%s", rec.Code, rec.Body.String())
	}
	names = zipNames(t, rec.Body.Bytes())
	if !names["forward-ports.nft"] {
		t.Fatalf("expected nft in zip: %v", names)
	}
}

func TestConfigValidateAndPut(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/config/resources", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var cur map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &cur)
	content, _ := cur["content"].(string)
	mtime, _ := cur["mtime"].(string)
	etag, _ := cur["etag"].(string)

	// Invalid YAML
	badBody, _ := json.Marshal(map[string]string{"content": "servers: [\n"})
	req = httptest.NewRequest(http.MethodPost, "/api/config/resources/validate", bytes.NewReader(badBody))
	authedCSRF(req, token, csrf)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("bad validate status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Valid no-op
	okBody, _ := json.Marshal(map[string]string{"content": content})
	req = httptest.NewRequest(http.MethodPost, "/api/config/resources/validate", bytes.NewReader(okBody))
	authedCSRF(req, token, csrf)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("ok validate status=%d body=%s", rec.Code, rec.Body.String())
	}
	var vr configValidateResult
	if err := json.Unmarshal(rec.Body.Bytes(), &vr); err != nil {
		t.Fatal(err)
	}
	if !vr.OK || vr.Diff == "" {
		t.Fatalf("validate result=%+v", vr)
	}

	path := filepath.Join(config.ResolveDataDir(srv.cfg.Root), "resources.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = string(raw)
	updated := strings.Replace(content, "enabled: false", "enabled: true", 1)
	if updated == content {
		t.Fatal("expected to flip an enabled: false")
	}
	if etag == "" {
		etag, _ = cur["etag"].(string)
	}
	_ = mtime

	putBody, _ := json.Marshal(map[string]string{"content": updated, "etag": etag})
	req = httptest.NewRequest(http.MethodPut, "/api/config/resources", bytes.NewReader(putBody))
	authedCSRF(req, token, csrf)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("put status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Stale etag → 409
	staleBody, _ := json.Marshal(map[string]string{"content": updated, "etag": etag})
	req = httptest.NewRequest(http.MethodPut, "/api/config/resources", bytes.NewReader(staleBody))
	authedCSRF(req, token, csrf)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale put status=%d body=%s", rec.Code, rec.Body.String())
	}

	audit := filepath.Join(config.ResolveDataDir(srv.cfg.Root), "panel-audit.log")
	b, err := os.ReadFile(audit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "config.resources.put") {
		t.Fatalf("audit missing put: %s", b)
	}
}

func TestConfigStandbyAllowsReadBlocksPut(t *testing.T) {
	srv, token, csrf := setupPanel(t)
	t.Setenv("PANEL_ROLE", "standby")
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/config/resources", nil)
	authed(req, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("standby get status=%d body=%s", rec.Code, rec.Body.String())
	}

	var cur map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &cur)
	body, _ := json.Marshal(map[string]any{
		"content": cur["content"],
		"etag":    cur["etag"],
		"mtime":   cur["mtime"],
	})
	req = httptest.NewRequest(http.MethodPut, "/api/config/resources", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("standby put status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/config/resources/validate", bytes.NewReader(body))
	authedCSRF(req, token, csrf)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 && rec.Code != 400 {
		t.Fatalf("standby validate should be allowed: %d %s", rec.Code, rec.Body.String())
	}
}

func zipNames(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, f := range zr.File {
		out[f.Name] = true
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	return out
}
