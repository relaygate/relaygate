package dataplane

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UpgradeOptions controls local binary/packaging upgrade via install.sh.
type UpgradeOptions struct {
	// Drain runs drain fail before upgrade and drain ok after (dual-active playbook).
	Drain bool
	// SkipInstall only prints the documented steps (no exec); useful for dry guidance.
	SkipInstall bool
}

// Upgrade upgrades packaging/binary by delegating to install.sh --upgrade.
// Does not re-implement install logic. Pass RELAYGATE_VERSION or RELAYGATE_TAR in the environment.
func Upgrade(root string, opt UpgradeOptions) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	version, tarPath, err := ResolveReleaseSpec(root)
	if err != nil {
		return err
	}

	fmt.Println("==> 变更分流：二进制 / packaging → relaygate upgrade（或 install.sh --upgrade）")
	fmt.Println("    ACL/nftables-only → firewall apply；resources/Envoy → reload")
	WarnIfDrainWaitShort(env.DrainWait, os.Stdout)

	if opt.Drain {
		fmt.Printf("==> drain fail（DRAIN_WAIT=%ds）\n", env.DrainWait)
		if err := Drain(root, "fail"); err != nil {
			return fmt.Errorf("drain fail: %w", err)
		}
	}

	installSh := filepath.Join(root, "install.sh")
	if _, err := os.Stat(installSh); err != nil {
		return fmt.Errorf("缺少 %s；请用官方 install.sh --upgrade 或将 release 树放到本机", installSh)
	}

	if opt.SkipInstall {
		fmt.Println("==> SkipInstall：请手动执行:")
		printUpgradeCommand(installSh, version, tarPath)
		return nil
	}

	fmt.Println("==> 委托 install.sh --upgrade（不复制安装逻辑）")
	cmd := exec.Command("bash", installSh, "--upgrade", "-y")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = upgradeEnv(os.Environ(), root, version, tarPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install.sh --upgrade 失败: %w（未回退 git；请检查 RELAYGATE_VERSION / RELAYGATE_TAR）", err)
	}

	if opt.Drain {
		fmt.Println("==> drain ok（恢复接流）")
		if err := Drain(root, "ok"); err != nil {
			fmt.Printf("WARN: drain ok: %v（Envoy 重启后可能已 LIVE）\n", err)
		}
	}

	fmt.Println("升级完成。建议: relaygate diag && relaygate smoke")
	return nil
}

func printUpgradeCommand(installSh, version, tarPath string) {
	parts := []string{"sudo", "NONINTERACTIVE=1"}
	if tarPath != "" {
		parts = append(parts, "RELAYGATE_TAR="+shellQuote(tarPath))
	}
	if version != "" {
		parts = append(parts, "RELAYGATE_VERSION="+shellQuote(version))
	}
	parts = append(parts, "bash", shellQuote(installSh), "--upgrade", "-y")
	fmt.Println(strings.Join(parts, " "))
}

func upgradeEnv(base []string, root, version, tarPath string) []string {
	out := append([]string{}, base...)
	set := func(k, v string) {
		prefix := k + "="
		for i, e := range out {
			if strings.HasPrefix(e, prefix) {
				out[i] = prefix + v
				return
			}
		}
		out = append(out, prefix+v)
	}
	set("NONINTERACTIVE", "1")
	set("RELAYGATE_INSTALL_DIR", root)
	if version != "" {
		set("RELAYGATE_VERSION", version)
	}
	if tarPath != "" {
		set("RELAYGATE_TAR", tarPath)
	}
	return out
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ResolveReleaseSpec picks a release from env.
// Preference: RELAYGATE_TAR > RELAYGATE_VERSION > DEPLOY_REF > "latest"（由 install.sh 解析最新 GitHub Release tag）。
func ResolveReleaseSpec(root string) (version, tarPath string, err error) {
	_ = root
	tarPath = strings.TrimSpace(os.Getenv("RELAYGATE_TAR"))
	version = strings.TrimSpace(os.Getenv("RELAYGATE_VERSION"))
	if version == "" {
		version = strings.TrimSpace(os.Getenv("DEPLOY_REF"))
	}
	if tarPath != "" {
		if st, statErr := os.Stat(tarPath); statErr != nil || st.IsDir() {
			return "", "", fmt.Errorf("RELAYGATE_TAR 无效: %s", tarPath)
		}
		return version, tarPath, nil
	}
	switch strings.ToLower(version) {
	case "", "latest":
		return "latest", "", nil
	case "master", "main":
		return "", "", fmt.Errorf("RELAYGATE_VERSION 不能为 %q；请省略以用最新 Release，或指定具体 tag / RELAYGATE_TAR", version)
	default:
		return version, "", nil
	}
}

// ChangePathHint prints the change-routing cheat sheet.
func ChangePathHint() {
	fmt.Println("变更分流:")
	fmt.Println("  ACL / nftables-only     → relaygate firewall apply")
	fmt.Println("  resources / Envoy 配置  → relaygate reload（含 drain）")
	fmt.Println("  二进制 / packaging      → relaygate upgrade [--drain] 或 install.sh --upgrade")
}
