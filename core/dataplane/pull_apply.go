package dataplane

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
	"github.com/relaygate/relaygate/core/xds"
)

// LayerStatus is the product-facing result for one security domain.
type LayerStatus string

const (
	LayerStatusVerified LayerStatus = "verified"
	LayerStatusSkipped  LayerStatus = "skipped"
	LayerStatusFailed   LayerStatus = "failed"
)

// Domain ids for status JSON / failed_at (product domains, not execution components).
const (
	DomainKernel   = "kernel"
	DomainNIC      = "nic"
	DomainFirewall = "firewall"
	DomainGateway  = "gateway"
)

// LayerResult records apply/verify outcome for one security domain.
type LayerResult struct {
	Module string      `json:"module"` // kernel | nic | firewall | gateway
	Status LayerStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// PullApplyStatus is written under DataDir after agent AfterPull pipeline.
// Keys are product domains (see docs/security-domains.md).
type PullApplyStatus struct {
	Version  string      `json:"version"`
	At       string      `json:"at"`
	HostAuto bool        `json:"host_security_auto_apply"`
	Kernel   LayerResult `json:"kernel"`
	NIC      LayerResult `json:"nic"`
	Firewall LayerResult `json:"firewall"`
	Gateway  LayerResult `json:"gateway"`
	OK       bool        `json:"ok"`
	FailedAt string      `json:"failed_at,omitempty"` // domain name if any
}

// PullApplyOptions configures the post-pull materialization pipeline.
type PullApplyOptions struct {
	Root    string
	Version string
	Env     Env
	Stdout  io.Writer
	Stderr  io.Writer
	// SkipGateway skips gateway HotApply (tests / verify-only host path).
	SkipGateway bool
}

// AfterPullApply runs: optional host kernel → nic (skip) → firewall → gateway HotApply,
// verifying each step. applied-version must only be updated when this returns nil.
func AfterPullApply(opts PullApplyOptions) error {
	stdout, stderr := opts.Stdout, opts.Stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	root := opts.Root
	env := opts.Env
	hostAuto := env.HostSecurityAutoApply()

	st := PullApplyStatus{
		Version:  opts.Version,
		At:       time.Now().UTC().Format(time.RFC3339),
		HostAuto: hostAuto,
		Kernel:   LayerResult{Module: DomainKernel, Status: LayerStatusSkipped},
		NIC: LayerResult{
			Module: DomainNIC,
			Status: LayerStatusSkipped,
			Detail: "网卡域预留（未实现）",
		},
		Firewall: LayerResult{Module: DomainFirewall, Status: LayerStatusSkipped},
		Gateway:  LayerResult{Module: DomainGateway, Status: LayerStatusSkipped},
	}
	fail := func(domain string, err error) error {
		st.OK = false
		st.FailedAt = domain
		_ = writePullApplyStatus(root, st)
		return err
	}

	skipHostDetail := "主机侧自动应用未启用（纯节点 PANEL_ENABLED=0 默认开启；主控默认关闭。显式 SECURITY_AUTO_APPLY=1/0 可覆盖）"
	if !hostAuto {
		st.Kernel.Detail = skipHostDetail
		st.Firewall.Detail = skipHostDetail
		logf(stdout, "主机侧（内核 / 防火墙）：跳过自动应用（%s）", skipHostDetail)
	} else {
		kernelOn, err := kernelSynEnabled(root)
		if err != nil {
			st.Kernel = LayerResult{Module: DomainKernel, Status: LayerStatusFailed, Error: err.Error()}
			return fail(DomainKernel, fmt.Errorf("读取内核策略失败：%w", err))
		}
		if !kernelOn {
			st.Kernel = LayerResult{
				Module: DomainKernel,
				Status: LayerStatusSkipped,
				Detail: "kernel_syn 已关闭，未改主机内核参数",
			}
			logf(stdout, "==> [内核] 跳过（策略关闭）")
		} else {
			logf(stdout, "==> [内核] 按配置应用并验证")
			if err := ApplyKernelHardenFromResources(root); err != nil {
				st.Kernel = LayerResult{Module: DomainKernel, Status: LayerStatusFailed, Error: err.Error()}
				return fail(DomainKernel, fmt.Errorf("内核应用失败：%w", err))
			}
			if err := VerifyKernelHarden(root); err != nil {
				st.Kernel = LayerResult{Module: DomainKernel, Status: LayerStatusFailed, Error: err.Error()}
				return fail(DomainKernel, fmt.Errorf("内核校验失败（未标记已应用）：%w", err))
			}
			st.Kernel = LayerResult{Module: DomainKernel, Status: LayerStatusVerified, Detail: "内核参数与配置一致"}
			logf(stdout, "==> [内核] 校验通过")
		}

		logf(stdout, "==> [网卡] 跳过（预留）")

		logf(stdout, "==> [防火墙] 按配置应用并验证")
		if err := ApplyNftablesConfirmed(root); err != nil {
			st.Firewall = LayerResult{Module: DomainFirewall, Status: LayerStatusFailed, Error: err.Error()}
			return fail(DomainFirewall, fmt.Errorf("防火墙应用失败：%w", err))
		}
		if err := VerifyNftablesLoaded(root); err != nil {
			st.Firewall = LayerResult{Module: DomainFirewall, Status: LayerStatusFailed, Error: err.Error()}
			return fail(DomainFirewall, fmt.Errorf("防火墙校验失败（未标记已应用）：%w", err))
		}
		st.Firewall = LayerResult{Module: DomainFirewall, Status: LayerStatusVerified, Detail: "防火墙规则集已加载"}
		logf(stdout, "==> [防火墙] 校验通过")
	}

	if opts.SkipGateway {
		st.Gateway.Detail = "跳过网关（测试）"
		st.OK = true
		_ = writePullApplyStatus(root, st)
		return nil
	}

	if !env.XDSEnabled {
		st.Gateway = LayerResult{Module: DomainGateway, Status: LayerStatusFailed, Error: "热更新已关闭"}
		return fail(DomainGateway, fmt.Errorf("热更新已关闭：配置已落盘，请手动执行 reload --hard；成功前 applied 不会更新"))
	}

	logf(stdout, "==> [网关] HotApply")
	xds.SetDiskPublishHandler(func(nodeID string) (string, error) {
		e, err := LoadEnv(root)
		if err != nil {
			return "", err
		}
		srv := xds.Global().Server()
		if srv == nil {
			return "", fmt.Errorf("本机热更新服务未运行")
		}
		return PublishSnapshotFromDisk(root, e, nodeID, srv.Publisher)
	})
	if xds.Global().Server() == nil {
		if err := PublishInitialSnapshot(root, env); err != nil {
			st.Gateway = LayerResult{Module: DomainGateway, Status: LayerStatusFailed, Error: err.Error()}
			return fail(DomainGateway, fmt.Errorf("启动本机热更新服务失败（已落盘，applied 未更新）：%w", err))
		}
	}
	if err := HotApplyTo(root, env, stdout, stderr); err != nil {
		st.Gateway = LayerResult{Module: DomainGateway, Status: LayerStatusFailed, Error: err.Error()}
		return fail(DomainGateway, err)
	}
	st.Gateway = LayerResult{Module: DomainGateway, Status: LayerStatusVerified, Detail: "网关热更新已确认（ACK）"}
	st.OK = true
	_ = writePullApplyStatus(root, st)
	logf(stdout, "拉取后落地完成：内核=%s 网卡=%s 防火墙=%s 网关=%s",
		st.Kernel.Status, st.NIC.Status, st.Firewall.Status, st.Gateway.Status)
	return nil
}

func kernelSynEnabled(root string) (bool, error) {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return false, err
	}
	return res.Security.EffectiveKernelSyn() != nil, nil
}

