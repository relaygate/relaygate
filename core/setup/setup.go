package setup

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/dataplane"
)

// defaultAdminPassword is written when panel/grafana secret files are first created.
// Weak by design for local/bootstrap; change in production.
const defaultAdminPassword = "relaygate"

var gatewayNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// Options for first-boot configuration.
type Options struct {
	Root           string
	NonInteractive bool
	GatewayName    string
	PublicIP       string
	SSHPort        string
	EnablePanel    string
	EnableGrafana  string
	SecretsDir     string
	ImageTag       string
	ApplySysctl    bool
	Upgrade        bool
	// ResetDefaults overwrites DataDir/resources.yaml and inventory from versioned templates.
	// Never silently overwrite without this flag.
	ResetDefaults bool
}

// Run writes/updates .env and secrets scaffolding, then seeds DataDir defaults.
func Run(opt Options) error {
	if opt.Root == "" {
		return fmt.Errorf("root required")
	}
	if opt.SecretsDir == "" {
		opt.SecretsDir = config.Getenv("RELAYGATE_SECRETS_DIR", config.DefaultSecretsDir)
	}
	if opt.NonInteractive || os.Getenv("NONINTERACTIVE") == "1" {
		opt.NonInteractive = true
	}
	if v := os.Getenv("GATEWAY_NAME"); v != "" && opt.GatewayName == "" {
		opt.GatewayName = v
	}
	if v := os.Getenv("GATEWAY_PUBLIC_IP"); v != "" && opt.PublicIP == "" {
		opt.PublicIP = v
	}
	if v := os.Getenv("GATEWAY_SSH_PORT"); v != "" && opt.SSHPort == "" {
		opt.SSHPort = v
	}
	if v := os.Getenv("ENABLE_PANEL"); v != "" && opt.EnablePanel == "" {
		opt.EnablePanel = v
	}
	if v := os.Getenv("ENABLE_GRAFANA"); v != "" && opt.EnableGrafana == "" {
		opt.EnableGrafana = v
	}

	var err error
	opt, err = collectSettings(opt)
	if err != nil {
		return err
	}
	if err := writeEnv(opt); err != nil {
		return err
	}
	if err := ensureSecrets(opt); err != nil {
		return err
	}
	if err := dataplane.SeedDefaults(opt.Root, opt.ResetDefaults); err != nil {
		return err
	}
	if opt.ApplySysctl {
		_ = dataplane.ApplySysctl(opt.Root, opt.GatewayName)
	}
	fmt.Println("==> setup 完成")
	fmt.Printf("产品根: %s\n", opt.Root)
	fmt.Printf("运行态 DataDir: %s\n", config.ResolveDataDir(opt.Root))
	fmt.Printf("配置: %s/.env；密钥: %s\n", opt.Root, opt.SecretsDir)
	return nil
}

