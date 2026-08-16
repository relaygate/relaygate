package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveComposeProfiles(t *testing.T) {
	got := resolveComposeProfiles(Options{PanelEnabled: "1"})
	if got != "control" {
		t.Fatalf("control default = %q, want control", got)
	}
	got = resolveComposeProfiles(Options{PanelEnabled: "0"})
	if got != "node" {
		t.Fatalf("node default = %q, want node", got)
	}
	// Grafana flag must not change role profiles.
	got = resolveComposeProfiles(Options{GrafanaEnabled: "0", PanelEnabled: "1"})
	if got != "control" {
		t.Fatalf("control ignores GRAFANA_ENABLED = %q", got)
	}
}

func TestMigrateComposeProfiles(t *testing.T) {
	optControl := Options{PanelEnabled: "1"}
	optNode := Options{PanelEnabled: "0"}

	if got := migrateComposeProfiles("with-metrics,with-grafana,with-loki,with-logs", optControl); got != "control" {
		t.Fatalf("legacy control stack = %q", got)
	}
	if got := migrateComposeProfiles("with-metrics", optNode); got != "node" {
		t.Fatalf("legacy node metrics = %q", got)
	}
	if got := migrateComposeProfiles("with-metrics,with-logs", optNode); got != "node,alloy" {
		t.Fatalf("legacy node+logs = %q", got)
	}
	if got := migrateComposeProfiles("node,alloy", optNode); got != "node,alloy" {
		t.Fatalf("keep alloy = %q", got)
	}
	if got := migrateComposeProfiles("control", optControl); got != "control" {
		t.Fatalf("already control = %q", got)
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
