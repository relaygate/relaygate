package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runtimeDataDirs are created empty under data/ (runtime only; not versioned).
var runtimeDataDirs = []string{
	"envoy",
	"firewall",
	"prometheus",
	"backups",
	"inventory",
}

// SeedDefaults ensures data/ runtime skeleton and copies versioned templates
// when targets are missing (or reset is true). Does not touch generated
// envoy/prometheus/firewall outputs or Grafana provisioning (those live under
// core/deploy and are mounted/rendered separately).
func SeedDefaults(root string, reset bool) error {
	if root == "" {
		return fmt.Errorf("root required")
	}
	if err := ensureDataDirs(root); err != nil {
		return err
	}
	if err := seedFile(root, "resources.example.yaml", filepath.Join("data", "resources.yaml"), reset,
		"占位 IP；请填入真实后端后 relaygate render / reload"); err != nil {
		return err
	}
	if err := seedFile(root, "gateways.env.example", filepath.Join("data", "inventory", "gateways.env"), reset,
		"多机 fleet 用；单机可忽略"); err != nil {
		return err
	}
	return nil
}

func ensureDataDirs(root string) error {
	for _, name := range runtimeDataDirs {
		if err := os.MkdirAll(filepath.Join(root, "data", name), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// seedFile copies a root-level versioned template into data/ when missing,
// or when reset is true. Never silently overwrites an existing target.
func seedFile(root, exampleRel, destRel string, reset bool, hint string) error {
	dst := filepath.Join(root, destRel)
	if !reset {
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			return nil
		}
	}
	src := filepath.Join(root, exampleRel)
	b, err := os.ReadFile(src)
	if err != nil {
		if reset || !os.IsNotExist(err) {
			return fmt.Errorf("无法读取模板 %s: %w", src, err)
		}
		// Fresh tree without example: leave data/ empty; apply/validate will fail loudly.
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o660)
	if strings.HasSuffix(destRel, ".env") {
		mode = 0o600
	}
	if err := os.WriteFile(dst, b, mode); err != nil {
		return err
	}
	action := "已从模板生成"
	if reset {
		action = "已按 --reset-defaults 覆盖"
	}
	fmt.Printf("WARN: %s %s（%s）\n", action, dst, hint)
	return nil
}
