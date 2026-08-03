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
	"github.com/relaygate/relaygate/core/diag"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/host"
	"github.com/relaygate/relaygate/core/dataplane"
	"github.com/relaygate/relaygate/core/panel"
	"github.com/relaygate/relaygate/core/profile"
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
	case "acl":
		return runACL(args[1:])
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
		fmt.Fprintln(fs.Output(), "  二进制/packaging 升级：委托 install.sh --upgrade（默认最新 Release；可设 RELAYGATE_VERSION / RELAYGATE_TAR）")
		fmt.Fprintln(fs.Output(), "  ACL/nftables → firewall apply；resources/Envoy → reload；本命令仅用于产物升级")
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
			fmt.Fprintln(os.Stderr, "  check  渲染并校验 nftables（默认，不改主机规则）")
			fmt.Fprintln(os.Stderr, "  apply  应用规则（需 root；非交互另需 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES）")
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

func runACL(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate acl list")
		fmt.Fprintln(os.Stderr, "       relaygate acl add deny|allow CIDR")
		fmt.Fprintln(os.Stderr, "       relaygate acl remove deny|allow CIDR")
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
		_ = res.ACL.NormalizeACL()
		fmt.Println("deny:")
		if len(res.ACL.Deny) == 0 {
			fmt.Println("  (empty)")
		}
		for _, c := range res.ACL.Deny {
			fmt.Printf("  - %s\n", c)
		}
		fmt.Println("allow:")
		if len(res.ACL.Allow) == 0 {
			fmt.Println("  (empty — 非严格模式)")
		} else {
			fmt.Println("  (strict — 仅下列 CIDR 可进转发口)")
		}
		for _, c := range res.ACL.Allow {
			fmt.Printf("  - %s\n", c)
		}
		fmt.Println("变更分流：ACL → validate + sudo relaygate firewall apply（无需 reload Envoy）")
		return 0
	case "add", "remove":
		if len(args) != 3 {
			fmt.Fprintf(os.Stderr, "usage: relaygate acl %s deny|allow CIDR\n", args[0])
			return 2
		}
		list, cidr := args[1], args[2]
		res, err := resources.Load(resPath)
		if err != nil {
			return exitErr(err)
		}
		var canonical string
		if args[0] == "add" {
			canonical, err = res.AddACLEntry(list, cidr)
		} else {
			canonical, err = res.RemoveACLEntry(list, cidr)
		}
		if err != nil {
			return exitErr(err)
		}
		if err := resources.Save(resPath, res); err != nil {
			return exitErr(err)
		}
		fmt.Printf("%s %s %s → %s\n", args[0], list, canonical, resPath)
		fmt.Println("变更分流：ACL → validate + sudo relaygate firewall apply（无需 reload Envoy）")
		return 0
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: relaygate acl list|add|remove …")
		return 2
	default:
		fmt.Fprintf(os.Stderr, "未知 acl 子命令: %s\n", args[0])
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
		fmt.Println("已写入 defaults。请: relaygate validate && relaygate reload")
		fmt.Println("若改了 nftables 档位，另需: sudo relaygate firewall apply")
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
	for _, rule := range res.Rules {
		if rule.Entry != "production" || (!*all && rule.Server != server) {
			continue
		}
		matched++
		ok, err := resources.PatchRuleEnabledInPlace(resourcesPath, rule.Name, enabled)
		if err != nil {
			return exitErr(err)
		}
		if ok {
			changed++
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			fmt.Printf("%s: %s\n", state, rule.Name)
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
	enabled := res.EnabledRules()
	fmt.Printf("启用转发: %d\n", len(enabled))
	for _, rule := range enabled {
		fmt.Printf("  - %s: %s/%d -> %s (%s)\n",
			rule.Name, strings.ToUpper(rule.Protocol), rule.ListenPort, rule.Server, rule.Entry)
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
  relaygate acl list|add|remove deny|allow CIDR
  relaygate profile list|show|apply NAME
  relaygate changes [--limit N]

本机网关:
  relaygate apply                 # 校验 + compose up（首次/全量）
  relaygate reload                # 本机应用（优先热更新）
  relaygate reload --hard         # 强制摘流并重启 Envoy
  relaygate rollback [STAMP]      # 回滚并重建 Envoy（会断现有连接）
  relaygate drain fail|ok|status
  relaygate upgrade [--drain]     # 委托 install.sh --upgrade（主控/节点同一命令，保留角色）

检查:
  relaygate smoke [HOST]
  relaygate canary [HOST]
  relaygate baseline
  relaygate diag                # admin/drain/热更新/可选云入口清单

防火墙 / Panel:
  relaygate firewall [check|apply]   # ACL/nftables-only；默认 check
  relaygate panel                    # 前台运行管理面
  relaygate panel install|uninstall  # systemd（需 root）

机群（主控）:
  relaygate fleet status
  relaygate fleet publish              # 确认 PUBLISH_FLEET
  relaygate fleet join <name>          # 打印一句话节点安装命令
  relaygate fleet leave <name>         # 确认 FLEET_LEAVE

节点代理:
  relaygate agent run                  # 心跳 + 拉取（可内嵌本机 ADS）
  relaygate agent pull                 # 拉一次并落盘
  relaygate agent install              # systemd（需 root；一句话接入会自动调用）

一键安装 / 升级（见 install.sh --help）:
  主控: curl …/install.sh | sudo env ENABLE_PANEL=1 NONINTERACTIVE=1 bash -s -- -y
  节点: fleet join 输出的一行，或 PRIMARY_URL+AGENT_TOKEN+GATEWAY_NAME
  升级: curl …/install.sh | sudo bash -s -- --upgrade -y

变更分流:
  ACL / nftables-only     → firewall apply
  resources / Envoy 配置  → reload（本机）或 fleet publish（机群）
  二进制 / packaging      → upgrade [--drain] 或 install.sh --upgrade

  relaygate version`)
}

func runFleet(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate fleet status|publish|join|leave")
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
			fmt.Printf("  %-16s role=%-7s status=%-12s applied=%s heartbeat=%s\n",
				n.Name, n.Role, n.Status, n.AppliedVersion, n.LastHeartbeat)
		}
		return 0
	case "publish":
		if err := requireConfirm("PUBLISH_FLEET", "将当前业务配置发布为机群新版本；各网关节点将自行拉取并在本机热更新。"); err != nil {
			return exitErr(err)
		}
		res, err := agent.Publish(root)
		if err != nil {
			return exitErr(err)
		}
		fmt.Printf("已发布版本 %s\n", res.Version)
		return 0
	case "join":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: relaygate fleet join <name>")
			return 2
		}
		res, err := agent.JoinNode(root, args[1], os.Getenv("PRIMARY_URL"))
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
		if err := requireConfirm("FLEET_LEAVE", "将从机群名册移除该网关节点并吊销其代理凭证。"); err != nil {
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
		fmt.Fprintln(os.Stderr, "usage: relaygate fleet status|publish|join|leave")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知 fleet 子命令: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: relaygate fleet status|publish|join|leave")
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
		fmt.Printf("已拉取并落盘版本 %s\n", ver)
		fmt.Println("提示: 本机热更新请执行 relaygate reload（或由 agent run 的 AfterPull 触发）。")
		return 0
	case "run":
		env, err := dataplane.LoadEnv(root)
		if err != nil {
			return exitErr(err)
		}
		after := func(r, _ string) error {
			if !env.XDSEnabled {
				fmt.Println("热更新已关闭：已落盘，请手动 reload --hard")
				return nil
			}
			xds.SetDiskPublishHandler(func(nodeID string) (string, error) {
				e, err := dataplane.LoadEnv(r)
				if err != nil {
					return "", err
				}
				srv := xds.Global().Server()
				if srv == nil {
					return "", fmt.Errorf("本机热更新服务未运行")
				}
				return dataplane.PublishSnapshotFromDisk(r, e, nodeID, srv.Publisher)
			})
			if xds.Global().Server() == nil {
				if err := dataplane.PublishInitialSnapshot(r, env); err != nil {
					fmt.Fprintf(os.Stderr, "启动本机热更新服务: %v（仍保留已拉取配置）\n", err)
					return nil
				}
			}
			return dataplane.HotApplyTo(r, env, os.Stdout, os.Stderr)
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

func requireConfirm(phrase, risk string) error {
	if v := strings.TrimSpace(os.Getenv("RELAYGATE_CONFIRM")); v == phrase {
		return nil
	}
	fmt.Fprintln(os.Stderr, risk)
	fmt.Fprintf(os.Stderr, "请输入 %s 确认（非交互可设 RELAYGATE_CONFIRM=%s）: ", phrase, phrase)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return fmt.Errorf("需要确认词 %s", phrase)
	}
	if strings.TrimSpace(line) != phrase {
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
