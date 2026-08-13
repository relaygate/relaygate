package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"bufio"

	"github.com/relaygate/relaygate/core/agent"
	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/confirm"
	"github.com/relaygate/relaygate/core/dataplane"
	"github.com/relaygate/relaygate/core/diag"
	"github.com/relaygate/relaygate/core/host"
	"github.com/relaygate/relaygate/core/panel"
	"github.com/relaygate/relaygate/core/profile"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/resources"
	"github.com/relaygate/relaygate/core/setup"
	"github.com/relaygate/relaygate/core/xds"
)

// Version is injected from main via -ldflags or default.
var Version = "dev"

// Run dispatches relaygate subcommands. Returns process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "render":
		return runRender(args[1:])
	case "validate":
		return exitErr(dataplane.Validate(mustRoot()))
	case "apply":
		return exitErr(dataplane.Apply(mustRoot()))
	case "reload":
		opts := dataplane.ReloadOptions{}
		rest := parseReloadArgs(args[1:], &opts)
		if !opts.ForceHard {
			fmt.Fprintln(os.Stderr, "警告: 若需硬重启会断开本机现有连接；可热更时将无 drain 热推。双活请先 drain 本节点。")
		}
		_ = rest
		return exitErr(reloadWithOpts(opts))
	case "rollback":
		stamp := ""
		if len(args) > 1 {
			stamp = args[1]
		}
		fmt.Fprintln(os.Stderr, "警告: rollback 会重启本机 Envoy，断开本网关上全部现有连接。")
		return exitErr(dataplane.Rollback(mustRoot(), stamp))
	case "drain":
		return runDrain(args[1:])
	case "smoke":
		hostArg := ""
		if len(args) > 1 {
			hostArg = args[1]
		}
		return exitErr(dataplane.Smoke(mustRoot(), hostArg))
	case "canary":
		hostArg := ""
		if len(args) > 1 {
			hostArg = args[1]
		}
		return exitErr(dataplane.Canary(mustRoot(), hostArg))
	case "firewall":
		return runFirewall(args[1:])
	case "security":
		return runSecurity(args[1:])
	case "changes":
		return runChanges(args[1:])
	case "profile":
		return runProfile(args[1:])
	case "baseline":
		out := ""
		if len(args) > 1 {
			out = args[1]
		}
		return exitErr(dataplane.Baseline(mustRoot(), out))
	case "fleet":
		return runFleet(args[1:])
	case "agent":
		return runAgent(args[1:])
	case "xds":
		// 非产品主命令：高级调试；节点请用 agent run
		return runXDS(args[1:])
	case "upgrade":
		return runUpgrade(args[1:])
	case "setup":
		return runSetup(args[1:])
	case "diag":
		return runDiag(args[1:])
	case "server":
		return runServer(args[1:])
	case "panel":
		return runPanel(args[1:])
	case "version":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: relaygate version")
			return 2
		}
		fmt.Printf("relaygate %s\n", Version)
		return 0
	case "help", "-h", "--help":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func mustRoot() string {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	return root
}

func exitErr(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(interface{ ExitCode() int }); ok {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		return ee.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	return 1
}

func runDrain(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate drain fail|ok|status")
		return 2
	}
	switch args[0] {
	case "fail", "drain":
		fmt.Fprintln(os.Stderr, "警告: drain fail 会使本机探活失败并停止承接新连接；单节点生产请确认维护窗口。已有长连接通常仍保留。若前置了可选云 L4 入口，目标组会随之摘除本节点。")
		return exitErr(dataplane.Drain(mustRoot(), args[0]))
	case "ok", "undrain", "status":
		return exitErr(dataplane.Drain(mustRoot(), args[0]))
	default:
		fmt.Fprintln(os.Stderr, "usage: relaygate drain fail|ok|status")
		return 2
	}
}

