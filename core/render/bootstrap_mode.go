package render

import (
	"os"
	"strings"
)

// IsBootstrapMigrated reports whether on-disk envoy.yaml is ADS bootstrap
// (dynamic_resources + xds_cluster) rather than a full static_resources dump.
// Missing or unreadable files are treated as not migrated.
func IsBootstrapMigrated(envoyYAMLPath string) bool {
	b, err := os.ReadFile(envoyYAMLPath)
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "dynamic_resources") &&
		strings.Contains(s, "xds_cluster") &&
		strings.Contains(s, "cds_config")
}
