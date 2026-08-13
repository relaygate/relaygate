package panel

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/relaygate/relaygate/core/config"
	"github.com/relaygate/relaygate/core/dataplane"
	"github.com/relaygate/relaygate/core/profile"
	"github.com/relaygate/relaygate/core/resources"
)

func (s *Server) apiSecurityPreview(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.securityPreviewFromSaved(w, r)
	case http.MethodPost:
		s.securityPreviewFromDraft(w, r)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) apiSecurityProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	names, _ := profile.List(s.cfg.Root)
	type item struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Scenario    string `json:"scenario,omitempty"`
	}
	items := make([]item, 0, len(names))
	for _, name := range names {
		p, err := profile.Load(s.cfg.Root, name)
		if err != nil {
			continue
		}
		items = append(items, item{Name: name, Description: p.Description, Scenario: p.Scenario})
	}
	writeJSON(w, 200, map[string]any{"profiles": items})
}

func (s *Server) securityPreviewFromSaved(w http.ResponseWriter, r *http.Request) {
	resPath := config.ResolvePaths(s.cfg.Root).Resources
	res, err := resources.Load(resPath)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.writeSecurityPreview(w, res)
}

func (s *Server) securityPreviewFromDraft(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Access       *resources.SecurityAccess  `json:"access"`
		Protections  []resources.SecurityPolicy `json:"protections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	resPath := config.ResolvePaths(s.cfg.Root).Resources
	res, err := resources.Load(resPath)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	if body.Access != nil {
		res.Security.Access = body.Access
	}
	if len(body.Protections) > 0 {
		res.Security.Protections = body.Protections
	}
	res.Security.EnsureSecurityDefaults()
	s.writeSecurityPreview(w, res)
}

func (s *Server) writeSecurityPreview(w http.ResponseWriter, res *resources.Resources) {
	packaging := config.ResolvePaths(s.cfg.Root).Packaging
	fwdPorts, gatewayExcerpt, err := dataplane.PreviewSecurityNft(s.cfg.Root, res)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	prev, err := resources.BuildSecurityPreview(res, packaging, fwdPorts, gatewayExcerpt)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, prev)
}

func (s *Server) apiSecurityProfileApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(body.Name)
	p, err := profile.Load(s.cfg.Root, name)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	// Return merged security state for Panel (does not write resources.yaml).
	resPath := config.ResolvePaths(s.cfg.Root).Resources
	res, err := resources.Load(resPath)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	merged := res.Security
	profile.MergeSecurityInto(&merged, p.Security)
	writeJSON(w, 200, map[string]any{
		"name":         p.Name,
		"description":  p.Description,
		"scenario":     p.Scenario,
		"access":       merged.Access,
		"protections":  merged.Protections,
	})
}