func runUpgrade(args []string) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	drain := fs.Bool("drain", false, "升级前 drain fail、完成后 drain ok（双活）")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: relaygate upgrade [--drain]")
		fmt.Fprintln(fs.Output(), "  二进制/packaging 升级：委托 install.sh upgrade（默认最新 Release；可设 RELAYGATE_VERSION / RELAYGATE_TAR）")
		fmt.Fprintln(fs.Output(), "  ACL/防火墙 → firewall apply；resources/网关 → reload；本命令仅用于产物升级")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}
	if *drain {
		fmt.Fprintln(os.Stderr, "警告: --drain 会先摘流本节点；升级本身替换二进制/packaging，通常不重启 Envoy，但请按双活剧本操作。")
	} else {
		fmt.Fprintln(os.Stderr, "提示: 未加 --drain；双活生产建议 upgrade --drain，或先手动 drain fail。")
	}
	return exitErr(dataplane.Upgrade(mustRoot(), dataplane.UpgradeOptions{Drain: *drain}))
}

func runFirewall(args []string) int {
	apply := false
	rest := args
	if len(args) > 0 {
		switch args[0] {
		case "check":
			rest = args[1:]
			apply = false
		case "apply":
			rest = args[1:]
			apply = true
			_ = os.Setenv("APPLY_FIREWALL", "1")
		case "-h", "--help", "help":
			fmt.Fprintln(os.Stderr, "usage: relaygate firewall [check|apply]")
			fmt.Fprintln(os.Stderr, "  check  渲染并校验防火墙（默认，不改主机规则）")
			fmt.Fprintln(os.Stderr, "  apply  应用防火墙规则（需 root；非交互另需 FIREWALL_CONFIRM=Confirm）")
			return 2
		default:
			if os.Getenv("APPLY_FIREWALL") == "1" {
				apply = true
			}
			rest = args
		}
	} else if os.Getenv("APPLY_FIREWALL") == "1" {
		apply = true
	}
	_ = rest
	return exitErr(dataplane.Firewall(mustRoot(), apply))
}

