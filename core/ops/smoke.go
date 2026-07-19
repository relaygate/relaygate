package ops

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

// Canary probes TCP/UDP canary ports on host.
func Canary(root string, host string) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	tcpPort := env.TCPPort
	udpPort := env.UDPPort
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
			if strings.Contains(line, "cluster-server-01") {
				fmt.Println(line)
			}
		}
	}
	fmt.Println("下一步: relaygate server enable server-01 && relaygate reload")
	return nil
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
