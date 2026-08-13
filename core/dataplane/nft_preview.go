package dataplane

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/resources"
)

// PreviewSecurityNft renders forward-ports.nft and a gateway.nft excerpt for security preview.
func PreviewSecurityNft(root string, r *resources.Resources) (forwardPorts, gatewayExcerpt string, err error) {
	if r == nil {
		return "", "", fmt.Errorf("resources 不能为空")
	}
	r.Security.EnsureSecurityDefaults()
	forwardPorts = renderNftForwardPorts(r)

	env, err := LoadEnv(root)
	if err != nil {
		return "", "", err
	}
	sshPort := env.GatewaySSHPort
	if v := os.Getenv("SSH_PORT"); v != "" {
		sshPort = v
	}
	full, err := materializeGatewayNft(config.ResolvePaths(root).Packaging, sshPort, "forward-ports.nft", r.Security)
	if err != nil {
		return "", "", err
	}
	return forwardPorts, extractNftInputChain(full), nil
}

func renderNftForwardPorts(r *resources.Resources) string {
	_, nft, err := render.Render(r)
	if err != nil {
		return ""
	}
	return nft
}

// materializeGatewayNft fills gateway.nft template placeholders from sec (not from disk resources).
func materializeGatewayNft(packagingRoot, sshPort, forwardPortsInclude string, sec resources.Security) (string, error) {
	src := filepath.Join(packagingRoot, "firewall", "gateway.nft")
	b, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	nft := sec.EffectiveFirewallRates()
	tcpRate := nft.TCPNewConnPerIP
	udpRate := nft.UDPPPSPerIP
	tcpBurst := nft.TCPBurst
	udpBurst := nft.UDPBurst
	if !sec.PolicyEnabled(resources.PolicyFirewallNewConnLimit) {
		tcpRate = resources.DisabledFirewallRatePerIP
		tcpBurst = resources.DisabledFirewallBurst
	}
	if !sec.PolicyEnabled(resources.PolicyFirewallUDPLimit) {
		udpRate = resources.DisabledFirewallRatePerIP
		udpBurst = resources.DisabledFirewallBurst
	}

	body := string(b)
	body = strings.ReplaceAll(body, "@INLINE_TCP_NEW_CONN_RATE@", tcpRate)
	body = strings.ReplaceAll(body, "@INLINE_UDP_PPS_RATE@", udpRate)
	body = strings.ReplaceAll(body, "@INLINE_TCP_NEW_CONN_BURST@", fmt.Sprintf("%d", tcpBurst))
	body = strings.ReplaceAll(body, "@INLINE_UDP_PPS_BURST@", fmt.Sprintf("%d", udpBurst))

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "include ") {
			lines[i] = fmt.Sprintf(`include %q`, forwardPortsInclude)
			continue
		}
		if strings.HasPrefix(line, "define SSH_PORT = ") {
			lines[i] = "define SSH_PORT = " + sshPort
		}
	}
	return strings.Join(lines, "\n"), nil
}

func extractNftInputChain(full string) string {
	const marker = "chain input"
	idx := strings.Index(full, marker)
	if idx < 0 {
		return full
	}
	rest := full[idx:]
	end := strings.Index(rest, "\n  chain forward")
	if end > 0 {
		return strings.TrimSpace(rest[:end])
	}
	return strings.TrimSpace(rest)
}
