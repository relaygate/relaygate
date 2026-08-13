package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
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

// Publish copies DataDir/resources.yaml into versions/<id>/ (with node identity
// stripped) and updates current. Permissions are set so the Panel user
// (group relaygate) can read even when the CLI runs as root.
func Publish(root string) (*PublishResult, error) {
	src := config.ResolvePaths(root).Resources
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("未找到业务配置。请先在配置编辑中保存意图，再执行 relaygate fleet publish")
		}
		return nil, err
	}
	r, err := resources.Load(src)
	if err != nil {
		return nil, err
	}
	resources.StripFleetNodeIdentity(r)

	ver := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(VersionsDir(root), ver)
	if err := os.MkdirAll(dir, 0o770); err != nil {
		return nil, err
	}
	_ = os.Chmod(dir, 0o770)
	dst := filepath.Join(dir, "resources.yaml")
	if err := resources.Save(dst, r); err != nil {
		return nil, err
	}
	meta := time.Now().UTC().Format(time.RFC3339)
	metaPath := filepath.Join(dir, "meta.txt")
	if err := os.WriteFile(metaPath, []byte(meta+"\n"), 0o640); err != nil {
		return nil, err
	}
	_ = os.Chmod(metaPath, 0o640)
	cur := filepath.Join(VersionsDir(root), currentVersionFile)
	tmp := cur + ".tmp"
	if err := os.WriteFile(tmp, []byte(ver+"\n"), 0o640); err != nil {
		return nil, err
	}
	_ = os.Chmod(tmp, 0o640)
	if err := os.Rename(tmp, cur); err != nil {
		return nil, err
	}
	secureFleetVersions(root, ver)
	return &PublishResult{Version: ver, Path: dst}, nil
}

// secureFleetVersions makes versions/<ver>/ and current readable by group relaygate.
func secureFleetVersions(root, ver string) {
	vdir := VersionsDir(root)
	_ = os.MkdirAll(vdir, 0o770)
	_ = os.Chmod(vdir, 0o770)
	paths := []string{
		vdir,
		filepath.Join(vdir, currentVersionFile),
	}
	if ver != "" {
		verDir := filepath.Join(vdir, ver)
		paths = append(paths, verDir,
			filepath.Join(verDir, "resources.yaml"),
			filepath.Join(verDir, "meta.txt"),
		)
	}
	hasGroup := exec.Command("getent", "group", "relaygate").Run() == nil
	for _, p := range paths {
		if st, err := os.Stat(p); err != nil {
			continue
		} else if st.IsDir() {
			_ = os.Chmod(p, 0o770)
		} else {
			_ = os.Chmod(p, 0o640)
		}
		if hasGroup {
			_ = exec.Command("chown", "root:relaygate", p).Run()
		}
	}
}

// ReadPublishedResources returns resources.yaml bytes for a version (empty ver = current).
// Node identity fields are stripped so nodes never inherit the control host name.
func ReadPublishedResources(root, version string) (ver string, data []byte, err error) {
	if version == "" {
		version, err = CurrentVersion(root)
		if err != nil {
			return "", nil, err
		}
		if version == "" {
			return "", nil, fmt.Errorf("尚无已发布的配置版本。请先在主控执行 relaygate fleet publish")
		}
	}
	path := filepath.Join(VersionsDir(root), version, "resources.yaml")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", nil, fmt.Errorf("未找到配置版本 %s", version)
		}
		return "", nil, err
	}
	r, err := resources.Load(path)
	if err != nil {
		return "", nil, err
	}
	resources.StripFleetNodeIdentity(r)
	body, err := marshalFleetResources(r)
	if err != nil {
		return "", nil, err
	}
	return version, body, nil
}

func marshalFleetResources(r *resources.Resources) ([]byte, error) {
	b, err := yaml.Marshal(r)
	if err != nil {
		return nil, err
	}
	header := []byte("# 由 relaygate 机群发布（已剥离节点身份）\n")
	return append(header, b...), nil
}
