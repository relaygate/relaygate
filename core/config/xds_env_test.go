package config

import (
	"os"
	"testing"
)

func TestLoadEnvXDSDefaultsOn(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDS_ENABLED", "")
	t.Setenv("XDS_PORT", "")
	_ = os.Unsetenv("XDS_ENABLED")
	_ = os.Unsetenv("XDS_PORT")

	env, err := LoadEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	if !env.XDSEnabled {
		t.Fatal("XDS_ENABLED must default on")
	}
	if env.XDSPort != "18000" {
		t.Fatalf("XDSPort=%q want 18000", env.XDSPort)
	}
}

func TestLoadEnvXDSDisabledExplicit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDS_ENABLED", "0")

	env, err := LoadEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	if env.XDSEnabled {
		t.Fatal("XDS_ENABLED=0 must disable hot apply")
	}
}

func TestLoadEnvXDSEnabled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDS_ENABLED", "1")
	t.Setenv("XDS_PORT", "18001")

	env, err := LoadEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	if !env.XDSEnabled {
		t.Fatal("want XDSEnabled")
	}
	if env.XDSPort != "18001" {
		t.Fatalf("XDSPort=%q", env.XDSPort)
	}
}

func TestEnvFlagEnabled(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"1", "true", "YES", "on"} {
		if !envFlagEnabled(v) {
			t.Fatalf("%q should be enabled", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "off", "maybe"} {
		if envFlagEnabled(v) {
			t.Fatalf("%q should be disabled", v)
		}
	}
}