func collectSettings(opt Options) (Options, error) {
	if opt.SSHPort == "" {
		opt.SSHPort = detectSSHPort()
	}
	detectedIP := ""
	if opt.PublicIP == "" {
		out, _ := exec.Command("curl", "-4fsS", "--max-time", "5", "https://api.ipify.org").CombinedOutput()
		detectedIP = strings.TrimSpace(string(out))
	}
	var err error
	opt.GatewayName, err = prompt("GATEWAY_NAME", "网关名称", firstNonEmpty(opt.GatewayName, "gateway-01"), opt.NonInteractive)
	if err != nil {
		return opt, err
	}
	opt.PublicIP, err = prompt("GATEWAY_PUBLIC_IP", "公网 IPv4", firstNonEmpty(opt.PublicIP, detectedIP), opt.NonInteractive)
	if err != nil {
		return opt, err
	}
	if !gatewayNameRe.MatchString(opt.GatewayName) {
		return opt, fmt.Errorf("网关名称格式无效")
	}
	// 非交互一键安装：未填公网 IP 时使用探测结果，避免主控/节点安装被打断。
	if opt.PublicIP == "" && opt.NonInteractive {
		opt.PublicIP = detectedIP
	}
	if ip := net.ParseIP(opt.PublicIP); ip == nil || ip.To4() == nil {
		return opt, fmt.Errorf("必须提供有效的 GATEWAY_PUBLIC_IP（可 export 后重试，或检查出网探测）")
	}
	if opt.EnablePanel == "" {
		if opt.NonInteractive {
			opt.EnablePanel = "1"
		} else if confirm("启用仅本机监听的 Panel？") {
			opt.EnablePanel = "1"
		} else {
			opt.EnablePanel = "0"
		}
	}
	if opt.EnableGrafana == "" {
		if opt.NonInteractive {
			// 节点组件默认不启中心 Grafana
			if opt.EnablePanel == "0" {
				opt.EnableGrafana = "0"
			} else {
				opt.EnableGrafana = "1"
			}
		} else if confirm("启用仅本机监听的 Grafana？") {
			opt.EnableGrafana = "1"
		} else {
			opt.EnableGrafana = "0"
		}
	}
	if opt.EnablePanel != "0" && opt.EnablePanel != "1" {
		return opt, fmt.Errorf("ENABLE_PANEL 只能是 0 或 1")
	}
	if opt.EnableGrafana != "0" && opt.EnableGrafana != "1" {
		return opt, fmt.Errorf("ENABLE_GRAFANA 只能是 0 或 1")
	}
	n, err := strconv.Atoi(opt.SSHPort)
	if err != nil || n < 1 || n > 65535 {
		return opt, fmt.Errorf("GATEWAY_SSH_PORT 无效")
	}
	if opt.ImageTag == "" {
		opt.ImageTag = detectImageTag(opt.Root)
	}
	return opt, nil
}

func writeEnv(opt Options) error {
	envPath := filepath.Join(opt.Root, ".env")
	profiles := ""
	if opt.EnableGrafana == "1" {
		// 主节点默认带 Loki + Fluent Bit（TCP access 日志）；从节点自行改 COMPOSE_PROFILES
		profiles = "with-grafana,with-loki,with-logs"
	}
	grafanaURL := ""
	if opt.EnablePanel == "1" && opt.EnableGrafana == "1" {
		grafanaURL = "http://127.0.0.1:3000"
	}

	dataDir := config.ResolveDataDir(opt.Root)
	_ = os.Setenv("RELAYGATE_DATA_DIR", dataDir)

	if _, err := os.Stat(envPath); err == nil {
		fmt.Println("==> 保留现有 .env（更新受管字段）")
		return patchExistingEnv(envPath, opt, profiles, grafanaURL, dataDir)
	}

	panelRole := "primary"
	if opt.EnablePanel == "0" {
		panelRole = "standby"
		if profiles == "" {
			profiles = "with-logs"
		}
	}
	controlURL := strings.TrimSpace(os.Getenv("CONTROL_URL"))
	agentTokFile := strings.TrimSpace(os.Getenv("AGENT_TOKEN_FILE"))
	if agentTokFile == "" && strings.TrimSpace(os.Getenv("AGENT_TOKEN")) != "" {
		agentTokFile = filepath.Join(opt.SecretsDir, "agent.token")
	}
	nodeExtras := ""
	if controlURL != "" {
		nodeExtras += fmt.Sprintf("CONTROL_URL=%s\n", controlURL)
	}
	if agentTokFile != "" {
		nodeExtras += fmt.Sprintf("AGENT_TOKEN_FILE=%s\n", agentTokFile)
	}

	fmt.Printf("==> 生成 %s\n", envPath)
	body := fmt.Sprintf(`GATEWAY_NAME=%s
GATEWAY_PUBLIC_IP=%s
GATEWAY_SSH_PORT=%s
PANEL_ROLE=%s
ENABLE_PANEL=%s
ENABLE_GRAFANA=%s
COMPOSE_PROJECT_NAME=relaygate-%s
COMPOSE_PROFILES=%s
ENVOY_IMAGE=envoyproxy/envoy:v1.39.0
ENVOY_ADMIN_PORT=9901
ENVOY_CONCURRENCY=0
XDS_ENABLED=1
IMAGE_TAG=%s
PANEL_BIND=0.0.0.0:9000
GRAFANA_ADMIN_USER=admin
GRAFANA_URL=%s
GRAFANA_ROOT_URL=/grafana/
GRAFANA_ANONYMOUS=true
PROMETHEUS_RETENTION=15d
RELAYGATE_SECRETS_DIR=%s
RELAYGATE_DATA_DIR=%s
%s# 公网直连暴露默认 off；前面有云 LB 发 PROXY 时再改 v2
PROXY_PROTOCOL=off
`, opt.GatewayName, opt.PublicIP, opt.SSHPort, panelRole, opt.EnablePanel, opt.EnableGrafana, opt.GatewayName,
		profiles, opt.ImageTag, grafanaURL, opt.SecretsDir, dataDir, nodeExtras)
	if err := os.WriteFile(envPath, []byte(body), 0o640); err != nil {
		return err
	}
	return secureEnvFile(envPath)
}

