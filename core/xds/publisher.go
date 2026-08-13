package xds

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

// Snapshot holds a versioned CDS+LDS payload for Publisher.
type Snapshot struct {
	Version string
	NodeID  string
	// Inner is the go-control-plane snapshot (nil for MemoryPublisher stubs).
	Inner *cache.Snapshot
}

// Publisher is the HotApply-facing surface (Panel / CLI / node agent).
type Publisher interface {
	SetSnapshot(nodeID string, snap Snapshot) error
	LastVersion(nodeID string) string
	Rollback(nodeID string) error
	NextVersion() string
}

// CachePublisher stores snapshots in a go-control-plane SnapshotCache.
type CachePublisher struct {
	cache    cache.SnapshotCache
	mu       sync.Mutex
	history  map[string][]*cache.Snapshot // nodeID -> ordered snapshots
	versions map[string]string
	seq      atomic.Uint64
}

// NewCachePublisher returns a Publisher backed by SnapshotCache (ADS=true).
func NewCachePublisher(c cache.SnapshotCache) *CachePublisher {
	return &CachePublisher{
		cache:    c,
		history:  make(map[string][]*cache.Snapshot),
		versions: make(map[string]string),
	}
}

// NextVersion returns a monotonic version string.
func (p *CachePublisher) NextVersion() string {
	return fmt.Sprintf("%d", p.seq.Add(1))
}

// SetSnapshot implements Publisher.
func (p *CachePublisher) SetSnapshot(nodeID string, snap Snapshot) error {
	if nodeID == "" {
		return fmt.Errorf("xds: empty nodeID")
	}
	if snap.Inner == nil {
		return fmt.Errorf("xds: snapshot inner is nil")
	}
	if snap.Version == "" {
		snap.Version = p.NextVersion()
	}
	if err := p.cache.SetSnapshot(context.Background(), nodeID, snap.Inner); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.versions[nodeID] = snap.Version
	h := p.history[nodeID]
	if len(h) == 0 || h[len(h)-1] != snap.Inner {
		p.history[nodeID] = append(h, snap.Inner)
	}
	return nil
}

// LastVersion implements Publisher.
func (p *CachePublisher) LastVersion(nodeID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.versions[nodeID]
}

// Rollback implements Publisher.
func (p *CachePublisher) Rollback(nodeID string) error {
	p.mu.Lock()
	h := p.history[nodeID]
	if len(h) < 2 {
		p.mu.Unlock()
		return fmt.Errorf("xds: no previous snapshot for %s", nodeID)
	}
	prevSnap := h[len(h)-2]
	p.history[nodeID] = h[:len(h)-1]
	prevVer := prevSnap.GetVersion(resource.ClusterType)
	if prevVer == "" {
		prevVer = prevSnap.GetVersion(resource.ListenerType)
	}
	p.mu.Unlock()

	if err := p.cache.SetSnapshot(context.Background(), nodeID, prevSnap); err != nil {
		return err
	}
	p.mu.Lock()
	p.versions[nodeID] = prevVer
	p.mu.Unlock()
	return nil
}

// MemoryPublisher is an in-process snapshot store for unit tests (no gRPC).
type MemoryPublisher struct {
	mu    sync.RWMutex
	snaps map[string]Snapshot
	hist  map[string][]string
	seq   atomic.Uint64
}

// NewMemoryPublisher returns an empty in-memory Publisher.
func NewMemoryPublisher() *MemoryPublisher {
	return &MemoryPublisher{
		snaps: make(map[string]Snapshot),
		hist:  make(map[string][]string),
	}
}

// NextVersion returns a monotonic version string suitable for Envoy snapshots.
func (p *MemoryPublisher) NextVersion() string {
	return fmt.Sprintf("%d", p.seq.Add(1))
}

// SetSnapshot implements Publisher.
func (p *MemoryPublisher) SetSnapshot(nodeID string, snap Snapshot) error {
	if nodeID == "" {
		return fmt.Errorf("xds: empty nodeID")
	}
	if snap.Version == "" {
		snap.Version = p.NextVersion()
	}
	snap.NodeID = nodeID
	p.mu.Lock()
	p.snaps[nodeID] = snap
	h := p.hist[nodeID]
	if len(h) == 0 || h[len(h)-1] != snap.Version {
		p.hist[nodeID] = append(h, snap.Version)
	}
	p.mu.Unlock()
	return nil
}

// LastVersion implements Publisher.
func (p *MemoryPublisher) LastVersion(nodeID string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.snaps[nodeID].Version
}

// Rollback implements Publisher.
func (p *MemoryPublisher) Rollback(nodeID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	h := p.hist[nodeID]
	if len(h) < 2 {
		return fmt.Errorf("xds: no previous snapshot for %s", nodeID)
	}
	prevVer := h[len(h)-2]
	p.hist[nodeID] = h[:len(h)-1]
	snap := p.snaps[nodeID]
	snap.Version = prevVer
	p.snaps[nodeID] = snap
	return nil
}

// Get returns the last snapshot for nodeID, if any.
func (p *MemoryPublisher) Get(nodeID string) (Snapshot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.snaps[nodeID]
	return s, ok
}

// EmptySnapshot builds an empty CDS+LDS snapshot with version.
func EmptySnapshot(version string) (*cache.Snapshot, error) {
	return cache.NewSnapshot(version, map[resource.Type][]types.Resource{
		resource.ClusterType:  []types.Resource{},
		resource.ListenerType: []types.Resource{},
	})
}

// DefaultACKTimeout is the default wait for Envoy to apply a pushed snapshot.
const DefaultACKTimeout = 15 * time.Second
