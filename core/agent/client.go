package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/resources"
)

// Client talks to the control-plane Panel agent APIs.
type Client struct {
	ControlURL string
	Token      string
	HTTP       *http.Client
}

// LoadClientFromEnv builds a Client from CONTROL_URL + AGENT_TOKEN_FILE (or AGENT_TOKEN).
func LoadClientFromEnv() (*Client, error) {
	url := strings.TrimSpace(os.Getenv("CONTROL_URL"))
	if url == "" {
		return nil, fmt.Errorf("未设置 CONTROL_URL。请在节点 .env 中填写主控地址（例如 http://203.0.113.10:9000）")
	}
	token := strings.TrimSpace(os.Getenv("AGENT_TOKEN"))
	if token == "" {
		tokFile := strings.TrimSpace(os.Getenv("AGENT_TOKEN_FILE"))
		if tokFile == "" {
			return nil, fmt.Errorf("未设置 AGENT_TOKEN_FILE（或 AGENT_TOKEN）。请将接入时发放的令牌写入文件")
		}
		b, err := os.ReadFile(tokFile)
		if err != nil {
			return nil, fmt.Errorf("无法读取代理令牌文件：请确认路径与权限后重试")
		}
		token = strings.TrimSpace(string(b))
	}
	if token == "" {
		return nil, fmt.Errorf("代理令牌为空。请重新接入节点或轮换令牌")
	}
	return &Client{
		ControlURL: strings.TrimRight(url, "/"),
		Token:      token,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type versionResp struct {
	Version string `json:"version"`
	Body    string `json:"resources_yaml"`
}

// PullOnce fetches the current published config and writes it to local DataDir.
// It does NOT update applied-version — that happens only after HotApply succeeds
// (see MarkApplied / Run AfterPull).
func (c *Client) PullOnce(root string) (version string, err error) {
	req, err := http.NewRequest(http.MethodGet, c.ControlURL+"/api/agent/config", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("无法连接主控：请检查 CONTROL_URL 与网络后重试")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("拉取配置失败（主控拒绝或令牌无效）。请确认节点已接入且令牌未吊销")
	}
	var vr versionResp
	if err := json.Unmarshal(body, &vr); err != nil {
		return "", fmt.Errorf("主控返回的配置无法解析")
	}
	if vr.Version == "" || vr.Body == "" {
		return "", fmt.Errorf("主控尚无已发布版本。请先在主控执行「发布到机群」")
	}

	tmpParse := filepath.Join(config.ResolveDataDir(root), "resources.pull.tmp.yaml")
	if err := os.MkdirAll(filepath.Dir(tmpParse), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(tmpParse, []byte(vr.Body), 0o640); err != nil {
		return "", err
	}
	defer os.Remove(tmpParse)

	r, err := resources.Load(tmpParse)
	if err != nil {
		return "", fmt.Errorf("主控返回的配置与本机不兼容（请升级节点到与主控相同版本后再拉取）：%v", err)
	}
	resources.StripFleetNodeIdentity(r)
	name, pubIP := localInstallIdentity(root)
	resources.ApplyLocalNodeIdentity(r, name, pubIP)

	dst := config.ResolvePaths(root).Resources
	if err := resources.Save(dst, r); err != nil {
		return "", err
	}
	_ = os.WriteFile(filepath.Join(config.ResolveDataDir(root), "pulled-version"), []byte(vr.Version+"\n"), 0o640)
	return vr.Version, nil
}

// localInstallIdentity prefers process env, then explicit keys in root/.env.
// Does not invent defaults (avoids stamping LoadEnv's 127.0.0.1 / gateway-01).
func localInstallIdentity(root string) (name, publicIP string) {
	name = strings.TrimSpace(os.Getenv("GATEWAY_NAME"))
	publicIP = strings.TrimSpace(os.Getenv("GATEWAY_PUBLIC_IP"))
	env, err := config.LoadEnv(root)
	if err != nil {
		return name, publicIP
	}
	if name == "" {
		name = strings.TrimSpace(env.Raw["GATEWAY_NAME"])
	}
	if publicIP == "" {
		publicIP = strings.TrimSpace(env.Raw["GATEWAY_PUBLIC_IP"])
	}
	return name, publicIP
}

// MarkApplied records that version was successfully HotApplied (incl. xDS ACK).
func MarkApplied(root, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("applied 版本为空")
	}
	path := filepath.Join(config.ResolveDataDir(root), "applied-version")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(version+"\n"), 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Heartbeat reports applied version to the control plane.
func (c *Client) Heartbeat(appliedVersion string) error {
	payload, _ := json.Marshal(map[string]string{"applied_version": appliedVersion})
	req, err := http.NewRequest(http.MethodPost, c.ControlURL+"/api/agent/heartbeat", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("心跳失败：无法连接主控")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("心跳被主控拒绝。请确认节点令牌有效")
	}
	return nil
}

// LocalAppliedVersion reads DataDir/applied-version.
func LocalAppliedVersion(root string) string {
	b, err := os.ReadFile(filepath.Join(config.ResolveDataDir(root), "applied-version"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
