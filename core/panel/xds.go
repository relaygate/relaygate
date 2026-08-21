package panel

import (
	"log"

	"github.com/relaygate/relaygate/core/dataplane"
)

// StartXDSSidecar starts loopback ADS and publishes initial snapshot when XDS_ENABLED=1.
func StartXDSSidecar(root string) error {
	env, err := dataplane.LoadEnv(root)
	if err != nil {
		return err
	}
	if err := dataplane.EnsureGatewayADS(root, env); err != nil {
		return err
	}
	if !env.XDSEnabled {
		return nil
	}
	log.Printf("xDS ADS listening (port %s), initial snapshot published for %s", env.XDSPort, env.GatewayName)
	return nil
}
