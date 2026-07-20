package doctor

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/ops"
	"github.com/relaygate/relaygate/core/resources"
)

// Options for preflight checks.
type Options struct {
	Root        string
	EnablePanel bool
	EnableGraf  bool
	StrictPorts bool // fail on occupied ports
}

// Run performs env/docker/ports/envoy readiness checks.
func Run(opt Options) error {
	if opt.Root == "" {
		return fmt.Errorf("root required")
	}
	var failures []string
	check := func(name string, fn func() error) {
		fmt.Printf("-- %s --\n", name)
		if err := fn(); err != nil {
			fmt.Printf("FAIL: %v\n", err)
			failures = append(failures, name+": "+err.Error())
			return
		}
		fmt.Println("OK")
	}

	env, err := ops.LoadEnv(opt.Root)
	if err != nil {
		return err
	}
	if opt.EnablePanel || env.EnablePanel == "1" {
		opt.EnablePanel = true
	}
	if opt.EnableGraf || strings.Contains(env.Raw["COMPOSE_PROFILES"], "with-grafana") {
		opt.EnableGraf = true
	}

	check(".env", func() error {
		if _, err := os.Stat(filepath.Join(opt.Root, ".env")); err != nil {
			return fmt.Errorf("缺少 .env（先跑 relaygate setup）")
		}
		if env.GatewayName == "" {
			return fmt.Errorf("GATEWAY_NAME 为空")
		}
		return nil
	})

	check("resources.yaml", func() error {
		p := config.ResolvePaths(opt.Root).Resources
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("缺少 %s（先跑 relaygate setup）", p)
		}
		res, err := resources.Load(p)
		if err != nil {
			return err
		}
		if err := res.Validate(); err != nil {
			return fmt.Errorf("校验失败: %w", err)
		}
		return nil
	})

	check("docker", func() error {
		if !ops.LookPath("docker") {
			return fmt.Errorf("docker 未安装")
		}
		if out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
			return fmt.Errorf("docker daemon 不可用: %s", strings.TrimSpace(string(out)))
		}
		if err := exec.Command("docker", "compose", "version").Run(); err != nil {
			return fmt.Errorf("docker compose v2 不可用")
		}
		return nil
	})

	check("binary", func() error {
		bin := filepath.Join(opt.Root, "bin", "relaygate")
		st, err := os.Stat(bin)
		if err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			if ops.LookPath("relaygate") {
				return nil
			}
			return fmt.Errorf("缺少 %s", bin)
		}
		return nil
	})

	check("ports", func() error {
		ports := []int{9901, 9090, 9100}
		if opt.EnablePanel {
			ports = append(ports, 9000)
		}
		if opt.EnableGraf {
			ports = append(ports, 3000)
		}
		var occupied []string
		for _, p := range ports {
			if portListening(p) {
				occupied = append(occupied, strconv.Itoa(p))
			}
		}
		if len(occupied) == 0 {
			return nil
		}
		msg := fmt.Sprintf("端口已占用: %s", strings.Join(occupied, ", "))
		if opt.StrictPorts {
			return fmt.Errorf("%s", msg)
		}
		fmt.Printf("WARN: %s（若已是本机 RelayGate 可忽略）\n", msg)
		return nil
	})

	check("dual-active env", func() error {
		role := strings.ToLower(strings.TrimSpace(env.PanelRole))
		if role != "primary" && role != "standby" {
			return fmt.Errorf("PANEL_ROLE=%q 无效（须 primary|standby）", env.PanelRole)
		}
		if role == "standby" && env.EnablePanel == "1" {
			fmt.Println("WARN: PANEL_ROLE=standby 但 ENABLE_PANEL=1（从节点建议 ENABLE_PANEL=0；若误启则 Panel 写保护）")
		}
		if role == "primary" && env.EnablePanel != "1" {
			fmt.Println("WARN: PANEL_ROLE=primary 但 ENABLE_PANEL!=1（主管理节点通常需要 Panel）")
		}
		fmt.Printf("PANEL_ROLE=%s ENABLE_PANEL=%s DRAIN_WAIT=%ds ENVOY_ADMIN_PORT=%s\n",
			env.PanelRole, env.EnablePanel, env.DrainWait, env.EnvoyAdminPort)
		return nil
	})

	check("envoy admin", func() error {
		port := env.EnvoyAdminPort
		if port == "" {
			return fmt.Errorf("ENVOY_ADMIN_PORT 为空")
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("ENVOY_ADMIN_PORT 无效: %s", port)
		}
		resPath := config.ResolvePaths(opt.Root).Resources
		if res, err := resources.Load(resPath); err == nil {
			addr := strings.TrimSpace(res.Meta.AdminAddress)
			if addr != "" && addr != "127.0.0.1" && addr != "::1" {
				fmt.Printf("WARN: resources meta.admin_address=%s（建议 127.0.0.1，勿对公网暴露 admin）\n", addr)
			}
			if res.Meta.AdminPort > 0 && strconv.Itoa(res.Meta.AdminPort) != port {
				fmt.Printf("WARN: resources admin_port=%d 与 ENVOY_ADMIN_PORT=%s 不一致\n", res.Meta.AdminPort, port)
			}
		}
		url := env.AdminURL("/ready")
		if !ops.HTTPGetOK(url) {
			return fmt.Errorf("Envoy admin 不可达（%s）；部署后应可访问", url)
		}
		return nil
	})

	check("envoy ready", func() error {
		url := env.AdminURL("/ready")
		if ops.HTTPGetOK(url) {
			return nil
		}
		return fmt.Errorf("Envoy 未 ready（%s）；部署后应可访问", url)
	})

	check("drain endpoints", func() error {
		readyURL := env.AdminURL("/ready")
		if !ops.HTTPGetOK(readyURL) {
			return fmt.Errorf("Envoy 未运行，无法检查 /healthcheck/*（%s）", readyURL)
		}
		failURL := env.AdminURL("/healthcheck/fail")
		okURL := env.AdminURL("/healthcheck/ok")
		client := &http.Client{Timeout: 2 * time.Second}
		for _, u := range []string{failURL, okURL} {
			resp, err := client.Get(u)
			if err != nil {
				return fmt.Errorf("无法连接 %s: %w（drain 将失败）", u, err)
			}
			_ = resp.Body.Close()
			fmt.Printf("reachable %s (HTTP %d)\n", u, resp.StatusCode)
		}
		fmt.Printf("DRAIN_WAIT=%ds（CLI drain fail 未设 env 时默认等 15s；reload 用此值）\n", env.DrainWait)
		return nil
	})

	if opt.EnablePanel {
		check("panel login", func() error {
			if ops.HTTPGetOK("http://127.0.0.1:9000/login") {
				return nil
			}
			return fmt.Errorf("Panel 未响应 http://127.0.0.1:9000/login")
		})
	}

	fmt.Println()
	if len(failures) > 0 {
		fmt.Println("doctor 发现以下问题:")
		for _, f := range failures {
			fmt.Println(" -", f)
		}
		hard := 0
		for _, f := range failures {
			if strings.HasPrefix(f, ".env") || strings.HasPrefix(f, "resources") ||
				strings.HasPrefix(f, "docker") || strings.HasPrefix(f, "binary") ||
				strings.HasPrefix(f, "dual-active") {
				hard++
			}
			if opt.StrictPorts && strings.HasPrefix(f, "ports") {
				hard++
			}
		}
		if hard > 0 {
			return fmt.Errorf("doctor 失败（%d 项）", hard)
		}
		fmt.Println("（Envoy/Panel/drain 未就绪在首次 apply 前属预期）")
	}
	fmt.Println("doctor 完成")
	return nil
}

func portListening(port int) bool {
	if ops.LookPath("ss") {
		out, err := exec.Command("ss", "-H", "-lnt", fmt.Sprintf("( sport = :%d )", port)).CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return true
		}
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true
	}
	_ = ln.Close()
	time.Sleep(10 * time.Millisecond)
	return false
}
