package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDataDirSourceTree(t *testing.T) {
	IsolateDataDirEnv(t)
	root := t.TempDir()
	mustMk(t, filepath.Join(root, "core", "cmd", "relaygate"))
	got := ResolveDataDir(root)
	if got != filepath.Join(root, DevDataDirName) {
		t.Fatalf("got %s want .runtime under root", got)
	}
}

func TestResolveDataDirInstallTree(t *testing.T) {
	IsolateDataDirEnv(t)
	root := t.TempDir()
	got := ResolveDataDir(root)
	if got != filepath.Join(root, InstallDataDirName) {
		t.Fatalf("got %s want data under install root", got)
	}
}

func TestResolveDataDirEnvOverride(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom-rt")
	t.Setenv("RELAYGATE_DATA_DIR", custom)
	got := ResolveDataDir(root)
	if got != custom {
		t.Fatalf("got %s want %s", got, custom)
	}
}

func TestResolveDataDirRelativeEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", "var/lib")
	got := ResolveDataDir(root)
	if got != filepath.Join(root, "var", "lib") {
		t.Fatalf("got %s", got)
	}
}

func TestFindRootPrefersPanelRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PANEL_ROOT", root)
	got, err := FindRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("got %s want %s", got, root)
	}
}

func mustMk(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
