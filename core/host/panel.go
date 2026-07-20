// Package host implements host-level install tasks (systemd Panel, users, sudoers).
// It is separate from core/panel (HTTP management UI) and core/ops (data-plane workflows).
package host

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/ops"
)

// PanelInstallOptions configures systemd panel installation.
type PanelInstallOptions struct {
	InstallDir string
	SecretsDir string
	DryRun     bool
	EnableNow  bool
	GrafanaURL string
	PanelRole  string
}

// PanelInstall installs Panel as a host systemd service (binary mode).
func PanelInstall(root string, opt PanelInstallOptions) error {
	if opt.InstallDir == "" {
		opt.InstallDir = config.Getenv("RELAYGATE_INSTALL_DIR", config.DefaultInstallDir)
	}
	if opt.SecretsDir == "" {
		opt.SecretsDir = config.Getenv("RELAYGATE_SECRETS_DIR", config.DefaultSecretsDir)
	}
	if opt.GrafanaURL == "" {
		opt.GrafanaURL = config.Getenv("GRAFANA_URL", "http://127.0.0.1:3000")
	}
	if os.Getenv("DRY_RUN") == "1" {
		opt.DryRun = true
	}
	if os.Getenv("ENABLE_NOW") == "0" {
		opt.EnableNow = false
	} else if !opt.DryRun {
		opt.EnableNow = true
	}
	if os.Getenv("ENABLE_NOW") == "1" {
		opt.EnableNow = true
	}

	if !ops.IsRoot() && !opt.DryRun {
		return fmt.Errorf("请以 root 运行（或 DRY_RUN=1）")
	}
	if _, err := os.Stat(opt.InstallDir); err != nil {
		return fmt.Errorf("安装目录不存在: %s", opt.InstallDir)
	}
	bin := filepath.Join(opt.InstallDir, "bin", "relaygate")
	if st, err := os.Stat(bin); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		return fmt.Errorf("缺少可执行文件: %s", bin)
	}

	dataDir := config.ResolveDataDir(opt.InstallDir)
	unitSrc := filepath.Join(root, config.PackagingDirName, "systemd", "relaygate-panel.service")
	helperSrc := filepath.Join(root, config.PackagingDirName, "systemd", "relaygate-apply")
	sudoersSrc := filepath.Join(root, config.PackagingDirName, "systemd", "relaygate-panel.sudoers")
	if _, err := os.Stat(filepath.Join(opt.InstallDir, config.PackagingDirName, "systemd", "relaygate-panel.service")); err == nil {
		unitSrc = filepath.Join(opt.InstallDir, config.PackagingDirName, "systemd", "relaygate-panel.service")
		helperSrc = filepath.Join(opt.InstallDir, config.PackagingDirName, "systemd", "relaygate-apply")
		sudoersSrc = filepath.Join(opt.InstallDir, config.PackagingDirName, "systemd", "relaygate-panel.sudoers")
	}
	for _, p := range []string{unitSrc, helperSrc, sudoersSrc} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("缺少 packaging/systemd 模板: %s", p)
		}
	}

	fmt.Printf("==> 安装 Panel systemd 服务（INSTALL_DIR=%s DATA_DIR=%s）\n", opt.InstallDir, dataDir)
	if err := ensurePanelUser(opt); err != nil {
		return err
	}
	if err := fixPanelPermissions(opt, dataDir); err != nil {
		return err
	}
	if err := installHelperAndSudoers(opt, helperSrc, sudoersSrc); err != nil {
		return err
	}
	unitDst := "/etc/systemd/system/relaygate-panel.service"
	if err := renderUnit(opt, unitSrc, unitDst); err != nil {
		return err
	}
	if err := writePanelEnv(opt, dataDir); err != nil {
		return err
	}

	run := panelRun(opt)
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if opt.EnableNow {
		if err := run("systemctl", "enable", "--now", "relaygate-panel.service"); err != nil {
			return err
		}
		fmt.Println("==> 已 enable --now relaygate-panel")
	} else {
		_ = run("systemctl", "enable", "relaygate-panel.service")
		_ = run("systemctl", "stop", "relaygate-panel.service")
		fmt.Println("==> 已 enable；未启动（ENABLE_NOW=0）")
	}
	fmt.Println("==> 状态: systemctl status relaygate-panel")
	fmt.Println("==> 日志: journalctl -u relaygate-panel -f")
	return nil
}

func panelRun(opt PanelInstallOptions) func(name string, args ...string) error {
	return func(name string, args ...string) error {
		if opt.DryRun {
			fmt.Printf("[dry-run] %s %s\n", name, strings.Join(args, " "))
			return nil
		}
		return ops.RunCmd(opt.InstallDir, name, args...)
	}
}