func patchExistingEnv(path string, opt Options, profiles, grafanaURL, dataDir string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	set := map[string]string{
		"IMAGE_TAG":             opt.ImageTag,
		"ENABLE_PANEL":          opt.EnablePanel,
		"ENABLE_GRAFANA":        opt.EnableGrafana,
		"GATEWAY_NAME":          opt.GatewayName,
		"GATEWAY_PUBLIC_IP":     opt.PublicIP,
		"GATEWAY_SSH_PORT":      opt.SSHPort,
		"RELAYGATE_DATA_DIR":    dataDir,
		"RELAYGATE_SECRETS_DIR": opt.SecretsDir,
	}
	if opt.EnablePanel == "1" && opt.EnableGrafana == "1" {
		set["GRAFANA_URL"] = grafanaURL
	} else if opt.EnableGrafana == "0" {
		set["GRAFANA_URL"] = ""
	}
	if opt.Upgrade || opt.ImageTag != "" {
		set["IMAGE_TAG"] = opt.ImageTag
	}
	seen := map[string]bool{}
	var out []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "PANEL_IMAGE=") {
			continue
		}
		key, _, ok := strings.Cut(trim, "=")
		if ok {
			key = strings.TrimSpace(key)
			if v, hit := set[key]; hit {
				out = append(out, key+"="+v)
				seen[key] = true
				continue
			}
			if key == "COMPOSE_PROFILES" {
				val := strings.TrimSpace(strings.TrimPrefix(trim, "COMPOSE_PROFILES="))
				parts := strings.Split(val, ",")
				var cleaned []string
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p == "" || p == "with-panel" {
						continue
					}
					if opt.EnableGrafana == "0" && (p == "with-grafana" || p == "with-loki") {
						continue
					}
					cleaned = append(cleaned, p)
				}
				if opt.EnableGrafana == "1" && profiles != "" {
					for _, want := range strings.Split(profiles, ",") {
						want = strings.TrimSpace(want)
						if want == "" {
							continue
						}
						has := false
						for _, c := range cleaned {
							if c == want {
								has = true
								break
							}
						}
						if !has {
							cleaned = append(cleaned, want)
						}
					}
				}
				out = append(out, "COMPOSE_PROFILES="+strings.Join(cleaned, ","))
				seen["COMPOSE_PROFILES"] = true
				continue
			}
		}
		out = append(out, line)
	}
	for k, v := range set {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	if !seen["COMPOSE_PROFILES"] && profiles != "" {
		out = append(out, "COMPOSE_PROFILES="+profiles)
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o640); err != nil {
		return err
	}
	return secureEnvFile(path)
}

// secureEnvFile makes .env readable by the Panel user (group relaygate) without
// world access. WriteFile mode is masked by umask (often 077 → 0600), so chmod
// explicitly; chown is best-effort when the group exists (panel install later).
func secureEnvFile(path string) error {
	_ = os.Chmod(path, 0o640)
	if exec.Command("getent", "group", "relaygate").Run() == nil {
		_ = dataplane.RunCmd(filepath.Dir(path), "chown", "root:relaygate", path)
	}
	return nil
}