func runSecurity(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate security list|kernel-conf|apply-kernel|apply-nic|verify")
		fmt.Fprintln(os.Stderr, "  名单与参数编辑 security.access / security.protections；防火墙用 sudo relaygate firewall apply")
		return 2
	}
	root := mustRoot()
	resPath, _, _ := resources.DefaultPaths(root)
	switch args[0] {
	case "list":
		res, err := resources.Load(resPath)
		if err != nil {
			return exitErr(err)
		}
		res.Security.EnsureSecurityDefaults()
		if a := res.Security.Access; a != nil {
			state := "on"
			if !a.Enabled {
				state = "off"
			}
			fmt.Printf("access (firewall ACL) [%s]\n", state)
			for _, c := range a.Deny {
				fmt.Printf("  deny: %s\n", c)
			}
			for _, c := range a.Allow {
				fmt.Printf("  allow: %s\n", c)
			}
		}
		for _, p := range res.Security.Protections {
			state := "on"
			if !p.Enabled {
				state = "off"
			}
			tags := strings.Join(p.AttackTags, ",")
			fmt.Printf("%s (%s) [%s] %s\n", p.ID, p.Type, tags, state)
			if p.ID == resources.PolicyKernelSyn && p.Enabled {
				fmt.Printf("  tcp_syncookies=%d tcp_max_syn_backlog=%d\n",
					p.Params.TcpSyncookies, p.Params.TcpMaxSynBacklog)
			}
			if p.ID == resources.PolicyNICEgressShape && p.Enabled {
				dev := p.Params.Device
				if strings.TrimSpace(dev) == "" {
					dev = "auto"
				}
				rate := p.Params.Rate
				if strings.TrimSpace(rate) == "" {
					rate = resources.DefaultNICEgressRate
				}
				fmt.Printf("  device=%s rate=%s\n", dev, rate)
			}
			if p.ID == resources.PolicyNICIngressPolice && p.Enabled {
				dev := p.Params.Device
				if strings.TrimSpace(dev) == "" {
					dev = "auto"
				}
				rate := p.Params.Rate
				if strings.TrimSpace(rate) == "" {
					rate = resources.DefaultNICIngressRate
				}
				fmt.Printf("  device=%s rate=%s\n", dev, rate)
			}
		}
		fmt.Println("防火墙策略生效: validate + sudo relaygate firewall apply；网关策略: reload；内核(kernel_syn): relaygate security apply-kernel --verify；网卡(nic_egress_shape / nic_ingress_police): relaygate security apply-nic --verify")
		fmt.Println("节点 agent 拉取后默认自动应用主机侧（PANEL_ENABLED=0）；主控默认不自动应用（见 SECURITY_AUTO_APPLY）")
		return 0
	case "kernel-conf":
		res, err := resources.Load(resPath)
		if err != nil {
			return exitErr(err)
		}
		fmt.Print(resources.RenderKernelHardenConf(&res.Security))
		return 0
	case "apply-kernel":
		verify := false
		rest := args[1:]
		for _, a := range rest {
			switch a {
			case "--verify":
				verify = true
			case "-h", "--help", "help":
				fmt.Fprintln(os.Stderr, "usage: relaygate security apply-kernel [--verify]")
				fmt.Fprintln(os.Stderr, "  内核（sysctl）：按 resources.yaml 的 kernel_syn 写入主机内核参数（需 root）")
				fmt.Fprintln(os.Stderr, "  --verify  应用后校验键值是否生效")
				return 2
			default:
				fmt.Fprintf(os.Stderr, "未知参数: %s\n", a)
				return 2
			}
		}
		if err := requireConfirm("将按当前配置写入内核参数叠加并应用。"); err != nil {
			return exitErr(err)
		}
		if err := dataplane.ApplyKernelHardenFromResources(root); err != nil {
			return exitErr(err)
		}
		if verify {
			if err := dataplane.VerifyKernelHarden(root); err != nil {
				return exitErr(err)
			}
			fmt.Println("内核校验通过")
		}
		return 0
	case "apply-nic":
		verify := false
		rest := args[1:]
		for _, a := range rest {
			switch a {
			case "--verify":
				verify = true
			case "-h", "--help", "help":
				fmt.Fprintln(os.Stderr, "usage: relaygate security apply-nic [--verify]")
				fmt.Fprintln(os.Stderr, "  网卡（tc）：按 resources.yaml 已启用的 nic_egress_shape / nic_ingress_police 应用（需 root）")
				fmt.Fprintln(os.Stderr, "  --verify  应用后校验 qdisc / police 是否生效")
				fmt.Fprintln(os.Stderr, "  低带宽示例 rate=3mbit；device 空则探测默认路由口")
				fmt.Fprintln(os.Stderr, "  主控 PANEL_ENABLED=1 默认不会在拉取后自动 apply；请仅在目标节点或明确测试环境执行")
				fmt.Fprintln(os.Stderr, "  回滚出口（手动）：sudo tc qdisc del dev <iface> root")
				fmt.Fprintln(os.Stderr, "  回滚入向（手动）：sudo tc qdisc del dev <iface> ingress")
				fmt.Fprintln(os.Stderr, "  关闭策略不会自动删除已有 qdisc / police")
				return 2
			default:
				fmt.Fprintf(os.Stderr, "未知参数: %s\n", a)
				return 2
			}
		}
		if err := requireConfirm("将按当前配置对本机业务口应用已启用的网卡出口整形与/或入向限速（可能影响吞吐；关闭策略不会自动清除 qdisc/police）。"); err != nil {
			return exitErr(err)
		}
		if err := dataplane.ApplyNICShapeFromResources(root); err != nil {
			return exitErr(err)
		}
		if verify {
			if err := dataplane.VerifyNICShape(root); err != nil {
				return exitErr(err)
			}
			fmt.Println("网卡校验通过")
		}
		return 0
	case "verify":
		env, err := dataplane.LoadEnv(root)
		if err != nil {
			return exitErr(err)
		}
		return exitErr(dataplane.VerifySecurityLayers(root, env, os.Stdout))
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: relaygate security list|kernel-conf|apply-kernel|apply-nic|verify")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "未知 security 子命令: %s\n", args[0])
		return 2
	}
}