func ensurePanelUser(opt PanelInstallOptions) error {
	run := panelRun(opt)
	if exec.Command("getent", "group", "relaygate").Run() != nil {
		if err := run("groupadd", "--system", "relaygate"); err != nil {
			return err
		}
	}
	if exec.Command("getent", "passwd", "relaygate").Run() != nil {
		if err := run("useradd", "--system", "--gid", "relaygate", "--home-dir", opt.InstallDir,
			"--shell", "/usr/sbin/nologin", "--comment", "RelayGate Panel", "relaygate"); err != nil {
			return err
		}
	}
	out, _ := exec.Command("id", "-nG", "relaygate").CombinedOutput()
	for _, g := range strings.Fields(string(out)) {
		if g == "docker" {
			return fmt.Errorf("用户 relaygate 在 docker 组中；请先: gpasswd -d relaygate docker")
		}
	}
	return nil
}

func fixPanelPermissions(opt PanelInstallOptions, dataDir string) error {
	if opt.DryRun {
		fmt.Println("[dry-run] fix permissions")
		return nil
	}
	dirs := []struct {
		path string
		mode os.FileMode
		uid  string
		gid  string
	}{
		{opt.InstallDir, 0o755, "root", "root"},
		{filepath.Join(opt.InstallDir, "bin"), 0o755, "root", "root"},
		{filepath.Join(opt.InstallDir, "frontend"), 0o755, "root", "root"},
		{dataDir, 0o770, "root", "relaygate"},
		{filepath.Join(dataDir, "envoy"), 0o770, "root", "relaygate"},
		{filepath.Join(dataDir, "firewall"), 0o770, "root", "relaygate"},
		{filepath.Join(dataDir, "prometheus"), 0o770, "root", "relaygate"},
		{filepath.Join(dataDir, "backups"), 0o770, "root", "relaygate"},
		{opt.SecretsDir, 0o750, "root", "relaygate"},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.mode); err != nil {
			return err
		}
		_ = ops.RunCmd(opt.InstallDir, "chown", d.uid+":"+d.gid, d.path)
		_ = os.Chmod(d.path, d.mode)
	}
	bin := filepath.Join(opt.InstallDir, "bin", "relaygate")
	_ = ops.RunCmd(opt.InstallDir, "chown", "root:root", bin)
	_ = os.Chmod(bin, 0o755)
	res := filepath.Join(dataDir, "resources.yaml")
	if _, err := os.Stat(res); err == nil {
		_ = ops.RunCmd(opt.InstallDir, "chown", "root:relaygate", res)
		_ = os.Chmod(res, 0o660)
	}
	panelPW := filepath.Join(opt.SecretsDir, "panel_admin_password")
	if _, err := os.Stat(panelPW); err == nil {
		_ = ops.RunCmd(opt.InstallDir, "chown", "root:relaygate", panelPW)
		_ = os.Chmod(panelPW, 0o640)
	}
	grafPW := filepath.Join(opt.SecretsDir, "grafana_admin_password")
	if _, err := os.Stat(grafPW); err == nil {
		_ = ops.RunCmd(opt.InstallDir, "chown", "root:root", grafPW)
		// 0640：Grafana 容器 gid=0 可读；勿用 0600（容器内读失败）
		_ = os.Chmod(grafPW, 0o640)
	}
	return nil
}