func ensureSecrets(opt Options) error {
	if err := os.MkdirAll(opt.SecretsDir, 0o750); err != nil {
		return err
	}
	// 纠正 umask：MkdirAll(0750) 在 umask 077 下会变成 0700
	_ = os.Chmod(opt.SecretsDir, 0o750)
	if parent := filepath.Dir(opt.SecretsDir); parent != "" && parent != "/" && parent != "." {
		_ = os.Chmod(parent, 0o750)
		if exec.Command("getent", "group", "relaygate").Run() == nil {
			_ = dataplane.RunCmd(filepath.Dir(opt.SecretsDir), "chown", "root:relaygate", parent)
		}
	}
	if exec.Command("getent", "group", "relaygate").Run() == nil {
		_ = dataplane.RunCmd(filepath.Dir(opt.SecretsDir), "chown", "root:relaygate", opt.SecretsDir)
	}
	for _, name := range []string{"panel_admin_password", "grafana_admin_password"} {
		p := filepath.Join(opt.SecretsDir, name)
		st, err := os.Stat(p)
		if err == nil && st.Size() > 0 {
			// Grafana 镜像以 uid=472 gid=0 运行；密钥需对 root 组可读（0640），
			// 0600 会导致容器读不到 GF_SECURITY_ADMIN_PASSWORD__FILE。
			if name == "grafana_admin_password" {
				_ = os.Chmod(p, 0o640)
			} else if exec.Command("getent", "group", "relaygate").Run() == nil {
				_ = dataplane.RunCmd(filepath.Dir(p), "chown", "root:relaygate", p)
				_ = os.Chmod(p, 0o640)
			}
			continue
		}
		if err := os.WriteFile(p, []byte(defaultAdminPassword+"\n"), 0o640); err != nil {
			return err
		}
		_ = os.Chmod(p, 0o640)
		if name == "panel_admin_password" && exec.Command("getent", "group", "relaygate").Run() == nil {
			_ = dataplane.RunCmd(filepath.Dir(p), "chown", "root:relaygate", p)
		}
	}
	fmt.Printf("==> 密钥保存在 %s（默认密码弱，生产务必修改；不会打印明文）\n", opt.SecretsDir)
	return nil
}

func detectSSHPort() string {
	out, err := exec.Command("sshd", "-T").CombinedOutput()
	if err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(out)))
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) == 2 && fields[0] == "port" {
				return fields[1]
			}
		}
	}
	return config.Getenv("GATEWAY_SSH_PORT", config.DefaultSSHPort)
}

func detectImageTag(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--short=12", "HEAD").CombinedOutput()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return time.Now().Format("200601021504")
}

func prompt(envKey, message, def string, nonInteractive bool) (string, error) {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v, nil
	}
	if def != "" && nonInteractive {
		return def, nil
	}
	if nonInteractive {
		if def == "" {
			return "", fmt.Errorf("NONINTERACTIVE=1 时必须设置 %s", envKey)
		}
		return def, nil
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return "", fmt.Errorf("无交互终端；请设置 NONINTERACTIVE=1 及所需环境变量")
	}
	defer tty.Close()
	if def != "" {
		fmt.Printf("%s [%s]: ", message, def)
	} else {
		fmt.Printf("%s: ", message)
	}
	sc := bufio.NewScanner(tty)
	if !sc.Scan() {
		return "", fmt.Errorf("读取输入失败")
	}
	v := strings.TrimSpace(sc.Text())
	if v == "" {
		v = def
	}
	return v, nil
}

func confirm(message string) bool {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	defer tty.Close()
	fmt.Printf("%s [y/N]: ", message)
	sc := bufio.NewScanner(tty)
	if !sc.Scan() {
		return false
	}
	a := strings.TrimSpace(sc.Text())
	return a == "y" || a == "Y"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
