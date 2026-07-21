package resources

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// ACL holds host-firewall IP allow/deny sets (nftables is the source of truth).
// Rendered as ACL_DENY / ACL_ALLOW set defines — not "forwarding rules".
// When Allow is non-empty, only listed CIDRs may reach forward ports (strict mode).
// SSH remains governed by gateway.nft SSH_PORT rules, not this list.
type ACL struct {
	Deny  []string `yaml:"deny,omitempty"`
	Allow []string `yaml:"allow,omitempty"`
}

// NormalizeCIDR accepts "1.2.3.4" or "1.2.3.4/32" and returns canonical CIDR form.
func NormalizeCIDR(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil {
			return "", fmt.Errorf("无效地址: %s", raw)
		}
		if v4 := ip.To4(); v4 != nil {
			return v4.String() + "/32", nil
		}
		return ip.String() + "/128", nil
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return "", fmt.Errorf("无效 CIDR: %s", raw)
	}
	ones, _ := n.Mask.Size()
	return fmt.Sprintf("%s/%d", n.IP.String(), ones), nil
}

// NormalizeACL trims, validates, and de-duplicates CIDR entries in place.
func (a *ACL) NormalizeACL() error {
	if a == nil {
		return nil
	}
	deny, err := normalizeCIDRList(a.Deny)
	if err != nil {
		return fmt.Errorf("acl.deny: %w", err)
	}
	allow, err := normalizeCIDRList(a.Allow)
	if err != nil {
		return fmt.Errorf("acl.allow: %w", err)
	}
	a.Deny = deny
	a.Allow = allow
	return nil
}

func normalizeCIDRList(in []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		cid, err := NormalizeCIDR(raw)
		if err != nil {
			return nil, err
		}
		if cid == "" {
			continue
		}
		if _, ok := seen[cid]; ok {
			continue
		}
		seen[cid] = struct{}{}
		out = append(out, cid)
	}
	sort.Strings(out)
	return out, nil
}

// AddACLEntry appends a CIDR to deny or allow ("deny"|"allow").
func (r *Resources) AddACLEntry(list, cidr string) (string, error) {
	canonical, err := NormalizeCIDR(cidr)
	if err != nil {
		return "", err
	}
	if canonical == "" {
		return "", fmt.Errorf("CIDR 不能为空")
	}
	switch strings.ToLower(strings.TrimSpace(list)) {
	case "deny":
		for _, e := range r.ACL.Deny {
			if e == canonical {
				return canonical, fmt.Errorf("已存在: %s", canonical)
			}
		}
		r.ACL.Deny = append(r.ACL.Deny, canonical)
		sort.Strings(r.ACL.Deny)
	case "allow":
		for _, e := range r.ACL.Allow {
			if e == canonical {
				return canonical, fmt.Errorf("已存在: %s", canonical)
			}
		}
		r.ACL.Allow = append(r.ACL.Allow, canonical)
		sort.Strings(r.ACL.Allow)
	default:
		return "", fmt.Errorf("名单须为 deny 或 allow，当前: %s", list)
	}
	return canonical, nil
}

// RemoveACLEntry removes a CIDR from deny or allow.
func (r *Resources) RemoveACLEntry(list, cidr string) (string, error) {
	canonical, err := NormalizeCIDR(cidr)
	if err != nil {
		return "", err
	}
	if canonical == "" {
		return "", fmt.Errorf("CIDR 不能为空")
	}
	switch strings.ToLower(strings.TrimSpace(list)) {
	case "deny":
		next, ok := removeFromList(r.ACL.Deny, canonical)
		if !ok {
			return "", fmt.Errorf("未找到: %s", canonical)
		}
		r.ACL.Deny = next
	case "allow":
		next, ok := removeFromList(r.ACL.Allow, canonical)
		if !ok {
			return "", fmt.Errorf("未找到: %s", canonical)
		}
		r.ACL.Allow = next
	default:
		return "", fmt.Errorf("名单须为 deny 或 allow，当前: %s", list)
	}
	return canonical, nil
}

func removeFromList(in []string, want string) ([]string, bool) {
	out := make([]string, 0, len(in))
	found := false
	for _, e := range in {
		if e == want {
			found = true
			continue
		}
		out = append(out, e)
	}
	return out, found
}
