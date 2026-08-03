package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
)

const currentVersionFile = "current"

// VersionsDir returns DataDir/versions.
func VersionsDir(root string) string {
	return filepath.Join(config.ResolveDataDir(root), "versions")
}

// PublishMeta describes the currently published fleet config version.
type PublishMeta struct {
	Version   string `json:"version"`
	Published string `json:"published_at"`
	Source    string `json:"source,omitempty"`
}

// CurrentVersion reads the published version id (empty if none).
func CurrentVersion(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(VersionsDir(root), currentVersionFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// CurrentMeta returns publish metadata for the current version.
func CurrentMeta(root string) (*PublishMeta, error) {
	ver, err := CurrentVersion(root)
	if err != nil {
		return nil, err
	}
	if ver == "" {
		return &PublishMeta{}, nil
	}
	metaPath := filepath.Join(VersionsDir(root), ver, "meta.txt")
	published := ""
	if b, err := os.ReadFile(metaPath); err == nil {
		published = strings.TrimSpace(string(b))
	}
	return &PublishMeta{
		Version:   ver,
		Published: published,
		Source:    filepath.Join(VersionsDir(root), ver, "resources.yaml"),
	}, nil
}

// PublishResult is returned after publishing intent as a new fleet version.
type PublishResult struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

// Publish copies DataDir/resources.yaml into versions/<id>/ and updates current.
func Publish(root string) (*PublishResult, error) {
	src := config.ResolvePaths(root).Resources
	b, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("未找到业务配置。请先在配置编辑中保存意图，再发布到机群")
		}
		return nil, err
	}
	ver := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(VersionsDir(root), ver)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dst := filepath.Join(dir, "resources.yaml")
	if err := os.WriteFile(dst, b, 0o640); err != nil {
		return nil, err
	}
	meta := time.Now().UTC().Format(time.RFC3339)
	if err := os.WriteFile(filepath.Join(dir, "meta.txt"), []byte(meta+"\n"), 0o640); err != nil {
		return nil, err
	}
	cur := filepath.Join(VersionsDir(root), currentVersionFile)
	tmp := cur + ".tmp"
	if err := os.WriteFile(tmp, []byte(ver+"\n"), 0o640); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, cur); err != nil {
		return nil, err
	}
	return &PublishResult{Version: ver, Path: dst}, nil
}

// ReadPublishedResources returns resources.yaml bytes for a version (empty ver = current).
func ReadPublishedResources(root, version string) (ver string, data []byte, err error) {
	if version == "" {
		version, err = CurrentVersion(root)
		if err != nil {
			return "", nil, err
		}
		if version == "" {
			return "", nil, fmt.Errorf("尚无已发布的配置版本。请先在主控执行发布到机群")
		}
	}
	path := filepath.Join(VersionsDir(root), version, "resources.yaml")
	data, err = os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, fmt.Errorf("未找到配置版本 %s", version)
		}
		return "", nil, err
	}
	return version, data, nil
}
