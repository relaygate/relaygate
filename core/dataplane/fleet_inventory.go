package dataplane

import (
	"os"
	"strings"

	"github.com/relaygate/relaygate/core/config"
)

// FleetNode is a read-only view of one gateway from inventory (Panel fleet overview).
type FleetNode struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	SSHPort   string `json:"ssh_port"`
	SSHUser   string `json:"ssh_user"`
	RemoteDir string `json:"remote_dir"`
	DataDir   string `json:"data_dir,omitempty"`
}

// FleetInventory loads gateways.env for Primary fleet overview (read-only; no SSH).
func FleetInventory(root string) (inventoryPath string, nodes []FleetNode, err error) {
	inventoryPath = config.ResolvePaths(root).Inventory
	vars, err := parseInventory(inventoryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return inventoryPath, nil, nil
		}
		return inventoryPath, nil, err
	}
	matrix := strings.TrimSpace(vars["GATEWAY_MATRIX"])
	if matrix == "" {
		return inventoryPath, nil, nil
	}
	for _, gw := range strings.Split(matrix, ",") {
		gw = strings.TrimSpace(gw)
		if gw == "" {
			continue
		}
		key := strings.ReplaceAll(gw, "-", "_")
		host := vars["HOST_"+key]
		port := vars["SSH_PORT_"+key]
		if port == "" {
			port = config.DefaultSSHPort
		}
		user := vars["SSH_USER_"+key]
		if user == "" {
			user = "root"
		}
		rdir := vars["REMOTE_DIR_"+key]
		if rdir == "" {
			rdir = config.DefaultInstallDir
		}
		node := FleetNode{
			Name:      gw,
			Host:      host,
			SSHPort:   port,
			SSHUser:   user,
			RemoteDir: rdir,
			DataDir:   strings.TrimSpace(vars["DATA_DIR_"+key]),
		}
		nodes = append(nodes, node)
	}
	return inventoryPath, nodes, nil
}

// FleetCLIHints returns copy-paste commands for fleet automation from Primary.
func FleetCLIHints(root string) []string {
	_ = root
	return []string{
		"relaygate fleet status",
		"relaygate fleet publish   # 确认 PUBLISH_FLEET",
		"relaygate fleet join gateway-02  # 确认 FLEET_JOIN",
		"relaygate fleet leave gateway-02 # 确认 FLEET_LEAVE",
		"# 节点：PRIMARY_URL=… AGENT_TOKEN_FILE=… relaygate agent run",
	}
}
