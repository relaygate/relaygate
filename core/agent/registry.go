package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/relaygate/relaygate/core/config"
)

// NodeRole is control (主控) or node (网关节点).
type NodeRole string

const (
	RoleControl NodeRole = "control"
	RoleNode    NodeRole = "node"
)

// Node is one entry in the node registry (nodes.yaml).
type Node struct {
	Name         string    `yaml:"name" json:"name"`
	Role         NodeRole  `yaml:"role" json:"role"`
	TokenHash    string    `yaml:"token_hash,omitempty" json:"-"`
	AppliedVer   string    `yaml:"applied_version,omitempty" json:"applied_version,omitempty"`
	LastHeartbeat string   `yaml:"last_heartbeat,omitempty" json:"last_heartbeat,omitempty"`
	CreatedAt    string    `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

// Registry is the on-disk node name book under DataDir/nodes.yaml.
type Registry struct {
	Nodes []Node `yaml:"nodes"`
}

var heartbeatMu sync.Mutex

// NodesPath returns DataDir/nodes.yaml.
func NodesPath(root string) string {
	return filepath.Join(config.ResolveDataDir(root), "nodes.yaml")
}

// TokensDir returns DataDir/agent-tokens.
func TokensDir(root string) string {
	return filepath.Join(config.ResolveDataDir(root), "agent-tokens")
}

// LoadRegistry reads nodes.yaml; missing file yields empty registry.
func LoadRegistry(root string) (*Registry, error) {
	path := NodesPath(root)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, err
	}
	var reg Registry
	if err := yaml.Unmarshal(b, &reg); err != nil {
		return nil, fmt.Errorf("解析节点名册失败: %w", err)
	}
	return &reg, nil
}

func saveRegistry(root string, reg *Registry) error {
	path := NodesPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(reg)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// JoinResult is returned after creating a join credential.
type JoinResult struct {
	Name           string `json:"name"`
	Token          string `json:"token"`
	TokenFileHint  string `json:"token_file_hint"`
	BootstrapHint  string `json:"bootstrap_hint"`
	PrimaryURLHint string `json:"primary_url_hint"`
}

// JoinNode registers a gateway node and issues a one-time agent token.
func JoinNode(root, name, primaryURL string) (*JoinResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("请填写节点名称")
	}
	reg, err := LoadRegistry(root)
	if err != nil {
		return nil, err
	}
	for _, n := range reg.Nodes {
		if n.Name == name {
			return nil, fmt.Errorf("节点 %s 已在名册中", name)
		}
	}
	token, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	tokDir := TokensDir(root)
	if err := os.MkdirAll(tokDir, 0o700); err != nil {
		return nil, err
	}
	tokPath := filepath.Join(tokDir, name+".token")
	if err := os.WriteFile(tokPath, []byte(token+"\n"), 0o600); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	reg.Nodes = append(reg.Nodes, Node{
		Name:      name,
		Role:      RoleNode,
		TokenHash: hashToken(token),
		CreatedAt: now,
	})
	if err := saveRegistry(root, reg); err != nil {
		return nil, err
	}
	if primaryURL == "" {
		primaryURL = strings.TrimSpace(os.Getenv("PRIMARY_URL"))
	}
	if primaryURL == "" {
		primaryURL = "http://203.0.113.10:9000"
	}
	hint := fmt.Sprintf(
		"# 在目标主机安装节点组件后：\n"+
			"cp packaging/node/env.example .env && chmod 600 .env\n"+
			"PRIMARY_URL=%s\nAGENT_TOKEN_FILE=/etc/relaygate/secrets/agent.token\nENABLE_PANEL=0\n"+
			"# 将令牌写入 AGENT_TOKEN_FILE 后执行：relaygate agent run\n"+
			"# 令牌已保存在主控：%s",
		primaryURL, tokPath,
	)
	return &JoinResult{
		Name:           name,
		Token:          token,
		TokenFileHint:  tokPath,
		BootstrapHint:  hint,
		PrimaryURLHint: primaryURL,
	}, nil
}

// LeaveResult summarizes retiring a node.
type LeaveResult struct {
	Name        string   `json:"name"`
	ManualHints []string `json:"manual_hints"`
}

// LeaveNode removes a node from the registry and revokes its token.
func LeaveNode(root, name string) (*LeaveResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("请指定要退役的节点名称")
	}
	reg, err := LoadRegistry(root)
	if err != nil {
		return nil, err
	}
	found := false
	out := make([]Node, 0, len(reg.Nodes))
	for _, n := range reg.Nodes {
		if n.Name == name {
			found = true
			continue
		}
		out = append(out, n)
	}
	if !found {
		return nil, fmt.Errorf("名册中未找到节点 %s", name)
	}
	reg.Nodes = out
	if err := saveRegistry(root, reg); err != nil {
		return nil, err
	}
	_ = os.Remove(filepath.Join(TokensDir(root), name+".token"))
	return &LeaveResult{
		Name: name,
		ManualHints: []string{
			"请在云控制台/Terraform 将该节点从负载均衡目标组摘除（本产品不调用云 API）。",
			"目标主机可停止 relaygate-agent 并卸载节点组件。",
		},
	}, nil
}

// LookupByToken finds a node whose token matches.
func LookupByToken(root, token string) (*Node, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("缺少代理令牌")
	}
	reg, err := LoadRegistry(root)
	if err != nil {
		return nil, err
	}
	h := hashToken(token)
	for i := range reg.Nodes {
		if reg.Nodes[i].TokenHash == h {
			n := reg.Nodes[i]
			return &n, nil
		}
	}
	// Also accept raw token file match (MVP convenience).
	tokDir := TokensDir(root)
	entries, _ := os.ReadDir(tokDir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(tokDir, e.Name()))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(b)) == token {
			name := strings.TrimSuffix(e.Name(), ".token")
			for i := range reg.Nodes {
				if reg.Nodes[i].Name == name {
					n := reg.Nodes[i]
					return &n, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("未授权的代理令牌")
}

// RecordHeartbeat updates last heartbeat and applied version for a node.
func RecordHeartbeat(root, name, appliedVersion string) error {
	heartbeatMu.Lock()
	defer heartbeatMu.Unlock()
	reg, err := LoadRegistry(root)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	found := false
	for i := range reg.Nodes {
		if reg.Nodes[i].Name == name {
			reg.Nodes[i].LastHeartbeat = now
			if appliedVersion != "" {
				reg.Nodes[i].AppliedVer = appliedVersion
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("名册中未找到节点 %s", name)
	}
	return saveRegistry(root, reg)
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
