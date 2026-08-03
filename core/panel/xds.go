package panel

import (
	"fmt"
	"log"

	"github.com/relaygate/relaygate/core/dataplane"
	"github.com/relaygate/relaygate/core/xds"
)

// StartXDSSidecar starts loopback ADS and publishes initial snapshot when XDS_ENABLED=1.
func StartXDSSidecar(root string) error {
	env, err := dataplane.LoadEnv(root)
	if err != nil {
		return err
	}
	if !env.XDSEnabled {
		return nil
	}
	xds.SetDiskPublishHandler(func(nodeID string) (string, error) {
		e, err := dataplane.LoadEnv(root)
		if err != nil {
			return "", err
		}
		srv := xds.Global().Server()
		if srv == nil {
			return "", fmt.Errorf("xds: ADS not running")
		}
		return dataplane.PublishSnapshotFromDisk(root, e, nodeID, srv.Publisher)
	})
	if err := dataplane.PublishInitialSnapshot(root, env); err != nil {
		return err
	}
	log.Printf("xDS ADS listening (port %s), initial snapshot published for %s", env.XDSPort, env.GatewayName)
	return nil
}
