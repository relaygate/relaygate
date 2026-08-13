package dataplane

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// maybePrivilegedReexec re-execs via RELAYGATE_PRIVILEGED_HELPER when non-root.
// args are passed to the helper (first arg is a whitelisted subcommand).
// Returns (handled=true, err) if reexec was attempted.
func maybePrivilegedReexec(stdout, stderr io.Writer, args ...string) (bool, error) {
	helper := os.Getenv("RELAYGATE_PRIVILEGED_HELPER")
	if helper == "" || IsRoot() {
		return false, nil
	}
	if st, err := os.Stat(helper); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		return true, fmt.Errorf("privileged helper 不可执行: %s", helper)
	}
	cmd := exec.Command("sudo", append([]string{"-n", helper}, args...)...)
	cmd.Stdout = stdout
	var errBuf bytes.Buffer
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, &errBuf)
	} else {
		cmd.Stderr = &errBuf
	}
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if detail := strings.TrimSpace(errBuf.String()); detail != "" {
			return true, fmt.Errorf("%s: %w", detail, err)
		}
		return true, err
	}
	return true, nil
}

// errNeedRootOrHelper is returned when an op requires root and no helper is configured.
func errNeedRootOrHelper() error {
	if os.Getenv("RELAYGATE_PRIVILEGED_HELPER") == "" {
		return fmt.Errorf("需要 root（或配置 RELAYGATE_PRIVILEGED_HELPER）")
	}
	return fmt.Errorf("需要 root")
}