// VerifySecurityLayers checks kernel / firewall / gateway ready without applying.
// Host domains that are not auto-apply targets can still be verified if present.
func VerifySecurityLayers(root string, env Env, stdout io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	var errs []string
	logf(stdout, "==> 校验内核")
	if err := VerifyKernelHarden(root); err != nil {
		errs = append(errs, "内核: "+err.Error())
		logf(stdout, "内核：失败 — %v", err)
	} else {
		logf(stdout, "内核：通过")
	}
	logf(stdout, "==> 校验网卡（预留，跳过）")
	logf(stdout, "网卡：跳过（未实现）")
	logf(stdout, "==> 校验防火墙")
	if err := VerifyNftablesLoaded(root); err != nil {
		errs = append(errs, "防火墙: "+err.Error())
		logf(stdout, "防火墙：失败 — %v", err)
	} else {
		logf(stdout, "防火墙：通过")
	}
	logf(stdout, "==> 校验网关 ready")
	if err := WaitHTTP(env.AdminURL("/ready"), 5, time.Second); err != nil {
		errs = append(errs, "网关: 未 ready")
		logf(stdout, "网关：失败 — 未 ready")
	} else {
		logf(stdout, "网关：ready")
	}
	if len(errs) > 0 {
		return fmt.Errorf("安全领域校验未全部通过：%s", strings.Join(errs, "；"))
	}
	return nil
}

func writePullApplyStatus(root string, st PullApplyStatus) error {
	dir := config.ResolveDataDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "security-apply-status.json")
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