func runChanges(args []string) int {
	fs := flag.NewFlagSet("changes", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("limit", 20, "最多显示条数")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: relaygate changes [--limit N]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return exitErr(dataplane.ListChanges(mustRoot(), *limit, os.Stdout))
}

func runProfile(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate profile list|show NAME|apply NAME")
		return 2
	}
	root := mustRoot()
	switch args[0] {
	case "list":
		names, err := profile.List(root)
		if err != nil {
			return exitErr(err)
		}
		if len(names) == 0 {
			fmt.Println("（无预设）")
			return 0
		}
		for _, n := range names {
			p, err := profile.Load(root, n)
			if err != nil {
				fmt.Printf("- %s\n", n)
				continue
			}
			desc := p.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("- %s — %s\n", p.Name, desc)
		}
		return 0
	case "show":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: relaygate profile show NAME")
			return 2
		}
		p, err := profile.Load(root, args[1])
		if err != nil {
			return exitErr(err)
		}
		fmt.Print(profile.FormatShow(p))
		return 0
	case "apply":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: relaygate profile apply NAME")
			return 2
		}
		sum, err := profile.Apply(root, args[1])
		if err != nil {
			return exitErr(err)
		}
		fmt.Print(sum.String())
		fmt.Println("已写入档位（defaults / security）。请: relaygate validate && relaygate reload")
		fmt.Println("若改了防火墙策略，另需: sudo relaygate firewall apply；内核: relaygate security apply-kernel --verify；网卡: relaygate security apply-nic --verify")
		return 0
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: relaygate profile list|show|apply")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "未知 profile 子命令: %s\n", args[0])
		return 2
	}
}

func runSetup(args []string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	nonInteractive := fs.Bool("noninteractive", false, "非交互（读环境变量）")
	applySysctl := fs.Bool("sysctl", false, "同时应用 packaging/sysctl")
	upgrade := fs.Bool("upgrade", false, "升级模式（保留 .env，更新 IMAGE_TAG 等）")
	resetDefaults := fs.Bool("reset-defaults", false, "用仓库模板覆盖 DataDir/resources.yaml 与 inventory（慎用）")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: relaygate setup [--noninteractive] [--sysctl] [--upgrade] [--reset-defaults]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if os.Getenv("NONINTERACTIVE") == "1" {
		*nonInteractive = true
	}
	return exitErr(setup.Run(setup.Options{
		Root:           mustRoot(),
		NonInteractive: *nonInteractive,
		ApplySysctl:    *applySysctl,
		Upgrade:        *upgrade,
		ResetDefaults:  *resetDefaults,
	}))
}

func runDiag(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	strict := fs.Bool("strict-ports", false, "端口占用视为失败")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: relaygate diag [--strict-ports]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return exitErr(diag.Run(diag.Options{
		Root:        mustRoot(),
		StrictPorts: *strict,
	}))
}

func runRender(args []string) int {
	root := mustRoot()
	defRes, defEnvoy, defNFT := resources.DefaultPaths(root)
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	resPath := flags.String("resources", defRes, "resources.yaml 路径")
	envoyOut := flags.String("envoy-out", defEnvoy, "envoy.yaml 输出路径")
	nftOut := flags.String("nft-out", defNFT, "forward-ports.nft 输出路径")
	checkOnly := flags.Bool("check-only", false, "仅校验，不写入文件")
	withObs := flags.Bool("observability", false, "同时渲染 Prometheus 等可观测性配置")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: relaygate render [--check-only] [--observability] [--resources PATH --envoy-out PATH --nft-out PATH]")
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	if *withObs {
		if err := dataplane.RenderObservability(root); err != nil {
			return exitErr(err)
		}
	}
	res, err := resources.Load(*resPath)
	if err != nil {
		return exitErr(err)
	}
	if err := res.Validate(); err != nil {
		return exitErr(err)
	}
	fmt.Print(render.Summarize(res))
	if *checkOnly {
		return 0
	}
	if err := render.Write(*envoyOut, *nftOut, res); err != nil {
		return exitErr(err)
	}
	fmt.Printf("已写入 %s\n", *envoyOut)
	fmt.Printf("已写入 %s\n", *nftOut)
	return 0
}

