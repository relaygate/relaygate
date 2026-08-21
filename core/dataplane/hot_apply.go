package dataplane

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/resources"
	"github.com/relaygate/relaygate/core/xds"
)

// HotApplyACKTimeout returns configured ACK wait or default.
func HotApplyACKTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("XDS_ACK_TIMEOUT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return xds.DefaultACKTimeout
}

// ApplyMode classifies how resources diff should be applied.
func ApplyMode(env Env, summary resources.ChangeSummary) string {
	return ApplyModeForRoot("", env, summary)
}

// ApplyModeForRoot is ApplyMode; when root is set, also checks on-disk bootstrap.
func ApplyModeForRoot(root string, env Env, summary resources.ChangeSummary) string {
	plan := summary.Classify()
	if !plan.NeedsReload {
		return "none"
	}
	if env.XDSEnabled && plan.CanHotApply && !plan.NeedsHardReload {
		if root == "" || render.IsBootstrapMigrated(config.ResolvePaths(root).EnvoyYAML) {
			return "hot"
		}
	}
	return "hard"
}

// BootstrapMigrated reports whether DataDir envoy.yaml is ADS bootstrap.
func BootstrapMigrated(root string) bool {
	return render.IsBootstrapMigrated(config.ResolvePaths(root).EnvoyYAML)
}

// HotApplyTo publishes Envoy CDS+LDS via in-process ADS without docker restart.
func HotApplyTo(root string, env Env, stdout, stderr io.Writer) error {
	totalStart := time.Now()
	stage := func(name string, fn func() error) error {
		start := time.Now()
		logf(stdout, "==> [%s] start", name)
		err := fn()
		elapsed := time.Since(start).Round(time.Millisecond)
		if err != nil {
			logf(stdout, "==> [%s] FAIL (%s): %v", name, elapsed, err)
			return err
		}
		logf(stdout, "==> [%s] ok (%s)", name, elapsed)
		return nil
	}

	if err := stage("backup", func() error {
		stamp, dir, _, err := BackupWithSummary(root, stdout)
		if err != nil {
			return err
		}
		logf(stdout, "backup stamp=%s dir=%s", stamp, dir)
		return nil
	}); err != nil {
		return err
	}

	p := config.ResolvePaths(root)
	res, err := resources.Load(p.Resources)
	if err != nil {
		return err
	}
	if err := res.Validate(); err != nil {
		return err
	}

	opt := render.OptionsFromEnv()
	bootOpt := render.BootstrapOptionsFromEnv(env.GatewayName, env.XDSPort, res)
	xdsPort := bootOpt.XDSPort
	nodeID := bootOpt.NodeID

	if err := stage("render-bootstrap", func() error {
		return render.WriteBootstrap(p.EnvoyYAML, p.ForwardPorts, res, bootOpt, opt)
	}); err != nil {
		return err
	}

	if err := stage("validate", func() error {
		tmp := filepath.Join(p.DataDir, "envoy", "validate-merge.yaml")
		if err := render.WriteMergedValidate(tmp, res, opt); err != nil {
			return err
		}
		defer os.Remove(tmp)
		// HotApply fail-closed: never publish when validate is skipped or fails.
		return validateEnvoyFile(root, env, tmp, false)
	}); err != nil {
		return WrapUserFacing(err)
	}

	srv, err := xds.EnsureLocalADS(xdsPort)
	if err != nil {
		return WrapUserFacing(fmt.Errorf("xds ADS: %w", err))
	}

	var version string
	if err := stage("publish", func() error {
		var pubErr error
		version, pubErr = PublishSnapshotFromDisk(root, env, nodeID, srv.Publisher)
		return pubErr
	}); err != nil {
		return WrapUserFacing(err)
	}

	if err := stage("ack", func() error {
		admin := env.AdminURL("")
		admin = strings.TrimSuffix(admin, "/")
		start := time.Now()
		if err := xds.WaitEnvoyApplied(admin, version, HotApplyACKTimeout()); err != nil {
			return err
		}
		xds.RecordHotApplyOK(version, time.Since(start))
		return nil
	}); err != nil {
		xds.RecordHotApplyFail()
		logf(stdout, "WARN: ACK wait failed, rolling back snapshot")
		rbErr := srv.Publisher.Rollback(nodeID)
		if rbErr != nil {
			logf(stdout, "WARN: rollback failed: %v", rbErr)
			return WrapUserFacing(fmt.Errorf("%v；回滚上一快照失败: %v", err, rbErr))
		}
		logf(stdout, "rollback ok: restored previous snapshot")
		return WrapUserFacing(fmt.Errorf("%v（已回滚上一快照）", err))
	}

	logf(stdout, "HotApply ok (%s) node=%s version=%s total=%s (no drain, no docker restart)",
		env.GatewayName, nodeID, version, time.Since(totalStart).Round(time.Millisecond))
	return nil
}

