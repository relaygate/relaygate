package resources

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Known product-contract keys for built-in protections (yaml/json tags on PolicyParams).
var policyParamKnownKeys = map[string]struct{}{
	"tcp_per_ip":            {},
	"per_sec":               {},
	"burst":                 {},
	"max_connections":       {},
	"udp_pps_per_ip":        {},
	"udp_burst":             {},
	"tcp_syncookies":        {},
	"tcp_max_syn_backlog":   {},
	"tcp_synack_retries":    {},
	"tcp_syn_retries":       {},
	"tcp_abort_on_overflow": {},
}

// IsZero reports whether p has no known fields and no Extra (for yaml omitempty).
func (p PolicyParams) IsZero() bool {
	return p.TCPPerIP == "" &&
		p.PerSec == 0 &&
		p.Burst == 0 &&
		p.MaxConnections == 0 &&
		p.UDPPPSPerIP == "" &&
		p.UDPBurst == 0 &&
		p.TcpSyncookies == 0 &&
		p.TcpMaxSynBacklog == 0 &&
		p.TcpSynackRetries == 0 &&
		p.TcpSynRetries == 0 &&
		p.TcpAbortOnOverflow == 0 &&
		len(p.Extra) == 0
}

// UnmarshalYAML keeps unknown keys in Extra so Load→Save does not drop them.
func (p *PolicyParams) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Kind == yaml.ScalarNode && (value.Tag == "!!null" || value.Value == "") {
		*p = PolicyParams{}
		return nil
	}
	var raw map[string]any
	if err := value.Decode(&raw); err != nil {
		return err
	}
	return p.fromMap(raw)
}

// MarshalYAML emits known keys plus Extra (Extra must not override known keys).
func (p PolicyParams) MarshalYAML() (any, error) {
	return p.toMap(), nil
}

// UnmarshalJSON keeps unknown keys in Extra for API round-trip.
func (p *PolicyParams) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*p = PolicyParams{}
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return p.fromMap(raw)
}

// MarshalJSON emits known keys plus Extra (Extra must not override known keys).
func (p PolicyParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.toMap())
}

func (p *PolicyParams) fromMap(raw map[string]any) error {
	out := PolicyParams{}
	extra := map[string]any{}
	for k, v := range raw {
		if _, known := policyParamKnownKeys[k]; !known {
			extra[k] = v
			continue
		}
		if v == nil {
			continue
		}
		switch k {
		case "tcp_per_ip":
			s, err := asString(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.TCPPerIP = s
		case "per_sec":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.PerSec = n
		case "burst":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.Burst = n
		case "max_connections":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.MaxConnections = n
		case "udp_pps_per_ip":
			s, err := asString(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.UDPPPSPerIP = s
		case "udp_burst":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.UDPBurst = n
		case "tcp_syncookies":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.TcpSyncookies = n
		case "tcp_max_syn_backlog":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.TcpMaxSynBacklog = n
		case "tcp_synack_retries":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.TcpSynackRetries = n
		case "tcp_syn_retries":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.TcpSynRetries = n
		case "tcp_abort_on_overflow":
			n, err := asInt(v)
			if err != nil {
				return fmt.Errorf("params.%s: %w", k, err)
			}
			out.TcpAbortOnOverflow = n
		}
	}
	if len(extra) > 0 {
		out.Extra = extra
	}
	*p = out
	return nil
}

func (p PolicyParams) toMap() map[string]any {
	m := map[string]any{}
	if p.TCPPerIP != "" {
		m["tcp_per_ip"] = p.TCPPerIP
	}
	if p.PerSec != 0 {
		m["per_sec"] = p.PerSec
	}
	if p.Burst != 0 {
		m["burst"] = p.Burst
	}
	if p.MaxConnections != 0 {
		m["max_connections"] = p.MaxConnections
	}
	if p.UDPPPSPerIP != "" {
		m["udp_pps_per_ip"] = p.UDPPPSPerIP
	}
	if p.UDPBurst != 0 {
		m["udp_burst"] = p.UDPBurst
	}
	if p.TcpSyncookies != 0 {
		m["tcp_syncookies"] = p.TcpSyncookies
	}
	if p.TcpMaxSynBacklog != 0 {
		m["tcp_max_syn_backlog"] = p.TcpMaxSynBacklog
	}
	if p.TcpSynackRetries != 0 {
		m["tcp_synack_retries"] = p.TcpSynackRetries
	}
	if p.TcpSynRetries != 0 {
		m["tcp_syn_retries"] = p.TcpSynRetries
	}
	if p.TcpAbortOnOverflow != 0 {
		m["tcp_abort_on_overflow"] = p.TcpAbortOnOverflow
	}
	// Extra merges after known keys and never overrides them.
	for k, v := range p.Extra {
		if _, known := policyParamKnownKeys[k]; known {
			continue
		}
		if _, exists := m[k]; exists {
			continue
		}
		m[k] = v
	}
	return m
}

func asString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10), nil
		}
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(t), nil
	default:
		return "", fmt.Errorf("want string, got %T", v)
	}
}

func asInt(v any) (int, error) {
	switch t := v.(type) {
	case int:
		return t, nil
	case int64:
		return int(t), nil
	case uint64:
		return int(t), nil
	case float64:
		if t != float64(int(t)) {
			return 0, fmt.Errorf("want integer, got %v", t)
		}
		return int(t), nil
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0, err
		}
		return int(n), nil
	case string:
		n, err := strconv.Atoi(t)
		if err != nil {
			return 0, fmt.Errorf("want integer, got %q", t)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("want integer, got %T", v)
	}
}

// MergePolicyParamsExtra copies Extra keys from src into dst (src wins per key).
// Does not remove dst keys absent from src. Extra never feeds built-in Effective*.
func MergePolicyParamsExtra(dst *PolicyParams, src PolicyParams) {
	if dst == nil || len(src.Extra) == 0 {
		return
	}
	if dst.Extra == nil {
		dst.Extra = make(map[string]any, len(src.Extra))
	}
	for k, v := range src.Extra {
		if _, known := policyParamKnownKeys[k]; known {
			continue
		}
		dst.Extra[k] = v
	}
}

// diffPolicyParamsExtra appends Extra key changes (sorted). Extra is not used by Effective*.
func diffPolicyParamsExtra(id string, before, after PolicyParams) []string {
	keys := map[string]struct{}{}
	for k := range before.Extra {
		keys[k] = struct{}{}
	}
	for k := range after.Extra {
		keys[k] = struct{}{}
	}
	if len(keys) == 0 {
		return nil
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		if _, known := policyParamKnownKeys[k]; known {
			continue
		}
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	var parts []string
	for _, k := range ordered {
		var bv, av any
		if before.Extra != nil {
			bv = before.Extra[k]
		}
		if after.Extra != nil {
			av = after.Extra[k]
		}
		bs, as := fmt.Sprint(bv), fmt.Sprint(av)
		if bs == as {
			continue
		}
		parts = append(parts, fmt.Sprintf("security.protections.%s.params.%s %s→%s", id, k, bs, as))
	}
	return parts
}
