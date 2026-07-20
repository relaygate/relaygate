package ops

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Drain performs fail|ok|status against Envoy admin healthcheck endpoints.
func Drain(root string, action string) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	wait := env.DrainWait
	if os.Getenv("DRAIN_WAIT") == "" && (action == "fail" || action == "drain") {
		wait = 15
	}
	switch action {
	case "fail", "drain":
		fmt.Printf("==> %s: healthcheck/fail (drain)\n", env.GatewayName)
		if err := HTTPPost(env.AdminURL("/healthcheck/fail")); err != nil {
			return fmt.Errorf("healthcheck/fail: %w（Envoy 可能未运行或 admin 不可达）", err)
		}
		ready, _ := HTTPGet(env.AdminURL("/ready"))
		fmt.Print(ready)
		fmt.Println()
		bodyUpper := strings.ToUpper(strings.TrimSpace(ready))
		if strings.Contains(bodyUpper, "LIVE") {
			fmt.Println("WARN: /ready 仍含 LIVE；LB 可能尚未摘流，请拉长 DRAIN_WAIT 或核对 NLB HC")
		} else {
			fmt.Println("OK: /ready 已非 LIVE（NLB 应开始将本目标标为 unhealthy）")
		}
		fmt.Printf("等待 LB 健康检查失败窗口（建议 %ds）…\n", wait)
		time.Sleep(time.Duration(wait) * time.Second)
		// Re-check after wait
		ready2, _ := HTTPGet(env.AdminURL("/ready"))
		fmt.Printf("等待后 /ready: %s", strings.TrimSpace(ready2))
		fmt.Println()
		DrainHint(env, wait, os.Stdout)
	case "ok", "undrain":
		fmt.Printf("==> %s: healthcheck/ok (undrain)\n", env.GatewayName)
		if err := HTTPPost(env.AdminURL("/healthcheck/ok")); err != nil {
			return fmt.Errorf("healthcheck/ok: %w", err)
		}
		ready, err := HTTPGet(env.AdminURL("/ready"))
		fmt.Print(ready)
		fmt.Println()
		if strings.Contains(strings.ToUpper(strings.TrimSpace(ready)), "LIVE") {
			fmt.Println("OK: /ready=LIVE，可纳入 NLB；建议 smoke 验收")
		}
		return err
	case "status":
		fmt.Printf("==> %s ready:\n", env.GatewayName)
		ready, err := HTTPGet(env.AdminURL("/ready"))
		fmt.Print(ready)
		fmt.Println()
		fmt.Printf("DRAIN_WAIT=%ds PANEL_ROLE=%s\n", env.DrainWait, env.PanelRole)
		fmt.Printf("探活 URL: %s\n", env.AdminURL("/ready"))
		fmt.Println("NLB：HTTP 探 /ready 或 TCP 探 admin 端口；维护用 drain fail → 等窗口 → 变更 → drain ok")
		return err
	default:
		return fmt.Errorf("usage: relaygate drain fail|ok|status")
	}
	return nil
}

// DrainFailQuick posts healthcheck/fail without long wait (for reload).
// Returns error if admin is unreachable so callers can decide whether to abort.
func DrainFailQuick(root string, env Env, waitSec int) error {
	fmt.Printf("==> draining %s (/healthcheck/fail → LB 摘流)\n", env.EnvoyContainer())
	if err := HTTPPost(env.AdminURL("/healthcheck/fail")); err != nil {
		return fmt.Errorf("healthcheck/fail: %w", err)
	}
	if waitSec < 0 {
		waitSec = env.DrainWait
	}
	time.Sleep(time.Duration(waitSec) * time.Second)
	return nil
}