func validateEnvoyFile(root string, env Env, yamlPath string, skipWarn bool) error {
	if !LookPath("docker") {
		if skipWarn {
			warnf("未安装 docker，跳过 envoy --mode validate")
			return nil
		}
		return fmt.Errorf("docker 不可用")
	}
	logsDir := filepath.Join(config.ResolvePaths(root).DataDir, "envoy", "logs")
	if err := os.MkdirAll(logsDir, 0o777); err != nil {
		return fmt.Errorf("创建 Envoy 日志目录: %w", err)
	}
	err := RunCmd(root, "docker", "run", "--rm",
		"-v", yamlPath+":/etc/envoy/envoy.yaml:ro",
		"-v", logsDir+":/var/log/envoy",
		env.EnvoyImage,
		"/usr/local/bin/envoy", "-c", "/etc/envoy/envoy.yaml", "--mode", "validate")
	if err != nil && skipWarn {
		warnf("envoy validate 容器未能运行，继续")
		return nil
	}
	return err
}

// PublishSnapshotFromDisk builds CDS+LDS from DataDir/resources.yaml and publishes.
func PublishSnapshotFromDisk(root string, env Env, nodeID string, pub xds.Publisher) (string, error) {
	if peer, ok := pub.(*xds.PeerPublisher); ok {
		if err := peer.SetSnapshot(nodeID, xds.Snapshot{NodeID: nodeID}); err != nil {
			return "", err
		}
		return peer.LastVersion(nodeID), nil
	}
	p := config.ResolvePaths(root)
	res, err := resources.Load(p.Resources)
	if err != nil {
		return "", err
	}
	if err := res.Validate(); err != nil {
		return "", err
	}
	opt := render.OptionsFromEnv()
	version := pub.NextVersion()
	inner, err := render.NewSnapshotFromResources(res, opt, version)
	if err != nil {
		return "", err
	}
	if err := pub.SetSnapshot(nodeID, xds.Snapshot{
		Version: version,
		NodeID:  nodeID,
		Inner:   inner,
	}); err != nil {
		return "", err
	}
	return version, nil
}

// PublishInitialSnapshot loads current resources and publishes to ADS (Panel startup).
func PublishInitialSnapshot(root string, env Env) error {
	bootOpt := render.BootstrapOptionsFromEnv(env.GatewayName, env.XDSPort, nil)
	srv, err := xds.EnsureLocalADS(bootOpt.XDSPort)
	if err != nil {
		return err
	}
	_, err = PublishSnapshotFromDisk(root, env, bootOpt.NodeID, srv.Publisher)
	return err
}

// EnsureGatewayADS starts loopback ADS (if needed) and publishes the current
// on-disk resources snapshot. Used after agent restart when applied-version is
// already aligned (skip host AfterPull / full HotApply) so Envoy can still
// fetch CDS+LDS from 127.0.0.1 ADS.
func EnsureGatewayADS(root string, env Env) error {
	if !env.XDSEnabled {
		return nil
	}
	xds.SetDiskPublishHandler(func(nodeID string) (string, error) {
		e, err := LoadEnv(root)
		if err != nil {
			return "", err
		}
		srv := xds.Global().Server()
		if srv == nil {
			return "", fmt.Errorf("本机热更新服务未运行")
		}
		return PublishSnapshotFromDisk(root, e, nodeID, srv.Publisher)
	})
	return PublishInitialSnapshot(root, env)
}
