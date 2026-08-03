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
)

// Client talks to the control-plane Panel agent APIs.
type Client struct {
	PrimaryURL string
	Token      string
	HTTP       *http.Client
}

// LoadClientFromEnv builds a Client from PRIMARY_URL + AGENT_TOKEN_FILE (or AGENT_TOKEN).
func LoadClientFromEnv() (*Client, error) {
	url := strings.TrimSpace(os.Getenv("PRIMARY_URL"))
	if url == "" {
		return nil, fmt.Errorf("未设置 PRIMARY_URL。请在节点 .env 中填写主控地址（例如 http://203.0.113.10:9000）")
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
		PrimaryURL: strings.TrimRight(url, "/"),
		Token:      token,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type versionResp struct {
	Version string `json:"version"`
	Body    string `json:"resources_yaml"`
}

// PullOnce fetches the current published config and writes it to local DataDir.
func (c *Client) PullOnce(root string) (version string, err error) {
	req, err := http.NewRequest(http.MethodGet, c.PrimaryURL+"/api/agent/config", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("无法连接主控：请检查 PRIMARY_URL 与网络后重试")
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
	dst := config.ResolvePaths(root).Resources
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, []byte(vr.Body), 0o640); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	_ = os.WriteFile(filepath.Join(config.ResolveDataDir(root), "applied-version"), []byte(vr.Version+"\n"), 0o640)
	return vr.Version, nil
}

// Heartbeat reports applied version to the control plane.
func (c *Client) Heartbeat(appliedVersion string) error {
	payload, _ := json.Marshal(map[string]string{"applied_version": appliedVersion})
	req, err := http.NewRequest(http.MethodPost, c.PrimaryURL+"/api/agent/heartbeat", bytes.NewReader(payload))
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
