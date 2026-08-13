package dataplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relaygate/relaygate/core/resources"
)

func TestAfterPullApplySkipsHostWhenControlDefault(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{Security: resources.DefaultSecurity()}
	if err := resources.Save(filepath.Join(data, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}

	env := Env{PanelEnabled: "1", SecurityAutoApply: "", XDSEnabled: true}
	err := AfterPullApply(PullApplyOptions{
		Root:        root,
		Env:         env,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		SkipGateway: true,
	})
	if err != nil {
		t.Fatalf("expected host skip + skip gateway ok: %v", err)
	}
	stPath := filepath.Join(data, "security-apply-status.json")
	b, err := os.ReadFile(stPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, `"module": "kernel"`) || !strings.Contains(body, `"status": "skipped"`) {
		t.Fatalf("want kernel skipped, got %s", body)
	}
	if !strings.Contains(body, `"module": "firewall"`) {
		t.Fatalf("want firewall module, got %s", body)
	}
	if !strings.Contains(body, `"nic"`) {
		t.Fatalf("want nic key (reserved), got %s", body)
	}
	if strings.Contains(body, `"ok": false`) {
		t.Fatalf("want ok true, got %s", body)
	}
}

func TestAfterPullApplyFailsClosedOnKernelApply(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{Security: resources.DefaultSecurity()}
	sp := res.Security.PolicyByID(resources.PolicyKernelSyn)
	sp.Params.TcpMaxSynBacklog = 999999 // unlikely to match live without apply as root
	if err := resources.Save(filepath.Join(data, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}

	// Force host auto on but we are not root — ApplyKernelHarden should fail
	// before verify when helper is unset.
	if IsRoot() {
		t.Skip("running as root; skip fail-closed non-root path")
	}
	env := Env{PanelEnabled: "0", SecurityAutoApply: "1", XDSEnabled: true}
	err := AfterPullApply(PullApplyOptions{
		Root:        root,
		Env:         env,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		SkipGateway: true,
	})
	if err == nil {
		t.Fatal("expected failure when host apply cannot run")
	}
	stPath := filepath.Join(data, "security-apply-status.json")
	b, err := os.ReadFile(stPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"ok": false`) {
		t.Fatalf("want ok false on failed apply, got %s", b)
	}
	if !strings.Contains(string(b), `"failed_at": "kernel"`) {
		t.Fatalf("want failed_at kernel, got %s", b)
	}
}

func TestAfterPullApplySkipsKernelWhenPolicyOff(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELAYGATE_DATA_DIR", filepath.Join(root, "data"))
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	res := &resources.Resources{Security: resources.DefaultSecurity()}
	sp := res.Security.PolicyByID(resources.PolicyKernelSyn)
	sp.Enabled = false
	if err := resources.Save(filepath.Join(data, "resources.yaml"), res); err != nil {
		t.Fatal(err)
	}

	env := Env{PanelEnabled: "0", SecurityAutoApply: "1", XDSEnabled: true}
	_ = AfterPullApply(PullApplyOptions{
		Root:        root,
		Env:         env,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		SkipGateway: true,
	})
	// Non-root CI usually fails at firewall; kernel must already be recorded as skipped.
	stPath := filepath.Join(data, "security-apply-status.json")
	b, err := os.ReadFile(stPath)
	if err != nil {
		t.Fatal(err)
	}
	var st PullApplyStatus
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	if st.Kernel.Status != LayerStatusSkipped {
		t.Fatalf("kernel status=%q want skipped; detail=%q body=%s", st.Kernel.Status, st.Kernel.Detail, b)
	}
	if st.FailedAt == DomainKernel {
		t.Fatalf("must not fail at kernel when policy off: %s", b)
	}
}
