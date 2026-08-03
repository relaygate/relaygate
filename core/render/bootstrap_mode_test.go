package render

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsBootstrapMigrated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	migrated := filepath.Join(dir, "migrated.yaml")
	static := filepath.Join(dir, "static.yaml")
	missing := filepath.Join(dir, "missing.yaml")

	if err := os.WriteFile(migrated, []byte(`
node: {id: gateway-01}
static_resources:
  clusters:
    - name: xds_cluster
dynamic_resources:
  cds_config: {ads: {}}
  lds_config: {ads: {}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(static, []byte(`
static_resources:
  listeners:
    - name: ingress-foo
  clusters:
    - name: upstream-server-01-tcp
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !IsBootstrapMigrated(migrated) {
		t.Fatal("expected migrated")
	}
	if IsBootstrapMigrated(static) {
		t.Fatal("full static must not count as migrated")
	}
	if IsBootstrapMigrated(missing) {
		t.Fatal("missing file must not count as migrated")
	}
}
