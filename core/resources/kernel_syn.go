package resources

import (
	"fmt"
	"strings"
)

// Default kernel_syn param values (aligned with packaging/security/sysctl-tcp-harden.conf).
const (
	DefaultTcpSyncookies      = 1
	DefaultTcpMaxSynBacklog   = 8192
	DefaultTcpSynackRetries   = 2
	DefaultTcpSynRetries      = 3
	DefaultTcpAbortOnOverflow = 0
)

// KernelSynParams is the effective kernel overlay for policy kernel_syn (applied via sysctl).
type KernelSynParams struct {
	TcpSyncookies      int
	TcpMaxSynBacklog   int
	TcpSynackRetries   int
	TcpSynRetries      int
	TcpAbortOnOverflow int
}

// EffectiveKernelSyn returns kernel knobs when kernel_syn is enabled; nil when off.
// Uses typed Params fields only; PolicyParams.Extra is ignored.
func (s *Security) EffectiveKernelSyn() *KernelSynParams {
	s.EnsureSecurityDefaults()
	if !s.PolicyEnabled(PolicyKernelSyn) {
		return nil
	}
	p := s.PolicyByID(PolicyKernelSyn)
	if p == nil {
		return nil
	}
	return &KernelSynParams{
		TcpSyncookies:      p.Params.TcpSyncookies,
		TcpMaxSynBacklog:   p.Params.TcpMaxSynBacklog,
		TcpSynackRetries:   p.Params.TcpSynackRetries,
		TcpSynRetries:      p.Params.TcpSynRetries,
		TcpAbortOnOverflow: p.Params.TcpAbortOnOverflow,
	}
}

func applyKernelSynDefaults(p *SecurityPolicy, d SecurityPolicy) {
	wasUnset := p.Params.TcpMaxSynBacklog <= 0 &&
		p.Params.TcpSynackRetries <= 0 &&
		p.Params.TcpSynRetries <= 0 &&
		p.Params.TcpSyncookies == 0
	if p.Params.TcpMaxSynBacklog <= 0 {
		p.Params.TcpMaxSynBacklog = d.Params.TcpMaxSynBacklog
	}
	if p.Params.TcpSynackRetries <= 0 {
		p.Params.TcpSynackRetries = d.Params.TcpSynackRetries
	}
	if p.Params.TcpSynRetries <= 0 {
		p.Params.TcpSynRetries = d.Params.TcpSynRetries
	}
	if wasUnset {
		p.Params.TcpSyncookies = d.Params.TcpSyncookies
		p.Params.TcpAbortOnOverflow = d.Params.TcpAbortOnOverflow
	}
}

func normalizeKernelSynParams(p *PolicyParams) error {
	if p.TcpSyncookies != 0 && p.TcpSyncookies != 1 {
		return fmt.Errorf("tcp_syncookies 须为 0 或 1")
	}
	if p.TcpMaxSynBacklog < 1 {
		return fmt.Errorf("tcp_max_syn_backlog 须 ≥ 1")
	}
	if p.TcpSynackRetries < 0 {
		return fmt.Errorf("tcp_synack_retries 须 ≥ 0")
	}
	if p.TcpSynRetries < 0 {
		return fmt.Errorf("tcp_syn_retries 须 ≥ 0")
	}
	if p.TcpAbortOnOverflow != 0 && p.TcpAbortOnOverflow != 1 {
		return fmt.Errorf("tcp_abort_on_overflow 须为 0 或 1")
	}
	return nil
}

// RenderKernelHardenConf renders the kernel overlay file from security.protections[kernel_syn].
func RenderKernelHardenConf(s *Security) string {
	p := s.EffectiveKernelSyn()
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("# RelayGate 可选：TCP 握手 / SYN Flood 防护（内核域 · sysctl overlay）\n")
	b.WriteString("# 由 relaygate security kernel-conf / apply-kernel 生成\n")
	b.WriteString("# 不替代 packaging/sysctl/gateway.conf（somaxconn / 缓冲 / file-max 等仍由后者负责）。\n")
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("net.ipv4.tcp_syncookies = %d\n", p.TcpSyncookies))
	b.WriteString(fmt.Sprintf("net.ipv4.tcp_max_syn_backlog = %d\n", p.TcpMaxSynBacklog))
	b.WriteString(fmt.Sprintf("net.ipv4.tcp_synack_retries = %d\n", p.TcpSynackRetries))
	b.WriteString(fmt.Sprintf("net.ipv4.tcp_syn_retries = %d\n", p.TcpSynRetries))
	b.WriteString(fmt.Sprintf("net.ipv4.tcp_abort_on_overflow = %d\n", p.TcpAbortOnOverflow))
	return b.String()
}
