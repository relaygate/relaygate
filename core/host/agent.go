package host

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/dataplane"
)

// AgentInstallOptions configures systemd agent installation on a gateway node.
type AgentInstallOptions struct {
	InstallDir string
	DryRun     bool
	EnableNow  bool
}

// AgentInstall installs the node agent as a host systemd service.
func AgentInstall(root string, opt AgentInstallOptions) error {
	if opt.InstallDir == "" {
		opt.InstallDir = config.Getenv("RELAYGATE_INSTALL_DIR", config.DefaultInstallDir)
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

	if !dataplane.IsRoot() && !opt.DryRun {
		return fmt.Errorf("请以 root 运行（或 DRY_RUN=1）")
	}
	if _, err := os.Stat(opt.InstallDir); err != nil {
		return fmt.Errorf("安装目录不存在: %s", opt.InstallDir)
	}
	bin := filepath.Join(opt.InstallDir, "bin", "relaygate")
	if st, err := os.Stat(bin); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		return fmt.Errorf("缺少可执行文件: %s", bin)
	}

	unitSrc := filepath.Join(root, config.PackagingDirName, "systemd", "relaygate-agent.service")
	if _, err := os.Stat(filepath.Join(opt.InstallDir, config.PackagingDirName, "systemd", "relaygate-agent.service")); err == nil {
		unitSrc = filepath.Join(opt.InstallDir, config.PackagingDirName, "systemd", "relaygate-agent.service")
	}
	if _, err := os.Stat(unitSrc); err != nil {
		return fmt.Errorf("缺少 packaging/systemd 模板: %s", unitSrc)
	}

	fmt.Printf("==> 安装节点 Agent systemd 服务（INSTALL_DIR=%s）\n", opt.InstallDir)
	unitDst := "/etc/systemd/system/relaygate-agent.service"
	panelOpt := PanelInstallOptions{InstallDir: opt.InstallDir, DryRun: opt.DryRun}
	if err := renderUnit(panelOpt, unitSrc, unitDst); err != nil {
		return err
	}
	run := panelRun(panelOpt)
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "enable", "relaygate-agent.service"); err != nil {
		return err
	}
	if opt.EnableNow {
		// restart：升级后替换二进制时 enable --now 不会重启已在跑的单元
		if err := run("systemctl", "restart", "relaygate-agent.service"); err != nil {
			return fmt.Errorf("启动 relaygate-agent 失败: %w", err)
		}
		fmt.Println("==> relaygate-agent 已启用并启动")
	} else {
		fmt.Println("==> relaygate-agent 已启用（未立即启动）")
	}
	return nil
}
