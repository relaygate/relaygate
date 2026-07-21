package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/config"
)

func TestResolveReleaseSpecPrefersTar(t *testing.T) {
	root := t.TempDir()
	tar := filepath.Join(root, "rel.tar.gz")
	if err := os.WriteFile(tar, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELAYGATE_TAR", tar)
	t.Setenv("RELAYGATE_VERSION", "")
	t.Setenv("DEPLOY_REF", "")
	ver, gotTar, err := ResolveReleaseSpec(root)
	if err != nil {
		t.Fatal(err)
	}
	if gotTar != tar {
		t.Fatalf("tar=%q", gotTar)
	}
	_ = ver
}

func TestResolveReleaseSpecRejectsFloating(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_TAR", "")
	t.Setenv("RELAYGATE_VERSION", "latest")
	t.Setenv("DEPLOY_REF", "")
	_, _, err := ResolveReleaseSpec(root)
	if err == nil {
		t.Fatal("expected error for floating version")
	}
}

func TestResolveReleaseSpecFromDEPLOY_REF(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_TAR", "")
	t.Setenv("RELAYGATE_VERSION", "")
	t.Setenv("DEPLOY_REF", "v1.2.3")
	ver, tar, err := ResolveReleaseSpec(root)
	if err != nil {
		t.Fatal(err)
	}
	if ver != "v1.2.3" || tar != "" {
		t.Fatalf("ver=%q tar=%q", ver, tar)
	}
}

func TestFleetRemoteUpgradeCmd(t *testing.T) {
	cmd := fleetRemoteUpgradeCmd("/opt/relaygate", "v0.1.0", "/tmp/pkg.tar.gz")
	for _, want := range []string{
		"RELAYGATE_VERSION='v0.1.0'",
		"RELAYGATE_TAR='/tmp/pkg.tar.gz'",
		"bash ./install.sh --upgrade -y",
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("missing %q in %s", want, cmd)
		}
	}
	for _, ban := range []string{"git fetch --all", "git pull --ff-only", "git checkout "} {
		if strings.Contains(cmd, ban) {
			t.Fatalf("must not invoke %q: %s", ban, cmd)
		}
	}
}

func TestDefaultDrainWaitMatchesNLBTemplate(t *testing.T) {
	// packaging/terraform/nlb: unhealthy_threshold=3, interval=10
	if config.DefaultDrainWaitSec != 30 || config.RecommendedDrainWaitSec != 30 {
		t.Fatalf("got default=%d recommended=%d want 30", config.DefaultDrainWaitSec, config.RecommendedDrainWaitSec)
	}
}

func TestLoadEnvDefaultDrainWait(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DRAIN_WAIT", "")
	// Clear any inherited DRAIN_WAIT from process; LoadEnv uses Getenv after LoadDotEnv.
	_ = os.Unsetenv("DRAIN_WAIT")
	env, err := config.LoadEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	if env.DrainWait != config.DefaultDrainWaitSec {
		t.Fatalf("DrainWait=%d want %d", env.DrainWait, config.DefaultDrainWaitSec)
	}
}
