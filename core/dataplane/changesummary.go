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

// DiscardBackup removes a backup directory written by an apply that then
// failed, and rewinds the "latest" pointer to the previous successful backup.
//
// Reload decides whether anything changed by diffing resources.yaml against
// the latest backup. If a failed apply leaves its own backup behind, that diff
// becomes empty and every later reload silently reports "no change": the
// gateway keeps running the old config while the UI says everything is fine.
func DiscardBackup(root, stamp string) error {
	if stamp == "" {
		return nil
	}
	p := config.ResolvePaths(root)
	if err := os.RemoveAll(filepath.Join(p.Backups, stamp)); err != nil {
		return err
	}
	prev, err := latestBackupStamp(p.Backups, stamp)
	if err != nil {
		return err
	}
	latestPath := filepath.Join(p.Backups, "latest")
	if prev == "" {
		if err := os.Remove(latestPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(latestPath, []byte(prev+"\n"), 0o644)
}

// latestBackupStamp returns the newest backup stamp that is not exclude.
func latestBackupStamp(dir, exclude string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	best := ""
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || name == exclude || len(name) != len("20060102-150405") {
			continue
		}
		if _, err := time.Parse("20060102-150405", name); err != nil {
			continue
		}
		if name > best {
			best = name
		}
	}
	return best, nil
}
