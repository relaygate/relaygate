package profile

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Unsetenv("RELAYGATE_DATA_DIR")
	os.Exit(m.Run())
}
