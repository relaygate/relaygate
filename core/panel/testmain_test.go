package panel

import (
	"os"
	"testing"
)

// Clear exported DataDir so panel helpers never resolve to a real checkout
// DataDir when RELAYGATE_DATA_DIR is set in the developer shell.
func TestMain(m *testing.M) {
	_ = os.Unsetenv("RELAYGATE_DATA_DIR")
	os.Exit(m.Run())
}
