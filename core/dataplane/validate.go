package dataplane

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/relaygate/relaygate/core/config"
)

// RenderObservability renders DataDir/prometheus/prometheus.yml from packaging template + env.
func RenderObservability(root string) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	p := config.ResolvePaths(root)
	tpl := filepath.Join(p.Packaging, "prometheus", "prometheus.yml.tpl")
	out := p.PromYAML
	b, err := os.ReadFile(tpl)
	if err != nil {
		return fmt.Errorf("missing %s: %w", tpl, err)
	}
	content := string(b)
	repl := map[string]string{
		"${GATEWAY_NAME}":                env.GatewayName,
		"${ENVOY_ADMIN_PORT}":            env.EnvoyAdminPort,
		"${PROMETHEUS_REMOTE_WRITE_URL}": env.PrometheusRemoteWriteURL,
	}
	for k, v := range repl {
		content = strings.ReplaceAll(content, k, v)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	// Prometheus 容器以 nobody 运行，须能读 credentials_file；勿直接挂 0600 的 secrets。
	credPath := filepath.Join(filepath.Dir(out), "agent.token")
	if err := syncPrometheusAgentToken(env, credPath); err != nil {
		return err
	}
	if strings.TrimSpace(env.PrometheusRemoteWriteURL) != "" {
		rw := fmt.Sprintf(`

remote_write:
  - url: %s
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/agent.token
    queue_config:
      max_samples_per_send: 1000
      capacity: 10000
      max_shards: 4
`, env.PrometheusRemoteWriteURL)
		content += rw
	}
	// 主控（control）接 Alertmanager，使 gateway-alerts.yml 可真正投递。
	// 节点仅指标栈时不写 alerting，避免连不上本机 :9093。
	if shouldWireAlertmanager(env) {
		amURL := firstNonEmpty(
			os.Getenv("ALERTMANAGER_URL"),
			env.Raw["ALERTMANAGER_URL"],
			"http://127.0.0.1:9093",
		)
		content += fmt.Sprintf(`

alerting:
  alertmanagers:
    - static_configs:
        - targets: ["%s"]
`, alertmanagerStaticTarget(amURL))
	}
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, out); err != nil {
		return err
	}
	fmt.Printf("rendered %s (gateway=%s)\n", out, env.GatewayName)
	return nil
}

