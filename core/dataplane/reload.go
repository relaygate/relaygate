package dataplane

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/resources"
)

// Reload renders config, drains, restarts Envoy, waits for ready.
// When RELAYGATE_PRIVILEGED_HELPER is set and not root, re-execs via sudo.
//
// Connection impact on HardReload (NeedsHardReload, --hard, or XDS off): docker
// restart / --force-recreate terminates ALL existing L4 TCP/UDP flows on this
// gateway. Drain only flips /healthcheck so NLB stops new targets; it does NOT
// preserve established connections.
//
// Default path (XDS on): CanHotApply → HotApply (CDS/LDS, no drain).
// HardReloadTo remains for bootstrap/image/NeedsHardReload and escape hatches.
func Reload(root string) error {
	return ReloadTo(root, os.Stdout, os.Stderr, ReloadOptions{})
}

// ReloadOptions controls reload routing.
type ReloadOptions struct {
	ForceHard bool // CLI --hard: always drain+restart
}

// ReloadTo is like Reload but writes progress to the given writers (Panel capture).
// HotApply is preferred for CanHotApply diffs unless ForceHard is set or
// bootstrap is not yet ADS (falls back to HardReloadTo).
func ReloadTo(root string, stdout, stderr io.Writer, opts ReloadOptions) error {
	if handled, err := maybePrivilegedReexec(stdout, stderr, "reload"); handled {
		return err
	}
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}

	if opts.ForceHard {
		logf(stdout, "reload --hard: forcing drain+restart")
		return HardReloadTo(root, env, stdout, stderr)
	}

	if env.XDSEnabled {
		paths := config.ResolvePaths(root)
		res, err := resources.Load(paths.Resources)
		if err != nil {
			return err
		}
		before, _, _ := resources.LoadPreviousBackupResources(root)
		plan := resources.Diff(before, res).Classify()
		if !plan.NeedsReload && !plan.NeedsHardReload {
			logf(stdout, "无 Envoy 变更，跳过 reload")
			return nil
		}
		migrated := render.IsBootstrapMigrated(paths.EnvoyYAML)
		if plan.CanHotApply && !plan.NeedsHardReload && migrated {
			logf(stdout, "HotApply (no drain)")
			return HotApplyTo(root, env, stdout, stderr)
		}
		if plan.CanHotApply && !plan.NeedsHardReload && !migrated {
			logf(stdout, "bootstrap 尚无 dynamic_resources：改走 HardReload")
		} else if plan.NeedsHardReload {
			logf(stdout, "NeedsHardReload (meta/bootstrap): falling back to HardReload")
		}
	}

	return HardReloadTo(root, env, stdout, stderr)
}

// HardReloadTo is the classic drain → docker restart / force-recreate → ready path.
// Kept as the permanent fallback for bootstrap/meta/image changes and when xDS is off.
func HardReloadTo(root string, env Env, stdout, stderr io.Writer) error {
	logf(stdout, "WARN: 将重启本机 Envoy，会断开本网关上全部现有连接")

	totalStart := time.Now()
	stage := func(name string, fn func() error) error {
		start := time.Now()
		logf(stdout, "==> [%s] start", name)
		err := fn()
		elapsed := time.Since(start).Round(time.Millisecond)
		if err != nil {
			logf(stdout, "==> [%s] FAIL (%s): %v", name, elapsed, err)
			return err
		}
		logf(stdout, "==> [%s] ok (%s)", name, elapsed)
		return nil
	}

	if err := stage("backup", func() error {
		stamp, dir, _, err := BackupWithSummary(root, stdout)
		if err != nil {
			return err
		}
		logf(stdout, "backup stamp=%s dir=%s", stamp, dir)
		return nil
	}); err != nil {
		return err
	}

	if err := stage("render", func() error {
		return RenderConfig(root, false)
	}); err != nil {
		return err
	}

	_ = stage("envoy-validate", func() error {
		return ValidateEnvoyContainer(root, env, true)
	})

	WarnIfDrainWaitShort(env.DrainWait, stdout)

	if err := stage("drain", func() error {
		if err := DrainFailQuickQuiet(root, env, env.DrainWait, stdout); err != nil {
			logf(stdout, "WARN: drain 失败，继续 reload（Envoy 可能尚未运行）: %v", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := stage("restart", func() error {
		if DockerInspectOK(env.EnvoyContainer()) {
			return RunCmdIO(root, stdout, stderr, "docker", "restart", env.EnvoyContainer())
		}
		args := append(ComposeArgs(root, true), "up", "-d", "--force-recreate", "--no-deps", "envoy")
		return RunCmdIO(root, stdout, stderr, "docker", args...)
	}); err != nil {
		return err
	}

	if err := stage("ready", func() error {
		readyURL := env.AdminURL("/ready")
		for i := 0; i < 45; i++ {
			if HTTPGetOK(readyURL) {
				return nil
			}
			time.Sleep(2 * time.Second)
		}
		return fmt.Errorf("Envoy 未 ready (%s)", env.GatewayName)
	}); err != nil {
		return err
	}

	_ = stage("undrain", func() error {
		if err := HTTPPost(env.AdminURL("/healthcheck/ok")); err != nil {
			logf(stdout, "WARN: healthcheck/ok: %v", err)
		}
		return nil
	})

	logf(stdout, "Envoy reloaded and ready (%s) total=%s DRAIN_WAIT=%ds",
		env.GatewayName, time.Since(totalStart).Round(time.Millisecond), env.DrainWait)
	return nil
}

// ReloadCapture runs Reload and returns combined output.
func ReloadCapture(root string) (string, error) {
	var buf bytes.Buffer
	err := ReloadTo(root, &buf, &buf, ReloadOptions{})
	return buf.String(), err
}

// DrainFailQuickQuiet is DrainFailQuick with progress to a writer.
func DrainFailQuickQuiet(root string, env Env, waitSec int, w io.Writer) error {
	if waitSec < 0 {
		waitSec = env.DrainWait
	}
	logf(w, "draining %s (/healthcheck/fail → LB 摘流, wait=%ds, 建议≥%ds)",
		env.EnvoyContainer(), waitSec, config.RecommendedDrainWaitSec)
	WarnIfDrainWaitShort(waitSec, w)
	if err := HTTPPost(env.AdminURL("/healthcheck/fail")); err != nil {
		return fmt.Errorf("healthcheck/fail: %w", err)
	}
	time.Sleep(time.Duration(waitSec) * time.Second)
	return nil
}