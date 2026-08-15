package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentMetricsWriteRequiresToken(t *testing.T) {
	srv, _, _ := setupPanel(t)
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/metrics/write", strings.NewReader("snappy"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentMetricsWriteProxiesToPrometheus(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody string
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(prom.Close)

	srv, token, csrf := setupPanel(t)
	srv.cfg.PrometheusURL = prom.URL
	h := srv.Handler()

	joinBody, _ := json.Marshal(map[string]string{"name": "gateway-08"})
	joinReq := httptest.NewRequest(http.MethodPost, "/api/ops/fleet/join", strings.NewReader(string(joinBody)))
	authedCSRF(joinReq, token, csrf)
	joinRec := httptest.NewRecorder()
	h.ServeHTTP(joinRec, joinReq)
	if joinRec.Code != 200 {
		t.Fatalf("join status=%d body=%s", joinRec.Code, joinRec.Body.String())
	}
	var joinResp map[string]any
	if err := json.Unmarshal(joinRec.Body.Bytes(), &joinResp); err != nil {
		t.Fatal(err)
	}
	agentToken, _ := joinResp["token"].(string)
	if agentToken == "" {
		t.Fatal("missing join token")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/agent/metrics/write", strings.NewReader("payload"))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("write status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/api/v1/write" {
		t.Fatalf("prom path=%q", gotPath)
	}
	if gotAuth != "" {
		t.Fatalf("agent token must not be forwarded")
	}
	if gotBody != "payload" {
		t.Fatalf("body=%q", gotBody)
	}
}
