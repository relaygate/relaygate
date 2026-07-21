package ops

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

// ListChanges prints recent backup change-summary.txt entries (newest first).
func ListChanges(root string, limit int, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}
	if limit <= 0 {
		limit = 20
	}
	entries, err := resources.ListChangeSummaries(root, limit)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(w, "尚无 change-summary（先执行 apply/reload）")
		return nil
	}
	for i, e := range entries {
		if i > 0 {
			fmt.Fprintln(w, "---")
		}
		fmt.Fprintf(w, "[%s] %s\n", e.Stamp, e.Path)
		fmt.Fprint(w, e.Summary)
		if !strings.HasSuffix(e.Summary, "\n") {
			fmt.Fprintln(w)
		}
	}
	return nil
}

// DrainHint prints NLB/high-protection maintenance checklist after drain fail.
func DrainHint(env Env, waitSec int, w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "==> NLB / 高防协同提示（不接云 SDK，请在控制台核对）")
	fmt.Fprintf(w, "  - DRAIN_WAIT=%ds（建议 ≥ %ds = NLB HC unhealthy_threshold×interval，见 packaging/terraform/nlb）\n",
		waitSec, config.RecommendedDrainWaitSec)
	WarnIfDrainWaitShort(waitSec, w)
	fmt.Fprintln(w, "  - 目标探测：Envoy admin /ready（NLB HTTP）或 admin 端口 TCP（UDP TG 常见）")
	fmt.Fprintf(w, "  - 本机 /ready: %s\n", env.AdminURL("/ready"))
	fmt.Fprintln(w, "  - 请在云控制台确认本实例已 unhealthy / draining 后再改配置、reload 或 upgrade")
	fmt.Fprintln(w, "  - 高防回源 IP 应等于各网关 GATEWAY_PUBLIC_IP（双活放行全部）")
	fmt.Fprintln(w, "  - 恢复：relaygate drain ok 或等 reload/upgrade 内置 undrain")
}

// WarnIfDrainWaitShort emits a hard WARN when waitSec is below the NLB-aligned recommendation.
func WarnIfDrainWaitShort(waitSec int, w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	if waitSec < config.RecommendedDrainWaitSec {
		fmt.Fprintf(w, "WARN: DRAIN_WAIT=%ds < 建议 %ds（NLB HC 3×10s）；摘流窗口可能不足，请调大 .env 中 DRAIN_WAIT\n",
			waitSec, config.RecommendedDrainWaitSec)
	}
}
