package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
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
	JoinCommand    string `json:"join_command"`
	PrimaryURLHint string `json:"primary_url_hint"`
}

// ResolvePrimaryURL picks the URL new nodes use to reach the control plane.
// Order: explicit → PRIMARY_URL → PANEL_PUBLIC_URL → http://GATEWAY_PUBLIC_IP:<panel port> → docs placeholder.
func ResolvePrimaryURL(explicit string) string {
	if s := strings.TrimSpace(explicit); s != "" {
		return strings.TrimRight(s, "/")
	}
	for _, k := range []string{"PRIMARY_URL", "PANEL_PUBLIC_URL"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	if ip := strings.TrimSpace(os.Getenv("GATEWAY_PUBLIC_IP")); ip != "" {
		port := "9000"
		if bind := strings.TrimSpace(os.Getenv("PANEL_BIND")); bind != "" {
			if _, p, err := net.SplitHostPort(bind); err == nil && p != "" {
				port = p
			}
		}
		return fmt.Sprintf("http://%s:%s", ip, port)
	}
	return "http://203.0.113.10:9000"
}

// InstallScriptURL is the curl target for the one-line bootstrap / upgrade.
func InstallScriptURL() string {
	if u := strings.TrimSpace(os.Getenv("RELAYGATE_INSTALL_SCRIPT_URL")); u != "" {
		return u
	}
	slug := strings.TrimSpace(os.Getenv("RELAYGATE_REPO_SLUG"))
	if slug == "" {
		slug = "relaygate/relaygate"
	}
	ref := strings.TrimSpace(os.Getenv("RELAYGATE_INSTALL_REF"))
	if ref == "" {
		ref = "master"
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/install.sh", slug, ref)
}

// FormatControlInstallCommand is the one-line control (Panel) install.
// Default admin password on first boot is "relaygate" (change in production).
func FormatControlInstallCommand() string {
	return fmt.Sprintf(
		"curl -fsSL %s | sudo env ENABLE_PANEL=1 NONINTERACTIVE=1 bash -s -- -y",
		shellSingleQuote(InstallScriptURL()),
	)
}

// FormatUpgradeCommand is the one-line upgrade for an existing install.
// Preserves .env / DataDir; role (Panel vs agent) is taken from the existing .env.
func FormatUpgradeCommand() string {
	return fmt.Sprintf(
		"curl -fsSL %s | sudo bash -s -- --upgrade -y",
		shellSingleQuote(InstallScriptURL()),
	)
}

// FormatJoinCommand builds a single shell line for remote node install + agent start.
// Contains PRIMARY_URL / GATEWAY_NAME / AGENT_TOKEN only — never Panel admin password.
func FormatJoinCommand(primaryURL, name, token string) string {
	primaryURL = strings.TrimRight(strings.TrimSpace(primaryURL), "/")
	name = strings.TrimSpace(name)
	token = strings.TrimSpace(token)
	script := InstallScriptURL()
	return fmt.Sprintf(
		"curl -fsSL %s | sudo env PRIMARY_URL=%s GATEWAY_NAME=%s AGENT_TOKEN=%s ENABLE_PANEL=0 ENABLE_GRAFANA=0 NONINTERACTIVE=1 bash -s -- -y",
		shellSingleQuote(script),
		shellSingleQuote(primaryURL),
		shellSingleQuote(name),
		shellSingleQuote(token),
	)
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// JoinNode registers a gateway node and issues a one-time agent token.
// Name should be a gateway id such as gateway-02 (zero-padded); not the control role label.
func JoinNode(root, name, primaryURL string) (*JoinResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("请填写节点名称")
	}
	switch strings.ToLower(name) {
	case "control", "primary", "主控":
		return nil, fmt.Errorf("请使用网关节点名（如 gateway-02），不要用主控角色名作名册成员")
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
	primaryURL = ResolvePrimaryURL(primaryURL)
	cmd := FormatJoinCommand(primaryURL, name, token)
	hint := fmt.Sprintf(
		"在目标主机以 root 执行下面一行即可安装节点并连接主控（含一次性令牌，勿写入公开日志）。\n\n%s\n\n"+
			"令牌副本保存在主控：%s\n"+
			"若前置了可选云 L4 入口：请在云控制台将新节点挂入目标组（本产品不调用云 API）。",
		cmd, tokPath,
	)
	return &JoinResult{
		Name:           name,
		Token:          token,
		TokenFileHint:  tokPath,
		BootstrapHint:  hint,
		JoinCommand:    cmd,
		PrimaryURLHint: primaryURL,
	}, nil
}

// LeaveResult summarizes retiring a node.
type LeaveResult struct {
	Name        string   `json:"name"`
	ManualHints []string `json:"manual_hints"`
}

// LeaveNode removes a gateway node from the registry and revokes its token.
// Control-role entries (if any) cannot be retired this way — stop the control
// component separately; local forwarding on the control host uses 本机应用.
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
			if n.Role == RoleControl {
				return nil, fmt.Errorf("不能退役主控条目 %s：机群退役仅针对网关节点；主控本机转发请用「本机应用」", name)
			}
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
			"若前置了可选云 L4 入口：请在云控制台/Terraform 将该节点从目标组摘除（本产品不调用云 API）。公网直连可跳过。",
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
