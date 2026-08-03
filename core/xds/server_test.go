package xds

import (
	"net"
	"os"
	"testing"
)

func TestIsEnabledFromEnvDefaultOn(t *testing.T) {
	t.Setenv("XDS_ENABLED", "")
	_ = os.Unsetenv("XDS_ENABLED")
	if !IsEnabledFromEnv() {
		t.Fatal("unset XDS_ENABLED should default on")
	}
	t.Setenv("XDS_ENABLED", "0")
	if IsEnabledFromEnv() {
		t.Fatal("XDS_ENABLED=0 should disable")
	}
}

func TestMemoryPublisherSetSnapshot(t *testing.T) {
	t.Parallel()
	p := NewMemoryPublisher()
	v1 := p.NextVersion()
	if err := p.SetSnapshot("gateway-01", Snapshot{Version: v1}); err != nil {
		t.Fatal(err)
	}
	if got := p.LastVersion("gateway-01"); got != v1 {
		t.Fatalf("LastVersion=%q want %q", got, v1)
	}
	v2 := p.NextVersion()
	_ = p.SetSnapshot("gateway-01", Snapshot{Version: v2})
	if got := p.LastVersion("gateway-01"); got != v2 {
		t.Fatalf("LastVersion=%q want %q", got, v2)
	}
	if _, ok := p.Get("missing"); ok {
		t.Fatal("expected miss")
	}
}

func TestServerAddrAndStart(t *testing.T) {
	t.Parallel()
	s := NewServer(nil)
	if s.Addr() != "127.0.0.1:18000" {
		t.Fatalf("Addr=%s", s.Addr())
	}
	// Use an ephemeral port so parallel CI / running Panel ADS do not collide.
	s.Port = 0
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.Port = ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if !s.Running() {
		t.Fatal("expected running")
	}
}
