package ops

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Baseline collects a read-only host baseline into backups/baseline-*.txt.
func Baseline(root string, outPath string) error {
	_ = LoadDotEnv(filepath.Join(root, ".env"))
	env, _ := LoadEnv(root)
	_ = os.MkdirAll(filepath.Join(root, "data", "backups"), 0o755)
	if outPath == "" {
		outPath = filepath.Join(root, "data", "backups", "baseline-"+time.Now().Format("20060102-150405")+".txt")
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := func(format string, args ...any) {
		fmt.Fprintf(f, format+"\n", args...)
		fmt.Printf(format+"\n", args...)
	}
	runSection := func(title string, name string, args ...string) {
		w("## %s", title)
		cmd := exec.Command(name, args...)
		cmd.Stdout = f
		cmd.Stderr = f
		_ = cmd.Run()
		// also mirror to stdout roughly
		out, _ := exec.Command(name, args...).CombinedOutput()
		fmt.Print(string(out))
		w("")
	}

	w("# %s runtime baseline collected at %s", env.GatewayName, time.Now().Format(time.RFC3339))
	w("")
	runSection("uname", "uname", "-a")
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		w("## os-release")
		w("%s", string(b))
	}
	runSection("cpu", "nproc")
	runSection("memory", "free", "-h")
	runSection("disk", "df", "-h", "/")
	runSection("addresses", "ip", "-br", "addr")
	runSection("routes", "ip", "route")
	runSection("listening sockets", "ss", "-lntup")
	w("## docker")
	if LookPath("docker") {
		out, _ := exec.Command("docker", "--version").CombinedOutput()
		w("%s", strings.TrimSpace(string(out)))
		out, _ = exec.Command("systemctl", "is-active", "docker").CombinedOutput()
		w("%s", strings.TrimSpace(string(out)))
		out, _ = exec.Command("docker", "ps", "--format", "table {{.Names}}\t{{.Status}}\t{{.Image}}").CombinedOutput()
		w("%s", string(out))
	} else {
		w("docker not installed")
	}
	w("## sysctl")
	out, _ := exec.Command("sysctl",
		"net.core.somaxconn", "net.ipv4.ip_local_port_range",
		"net.core.rmem_max", "net.core.wmem_max", "fs.file-max").CombinedOutput()
	w("%s", string(out))
	w("## firewall")
	if LookPath("nft") {
		out, _ := exec.Command("nft", "list", "ruleset").CombinedOutput()
		w("%s", string(out))
	} else if LookPath("iptables") {
		out, _ := exec.Command("iptables", "-S").CombinedOutput()
		w("%s", string(out))
	}
	fmt.Printf("\n已写入 %s\n", outPath)
	return nil
}

// Fleet deploys gateways from inventory one-by-one: drain → sync → reload → smoke.
func Fleet(root string, gatewaysCSV string) error {
	inventory := getenv("INVENTORY", filepath.Join(root, "data", "inventory", "gateways.env"))
	if _, err := os.Stat(inventory); err != nil {
		return fmt.Errorf("缺少 inventory: %s（请复制 gateways.env.example → data/inventory/gateways.env）", inventory)
	}
	vars, err := parseInventory(inventory)
	if err != nil {
		return err
	}
	if gatewaysCSV == "" {
		gatewaysCSV = getenv("GATEWAYS", vars["GATEWAY_MATRIX"])
	}
	if gatewaysCSV == "" {
		gatewaysCSV = "gateway-01,gateway-02"
	}
	sshOpts := strings.Fields(getenv("SSH_OPTS", "-o StrictHostKeyChecking=accept-new -o BatchMode=yes"))
	imageTag := os.Getenv("IMAGE_TAG")
	deployRef := strings.TrimSpace(os.Getenv("DEPLOY_REF"))
	if deployRef == "" {
		if b, err := os.ReadFile(filepath.Join(root, "RELEASE")); err == nil {
			deployRef = strings.TrimSpace(string(b))
		}
	}
	switch deployRef {
	case "", "master", "main", "latest":
		return fmt.Errorf("DEPLOY_REF 必须是不可变 tag 或 commit SHA（当前 %q）；请 export DEPLOY_REF=<tag|sha> 或写入 RELEASE", deployRef)
	}
	pauseSec := 10
	if v := os.Getenv("BATCH_PAUSE_SEC"); v != "" {
		fmt.Sscanf(v, "%d", &pauseSec)
	}

	for _, gw := range strings.Split(gatewaysCSV, ",") {
		gw = strings.TrimSpace(gw)
		if gw == "" {
			continue
		}
		key := strings.ReplaceAll(gw, "-", "_")
		host := vars["HOST_"+key]
		port := vars["SSH_PORT_"+key]
		if port == "" {
			port = "30455"
		}
		user := vars["SSH_USER_"+key]
		if user == "" {
			user = "root"
		}
		rdir := vars["REMOTE_DIR_"+key]
		if rdir == "" {
			rdir = "/opt/relaygate"
		}
		if host == "" {
			return fmt.Errorf("inventory 未定义 HOST_%s", key)
		}
		fmt.Printf("\n========== 分批部署: %s (%s@%s:%s) ==========\n", gw, user, host, port)
		sshBase := append([]string{}, sshOpts...)
		sshBase = append(sshBase, "-p", port, user+"@"+host)

		remote := func(script string) error {
			args := append(sshBase, script)
			return RunCmd(root, "ssh", args...)
		}

		fmt.Println("==> 1/5 drain")
		if err := remote(fmt.Sprintf("cd '%s' && ./bin/relaygate drain fail", rdir)); err != nil {
			return err
		}
		fmt.Println("==> 2/5 sync git / artifact")
		if err := remote(fmt.Sprintf("cd '%s' && git fetch --all && git checkout %q && git pull --ff-only", rdir, deployRef)); err != nil {
			return err
		}
		if imageTag != "" {
			sed := fmt.Sprintf(`cd '%s' && sed -i 's/^IMAGE_TAG=.*/IMAGE_TAG=%s/' .env || echo IMAGE_TAG=%s >> .env`, rdir, imageTag, imageTag)
			_ = remote(sed)
		}
		fmt.Println("==> 3/5 render + reload envoy")
		if err := remote(fmt.Sprintf("cd '%s' && ./bin/relaygate render --observability && ./bin/relaygate reload", rdir)); err != nil {
			return err
		}
		fmt.Println("==> 4/5 undrain + smoke")
		if err := remote(fmt.Sprintf("cd '%s' && ./bin/relaygate drain ok && ./bin/relaygate smoke 127.0.0.1", rdir)); err != nil {
			return err
		}
		fmt.Printf("==> 5/5 %s 完成，继续下一台前短暂等待\n", gw)
		time.Sleep(time.Duration(pauseSec) * time.Second)
	}
	fmt.Printf("\n全部分批部署完成: %s\n", gatewaysCSV)
	fmt.Println("回滚单台: ssh … 'cd /opt/relaygate && ./bin/relaygate rollback'")
	return nil
}

func parseInventory(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return m, sc.Err()
}
