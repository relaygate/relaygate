package config

import "testing"

// IsolateDataDirEnv clears RELAYGATE_DATA_DIR for the duration of t so
// ResolveDataDir / ResolvePaths use the test root instead of a
// developer-exported path (e.g. .runtime/data).
func IsolateDataDirEnv(t testing.TB) {
	t.Helper()
	t.Setenv("RELAYGATE_DATA_DIR", "")
}