func runServer(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate server status")
		fmt.Fprintln(os.Stderr, "       relaygate server enable|disable <server-01> | --all-production")
		return 2
	}
	if args[0] == "status" {
		return runServerStatus(args[1:])
	}
	if args[0] != "enable" && args[0] != "disable" {
		fmt.Fprintln(os.Stderr, "usage: relaygate server status")
		fmt.Fprintln(os.Stderr, "       relaygate server enable|disable <server-01> | --all-production")
		return 2
	}
	enabled := args[0] == "enable"
	flags := flag.NewFlagSet("server "+args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	all := flags.Bool("all-production", false, "修改全部正式转发（production）")
	resourcesFlag := flags.String("resources", "", "resources.yaml 路径")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: relaygate server %s <server-01> | --all-production\n", args[0])
	}
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if (*all && flags.NArg() != 0) || (!*all && flags.NArg() != 1) {
		flags.Usage()
		return 2
	}
	resourcesPath := *resourcesFlag
	if resourcesPath == "" {
		resourcesPath, _, _ = resources.DefaultPaths(mustRoot())
	}
	res, err := resources.Load(resourcesPath)
	if err != nil {
		return exitErr(err)
	}
	server := ""
	if !*all {
		server = flags.Arg(0)
	}
	changed := 0
	matched := 0
	for _, fwd := range res.Forwards {
		if fwd.Entry != "production" || (!*all && fwd.Upstream != server) {
			continue
		}
		matched++
		ok, err := resources.PatchForwardEnabledInPlace(resourcesPath, fwd.Name, enabled)
		if err != nil {
			return exitErr(err)
		}
		if ok {
			changed++
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			fmt.Printf("%s: %s\n", state, fwd.Name)
		}
	}
	if matched == 0 {
		return exitErr(fmt.Errorf("没有匹配的正式转发（production）"))
	}
	if changed == 0 {
		fmt.Println("没有转发被修改（已经是目标状态）")
		return 0
	}
	fmt.Printf("已更新 %s（%d 条）\n", resourcesPath, changed)
	// show post-change lifecycle
	if res2, err := resources.Load(resourcesPath); err == nil {
		fmt.Print(resources.FormatLifecycle(res2))
	}
	fmt.Println("请执行: relaygate reload   # 或 Panel → Apply")
	return 0
}

func runServerStatus(args []string) int {
	flags := flag.NewFlagSet("server status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	resourcesFlag := flags.String("resources", "", "resources.yaml 路径")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: relaygate server status [--resources PATH]")
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	resourcesPath := *resourcesFlag
	if resourcesPath == "" {
		resourcesPath, _, _ = resources.DefaultPaths(mustRoot())
	}
	res, err := resources.Load(resourcesPath)
	if err != nil {
		return exitErr(err)
	}
	fmt.Print(resources.FormatLifecycle(res))
	enabled := res.EnabledForwards()
	fmt.Printf("启用转发: %d\n", len(enabled))
	for _, fwd := range enabled {
		fmt.Printf("  - %s: %s/%d -> %s (%s)\n",
			fwd.Name, strings.ToUpper(fwd.Protocol), fwd.ListenPort, fwd.Upstream, fwd.Entry)
	}
	return 0
}

