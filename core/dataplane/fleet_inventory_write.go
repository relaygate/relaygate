package dataplane

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/relaygate/relaygate/core/config"
)

// SaveFleetNode upserts a gateway into DataDir/inventory/gateways.env (GATEWAY_MATRIX + HOST_*…).
// Bootstrap passwords / private keys must never be written here.
func SaveFleetNode(root string, node FleetNode) error {
	node.Name = strings.TrimSpace(node.Name)
	node.Host = strings.TrimSpace(node.Host)
	if node.Name == "" {
		return fmt.Errorf("网关名称不能为空")
	}
	if node.Host == "" {
		return fmt.Errorf("主机不能为空")
	}
	if node.SSHPort == "" {
		node.SSHPort = config.DefaultSSHPort
	}
	if node.SSHUser == "" {
		node.SSHUser = "root"
	}
	if node.RemoteDir == "" {
		node.RemoteDir = config.DefaultInstallDir
	}

	invPath := config.ResolvePaths(root).Inventory
	if err := os.MkdirAll(filepath.Dir(invPath), 0o755); err != nil {
		return err
	}
	vars, err := parseInventory(invPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if vars == nil {
		vars = map[string]string{}
	}

	matrix := splitCSV(vars["GATEWAY_MATRIX"])
	matrix = upsertCSV(matrix, node.Name)
	vars["GATEWAY_MATRIX"] = strings.Join(matrix, ",")

	key := inventoryKey(node.Name)
	vars["HOST_"+key] = node.Host
	vars["SSH_PORT_"+key] = node.SSHPort
	vars["SSH_USER_"+key] = node.SSHUser
	vars["REMOTE_DIR_"+key] = node.RemoteDir
	if strings.TrimSpace(node.DataDir) != "" {
		vars["DATA_DIR_"+key] = strings.TrimSpace(node.DataDir)
	}

	return writeInventoryFile(invPath, vars)
}

// RemoveFleetNode drops a gateway from GATEWAY_MATRIX and removes its HOST_*/SSH_* keys.
func RemoveFleetNode(root, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("网关名称不能为空")
	}
	invPath := config.ResolvePaths(root).Inventory
	vars, err := parseInventory(invPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("inventory 不存在: %s", invPath)
		}
		return err
	}
	matrix := splitCSV(vars["GATEWAY_MATRIX"])
	var next []string
	found := false
	for _, gw := range matrix {
		if gw == name {
			found = true
			continue
		}
		next = append(next, gw)
	}
	if !found {
		return fmt.Errorf("inventory 中无网关 %s", name)
	}
	vars["GATEWAY_MATRIX"] = strings.Join(next, ",")
	key := inventoryKey(name)
	for _, prefix := range []string{"HOST_", "SSH_PORT_", "SSH_USER_", "REMOTE_DIR_", "DATA_DIR_"} {
		delete(vars, prefix+key)
	}
	return writeInventoryFile(invPath, vars)
}

func inventoryKey(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func upsertCSV(list []string, name string) []string {
	for _, n := range list {
		if n == name {
			return list
		}
	}
	return append(list, name)
}

func writeInventoryFile(path string, vars map[string]string) error {
	var b strings.Builder
	b.WriteString("# RelayGate fleet inventory（Panel/CLI 维护；勿写入 SSH 私钥或密码）\n")
	b.WriteString("# 网关节点：ENABLE_PANEL=0（见 packaging/node/env.example）\n\n")
	matrix := strings.TrimSpace(vars["GATEWAY_MATRIX"])
	b.WriteString("GATEWAY_MATRIX=")
	b.WriteString(matrix)
	b.WriteString("\n\n")

	names := splitCSV(matrix)
	written := map[string]bool{"GATEWAY_MATRIX": true}
	for _, name := range names {
		key := inventoryKey(name)
		b.WriteString("# ")
		b.WriteString(name)
		b.WriteString("\n")
		for _, field := range []string{"HOST_", "SSH_PORT_", "SSH_USER_", "REMOTE_DIR_", "DATA_DIR_"} {
			k := field + key
			if v, ok := vars[k]; ok && strings.TrimSpace(v) != "" {
				b.WriteString(k)
				b.WriteString("=")
				b.WriteString(v)
				b.WriteString("\n")
				written[k] = true
			}
		}
		b.WriteString("\n")
	}

	// Preserve unrelated keys (e.g. DEPLOY_REF) in stable order.
	var extras []string
	for k := range vars {
		if written[k] {
			continue
		}
		extras = append(extras, k)
	}
	sort.Strings(extras)
	for _, k := range extras {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(vars[k])
		b.WriteString("\n")
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
