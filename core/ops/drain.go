package ops

import (
	"fmt"
	"os"
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
		fmt.Printf("等待 LB 健康检查失败窗口（建议 %ds）…\n", wait)
		time.Sleep(time.Duration(wait) * time.Second)
	case "ok", "undrain":
		fmt.Printf("==> %s: healthcheck/ok (undrain)\n", env.GatewayName)
		if err := HTTPPost(env.AdminURL("/healthcheck/ok")); err != nil {
			return fmt.Errorf("healthcheck/ok: %w", err)
		}
		ready, err := HTTPGet(env.AdminURL("/ready"))
		fmt.Print(ready)
		fmt.Println()
		return err
	case "status":
		fmt.Printf("==> %s ready:\n", env.GatewayName)
		ready, err := HTTPGet(env.AdminURL("/ready"))
		fmt.Print(ready)
		fmt.Println()
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