func runPanel(args []string) int {
	if len(args) == 0 {
		return runPanelServe()
	}
	switch args[0] {
	case "install":
		root := mustRoot()
		return exitErr(host.PanelInstall(root, host.PanelInstallOptions{
			InstallDir: config.Getenv("RELAYGATE_INSTALL_DIR", root),
			SecretsDir: config.Getenv("RELAYGATE_SECRETS_DIR", config.DefaultSecretsDir),
			GrafanaURL: os.Getenv("GRAFANA_URL"),
		}))
	case "uninstall":
		return exitErr(host.PanelUninstall(false, false))
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: relaygate panel")
		fmt.Fprintln(os.Stderr, "       relaygate panel install|uninstall")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "未知 panel 子命令: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: relaygate panel [install|uninstall]")
		return 2
	}
}

func runPanelServe() int {
	cfg := panel.Config{
		Root:          config.Getenv("PANEL_ROOT", ""),
		Bind:          config.Getenv("PANEL_BIND", config.DefaultPanelBind),
		AdminPassword: strings.TrimSpace(os.Getenv("PANEL_ADMIN_PASSWORD")),
		EnvoyAdminURL: config.Getenv("ENVOY_ADMIN_URL", "http://127.0.0.1:9901"),
		PrometheusURL: config.Getenv("PROMETHEUS_URL", "http://127.0.0.1:9090"),
		GrafanaURL:    grafanaURLFromEnv(),
	}
	srv, err := panel.New(cfg)
	if err != nil {
		return exitErr(err)
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("ERROR: %v", err)
		return 1
	}
	return 0
}

func repoRoot() (string, error) {
	if root := os.Getenv("PANEL_ROOT"); root != "" {
		return root, nil
	}
	return config.FindRoot()
}

func grafanaURLFromEnv() string {
	v, ok := os.LookupEnv("GRAFANA_URL")
	if !ok {
		return "http://127.0.0.1:3000"
	}
	return strings.TrimSpace(v)
}

func usage(out *os.File) {
	fmt.Fprintln(out, `RelayGate — Envoy L4 网关

首启:
  relaygate setup [--noninteractive] [--sysctl] [--upgrade] [--reset-defaults]
  relaygate diag [--strict-ports]

配置:
  relaygate render [--check-only] [--observability]
  relaygate validate
  relaygate server status
  relaygate server enable|disable <server-01>
  relaygate server enable --all-production
  relaygate security list
  relaygate security kernel-conf
  relaygate security apply-kernel [--verify]
  relaygate security apply-nic [--verify]
  relaygate security verify
  relaygate profile list|show|apply NAME
  relaygate changes [--limit N]

本机网关:
  relaygate apply                 # 校验 + compose up（首次/全量）
  relaygate reload                # 本机应用（优先热更新）
  relaygate reload --hard         # 强制摘流并重启 Envoy
  relaygate rollback [STAMP]      # 回滚并重建 Envoy（会断现有连接）
  relaygate drain fail|ok|status
  relaygate upgrade [--drain]     # 委托 install.sh upgrade（主控/节点同一命令，保留角色）

检查:
  relaygate smoke [HOST]
  relaygate canary [HOST]
  relaygate baseline
  relaygate diag                # admin/drain/热更新/可选云入口清单

防火墙 / Panel:
  relaygate firewall [check|apply]   # 防火墙安全策略；默认 check
  relaygate panel                    # 前台运行管理面
  relaygate panel install|uninstall  # systemd（需 root）

机群（主控）:
  relaygate fleet status
  relaygate fleet publish              # 输入 确认 或 Confirm
  relaygate fleet sync <name>          # 单节点立即拉取；输入 确认 或 Confirm
  relaygate fleet join <name>          # 打印一句话节点安装命令
  relaygate fleet leave <name>         # 输入 确认 或 Confirm

节点 agent:
  relaygate agent run                  # 心跳 + 拉取（可内嵌本机 ADS）
  relaygate agent pull                 # 拉一次并落盘
  relaygate agent install              # systemd（需 root；一句话接入会自动调用）

一键安装 / 升级（见 install.sh --help）:
  主控: curl …/install.sh | sudo bash -s -- control
  节点: fleet join / Panel「接入」生成的一行（node --control … --name … --token …）
  升级: curl …/install.sh | sudo bash -s -- upgrade

变更分流:
  ACL / 仅防火墙          → firewall apply
  resources / 网关配置    → reload（本机）或 fleet publish（机群）
  二进制 / packaging      → upgrade [--drain] 或 install.sh upgrade

  relaygate version`)
}

