// Package ops implements RelayGate data-plane workflows (apply, reload, seed, firewall…).
// Host Panel systemd install lives in core/host. Versioned templates live in packaging/.
// Runtime state lives in config.ResolveDataDir (never in the source-tree layout).
package ops

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/relaygate/relaygate/core/config"
)

// Env is an alias for config.Env for call-site convenience within ops.
type Env = config.Env

// LoadEnv loads root/.env and known keys via core/config.
func LoadEnv(root string) (Env, error) {
	return config.LoadEnv(root)
}

func getenv(key, fallback string) string {
	return config.Getenv(key, fallback)
}

func requireEnvFile(root string) error {
	path := filepath.Join(root, ".env")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("缺少 .env，请先: relaygate setup 或 cp .env.example .env")
	}
	return nil
}
