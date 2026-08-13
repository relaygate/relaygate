package dataplane

import (
	"fmt"
	"strings"
)

// VerifyNftablesLoaded checks that the RelayGate nftables ruleset is present
// (table inet relaygate with input chain). Does not mutate the host.
func VerifyNftablesLoaded(root string) error {
	if !LookPath("nft") {
		return fmt.Errorf("未找到 nftables 工具（nft），无法验证防火墙规则集")
	}
	out, err := RunCmdCapture(root, "nft", "list", "table", "inet", "relaygate")
	if err != nil {
		return fmt.Errorf("nftables 规则集未加载（缺少 table inet relaygate）：%w", err)
	}
	body := strings.ToLower(out)
	if !strings.Contains(body, "chain input") {
		return fmt.Errorf("nftables 规则集不完整：table inet relaygate 中缺少 chain input")
	}
	return nil
}

// ApplyNftablesConfirmed renders and applies the firewall ruleset from current
// resources.yaml without interactive confirm (caller already gated via
// SECURITY_AUTO_APPLY / Panel).
func ApplyNftablesConfirmed(root string) error {
	fmt.Println("防火墙：按 resources.yaml 渲染并应用规则集")
	return FirewallApplyConfirmed(root)
}