func runFleet(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate fleet status|publish|sync|join|leave")
		return 2
	}
	root := mustRoot()
	switch args[0] {
	case "status":
		published, nodes, err := agent.BuildStatus(root)
		if err != nil {
			return exitErr(err)
		}
		if published == "" {
			fmt.Println("当前发布版本:（尚无）")
		} else {
			fmt.Println("当前发布版本:", published)
		}
		if len(nodes) == 0 {
			fmt.Println("节点名册为空。可用 fleet join <name> 接入。")
			return 0
		}
		for _, n := range nodes {
			syncMark := ""
			if n.SyncPending {
				syncMark = " sync=pending"
			}
			fmt.Printf("  %-16s role=%-7s status=%-12s applied=%s heartbeat=%s%s\n",
				n.Name, n.Role, n.Status, n.AppliedVersion, n.LastHeartbeat, syncMark)
		}
		return 0
	case "publish":
		if err := requireConfirm("将当前业务配置发布为机群新版本；各网关节点将自行拉取并在本机热更新。"); err != nil {
			return exitErr(err)
		}
		res, err := agent.Publish(root)
		if err != nil {
			return exitErr(err)
		}
		fmt.Printf("已发布版本 %s\n", res.Version)
		return 0
	case "sync":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: relaygate fleet sync <name>")
			return 2
		}
		if err := requireConfirm("将标记该网关节点立即拉取并对齐当前已发布版本（仅该节点；应用时相关连接可能断开）。"); err != nil {
			return exitErr(err)
		}
		res, err := agent.RequestNodeSync(root, args[1])
		if err != nil {
			return exitErr(err)
		}
		fmt.Printf("已标记节点 %s 立即同步（下次心跳拉取并本机落地；不影响其他节点）\n", res.Name)
		return 0
	case "join":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: relaygate fleet join <name>")
			return 2
		}
		res, err := agent.JoinNode(root, args[1], os.Getenv("CONTROL_URL"))
		if err != nil {
			return exitErr(err)
		}
		fmt.Printf("已接入节点 %s（写入名册并签发一次性令牌；不影响主控与既有节点转发）\n", res.Name)
		fmt.Println()
		fmt.Println("在目标主机执行：")
		fmt.Println(res.JoinCommand)
		fmt.Println()
		fmt.Printf("令牌文件（主控侧备份）: %s\n", res.TokenFileHint)
		return 0
	case "leave":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: relaygate fleet leave <name>")
			return 2
		}
		if err := requireConfirm("将从机群名册移除该网关节点并吊销其代理凭证。"); err != nil {
			return exitErr(err)
		}
		res, err := agent.LeaveNode(root, args[1])
		if err != nil {
			return exitErr(err)
		}
		fmt.Printf("已退役节点 %s\n", res.Name)
		for _, h := range res.ManualHints {
			fmt.Println("-", h)
		}
		return 0
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: relaygate fleet status|publish|sync|join|leave")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知 fleet 子命令: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: relaygate fleet status|publish|sync|join|leave")
		return 2
	}
}

