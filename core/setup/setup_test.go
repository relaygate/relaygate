package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveComposeProfiles(t *testing.T) {
	t.Setenv("MINIMAL", "")
	t.Setenv("COMPOSE_PROFILES", "")
	_ = os.Unsetenv("MINIMAL")
	_ = os.Unsetenv("COMPOSE_PROFILES")

	got := resolveComposeProfiles(Options{GrafanaEnabled: "1", PanelEnabled: "1"})
	if got != "with-metrics,with-grafana,with-loki,with-logs" {
		t.Fatalf("control default = %q", got)
	}
	got = resolveComposeProfiles(Options{GrafanaEnabled: "0", PanelEnabled: "0"})
	if got != "" {
		t.Fatalf("node default = %q, want empty (Envoy + agent only)", got)
	}
	got = resolveComposeProfiles(Options{GrafanaEnabled: "0", PanelEnabled: "1"})
	if got != "with-metrics" {
		t.Fatalf("control without grafana = %q, want with-metrics", got)
	}

	t.Setenv("MINIMAL", "1")
	got = resolveComposeProfiles(Options{GrafanaEnabled: "1", PanelEnabled: "1"})
	if got != "with-metrics" {
		t.Fatalf("MINIMAL=1 = %q, want with-metrics", got)
	}
	_ = os.Unsetenv("MINIMAL")

	t.Setenv("COMPOSE_PROFILES", "with-logs")
	got = resolveComposeProfiles(Options{GrafanaEnabled: "0", PanelEnabled: "0"})
	if got != "with-logs" {
		t.Fatalf("node explicit with-logs = %q", got)
	}
	_ = os.Unsetenv("COMPOSE_PROFILES")
	t.Setenv("COMPOSE_PROFILES", "with-logs")
	got = resolveComposeProfiles(Options{GrafanaEnabled: "1", PanelEnabled: "1"})
	if got != "with-logs" {
		t.Fatalf("explicit COMPOSE_PROFILES = %q", got)
	}
	_ = os.Unsetenv("COMPOSE_PROFILES")

	t.Setenv("COMPOSE_PROFILES", "with-grafana,with-loki")
	got = resolveComposeProfiles(Options{GrafanaEnabled: "1", PanelEnabled: "1"})
	if got != "with-metrics,with-grafana,with-loki" {
		t.Fatalf("imply with-metrics for grafana = %q", got)
	}
}

func TestImplyMetricsForGrafana(t *testing.T) {
	if got := implyMetricsForGrafana(""); got != "" {
		t.Fatalf("empty = %q", got)
	}
	if got := implyMetricsForGrafana("with-logs"); got != "with-logs" {
		t.Fatalf("logs only = %q", got)
	}
	if got := implyMetricsForGrafana("with-metrics,with-grafana"); got != "with-metrics,with-grafana" {
		t.Fatalf("already has metrics = %q", got)
	}
	if got := implyMetricsForGrafana("with-grafana"); got != "with-metrics,with-grafana" {
		t.Fatalf("imply = %q", got)
	}
}

func TestDefaultAdminPassword(t *testing.T) {
	if defaultAdminPassword != "relaygate" {
		t.Fatalf("defaultAdminPassword = %q, want relaygate", defaultAdminPassword)
	}
}

func TestEnsureSecretsWritesDefaultPassword(t *testing.T) {
	dir := t.TempDir()
	opt := Options{SecretsDir: filepath.Join(dir, "secrets")}
	if err := ensureSecrets(opt); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"panel_admin_password", "grafana_admin_password"} {
		b, err := os.ReadFile(filepath.Join(opt.SecretsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(string(b))
		if got != defaultAdminPassword {
			t.Fatalf("%s = %q, want %q", name, got, defaultAdminPassword)
		}
	}
	// Existing non-empty files must not be overwritten.
	panel := filepath.Join(opt.SecretsDir, "panel_admin_password")
	if err := os.WriteFile(panel, []byte("custom-secret\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := ensureSecrets(opt); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(panel)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != "custom-secret" {
		t.Fatalf("existing password overwritten: %q", got)
	}
}
