package dataplane

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/config"
)

func TestRenderObservabilityRemoteWriteAuthAndTokenSync(t *testing.T) {
	root := t.TempDir()
	packaging := filepath.Join(root, "packaging", "prometheus")
	if err := os.MkdirAll(packaging, 0o755); err != nil {
		t.Fatal(err)
	}
	tpl := `# gateway: ${GATEWAY_NAME}
scrape_configs: []
`
	if err := os.WriteFile(filepath.Join(packaging, "prometheus.yml.tpl"), []byte(tpl), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	secrets := filepath.Join(root, "secrets")
	if err := os.MkdirAll(secrets, 0o755); err != nil {
		t.Fatal(err)
	}
	tokPath := filepath.Join(secrets, "agent.token")
	if err := os.WriteFile(tokPath, []byte("node-token-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envBody := strings.Join([]string{
		"GATEWAY_NAME=gateway-03",
		"ENVOY_ADMIN_PORT=9901",
		"RELAYGATE_DATA_DIR=" + data,
		"AGENT_TOKEN_FILE=" + tokPath,
		"PROMETHEUS_REMOTE_WRITE_URL=http://203.0.113.10:9000/api/agent/metrics/write",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(envBody), 0o640); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RELAYGATE_DATA_DIR", data)
	t.Setenv("AGENT_TOKEN_FILE", tokPath)
	t.Setenv("PROMETHEUS_REMOTE_WRITE_URL", "http://203.0.113.10:9000/api/agent/metrics/write")

	if err := RenderObservability(root); err != nil {
		t.Fatal(err)
	}
	prom := config.ResolvePaths(root).PromYAML
	body, err := os.ReadFile(prom)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"remote_write:",
		"url: http://203.0.113.10:9000/api/agent/metrics/write",
		"authorization:",
		"credentials_file: /etc/prometheus/agent.token",
		"gateway: gateway-03",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("prometheus.yml missing %q\n%s", want, s)
		}
	}
	cred := filepath.Join(data, "prometheus", "agent.token")
	got, err := os.ReadFile(cred)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "node-token-value" {
		t.Fatalf("synced token=%q", got)
	}
	st, err := os.Stat(cred)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o004 == 0 {
		t.Fatalf("agent.token should be world-readable for prometheus nobody, mode=%v", st.Mode())
	}
}