// shouldWireAlertmanager is true on control observability stacks or when
// ALERTMANAGER_URL is set. Nodes with metrics-only skip it.
func shouldWireAlertmanager(env Env) bool {
	if strings.TrimSpace(firstNonEmpty(os.Getenv("ALERTMANAGER_URL"), env.Raw["ALERTMANAGER_URL"])) != "" {
		return true
	}
	profiles := env.Raw["COMPOSE_PROFILES"]
	if profiles == "" {
		profiles = os.Getenv("COMPOSE_PROFILES")
	}
	for _, p := range strings.Split(profiles, ",") {
		switch strings.TrimSpace(p) {
		case "control", "with-grafana", "with-alerts": // with-*: pre-migration
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// alertmanagerStaticTarget turns http://host:port into host:port for Prometheus.
func alertmanagerStaticTarget(amURL string) string {
	u := strings.TrimSpace(amURL)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimSuffix(u, "/")
	if u == "" {
		return "127.0.0.1:9093"
	}
	return u
}

// syncPrometheusAgentToken copies AGENT_TOKEN_FILE into DataDir/prometheus/agent.token
// with mode 0644 so the Prometheus container (nobody) can read Bearer credentials.
// Always ensures the path exists so compose volume mounts succeed on control hosts.
func syncPrometheusAgentToken(env Env, dest string) error {
	src := strings.TrimSpace(os.Getenv("AGENT_TOKEN_FILE"))
	if src == "" {
		src = strings.TrimSpace(env.Raw["AGENT_TOKEN_FILE"])
	}
	if src != "" {
		b, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("读取 AGENT_TOKEN_FILE: %w", err)
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, dest)
	}
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return os.WriteFile(dest, nil, 0o644)
	}
	return nil
}

// ValidateEnvoyContainer runs envoy --mode validate via docker (best-effort if skipWarn).
func ValidateEnvoyContainer(root string, env Env, skipWarn bool) error {
	if !LookPath("docker") {
		if skipWarn {
			warnf("未安装 docker，跳过 envoy --mode validate")
			return nil
		}
		return fmt.Errorf("docker 不可用")
	}
	envoyYAML := config.ResolvePaths(root).EnvoyYAML
	if _, err := os.Stat(envoyYAML); err != nil {
		return fmt.Errorf("缺少 %s", envoyYAML)
	}
	logsDir := filepath.Join(config.ResolvePaths(root).DataDir, "envoy", "logs")
	if err := os.MkdirAll(logsDir, 0o777); err != nil {
		return fmt.Errorf("创建 Envoy 日志目录: %w", err)
	}
	err := RunCmd(root, "docker", "run", "--rm",
		"-v", envoyYAML+":/etc/envoy/envoy.yaml:ro",
		"-v", logsDir+":/var/log/envoy",
		env.EnvoyImage,
		"/usr/local/bin/envoy", "-c", "/etc/envoy/envoy.yaml", "--mode", "validate")
	if err != nil && skipWarn {
		warnf("envoy validate 容器未能运行，继续")
		return nil
	}
	return err
}

func ValidateNFT(root string) error {
	if !LookPath("nft") {
		warnf("未安装 nft，跳过防火墙语法检查")
		return nil
	}
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	_, path, err := writeFirewallRuntime(root, env.GatewaySSHPort)
	if err != nil {
		return err
	}
	return RunCmd(filepath.Dir(path), "nft", "-c", "-f", filepath.Base(path))
}

func ValidateCompose(root string, env Env) error {
	if !LookPath("docker") {
		return nil
	}
	args := []string{"compose", "-f", config.ComposeFileRel}
	envFile := filepath.Join(root, ".env")
	if _, err := os.Stat(envFile); err == nil {
		args = append(args, "--env-file", ".env", "config")
		return RunCmdIO(root, io.Discard, os.Stderr, "docker", args...)
	}
	cmd := exec.Command("docker", append(args, "config")...)
	cmd.Dir = root
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"GATEWAY_NAME="+env.GatewayName,
		"RELAYGATE_DATA_DIR="+env.DataDir,
	)
	return cmd.Run()
}

// Validate renders once then checks envoy/nft/compose. Fail-fast; no nested re-probes.
func Validate(root string) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	fmt.Printf("==> render (gateway=%s data=%s)\n", env.GatewayName, env.DataDir)
	if err := RenderObservability(root); err != nil {
		return err
	}
	if err := RenderConfig(root, false); err != nil {
		return err
	}

	prom, err := os.ReadFile(config.ResolvePaths(root).PromYAML)
	if err != nil {
		return err
	}
	needle := "gateway: " + env.GatewayName
	if !strings.Contains(string(prom), needle) {
		return fmt.Errorf("prometheus.yml 缺少 %q", needle)
	}

	fmt.Println("==> Envoy --mode validate")
	if err := ValidateEnvoyContainer(root, env, true); err != nil {
		return err
	}
	fmt.Println("==> nftables 语法检查")
	if err := ValidateNFT(root); err != nil {
		return err
	}
	// Ensure forward-ports.nft carries resources-derived rate limit defines
	nftBody, err := os.ReadFile(config.ResolvePaths(root).ForwardPorts)
	if err != nil {
		return fmt.Errorf("读取 forward-ports.nft: %w", err)
	}
	for _, needle := range []string{
		"FORWARD_TCP_PORTS", "FORWARD_UDP_PORTS",
		"FORWARD_TCP_NEW_CONN_RATE", "FORWARD_UDP_PPS_RATE",
		"ACL_DENY", "ACL_ALLOW", "ACL_ALLOW_STRICT",
	} {
		if !strings.Contains(string(nftBody), needle) {
			return fmt.Errorf("forward-ports.nft 缺少 %s（resources→nft 同源渲染失败）", needle)
		}
	}
	if LookPath("docker") {
		fmt.Println("==> compose 配置检查")
		if err := ValidateCompose(root, env); err != nil {
			return fmt.Errorf("compose config: %w", err)
		}
	}
	if GrafanaEnabled(env) {
		fmt.Println("==> Grafana 挂载源检查")
		if err := EnsureGrafanaRuntime(root, env); err != nil {
			return err
		}
	}
	fmt.Println("全部校验通过")
	return nil
}
