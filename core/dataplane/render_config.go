package dataplane

import (
	"fmt"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/resources"
)

// RenderConfig loads resources, validates, and writes envoy + nft under DataDir.
// When XDS_ENABLED=1, writes bootstrap + nft (business via ADS).
func RenderConfig(root string, checkOnly bool) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	resPath, envoyOut, nftOut := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return err
	}
	if err := res.Validate(); err != nil {
		return err
	}
	fmt.Print(render.Summarize(res))
	if checkOnly {
		return nil
	}
	if env.XDSEnabled {
		bootOpt := render.BootstrapOptionsFromEnv(env.GatewayName, env.XDSPort, res)
		opt := render.OptionsFromEnv()
		if err := render.WriteBootstrap(envoyOut, nftOut, res, bootOpt, opt); err != nil {
			return err
		}
		fmt.Printf("已写入 bootstrap %s (xDS mode)\n", envoyOut)
		fmt.Printf("已写入 %s\n", nftOut)
		return nil
	}
	if err := render.Write(envoyOut, nftOut, res); err != nil {
		return err
	}
	fmt.Printf("已写入 %s\n", envoyOut)
	fmt.Printf("已写入 %s\n", nftOut)
	return nil
}

// RenderConfigPaths writes envoy/nft using explicit paths (tests).
func RenderConfigPaths(root string, envoyOut, nftOut string, xdsEnabled bool) error {
	env, err := LoadEnv(root)
	if err != nil {
		return err
	}
	resPath := config.ResolvePaths(root).Resources
	res, err := resources.Load(resPath)
	if err != nil {
		return err
	}
	if err := res.Validate(); err != nil {
		return err
	}
	if xdsEnabled || env.XDSEnabled {
		bootOpt := render.BootstrapOptionsFromEnv(env.GatewayName, env.XDSPort, res)
		return render.WriteBootstrap(envoyOut, nftOut, res, bootOpt, render.OptionsFromEnv())
	}
	return render.Write(envoyOut, nftOut, res)
}
