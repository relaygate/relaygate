package ops

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

// Smoke runs post-deploy smoke checks; host is optional canary target (default 127.0.0.1).
func Smoke(root string, host string) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	fmt.Printf("==> smoke %s @ %s\n", env.GatewayName, host)

	fmt.Println("-- Envoy /ready --")
	ready, err := HTTPGet(env.AdminURL("/ready"))
	if err != nil {
		return fmt.Errorf("envoy /ready: %w", err)
	}
	fmt.Print(ready)
	fmt.Println()

	fmt.Println("-- Envoy /stats prometheus sample --")
	stats, err := HTTPGet(env.AdminURL("/stats/prometheus"))
	if err != nil {
		return fmt.Errorf("envoy stats: %w", err)
	}
	lines := strings.Split(stats, "\n")
	if len(lines) > 5 {
		lines = lines[:5]
	}
	_ = lines
	fmt.Println("stats OK")

	if HTTPGetOK("http://127.0.0.1:9090/-/ready") {
		fmt.Println("-- Prometheus ready --")
		out, _ := HTTPGet("http://127.0.0.1:9090/-/ready")
		fmt.Print(out)
		fmt.Println()
		fmt.Println("-- Prometheus external label gateway --")
		cfg, _ := HTTPGet("http://127.0.0.1:9090/api/v1/status/config")
		needle := "gateway: " + env.GatewayName
		if strings.Contains(cfg, needle) {
			fmt.Println("label OK")
		} else {
			warnf("未在 Prometheus config 中看到 %s（请先 render observability）", needle)
		}
	} else {
		warnf("Prometheus 未 ready，跳过标签检查")
	}

	cname := env.EnvoyContainer()
	if DockerInspectOK(cname) {
		fmt.Printf("-- container %s running --\n", cname)
		out, _ := RunCmdCapture(root, "docker", "inspect", "-f", "{{.State.Status}}", cname)
		fmt.Print(out)
	} else {
		warnf("容器 %s 不存在", cname)
	}

	_ = Canary(root, host) // best-effort like the shell script

	fmt.Printf("smoke OK: %s\n", env.GatewayName)
	return nil
}

// Canary probes TCP/UDP entry ports on host.
// Prefer enabled validation listen ports; if none, fall back to enabled production
// (common for live gateways with only正式入口). Env TCP_PORT/UDP_PORT is used only
// when resources.yaml cannot be loaded.
func Canary(root string, host string) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	tcpPort, udpPort, source, err := resolveCanaryPorts(root, env.TCPPort, env.UDPPort)
	if err != nil {
		return err
	}
	fmt.Printf("使用 %s 端口 TCP=%s UDP=%s\n", source, tcpPort, udpPort)
	timeoutSec, _ := strconv.Atoi(env.Timeout)
	if timeoutSec <= 0 {
		timeoutSec = 3
	}
	fmt.Printf("==> Canary 目标: %s TCP/UDP %s\n", host, tcpPort)

	fmt.Println("-- TCP connect --")
	if err := dialTCP(host, tcpPort, time.Duration(timeoutSec)*time.Second); err != nil {
		fmt.Println("TCP FAIL")
		return fmt.Errorf("TCP connect: %w", err)
	}
	fmt.Println("TCP OK")

	fmt.Println("-- UDP send --")
	_ = sendUDP(host, udpPort)
	fmt.Println("UDP datagram 已发送")

	if host == "127.0.0.1" || host == "localhost" {
		if HTTPGetOK(env.AdminURL("/ready")) {
			fmt.Println("Envoy /ready OK")
		}
		clusters, _ := HTTPGet(env.AdminURL("/clusters"))
		for _, line := range strings.Split(clusters, "\n") {
			if strings.Contains(line, "upstream-server-01") {
				fmt.Println(line)
			}
		}
	}
	fmt.Println("下一步: relaygate server enable <server> && relaygate reload")
	return nil
}

// resolveCanaryPorts picks probe ports from resources: validation → production.
// Env TCP_PORT/UDP_PORT is only used when resources.yaml cannot be loaded (bootstrap).
func resolveCanaryPorts(root, envTCP, envUDP string) (tcpPort, udpPort, source string, err error) {
	resPath := config.ResolvePaths(root).Resources
	res, loadErr := resources.Load(resPath)
	if loadErr != nil {
		fmt.Printf("WARN: 无法加载 resources（%v），回退 TCP_PORT/UDP_PORT env\n", loadErr)
		return applyCanaryPortPair(envTCP, envUDP, "TCP_PORT/UDP_PORT env")
	}
	fmt.Print(resources.FormatLifecycle(res))
	if t, u := res.ValidationListenPorts(); t > 0 || u > 0 {
		return applyCanaryPorts(t, u, "resources validation")
	}
	if t, u := res.ProductionListenPorts(); t > 0 || u > 0 {
		fmt.Println("INFO: 无启用的验证转发（validation），回退正式入口（production）")
		return applyCanaryPorts(t, u, "resources production")
	}
	return "", "", "", fmt.Errorf("无启用的 validation/production 入口可探测；请启用至少一条转发后 reload，或先配置验证入口")
}

func applyCanaryPorts(tcp, udp int, source string) (tcpPort, udpPort, src string, err error) {
	if tcp > 0 {
		tcpPort = strconv.Itoa(tcp)
	}
	if udp > 0 {
		udpPort = strconv.Itoa(udp)
	} else if tcp > 0 {
		udpPort = tcpPort
	}
	if tcpPort == "" && udpPort != "" {
		tcpPort = udpPort
	}
	if tcpPort == "" {
		return "", "", "", fmt.Errorf("未解析到可探测的 TCP/UDP 端口（来源 %s）", source)
	}
	if udpPort == "" {
		udpPort = tcpPort
	}
	return tcpPort, udpPort, source, nil
}

func applyCanaryPortPair(envTCP, envUDP, source string) (tcpPort, udpPort, src string, err error) {
	tcpPort = strings.TrimSpace(envTCP)
	udpPort = strings.TrimSpace(envUDP)
	if tcpPort == "" && udpPort != "" {
		tcpPort = udpPort
	}
	if udpPort == "" {
		udpPort = tcpPort
	}
	if tcpPort == "" {
		return "", "", "", fmt.Errorf("未设置 TCP_PORT/UDP_PORT，无法做 Canary 探测")
	}
	return tcpPort, udpPort, source, nil
}

func dialTCP(host, port string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		// fallback nc if present
		if LookPath("nc") {
			cmd := exec.Command("nc", "-z", "-w", strconv.Itoa(int(timeout.Seconds())), host, port)
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			return cmd.Run()
		}
		return err
	}
	_ = conn.Close()
	return nil
}

func sendUDP(host, port string) error {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, err = conn.Write([]byte("canary-ping\n"))
	return err
}