func runAgent(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate agent run|pull|install")
		return 2
	}
	root := mustRoot()
	switch args[0] {
	case "install":
		return exitErr(host.AgentInstall(root, host.AgentInstallOptions{
			InstallDir: config.Getenv("RELAYGATE_INSTALL_DIR", root),
		}))
	case "pull":
		client, err := agent.LoadClientFromEnv()
		if err != nil {
			return exitErr(err)
		}
		ver, err := client.PullOnce(root)
		if err != nil {
			return exitErr(err)
		}
		fmt.Printf("已拉取并落盘版本 %s（applied 未更新）\n", ver)
		fmt.Println("提示: agent run 完成拉取后落地（内核→网卡→防火墙→网关）成功后才会上报 applied；也可手动 security apply-kernel / apply-nic / firewall apply / reload。")
		return 0
	case "run":
		if _, err := dataplane.LoadEnv(root); err != nil {
			return exitErr(err)
		}
		after := func(r, ver string) error {
			e, err := dataplane.LoadEnv(r)
			if err != nil {
				return err
			}
			return dataplane.AfterPullApply(dataplane.PullApplyOptions{
				Root:    r,
				Version: ver,
				Env:     e,
				Stdout:  os.Stdout,
				Stderr:  os.Stderr,
			})
		}
		return exitErr(agent.Run(agent.RunOptions{Root: root, AfterPull: after}))
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: relaygate agent run|pull|install")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知 agent 子命令: %s\n", args[0])
		return 2
	}
}

func requireConfirm(risk string) error {
	if confirm.Match(os.Getenv("RELAYGATE_CONFIRM")) {
		return nil
	}
	fmt.Fprintln(os.Stderr, risk)
	fmt.Fprintf(os.Stderr, "请输入 确认 或 Confirm（非交互可设 RELAYGATE_CONFIRM=Confirm）: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return fmt.Errorf("需要确认（输入 确认 或 Confirm）")
	}
	if !confirm.Match(line) {
		return fmt.Errorf("确认词不匹配，已取消")
	}
	return nil
}

func reloadWithOpts(opts dataplane.ReloadOptions) error {
	root := mustRoot()
	if opts.ForceHard {
		env, err := dataplane.LoadEnv(root)
		if err != nil {
			return err
		}
		return dataplane.HardReloadTo(root, env, os.Stdout, os.Stderr)
	}
	return dataplane.ReloadTo(root, os.Stdout, os.Stderr, opts)
}

func parseReloadArgs(args []string, opts *dataplane.ReloadOptions) []string {
	var rest []string
	for _, a := range args {
		if a == "--hard" {
			opts.ForceHard = true
			continue
		}
		rest = append(rest, a)
	}
	return rest
}

func runXDS(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate xds serve|apply")
		return 2
	}
	root := mustRoot()
	switch args[0] {
	case "serve":
		env, err := dataplane.LoadEnv(root)
		if err != nil {
			return exitErr(err)
		}
		if !env.XDSEnabled {
			fmt.Fprintln(os.Stderr, "热更新已关闭，无法启动本机热更新服务")
			return 1
		}
		xds.SetDiskPublishHandler(func(nodeID string) (string, error) {
			e, err := dataplane.LoadEnv(root)
			if err != nil {
				return "", err
			}
			srv := xds.Global().Server()
			if srv == nil {
				return "", fmt.Errorf("xds: ADS not running")
			}
			return dataplane.PublishSnapshotFromDisk(root, e, nodeID, srv.Publisher)
		})
		if err := dataplane.PublishInitialSnapshot(root, env); err != nil {
			return exitErr(err)
		}
		fmt.Printf("xDS ADS on 127.0.0.1:%s (node=%s); Ctrl+C 退出\n", env.XDSPort, env.GatewayName)
		select {} // block until signal — thin agent resident
	case "apply":
		env, err := dataplane.LoadEnv(root)
		if err != nil {
			return exitErr(err)
		}
		if !env.XDSEnabled {
			fmt.Fprintln(os.Stderr, "热更新已关闭；请用 reload --hard")
			return 1
		}
		return exitErr(dataplane.HotApplyTo(root, env, os.Stdout, os.Stderr))
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: relaygate xds serve|apply")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知 xds 子命令: %s\n", args[0])
		return 2
	}
}
