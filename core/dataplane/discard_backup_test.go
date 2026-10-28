package dataplane

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/relaygate/relaygate/core/config"
)

func TestDiscardBackupRestoresPreviousBaseline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	backups := config.ResolvePaths(root).Backups
	older := filepath.Join(backups, "20260101-120000")
	newer := filepath.Join(backups, "20260101-130000")
	for _, dir := range []string{older, newer} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(backups, "latest"), []byte("20260101-130000\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DiscardBackup(root, "20260101-130000"); err != nil {
		t.Fatalf("DiscardBackup: %v", err)
	}
	if _, err := os.Stat(newer); !os.IsNotExist(err) {
		t.Fatalf("failed backup still present: %v", err)
	}
	if _, err := os.Stat(older); err != nil {
		t.Fatalf("previous backup removed: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(backups, "latest"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != "20260101-120000\n" {
		t.Fatalf("latest=%q want previous stamp", got)
	}
}

func TestDiscardBackupWithoutPreviousRemovesLatest(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	backups := config.ResolvePaths(root).Backups
	only := filepath.Join(backups, "20260101-130000")
	if err := os.MkdirAll(only, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backups, "latest"), []byte("20260101-130000\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DiscardBackup(root, "20260101-130000"); err != nil {
		t.Fatalf("DiscardBackup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backups, "latest")); !os.IsNotExist(err) {
		t.Fatalf("latest should be removed when no backup remains: %v", err)
	}
}
