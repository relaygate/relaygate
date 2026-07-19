package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/robot/proxy/internal/assets"
	"github.com/robot/proxy/internal/envoygen"
	"github.com/robot/proxy/internal/panel"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}

	switch args[0] {
	case "render":
		return runRender(args[1:])
	case "server":
		return runServer(args[1:])
	case "panel":
		return runPanel(args[1:])
	case "version":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: relaygate version")
			return 2
		}
		fmt.Printf("relaygate %s\n", version)
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

func runRender(args []string) int {
	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	defRes, defEnvoy, defNFT := assets.DefaultPaths(root)
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	resources := flags.String("resources", defRes, "resources.yaml 路径")
	envoyOut := flags.String("envoy-out", defEnvoy, "envoy.yaml 输出路径")
	nftOut := flags.String("nft-out", defNFT, "game-ports.nft 输出路径")
	checkOnly := flags.Bool("check-only", false, "仅校验，不写入文件")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "usage: relaygate render [--check-only] [--resources PATH --envoy-out PATH --nft-out PATH]")
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}

	res, err := assets.Load(*resources)
	if err != nil {
		return fail(err)
	}
	if err := res.Validate(); err != nil {
		return fail(err)
	}
	fmt.Print(envoygen.Summarize(res))
	if *checkOnly {
		return 0
	}
	if err := envoygen.Write(*envoyOut, *nftOut, res); err != nil {
		return fail(err)
	}
	fmt.Printf("已写入 %s\n", *envoyOut)
	fmt.Printf("已写入 %s\n", *nftOut)
	return 0
}

func runServer(args []string) int {
	if len(args) == 0 || (args[0] != "enable" && args[0] != "disable") {
		fmt.Fprintln(os.Stderr, "usage: relaygate server enable|disable <server-01> | --all-production")
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
		root, err := repoRoot()
		if err != nil {
			return fail(err)
		}
		resourcesPath, _, _ = assets.DefaultPaths(root)
	}
	res, err := assets.Load(resourcesPath)
	if err != nil {
		return fail(err)
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
		ok, err := assets.PatchRuleEnabledInPlace(resourcesPath, rule.Name, enabled)
		if err != nil {
			return fail(err)
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
		return fail(fmt.Errorf("没有匹配的 production 规则"))
	}
	if changed == 0 {
		fmt.Println("没有规则被修改（已经是目标状态）")
		return 0
	}
	fmt.Printf("已更新 %s（%d 条）\n", resourcesPath, changed)
	fmt.Println("请执行: ./bin/relaygate render && bash scripts/deploy.sh")
	return 0
}

func runPanel(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: relaygate panel")
		return 2
	}
	cfg := panel.Config{
		Root:          env("PANEL_ROOT", ""),
		Bind:          env("PANEL_BIND", "127.0.0.1:8080"),
		AdminPassword: os.Getenv("PANEL_ADMIN_PASSWORD"),
		EnvoyAdminURL: env("ENVOY_ADMIN_URL", "http://127.0.0.1:9901"),
		PrometheusURL: env("PROMETHEUS_URL", "http://127.0.0.1:9090"),
	}
	srv, err := panel.New(cfg)
	if err != nil {
		return fail(err)
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
	return assets.FindRoot()
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	return 1
}

func usage(out *os.File) {
	fmt.Fprintln(out, `RelayGate

usage:
  relaygate render [--check-only] [--resources PATH --envoy-out PATH --nft-out PATH]
  relaygate server enable <server-01>
  relaygate server disable <server-01>
  relaygate server enable --all-production
  relaygate panel
  relaygate version`)
}
