package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Default tests assume unset override; EnvOverride tests re-set via t.Setenv.
	_ = os.Unsetenv("RELAYGATE_DATA_DIR")
	os.Exit(m.Run())
}
