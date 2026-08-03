package dataplane

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

// BackupWithSummary diffs current resources against previous backups/latest,
// creates a new stamp backup (including change-summary.txt), and prints the summary.
func BackupWithSummary(root string, w io.Writer) (stamp, backupDir string, summary resources.ChangeSummary, err error) {
	p := config.ResolvePaths(root)
	after, err := resources.Load(p.Resources)
	if err != nil {
		return "", "", summary, err
	}
	before, _, err := resources.LoadPreviousBackupResources(root)
	if err != nil {
		return "", "", summary, err
	}
	summary = resources.Diff(before, after)
	if w != nil {
		fmt.Fprint(w, summary.String())
		fmt.Fprint(w, resources.FormatLifecycle(after))
	}

	stamp = time.Now().Format("20060102-150405")
	backupDir = filepath.Join(p.Backups, stamp)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", "", summary, err
	}
	for _, src := range []string{p.EnvoyYAML, p.Compose, p.Resources, p.PromYAML, p.ForwardPorts} {
		if b, readErr := os.ReadFile(src); readErr == nil {
			_ = os.WriteFile(filepath.Join(backupDir, filepath.Base(src)), b, 0o644)
		}
	}
	if err := resources.WriteChangeSummaryFile(backupDir, summary); err != nil {
		return stamp, backupDir, summary, err
	}
	_ = os.WriteFile(filepath.Join(p.Backups, "latest"), []byte(stamp+"\n"), 0o644)
	return stamp, backupDir, summary, nil
}
