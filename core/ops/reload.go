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
func ReloadTo(root string, stdout, stderr io.Writer) error {
	if handled, err := maybePrivilegedReexec(stdout, stderr); handled {
		return err
	}
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}

	if err := RenderConfig(root, false); err != nil {
		return err
	}

	_ = ValidateEnvoyContainer(root, env, true)

	if err := DrainFailQuick(root, env, env.DrainWait); err != nil {
		logf(stdout, "WARN: drain 失败，继续 reload（Envoy 可能尚未运行）: %v", err)
	}

	if DockerInspectOK(env.EnvoyContainer()) {
		if err := RunCmdIO(root, stdout, stderr, "docker", "restart", env.EnvoyContainer()); err != nil {
			return err
		}
	} else {
		args := append(ComposeArgs(root, true), "up", "-d", "--force-recreate", "--no-deps", "envoy")
		if err := RunCmdIO(root, stdout, stderr, "docker", args...); err != nil {
			return err
		}
	}

	readyURL := env.AdminURL("/ready")
	for i := 0; i < 45; i++ {
		if HTTPGetOK(readyURL) {
			_ = HTTPPost(env.AdminURL("/healthcheck/ok"))
			logf(stdout, "Envoy reloaded and ready (%s)", env.GatewayName)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("Envoy 未 ready (%s)", env.GatewayName)
}

// ReloadCapture runs Reload and returns combined output.
func ReloadCapture(root string) (string, error) {
	var buf bytes.Buffer
	err := ReloadTo(root, &buf, &buf)
	return buf.String(), err
}
