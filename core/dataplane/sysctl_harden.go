package dataplane

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

const sysctlHardenDest = "/etc/sysctl.d/99-relaygate-tcp-harden.conf"

// ApplyKernelHardenFromResources writes the sysctl overlay from security.policies[kernel_syn]
// and loads it via sysctl --system. When the policy is disabled, skips (does not remove
// an existing overlay). Requires root.
func ApplyKernelHardenFromResources(root string) error {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return err
	}
	body := resources.RenderKernelHardenConf(&res.Security)
	if strings.TrimSpace(body) == "" {
		fmt.Println("内核：kernel_syn 已关闭，跳过主机内核参数应用")
		return nil
	}
	if !IsRoot() {
		if handled, err := maybePrivilegedReexec(os.Stdout, os.Stderr, "sysctl-harden-apply"); handled {
			return err
		}
		return errNeedRootOrHelper()
	}
	if err := os.WriteFile(sysctlHardenDest, []byte(body), 0o644); err != nil {
		return fmt.Errorf("写入内核参数叠加失败：%w", err)
	}
	if err := RunCmd(root, "sysctl", "--system"); err != nil {
		return fmt.Errorf("加载内核参数失败：%w", err)
	}
	fmt.Printf("sysctl 已按配置应用：%s\n", sysctlHardenDest)
	return nil
}

// VerifyKernelHarden checks live kernel values against EffectiveKernelSyn.
// When the policy is disabled, returns nil (nothing to verify).
func VerifyKernelHarden(root string) error {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return err
	}
	want := res.Security.EffectiveKernelSyn()
	if want == nil {
		return nil
	}
	checks := []struct {
		key  string
		want int
	}{
		{"net.ipv4.tcp_syncookies", want.TcpSyncookies},
		{"net.ipv4.tcp_max_syn_backlog", want.TcpMaxSynBacklog},
		{"net.ipv4.tcp_synack_retries", want.TcpSynackRetries},
		{"net.ipv4.tcp_syn_retries", want.TcpSynRetries},
		{"net.ipv4.tcp_abort_on_overflow", want.TcpAbortOnOverflow},
	}
	var mismatches []string
	for _, c := range checks {
		got, err := readSysctlInt(root, c.key)
		if err != nil {
			return fmt.Errorf("读取 %s 失败：%w", c.key, err)
		}
		if got != c.want {
			mismatches = append(mismatches, fmt.Sprintf("%s 期望=%d 实际=%d", c.key, c.want, got))
		}
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("sysctl 未按配置生效：%s", strings.Join(mismatches, "；"))
	}
	return nil
}

func readSysctlInt(root, key string) (int, error) {
	out, err := RunCmdCapture(root, "sysctl", "-n", key)
	if err != nil {
		return 0, fmt.Errorf("%v (%s)", err, strings.TrimSpace(out))
	}
	s := strings.TrimSpace(out)
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("无法解析 %s=%q", key, s)
	}
	return n, nil
}

// KernelHardenDest returns the host path for the harden overlay (tests / docs).
func KernelHardenDest() string {
	return sysctlHardenDest
}

// WriteKernelHardenPreview writes rendered conf under DataDir for dry-run / agent logs.
func WriteKernelHardenPreview(root string) (string, error) {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return "", err
	}
	body := resources.RenderKernelHardenConf(&res.Security)
	dir := filepath.Join(config.ResolveDataDir(root), "security")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "sysctl-tcp-harden.preview.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
