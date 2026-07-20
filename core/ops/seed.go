package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/relaygate/relaygate/core/config"
)

// runtimeDataDirs are created empty under DataDir (runtime only; not versioned).
var runtimeDataDirs = []string{
	"envoy",
	"envoy/logs",
	"firewall",
	"prometheus",
	"backups",
	"inventory",
}

// SeedDefaults ensures DataDir runtime skeleton and copies versioned templates
// when targets are missing (or reset is true). Does not touch generated
// envoy/prometheus/firewall outputs or Grafana provisioning (those live under
// packaging/ and are mounted/rendered separately).
func SeedDefaults(root string, reset bool) error {
	if root == "" {
		return fmt.Errorf("root required")
	}
	p := config.ResolvePaths(root)
	if err := ensureDataDirs(p.DataDir); err != nil {
		return err
	}
	if err := seedFile(root, p.DataDir, "resources.example.yaml", filepath.Join(p.DataDir, "resources.yaml"), reset,
		"占位 IP；请填入真实后端后 relaygate render / reload"); err != nil {
		return err
	}
	if err := seedFile(root, p.DataDir, "gateways.env.example", filepath.Join(p.DataDir, "inventory", "gateways.env"), reset,
		"多机 fleet 用；单机可忽略"); err != nil {
		return err
	}
	return nil
}

func ensureDataDirs(dataDir string) error {
	for _, name := range runtimeDataDirs {
		mode := os.FileMode(0o755)
		if name == "envoy/logs" {
			// Envoy 容器 uid=101 需写 access log；bind mount 到 DataDir。
			mode = 0o777
		}
		if err := os.MkdirAll(filepath.Join(dataDir, name), mode); err != nil {
			return err
		}
	}
	return nil
}

// seedFile copies a root-level versioned template into DataDir when missing,
// or when reset is true. Never silently overwrites an existing target.
func seedFile(root, dataDir, exampleRel, dest string, reset bool, hint string) error {
	if !reset {
		if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
			return nil
		}
	}
	src := filepath.Join(root, exampleRel)
	b, err := os.ReadFile(src)
	if err != nil {
		if reset || !os.IsNotExist(err) {
			return fmt.Errorf("无法读取模板 %s: %w", src, err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o660)
	if strings.HasSuffix(dest, ".env") {
		mode = 0o600
	}
	if err := os.WriteFile(dest, b, mode); err != nil {
		return err
	}
	action := "已从模板生成"
	if reset {
		action = "已按 --reset-defaults 覆盖"
	}
	fmt.Printf("WARN: %s %s（%s）\n", action, dest, hint)
	return nil
}
