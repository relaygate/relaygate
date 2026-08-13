package agent

import (
	"strings"
	"time"
)

// AlignStatus is the product-facing alignment state for a node.
type AlignStatus string

const (
	StatusAligned      AlignStatus = "aligned"
	StatusDrifted      AlignStatus = "drifted"
	StatusOffline      AlignStatus = "offline"
	StatusUnauthorized AlignStatus = "unauthorized"
	StatusUnknown      AlignStatus = "unknown"
)

// NodeStatus is one row for fleet status UI/CLI.
type NodeStatus struct {
	Name             string      `json:"name"`
	Role             NodeRole    `json:"role"`
	Status           AlignStatus `json:"status"`
	AppliedVersion   string      `json:"applied_version,omitempty"`
	PublishedVersion string      `json:"published_version,omitempty"`
	LastHeartbeat    string      `json:"last_heartbeat,omitempty"`
	SyncPending      bool        `json:"sync_pending,omitempty"`
}

// OfflineAfter is how long without heartbeat before a node is offline.
const OfflineAfter = 2 * time.Minute

// BuildStatus builds alignment status from registry + current published version.
func BuildStatus(root string) (published string, nodes []NodeStatus, err error) {
	meta, err := CurrentMeta(root)
	if err != nil {
		return "", nil, err
	}
	published = meta.Version
	reg, err := LoadRegistry(root)
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC()
	for _, n := range reg.Nodes {
		st := NodeStatus{
			Name:             n.Name,
			Role:             n.Role,
			AppliedVersion:   n.AppliedVer,
			PublishedVersion: published,
			LastHeartbeat:    n.LastHeartbeat,
			SyncPending:      strings.TrimSpace(n.SyncRequestedAt) != "",
			Status:           StatusUnknown,
		}
		if n.LastHeartbeat == "" {
			st.Status = StatusUnauthorized
			if n.TokenHash != "" || n.CreatedAt != "" {
				st.Status = StatusOffline
			}
		} else if ts, err := time.Parse(time.RFC3339, n.LastHeartbeat); err != nil {
			st.Status = StatusUnknown
		} else if now.Sub(ts) > OfflineAfter {
			st.Status = StatusOffline
		} else if published != "" && n.AppliedVer == published {
			st.Status = StatusAligned
		} else if published != "" && n.AppliedVer != "" && n.AppliedVer != published {
			st.Status = StatusDrifted
		} else {
			st.Status = StatusDrifted
		}
		nodes = append(nodes, st)
	}
	return published, nodes, nil
}
