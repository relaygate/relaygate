package panel

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/render"
	"github.com/relaygate/relaygate/core/resources"

	"gopkg.in/yaml.v3"
)

type configYAMLError struct {
	Line *int   `json:"line,omitempty"`
	Path string `json:"path,omitempty"`
	Msg  string `json:"msg"`
}

type configValidateResult struct {
	OK     bool              `json:"ok"`
	Errors []configYAMLError `json:"errors,omitempty"`
	Diff   string            `json:"diff,omitempty"`
}

var yamlLineRe = regexp.MustCompile(`(?i)line\s+(\d+)`)

func (s *Server) apiConfigResources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.apiConfigResourcesGet(w, r)
	case http.MethodPut:
		s.apiConfigResourcesPut(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiConfigResourcesGet(w http.ResponseWriter, r *http.Request) {
	path := s.resourcesPath()
	content, mtime, err := readResourcesFile(path)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"content": string(content),
		"mtime":   mtime.UTC().Format(time.RFC3339Nano),
		"etag":    contentETag(content),
	})
}

func (s *Server) apiConfigResourcesPut(w http.ResponseWriter, r *http.Request) {
	content, baseMtime, baseETag, err := readConfigBody(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	path := s.resourcesPath()
	curContent, curMtime, err := readResourcesFile(path)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	curETag := contentETag(curContent)
	if baseETag == "" && baseMtime == "" {
		writeJSON(w, 400, map[string]any{"error": "etag or mtime required (optimistic concurrency)"})
		return
	}
	if baseETag != "" && baseETag != curETag {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "etag mismatch; reload and retry",
			"mtime":   curMtime.UTC().Format(time.RFC3339Nano),
			"etag":    curETag,
			"content": string(curContent),
		})
		return
	}
	if baseETag == "" && !mtimeEqual(baseMtime, curMtime) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "mtime mismatch; reload and retry",
			"mtime":   curMtime.UTC().Format(time.RFC3339Nano),
			"etag":    curETag,
			"content": string(curContent),
		})
		return
	}

	result := s.validateResourcesYAML(content)
	if !result.OK {
		writeJSON(w, 400, result)
		return
	}

	if err := writeFileAtomic(path, content); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	// Ensure mtime advances even on coarse filesystems when bytes change.
	now := time.Now().UTC()
	_ = os.Chtimes(path, now, now)
	_, newMtime, err := readResourcesFile(path)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.appendAudit("config.resources.put", fmt.Sprintf("bytes=%d etag=%s diff=%s", len(content), contentETag(content), compactOneLine(result.Diff)))
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"mtime":   newMtime.UTC().Format(time.RFC3339Nano),
		"etag":    contentETag(content),
		"diff":    result.Diff,
		"message": "saved; apply required (no auto reload)",
	})
}

func (s *Server) apiConfigResourcesValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	content, _, _, err := readConfigBody(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	result := s.validateResourcesYAML(content)
	code := 200
	if !result.OK {
		code = 400
	}
	writeJSON(w, code, result)
}

func (s *Server) apiConfigExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.EqualFold(r.URL.Query().Get("pack"), "zip") {
		s.apiConfigExportZip(w, r)
		return
	}
	path := s.resourcesPath()
	content, _, err := readResourcesFile(path)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="resources.yaml"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) apiConfigExportZip(w http.ResponseWriter, r *http.Request) {
	paths := config.ResolvePaths(s.cfg.Root)
	resContent, _, err := readResourcesFile(paths.Resources)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := zipWrite(zw, "resources.yaml", resContent); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}

	if nft, err := os.ReadFile(paths.ForwardPorts); err == nil {
		_ = zipWrite(zw, "forward-ports.nft", nft)
	}

	summary := "（无法生成摘要）\n"
	if res, err := resources.Load(paths.Resources); err == nil {
		summary = render.Summarize(res)
	}
	_ = zipWrite(zw, "SUMMARY.txt", []byte(summary))
	if err := zw.Close(); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="relaygate-config.zip"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) validateResourcesYAML(content []byte) configValidateResult {
	res, errs := parseResourcesYAML(content)
	if len(errs) > 0 {
		return configValidateResult{OK: false, Errors: errs}
	}
	if err := res.Validate(); err != nil {
		return configValidateResult{
			OK: false,
			Errors: []configYAMLError{{
				Msg: err.Error(),
			}},
		}
	}

	var before *resources.Resources
	if cur, err := s.load(); err == nil {
		before = cur
	}
	diff := resources.Diff(before, res).String()
	return configValidateResult{OK: true, Diff: diff}
}

func parseResourcesYAML(content []byte) (*resources.Resources, []configYAMLError) {
	var r resources.Resources
	if err := yaml.Unmarshal(content, &r); err != nil {
		return nil, []configYAMLError{yamlErrToConfig(err)}
	}
	return &r, nil
}

func yamlErrToConfig(err error) configYAMLError {
	msg := err.Error()
	out := configYAMLError{Msg: msg}
	if m := yamlLineRe.FindStringSubmatch(msg); len(m) == 2 {
		if n, convErr := strconv.Atoi(m[1]); convErr == nil {
			out.Line = &n
		}
	}
	if te, ok := err.(*yaml.TypeError); ok && len(te.Errors) > 0 {
		out.Msg = strings.Join(te.Errors, "; ")
		if m := yamlLineRe.FindStringSubmatch(out.Msg); len(m) == 2 {
			if n, convErr := strconv.Atoi(m[1]); convErr == nil {
				out.Line = &n
			}
		}
	}
	return out
}

func readResourcesFile(path string) ([]byte, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("stat resources.yaml: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read resources.yaml: %w", err)
	}
	return b, info.ModTime(), nil
}

func readConfigBody(r *http.Request) (content []byte, mtime, etag string, err error) {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return nil, "", "", fmt.Errorf("read body: %w", err)
	}
	mtime = strings.TrimSpace(r.Header.Get("X-Resources-Mtime"))
	etag = strings.TrimSpace(r.Header.Get("If-Match"))
	if etag == "" {
		etag = strings.TrimSpace(r.Header.Get("X-Resources-ETag"))
	}
	mtime = strings.Trim(mtime, `"'`)
	etag = strings.Trim(etag, `"'`)

	if strings.Contains(ct, "application/json") {
		var payload struct {
			Content string `json:"content"`
			Mtime   string `json:"mtime"`
			ETag    string `json:"etag"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, "", "", fmt.Errorf("invalid json body: %w", err)
		}
		if strings.TrimSpace(payload.Content) == "" {
			return nil, "", "", fmt.Errorf("content is required")
		}
		if payload.Mtime != "" {
			mtime = strings.TrimSpace(payload.Mtime)
		}
		if payload.ETag != "" {
			etag = strings.TrimSpace(payload.ETag)
		}
		return []byte(payload.Content), mtime, etag, nil
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return nil, "", "", fmt.Errorf("empty body")
	}
	return body, mtime, etag, nil
}

func contentETag(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func mtimeEqual(client string, disk time.Time) bool {
	client = strings.TrimSpace(strings.Trim(client, `"'`))
	if client == "" {
		return false
	}
	diskUTC := disk.UTC()
	formats := []string{time.RFC3339Nano, time.RFC3339}
	for _, f := range formats {
		if t, err := time.Parse(f, client); err == nil {
			return t.UTC().Equal(diskUTC) || t.UTC().UnixNano() == diskUTC.UnixNano()
		}
	}
	// Also accept exact string match against formatted disk mtime.
	return client == diskUTC.Format(time.RFC3339Nano) || client == diskUTC.Format(time.RFC3339)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".resources-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func zipWrite(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func compactOneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
