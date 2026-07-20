package ops

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"
)

// Reload renders config, drains, restarts Envoy, waits for ready.
// When RELAYGATE_PRIVILEGED_HELPER is set and not root, re-execs via sudo.
func Reload(root string) error {
	return ReloadTo(root, os.Stdout, os.Stderr)
}

// ReloadTo is like Reload but writes progress to the given writers (Panel capture).
// Stages are logged with elapsed timing for L4-friendly observability:
//   backup → render → validate → drain → restart → ready → undrain
func ReloadTo(root string, stdout, stderr io.Writer) error {
	if handled, err := maybePrivilegedReexec(stdout, stderr); handled {
		return err
	}
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}

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
	err := ReloadTo(root, &buf, &buf)
	return buf.String(), err
}

// DrainFailQuickQuiet is DrainFailQuick with progress to a writer.
func DrainFailQuickQuiet(root string, env Env, waitSec int, w io.Writer) error {
	logf(w, "draining %s (/healthcheck/fail → LB 摘流, wait=%ds)", env.EnvoyContainer(), waitSec)
	if err := HTTPPost(env.AdminURL("/healthcheck/fail")); err != nil {
		return fmt.Errorf("healthcheck/fail: %w", err)
	}
	if waitSec < 0 {
		waitSec = env.DrainWait
	}
	time.Sleep(time.Duration(waitSec) * time.Second)
	return nil
}
