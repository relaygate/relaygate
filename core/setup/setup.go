package setup

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/ops"
)

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
	// ResetDefaults overwrites data/resources.yaml and data/inventory/gateways.env
	// from versioned templates. Never silently overwrite without this flag.
	ResetDefaults bool
}

// Run writes/updates .env and secrets scaffolding, then seeds data/ defaults.
func Run(opt Options) error {
	if opt.Root == "" {
		return fmt.Errorf("root required")
	}
	if opt.SecretsDir == "" {
		opt.SecretsDir = getenv("RELAYGATE_SECRETS_DIR", "/etc/relaygate/secrets")
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
	if err := ops.SeedDefaults(opt.Root, opt.ResetDefaults); err != nil {
		return err
	}
	if opt.ApplySysctl {
		_ = ops.ApplySysctl(opt.Root, opt.GatewayName)
	}
	fmt.Println("==> setup 完成")
	fmt.Printf("配置: %s/.env；密钥: %s\n", opt.Root, opt.SecretsDir)
	return nil
}

func collectSettings(opt Options) (Options, error) {
	if opt.SSHPort == "" {
		opt.SSHPort = detectSSHPort()
	}
	detectedIP := ""
	if opt.PublicIP == "" && !opt.NonInteractive {
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
	if ip := net.ParseIP(opt.PublicIP); ip == nil || ip.To4() == nil {
		return opt, fmt.Errorf("必须提供有效的 GATEWAY_PUBLIC_IP")
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
			opt.EnableGrafana = "1"
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
		profiles = "with-grafana"
	}
	grafanaURL := ""
	if opt.EnablePanel == "1" && opt.EnableGrafana == "1" {
		grafanaURL = "http://127.0.0.1:3000"
	}

	if _, err := os.Stat(envPath); err == nil {
		fmt.Println("==> 保留现有 .env（更新受管字段）")
		return patchExistingEnv(envPath, opt, profiles, grafanaURL)
	}

	fmt.Printf("==> 生成 %s\n", envPath)
	body := fmt.Sprintf(`GATEWAY_NAME=%s
GATEWAY_PUBLIC_IP=%s
GATEWAY_SSH_PORT=%s
PANEL_ROLE=primary
ENABLE_PANEL=%s
ENABLE_GRAFANA=%s
COMPOSE_PROJECT_NAME=relaygate-%s
COMPOSE_PROFILES=%s
ENVOY_IMAGE=envoyproxy/envoy:v1.39.0
ENVOY_ADMIN_PORT=9901
ENVOY_CONCURRENCY=0
IMAGE_TAG=%s
PANEL_BIND=127.0.0.1:9000
GRAFANA_ADMIN_USER=admin
GRAFANA_URL=%s
GRAFANA_ROOT_URL=/grafana/
GRAFANA_ANONYMOUS=true
PROMETHEUS_RETENTION=15d
RELAYGATE_SECRETS_DIR=%s
`, opt.GatewayName, opt.PublicIP, opt.SSHPort, opt.EnablePanel, opt.EnableGrafana, opt.GatewayName,
		profiles, opt.ImageTag, grafanaURL, opt.SecretsDir)
	if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
		return err
	}
	return nil
}

func patchExistingEnv(path string, opt Options, profiles, grafanaURL string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	set := map[string]string{
		"IMAGE_TAG":         opt.ImageTag,
		"ENABLE_PANEL":      opt.EnablePanel,
		"ENABLE_GRAFANA":    opt.EnableGrafana,
		"GATEWAY_NAME":      opt.GatewayName,
		"GATEWAY_PUBLIC_IP": opt.PublicIP,
		"GATEWAY_SSH_PORT":  opt.SSHPort,
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
					if p == "with-grafana" && opt.EnableGrafana == "0" {
						continue
					}
					cleaned = append(cleaned, p)
				}
				if profiles != "" {
					has := false
					for _, c := range cleaned {
						if c == profiles {
							has = true
						}
					}
					if !has && opt.EnableGrafana == "1" {
						cleaned = append(cleaned, profiles)
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
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o600)
}

func ensureSecrets(opt Options) error {
	if err := os.MkdirAll(opt.SecretsDir, 0o750); err != nil {
		return err
	}
	for _, name := range []string{"panel_admin_password", "grafana_admin_password"} {
		p := filepath.Join(opt.SecretsDir, name)
		st, err := os.Stat(p)
		if err == nil && st.Size() > 0 {
			// Grafana 镜像以 uid=472 gid=0 运行；密钥需对 root 组可读（0640），
			// 0600 会导致容器读不到 GF_SECURITY_ADMIN_PASSWORD__FILE。
			if name == "grafana_admin_password" {
				_ = os.Chmod(p, 0o640)
			}
			continue
		}
		pw, err := randomPassword()
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(pw+"\n"), 0o640); err != nil {
			return err
		}
	}
	fmt.Printf("==> 密钥保存在 %s（不会打印明文）\n", opt.SecretsDir)
	return nil
}

func randomPassword() (string, error) {
	b := make([]byte, 36)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
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
	return getenv("GATEWAY_SSH_PORT", "22")
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

func getenv(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
