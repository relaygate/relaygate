package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/doctor"
	"github.com/relaygate/relaygate/core/envoygen"
	"github.com/relaygate/relaygate/core/host"
	"github.com/relaygate/relaygate/core/ops"
	"github.com/relaygate/relaygate/core/panel"
	"github.com/relaygate/relaygate/core/resources"
	"github.com/relaygate/relaygate/core/setup"
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
		return exitErr(ops.Validate(mustRoot()))
	case "apply":
		return exitErr(ops.Apply(mustRoot()))
	case "reload":
		return exitErr(ops.Reload(mustRoot()))
	case "rollback":
		stamp := ""
		if len(args) > 1 {
			stamp = args[1]
		}
		return exitErr(ops.Rollback(mustRoot(), stamp))
	case "drain":
		return runDrain(args[1:])
	case "smoke":
		hostArg := ""
		if len(args) > 1 {
			hostArg = args[1]
		}
		return exitErr(ops.Smoke(mustRoot(), hostArg))
	case "canary":
		hostArg := ""
		if len(args) > 1 {
			hostArg = args[1]
		}
		return exitErr(ops.Canary(mustRoot(), hostArg))
	case "firewall":
		return runFirewall(args[1:])
	case "baseline":
		out := ""
		if len(args) > 1 {
			out = args[1]
		}
		return exitErr(ops.Baseline(mustRoot(), out))
	case "fleet":
		return exitErr(ops.Fleet(mustRoot(), os.Getenv("GATEWAYS")))
	case "setup":
		return runSetup(args[1:])
	case "doctor":
		return runDoctor(args[1:])
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
	case "fail", "drain", "ok", "undrain", "status":
		return exitErr(ops.Drain(mustRoot(), args[0]))
	default:
		fmt.Fprintln(os.Stderr, "usage: relaygate drain fail|ok|status")
		return 2
	}
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
			fmt.Fprintln(os.Stderr, "  check  渲染并校验 nft（默认，不改主机规则）")
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
	return exitErr(ops.Firewall(mustRoot(), apply))
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

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	strict := fs.Bool("strict-ports", false, "端口占用视为失败")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: relaygate doctor [--strict-ports]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return exitErr(doctor.Run(doctor.Options{
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
		if err := ops.RenderObservability(root); err != nil {
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
	fmt.Print(envoygen.Summarize(res))
	if *checkOnly {
		return 0
	}
	if err := envoygen.Write(*envoyOut, *nftOut, res); err != nil {
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
	all := flags.Bool("all-production", false, "修改全部 production 规则")
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
		if rule.Kind != "production" || (!*all && rule.Server != server) {
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
		return exitErr(fmt.Errorf("没有匹配的 production 规则"))
	}
	if changed == 0 {
		fmt.Println("没有规则被修改（已经是目标状态）")
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
	fmt.Printf("启用规则: %d\n", len(enabled))
	for _, rule := range enabled {
		fmt.Printf("  - %s: %s/%d -> %s (%s)\n",
			rule.Name, strings.ToUpper(rule.Protocol), rule.ListenPort, rule.Server, rule.Kind)
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
		AdminPassword: os.Getenv("PANEL_ADMIN_PASSWORD"),
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
	fmt.Fprintln(out, `RelayGate — Envoy 游戏网关

首启:
  relaygate setup [--noninteractive] [--sysctl] [--upgrade] [--reset-defaults]
  relaygate doctor [--strict-ports]

配置:
  relaygate render [--check-only] [--observability]
  relaygate validate
  relaygate server status
  relaygate server enable|disable <server-01>
  relaygate server enable --all-production

数据面:
  relaygate apply                 # 校验 + compose up（首次/全量）
  relaygate reload                # 备份摘要 + drain + 重启 Envoy（分阶段计时）
  relaygate rollback [STAMP]
  relaygate drain fail|ok|status

检查:
  relaygate smoke [HOST]
  relaygate canary [HOST]
  relaygate baseline
  relaygate doctor                # 含 admin/drain/双活 env

防火墙 / Panel:
  relaygate firewall [check|apply]   # 默认 check，不改主机
  relaygate panel                    # 前台运行管理面
  relaygate panel install|uninstall  # systemd（需 root）

多机:
  relaygate fleet                 # 按 inventory 分批部署

  relaygate version`)
}
