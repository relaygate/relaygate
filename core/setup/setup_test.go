package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
