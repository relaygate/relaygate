package resources

import (
	"strings"
	"testing"
)

func TestEffectiveKernelSynDefaults(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	p := s.EffectiveKernelSyn()
	if p == nil {
		t.Fatal("want enabled kernel_syn")
	}
	if p.TcpSyncookies != DefaultTcpSyncookies ||
		p.TcpMaxSynBacklog != DefaultTcpMaxSynBacklog ||
		p.TcpSynackRetries != DefaultTcpSynackRetries ||
		p.TcpSynRetries != DefaultTcpSynRetries ||
		p.TcpAbortOnOverflow != DefaultTcpAbortOnOverflow {
		t.Fatalf("defaults: %+v", p)
	}
}

func TestEffectiveKernelSynDisabled(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	sp := s.PolicyByID(PolicyKernelSyn)
	sp.Enabled = false
	if s.EffectiveKernelSyn() != nil {
		t.Fatal("want nil when disabled")
	}
}

func TestRenderKernelHardenConfFromParams(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	sp := s.PolicyByID(PolicyKernelSyn)
	sp.Params.TcpMaxSynBacklog = 4096
	body := RenderKernelHardenConf(&s)
	for _, want := range []string{
		"net.ipv4.tcp_syncookies = 1",
		"net.ipv4.tcp_max_syn_backlog = 4096",
		"net.ipv4.tcp_synack_retries = 2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestNormalizeKernelSynParams(t *testing.T) {
	t.Parallel()
	s := DefaultSecurity()
	sp := s.PolicyByID(PolicyKernelSyn)
	sp.Params.TcpSyncookies = 2
	if err := s.NormalizeSecurity(); err == nil {
		t.Fatal("want error for invalid syncookies")
	}
	sp.Params.TcpSyncookies = 1
	sp.Params.TcpAbortOnOverflow = 2
	if err := s.NormalizeSecurity(); err == nil {
		t.Fatal("want error for invalid tcp_abort_on_overflow")
	}
}
