package dataplane

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/config"
)

func templateFixture(t *testing.T, root, name, body string) {
	t.Helper()
	mustWrite(t, filepath.Join(root, config.TemplatesDirRel, name), body)
}

func TestSeedDefaultsCreatesSkeletonAndCopiesTemplates(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	templateFixture(t, root, "resources.example.yaml", "meta:\n  gateway_name: example\n")
	templateFixture(t, root, "gateways.env.example", "GATEWAY_MATRIX=gateway-01\n")

	if err := SeedDefaults(root, false); err != nil {
		t.Fatal(err)
	}
	data := config.ResolveDataDir(root)
	for _, d := range runtimeDataDirs {
		if st, err := os.Stat(filepath.Join(data, d)); err != nil || !st.IsDir() {
			t.Fatalf("missing %s/%s: %v", data, d, err)
		}
	}
	res := mustRead(t, filepath.Join(data, "resources.yaml"))
	if !strings.Contains(res, "gateway_name: example") {
		t.Fatalf("resources not seeded: %q", res)
	}
	inv := mustRead(t, filepath.Join(data, "inventory", "gateways.env"))
	if !strings.Contains(inv, "GATEWAY_MATRIX") {
		t.Fatalf("inventory not seeded: %q", inv)
	}
}

func TestSeedDefaultsDoesNotOverwriteExisting(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	data := config.ResolveDataDir(root)
	templateFixture(t, root, "resources.example.yaml", "from: example\n")
	templateFixture(t, root, "gateways.env.example", "GATEWAY_MATRIX=from-example\n")
	mustWrite(t, filepath.Join(data, "resources.yaml"), "from: user\n")
	mustWrite(t, filepath.Join(data, "inventory", "gateways.env"), "GATEWAY_MATRIX=from-user\n")

	if err := SeedDefaults(root, false); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, filepath.Join(data, "resources.yaml")); got != "from: user\n" {
		t.Fatalf("resources overwritten: %q", got)
	}
	if got := mustRead(t, filepath.Join(data, "inventory", "gateways.env")); got != "GATEWAY_MATRIX=from-user\n" {
		t.Fatalf("inventory overwritten: %q", got)
	}
}

func TestSeedDefaultsRepairsInventoryWithoutMatrix(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	data := config.ResolveDataDir(root)
	templateFixture(t, root, "resources.example.yaml", "from: example\n")
	templateFixture(t, root, "gateways.env.example", "GATEWAY_MATRIX=gateway-01\n")
	mustWrite(t, filepath.Join(data, "inventory", "gateways.env"), "from=example\n")

	if err := SeedDefaults(root, false); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, filepath.Join(data, "inventory", "gateways.env"))
	if !strings.Contains(got, "GATEWAY_MATRIX=gateway-01") {
		t.Fatalf("invalid inventory not repaired: %q", got)
	}
}

func TestSeedDefaultsResetDefaults(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	data := config.ResolveDataDir(root)
	templateFixture(t, root, "resources.example.yaml", "from: example\n")
	templateFixture(t, root, "gateways.env.example", "from=example\n")
	mustWrite(t, filepath.Join(data, "resources.yaml"), "from: user\n")
	mustWrite(t, filepath.Join(data, "inventory", "gateways.env"), "from=user\n")

	if err := SeedDefaults(root, true); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, filepath.Join(data, "resources.yaml")); got != "from: example\n" {
		t.Fatalf("resources not reset: %q", got)
	}
	if got := mustRead(t, filepath.Join(data, "inventory", "gateways.env")); got != "from=example\n" {
		t.Fatalf("inventory not reset: %q", got)
	}
}

func TestSeedDefaultsMissingExampleIsOKWhenNotReset(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	data := config.ResolveDataDir(root)
	if err := SeedDefaults(root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "resources.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected no resources.yaml, err=%v", err)
	}
}

func TestSeedDefaultsResetRequiresExample(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	if err := SeedDefaults(root, true); err == nil {
		t.Fatal("expected error when reset without example")
	}
}

func TestSeedDefaultsSourceTreeUsesRuntimeDir(t *testing.T) {
	config.IsolateDataDirEnv(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "core", "cmd", "relaygate", "main.go"), "package main\n")
	templateFixture(t, root, "resources.example.yaml", "meta:\n  gateway_name: src\n")
	templateFixture(t, root, "gateways.env.example", "GATEWAY_MATRIX=gateway-01\n")
	if err := SeedDefaults(root, false); err != nil {
		t.Fatal(err)
	}
	data := config.ResolveDataDir(root)
	if filepath.Base(data) != config.DevDataDirName {
		t.Fatalf("source tree DataDir want .runtime, got %s", data)
	}
	if _, err := os.Stat(filepath.Join(data, "resources.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(err) {
		t.Fatalf("source tree must not create top-level data/: %v", err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
