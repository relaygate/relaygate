package xds

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// DefaultListenAddr is the host-loopback ADS bind address (Envoy uses host networking).
const DefaultListenAddr = "127.0.0.1"

// DefaultPort matches render.DefaultXDSPort / env XDS_PORT default.
const DefaultPort = 18000

// Server is the ADS gRPC listener + snapshot publisher.
type Server struct {
	ListenAddr string
	Port       int
	Publisher  Publisher

	cache   cache.SnapshotCache
	pub     *CachePublisher
	grpcSrv *grpc.Server
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
}

// NewServer returns a Server with loopback defaults. Does not bind until Start.
func NewServer(pub Publisher) *Server {
	s := &Server{
		ListenAddr: DefaultListenAddr,
		Port:       DefaultPort,
		Publisher:  pub,
	}
	if pub == nil {
		c := cache.NewSnapshotCache(true, cache.IDHash{}, nil)
		s.cache = c
		s.pub = NewCachePublisher(c)
		s.Publisher = s.pub
	}
	return s
}

// Addr returns host:port for bootstrap xds_cluster.
func (s *Server) Addr() string {
	addr := s.ListenAddr
	if addr == "" {
		addr = DefaultListenAddr
	}
	port := s.Port
	if port <= 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("%s:%d", addr, port)
}

// Start binds loopback ADS gRPC and serves until Stop.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	if s.cache == nil {
		s.cache = cache.NewSnapshotCache(true, cache.IDHash{}, nil)
		s.pub = NewCachePublisher(s.cache)
		s.Publisher = s.pub
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	grpcOpts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(1_000_000),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 5 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
	}
	grpcSrv := grpc.NewServer(grpcOpts...)
	xdsSrv := server.NewServer(ctx, s.cache, nil)
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcSrv, xdsSrv)

	lis, err := net.Listen("tcp", s.Addr())
	if err != nil {
		cancel()
		return fmt.Errorf("xds: listen %s: %w", s.Addr(), err)
	}
	s.grpcSrv = grpcSrv
	s.running = true
	ensureMgmtHTTP(s.Port, s.Publisher)
	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	return nil
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}
	s.running = false
	return nil
}

// Running reports whether ADS is listening.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// WaitEnvoyApplied polls Envoy admin until the published config version appears
// or timeout. Admin unreachable or CDS/LDS update_rejected are hard failures
// (never silent success — avoids mis-reporting HotApply OK).
func WaitEnvoyApplied(adminURL string, version string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultACKTimeout
	}
	base := strings.TrimRight(adminURL, "/")
	readyURL := base + "/ready"
	if !httpGetOK(readyURL) {
		return fmt.Errorf("xds: Envoy admin 不可达 (%s)；无法确认热更新是否生效", readyURL)
	}
	deadline := time.Now().Add(timeout)
	configURL := base + "/config_dump"
	statsURL := base + "/stats"
	baseCDS, baseLDS := readRejectCounters(statsURL)
	for time.Now().Before(deadline) {
		cds, lds := readRejectCounters(statsURL)
		if cds > baseCDS {
			return fmt.Errorf("xds: Envoy 配置被拒绝 (cluster_manager.cds.update_rejected %d→%d)", baseCDS, cds)
		}
		if lds > baseLDS {
			return fmt.Errorf("xds: Envoy 配置被拒绝 (listener_manager.lds.update_rejected %d→%d)", baseLDS, lds)
		}
		body, err := httpGet(configURL)
		if err == nil && version != "" && strings.Contains(body, version) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("xds: ACK timeout after %s (version %s)", timeout, version)
}

func readRejectCounters(statsURL string) (cds, lds int64) {
	body, err := httpGet(statsURL)
	if err != nil {
		return 0, 0
	}
	cds, _ = parseStatsCounter(body, "cluster_manager.cds.update_rejected")
	lds, _ = parseStatsCounter(body, "listener_manager.lds.update_rejected")
	return cds, lds
}

func parseStatsCounter(statsBody, name string) (int64, bool) {
	// Envoy text stats: "name: value"
	prefix := name + ": "
	for _, line := range strings.Split(statsBody, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		var n int64
		if _, err := fmt.Sscanf(strings.TrimPrefix(line, prefix), "%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

func httpGetOK(url string) bool {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func httpGet(url string) (string, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// PortListening reports whether tcp port is in use on loopback.
func PortListening(port int) bool {
	addr := fmt.Sprintf("%s:%d", DefaultListenAddr, port)
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// EnsureLocalADS starts or reuses the process-global ADS server on loopback.
func EnsureLocalADS(port int) (*Server, error) {
	return Global().Ensure(port)
}

var globalRegistry = &registry{}

type registry struct {
	mu     sync.Mutex
	server *Server
}

// Global returns the process-global xDS registry.
func Global() *registry {
	return globalRegistry
}

// MgmtPortListening reports whether the loopback mgmt HTTP port is open.
func MgmtPortListening(xdsPort int) bool {
	return PortListening(MgmtPort(xdsPort))
}

// Ensure starts ADS on loopback if not already running.
func (r *registry) Ensure(port int) (*Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil && r.server.Running() {
		return r.server, nil
	}
	if port <= 0 {
		port = DefaultPort
	}
	if PortListening(port) && !r.localRunning() {
		if !MgmtPortListening(port) {
			return nil, fmt.Errorf("xds: port %d in use but mgmt %d not listening (start panel or xds serve)", port, MgmtPort(port))
		}
		s := &Server{
			Port:      port,
			Publisher: &PeerPublisher{BaseURL: peerPublisherURL(port)},
		}
		r.server = s
		return s, nil
	}
	s := NewServer(nil)
	s.Port = port
	if err := s.Start(); err != nil {
		return nil, err
	}
	ensureMgmtHTTP(port, s.Publisher)
	r.server = s
	return s, nil
}

func (r *registry) localRunning() bool {
	return r.server != nil && r.server.Running()
}

// Server returns the running server or nil.
func (r *registry) Server() *Server {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.server
}

// PublishResources is a helper for tests: load snapshot via global publisher.
func (r *registry) Publish(nodeID string, snap Snapshot) error {
	srv, err := r.Ensure(DefaultPort)
	if err != nil {
		return err
	}
	return srv.Publisher.SetSnapshot(nodeID, snap)
}

// Stop stops the global ADS server (tests / CLI one-shot).
func (r *registry) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil {
		_ = r.server.Stop()
		r.server = nil
	}
}

// IsEnabledFromEnv reads XDS_ENABLED (default on when unset; 0/false/off disables).
func IsEnabledFromEnv() bool {
	v := strings.TrimSpace(os.Getenv("XDS_ENABLED"))
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
