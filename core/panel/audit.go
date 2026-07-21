package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
)

// appendAudit writes an append-only audit line (no passwords).
func (s *Server) appendAudit(action, detail string) {
	action = strings.TrimSpace(action)
	if action == "" {
		return
	}
	detail = strings.ReplaceAll(strings.TrimSpace(detail), "\n", " ")
	if len(detail) > 500 {
		detail = detail[:500] + "…"
	}
	dir := config.ResolveDataDir(s.cfg.Root)
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "panel-audit.log")
	line := fmt.Sprintf("%s\t%s\t%s\n", time.Now().UTC().Format(time.RFC3339), action, detail)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}
