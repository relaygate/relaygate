package xds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// MgmtPort returns the loopback HTTP port for cross-process snapshot publish.
func MgmtPort(xdsPort int) int {
	if xdsPort <= 0 {
		xdsPort = DefaultPort
	}
	return xdsPort + 1
}

// mgmtRequest is the JSON body for /v1/publish and /v1/rollback.
type mgmtRequest struct {
	NodeID string `json:"node_id"`
}

// startMgmtHTTP serves loopback-only publish/rollback for CLI HotApply when ADS
// is owned by panel or `relaygate xds serve` in another process.
func startMgmtHTTP(xdsPort int, pub Publisher) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/publish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req mgmtRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.NodeID == "" {
			http.Error(w, "node_id required", http.StatusBadRequest)
			return
		}
		if diskPublishHandler == nil {
			http.Error(w, "disk publish handler not configured", http.StatusServiceUnavailable)
			return
		}
		version, err := diskPublishHandler(req.NodeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": version})
	})
	mux.HandleFunc("/v1/rollback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req mgmtRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.NodeID == "" {
			http.Error(w, "node_id required", http.StatusBadRequest)
			return
		}
		if err := pub.Rollback(req.NodeID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	addr := fmt.Sprintf("%s:%d", DefaultListenAddr, MgmtPort(xdsPort))
	srv := &http.Server{Addr: addr, Handler: mux}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() { _ = srv.Serve(ln) }()
	return srv, nil
}

// SnapshotPublishHandler can rebuild and publish a snapshot from on-disk resources.
type SnapshotPublishHandler interface {
	PublishFromDisk(nodeID string) (string, error)
}

// diskPublishHandler is set by panel / xds serve for mgmt HTTP rebuild.
var diskPublishHandler func(nodeID string) (string, error)

// SetDiskPublishHandler registers rebuild-from-disk for loopback mgmt HTTP.
func SetDiskPublishHandler(fn func(nodeID string) (string, error)) {
	diskPublishHandler = fn
}

// PeerPublisher forwards SetSnapshot to a loopback mgmt HTTP peer (panel / xds serve).
type PeerPublisher struct {
	BaseURL     string
	lastVersion string
}

func peerPublisherURL(xdsPort int) string {
	return fmt.Sprintf("http://%s:%d", DefaultListenAddr, MgmtPort(xdsPort))
}

type peerPublishResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
}

// SetSnapshot implements Publisher.
func (p *PeerPublisher) SetSnapshot(nodeID string, snap Snapshot) error {
	_ = snap
	body, err := json.Marshal(mgmtRequest{NodeID: nodeID})
	if err != nil {
		return err
	}
	resp, err := http.Post(p.BaseURL+"/v1/publish", "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("xds peer publish: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("xds peer publish: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out peerPublishResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("xds peer publish: decode: %w", err)
	}
	if out.Version != "" {
		p.lastVersion = out.Version
	}
	return nil
}

// LastVersion returns the version reported by the last peer publish.
func (p *PeerPublisher) LastVersion(nodeID string) string {
	_ = nodeID
	return p.lastVersion
}

// Rollback implements Publisher.
func (p *PeerPublisher) Rollback(nodeID string) error {
	body, err := json.Marshal(mgmtRequest{NodeID: nodeID})
	if err != nil {
		return err
	}
	resp, err := http.Post(p.BaseURL+"/v1/rollback", "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("xds peer rollback: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("xds peer rollback: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// NextVersion is unused for peer publish (peer allocates version).
func (p *PeerPublisher) NextVersion() string {
	return ""
}

var mgmtOnce sync.Map // port -> *http.Server

func ensureMgmtHTTP(xdsPort int, pub Publisher) {
	if diskPublishHandler == nil {
		return
	}
	key := MgmtPort(xdsPort)
	if _, loaded := mgmtOnce.LoadOrStore(key, true); loaded {
		return
	}
	if _, err := startMgmtHTTP(xdsPort, pub); err != nil {
		mgmtOnce.Delete(key)
	}
}
