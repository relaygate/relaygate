package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaybePrivilegedReexecSkippedWhenRootOrUnset(t *testing.T) {
	t.Setenv("RELAYGATE_PRIVILEGED_HELPER", "")
	handled, err := maybePrivilegedReexec(os.Stdout, os.Stderr, "reload")
	if handled || err != nil {
		t.Fatalf("unset helper: handled=%v err=%v", handled, err)
	}
}

func TestMaybePrivilegedReexecMissingHelper(t *testing.T) {
	if IsRoot() {
		t.Skip("cannot test non-root helper path as root")
	}
	missing := filepath.Join(t.TempDir(), "no-such-helper")
	t.Setenv("RELAYGATE_PRIVILEGED_HELPER", missing)
	handled, err := maybePrivilegedReexec(os.Stdout, os.Stderr, "firewall-check")
	if !handled {
		t.Fatal("expected handled=true")
	}
	if err == nil {
		t.Fatal("expected error for missing helper")
	}
}

func TestErrNeedRootOrHelperMessage(t *testing.T) {
	t.Setenv("RELAYGATE_PRIVILEGED_HELPER", "")
	err := errNeedRootOrHelper()
	if err == nil || err.Error() == "需要 root" {
		t.Fatalf("want helper hint, got %v", err)
	}
	t.Setenv("RELAYGATE_PRIVILEGED_HELPER", "/usr/local/libexec/relaygate/apply")
	err = errNeedRootOrHelper()
	if err == nil || err.Error() != "需要 root" {
		t.Fatalf("got %v", err)
	}
}