func installHelperAndSudoers(opt PanelInstallOptions, helperSrc, sudoersSrc string) error {
	helperDir := "/usr/local/libexec/relaygate"
	helperPath := filepath.Join(helperDir, "apply")
	sudoersDst := "/etc/sudoers.d/relaygate-panel"
	if opt.DryRun {
		fmt.Printf("[dry-run] install helper %s → %s\n", helperSrc, helperPath)
		fmt.Printf("[dry-run] install sudoers → %s\n", sudoersDst)
		return nil
	}
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(helperSrc)
	if err != nil {
		return err
	}
	content := string(b)
	if opt.InstallDir != config.DefaultInstallDir {
		content = strings.ReplaceAll(content, `RELAYGATE_INSTALL_DIR:-/opt/relaygate`, "RELAYGATE_INSTALL_DIR:-"+opt.InstallDir)
		content = strings.ReplaceAll(content, `INSTALL_DIR="${RELAYGATE_INSTALL_DIR:-/opt/relaygate}"`,
			fmt.Sprintf(`INSTALL_DIR="${RELAYGATE_INSTALL_DIR:-%s}"`, opt.InstallDir))
	}
	if err := os.WriteFile(helperPath, []byte(content), 0o755); err != nil {
		return err
	}
	_ = ops.RunCmd(opt.InstallDir, "chown", "root:root", helperPath)

	sb, err := os.ReadFile(sudoersSrc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(sudoersDst, sb, 0o440); err != nil {
		return err
	}
	_ = ops.RunCmd(opt.InstallDir, "chown", "root:root", sudoersDst)
	if ops.LookPath("visudo") {
		if err := ops.RunCmd(opt.InstallDir, "visudo", "-cf", sudoersDst); err != nil {
			return fmt.Errorf("sudoers 校验失败: %s", sudoersDst)
		}
	}
	return nil
}

func renderUnit(opt PanelInstallOptions, src, dest string) error {
	if opt.DryRun {
		fmt.Printf("[dry-run] unit %s → %s\n", src, dest)
		return nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	content := string(b)
	if opt.InstallDir != config.DefaultInstallDir || opt.SecretsDir != config.DefaultSecretsDir {
		content = strings.ReplaceAll(content, config.DefaultInstallDir, opt.InstallDir)
		content = strings.ReplaceAll(content, config.DefaultSecretsDir, opt.SecretsDir)
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return err
	}
	return ops.RunCmd(opt.InstallDir, "chown", "root:root", dest)
}

func writePanelEnv(opt PanelInstallOptions, dataDir string) error {
	role := opt.PanelRole
	if role == "" {
		role = os.Getenv("PANEL_ROLE")
	}
	if role == "" {
		if b, err := os.ReadFile(filepath.Join(opt.InstallDir, ".env")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "PANEL_ROLE=") {
					role = strings.TrimSpace(strings.TrimPrefix(line, "PANEL_ROLE="))
				}
			}
		}
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "primary"
	}
	grafanaLine := "GRAFANA_URL="
	if opt.GrafanaURL != "" {
		grafanaLine = "GRAFANA_URL=" + opt.GrafanaURL
	}
	if opt.DryRun {
		fmt.Printf("[dry-run] 将写入 /etc/relaygate/panel.env (%s PANEL_ROLE=%s)\n", grafanaLine, role)
		return nil
	}
	if err := os.MkdirAll("/etc/relaygate", 0o750); err != nil {
		return err
	}
	_ = ops.RunCmd(opt.InstallDir, "chown", "root:relaygate", "/etc/relaygate")
	helperPath := "/usr/local/libexec/relaygate/apply"
	body := fmt.Sprintf(`# Managed by relaygate panel install — Panel systemd EnvironmentFile
PANEL_ROOT=%s
PANEL_BIND=127.0.0.1:9000
PANEL_ADMIN_PASSWORD_FILE=%s/panel_admin_password
PANEL_ROLE=%s
RELAYGATE_BIN=%s/bin/relaygate
RELAYGATE_PRIVILEGED_HELPER=%s
RELAYGATE_DATA_DIR=%s
ENVOY_ADMIN_URL=http://127.0.0.1:9901
PROMETHEUS_URL=http://127.0.0.1:9090
%s
`, opt.InstallDir, opt.SecretsDir, role, opt.InstallDir, helperPath, dataDir, grafanaLine)
	panelEnv := "/etc/relaygate/panel.env"
	if err := os.WriteFile(panelEnv, []byte(body), 0o640); err != nil {
		return err
	}
	_ = ops.RunCmd(opt.InstallDir, "chown", "root:relaygate", panelEnv)
	return os.Chmod(panelEnv, 0o640)
}

// PanelUninstall removes Panel systemd unit/helper/sudoers. PURGE=1 also removes user.
func PanelUninstall(purge, dryRun bool) error {
	if os.Getenv("PURGE") == "1" {
		purge = true
	}
	if os.Getenv("DRY_RUN") == "1" {
		dryRun = true
	}
	if !ops.IsRoot() && !dryRun {
		return fmt.Errorf("请以 root 运行（或 DRY_RUN=1）")
	}
	run := func(name string, args ...string) error {
		if dryRun {
			fmt.Printf("[dry-run] %s %s\n", name, strings.Join(args, " "))
			return nil
		}
		cmd := exec.Command(name, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	fmt.Println("==> 停止并禁用 relaygate-panel")
	if dryRun {
		fmt.Println("[dry-run] systemctl disable --now relaygate-panel.service")
	} else {
		_ = exec.Command("systemctl", "disable", "--now", "relaygate-panel.service").Run()
		_ = exec.Command("systemctl", "stop", "relaygate-panel.service").Run()
	}
	_ = run("rm", "-f", "/etc/systemd/system/relaygate-panel.service")
	_ = run("rm", "-f", "/etc/sudoers.d/relaygate-panel")
	_ = run("rm", "-f", "/usr/local/libexec/relaygate/apply")
	_ = run("rm", "-f", "/etc/relaygate/panel.env")
	if !dryRun {
		_ = os.Remove("/usr/local/libexec/relaygate")
	}
	_ = run("systemctl", "daemon-reload")
	_ = exec.Command("systemctl", "reset-failed", "relaygate-panel.service").Run()

	if purge {
		if dryRun {
			fmt.Println("[dry-run] 将删除系统用户/组 relaygate")
		} else {
			if exec.Command("getent", "passwd", "relaygate").Run() == nil {
				if err := exec.Command("userdel", "relaygate").Run(); err != nil {
					return fmt.Errorf("无法删除用户 relaygate")
				}
			}
			if exec.Command("getent", "group", "relaygate").Run() == nil {
				_ = exec.Command("groupdel", "relaygate").Run()
			}
		}
		fmt.Println("==> 已 purge 用户/组 relaygate（配置与密钥未删；用 install.sh --uninstall --purge）")
	} else {
		fmt.Println("==> 已移除 unit/helper/sudoers；保留用户 relaygate 与配置/密钥")
	}
	return nil
}
