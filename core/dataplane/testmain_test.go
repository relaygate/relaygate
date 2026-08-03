package dataplane

import (
	"os"
	"testing"
)

// Clear exported DataDir so parallel tests (which cannot use t.Setenv) never
// touch a developer machine's .runtime/data.
func TestMain(m *testing.M) {
	_ = os.Unsetenv("RELAYGATE_DATA_DIR")
	os.Exit(m.Run())
}
