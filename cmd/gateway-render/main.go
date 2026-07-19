package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/robot/proxy/internal/assets"
	"github.com/robot/proxy/internal/envoygen"
)

func main() {
	root, _ := assets.FindRoot()
	if env := os.Getenv("PANEL_ROOT"); env != "" {
		root = env
	}
	defRes, defEnvoy, defNFT := assets.DefaultPaths(root)

	resources := flag.String("resources", defRes, "path to resources.yaml")
	envoyOut := flag.String("envoy-out", defEnvoy, "envoy.yaml output")
	nftOut := flag.String("nft-out", defNFT, "game-ports.nft output")
	checkOnly := flag.Bool("check-only", false, "validate only")
	flag.Parse()

	args := flag.Args()
	if len(args) > 0 {
		os.Exit(runSubcommand(*resources, args))
	}

	res, err := assets.Load(*resources)
	if err != nil {
		fatal(err)
	}
	if err := res.Validate(); err != nil {
		fatal(err)
	}
	fmt.Print(envoygen.Summarize(res))
	if *checkOnly {
		return
	}
	if err := envoygen.Write(*envoyOut, *nftOut, res); err != nil {
		fatal(err)
	}
	fmt.Printf("已写入 %s\n", *envoyOut)
	fmt.Printf("已写入 %s\n", *nftOut)
}

func runSubcommand(resourcesPath string, args []string) int {
	cmd := args[0]
	switch cmd {
	case "enable", "disable":
		enabled := cmd == "enable"
		all := false
		server := ""
		for _, a := range args[1:] {
			if a == "--all-production" {
				all = true
				continue
			}
			server = a
		}
		if !all && server == "" {
			fmt.Fprintln(os.Stderr, "usage: gateway-render enable|disable <server-01> | --all-production")
			return 2
		}
		res, err := assets.Load(resourcesPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		changed := 0
		for _, rule := range res.Rules {
			if rule.Kind != "production" {
				continue
			}
			if !all && rule.Server != server {
				continue
			}
			ok, err := assets.PatchRuleEnabledInPlace(resourcesPath, rule.Name, enabled)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
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
		if changed == 0 {
			fmt.Println("没有规则被修改（可能已是目标状态或 server 名错误）")
			return 0
		}
		fmt.Printf("已更新 %s（%d 条）\n", resourcesPath, changed)
		fmt.Println("请执行: ./bin/gateway-render && bash scripts/deploy.sh")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", cmd)
		return 2
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}
