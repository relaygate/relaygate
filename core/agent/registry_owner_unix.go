//go:build unix

package agent

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// preserveRegistryOwner keeps uid/gid of an existing nodes.yaml (or relaygate
// group when creating as root) so Panel User=relaygate can keep reading the roster.
func preserveRegistryOwner(existingPath, tmpPath string) {
	if st, err := os.Stat(existingPath); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			_ = os.Chown(tmpPath, int(sys.Uid), int(sys.Gid))
		}
		return
	}
	if os.Geteuid() != 0 {
		return
	}
	g, err := user.LookupGroup("relaygate")
	if err != nil {
		return
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return
	}
	_ = os.Chown(tmpPath, 0, gid)
}
