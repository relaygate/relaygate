package status

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type EnvoyStatus struct {
	Ready           bool              `json:"ready"`
	ReadyBody       string            `json:"ready_body,omitempty"`
	HealthyClusters int               `json:"healthy_clusters"`
	ClusterLines    []string          `json:"cluster_lines,omitempty"`
	Error           string            `json:"error,omitempty"`
	Stats           map[string]string `json:"stats,omitempty"`
}

// RuleRateLimit is per-rule TCP local rate-limit hits (from Envoy rl_<rule> stat_prefix).
type RuleRateLimit struct {
	Rule   string  `json:"rule"`
	Prefix string  `json:"prefix"`
	Hits5m float64 `json:"hits_5m"`
}

type TrafficStatus struct {
	TCPActiveConnections float64         `json:"tcp_active_connections"`
	UDPActiveSessions    float64         `json:"udp_active_sessions"`
	LocalRateLimited5m   float64         `json:"local_rate_limited_5m"`
	TopLimitedRules      []RuleRateLimit `json:"top_limited_rules,omitempty"`
	Error                string          `json:"error,omitempty"`
}

type Client struct {
	EnvoyAdmin string
	Prometheus string
	HTTP       *http.Client
}

func New(envoyAdmin, prometheus string) *Client {
	if envoyAdmin == "" {
		envoyAdmin = "http://127.0.0.1:9901"
	}
	if prometheus == "" {
		prometheus = "http://127.0.0.1:9090"
	}
	return &Client{
		EnvoyAdmin: strings.TrimRight(envoyAdmin, "/"),
		Prometheus: strings.TrimRight(prometheus, "/"),
		HTTP:       &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *Client) Envoy() EnvoyStatus {
	st := EnvoyStatus{Stats: map[string]string{}}
	readyBody, err := c.get(c.EnvoyAdmin + "/ready")
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.ReadyBody = strings.TrimSpace(readyBody)
	st.Ready = strings.Contains(strings.ToUpper(st.ReadyBody), "LIVE")

	clusters, err := c.get(c.EnvoyAdmin + "/clusters")
	if err == nil {
		re := regexp.MustCompile(`(?m)^(cluster-server-[^:]+)::(health|healthy)\s+(\S+)`)
		healthy := map[string]bool{}
		for _, line := range strings.Split(clusters, "\n") {
			if strings.Contains(line, "cluster-server-") && (strings.Contains(line, "health") || strings.Contains(line, "::healthy")) {
				st.ClusterLines = append(st.ClusterLines, line)
			}
			m := re.FindStringSubmatch(line)
			if len(m) == 4 {
				if m[3] == "healthy" || m[3] == "1" || m[3] == "true" {
					healthy[m[1]] = true
				}
			}
		}
		// Fallback: count membership_healthy style lines
		if len(healthy) == 0 {
			for _, line := range strings.Split(clusters, "\n") {
				if strings.Contains(line, "cluster-server-") && strings.Contains(line, "::health_flags::healthy") {
					parts := strings.Split(line, "::")
					if len(parts) > 0 {
						healthy[parts[0]] = true
					}
				}
			}
		}
		st.HealthyClusters = len(healthy)
	}

	stats, err := c.get(c.EnvoyAdmin + "/stats")
	if err == nil {
		for _, key := range []string{
			"server.live",
			"server.uptime",
			"cluster_manager.active_clusters",
		} {
			if v, ok := findStat(stats, key); ok {
				st.Stats[key] = v
			}
		}
	}
	return st
}

func (c *Client) Traffic() TrafficStatus {
	st := TrafficStatus{}
	tcp, err1 := c.promQuery(`sum(envoy_cluster_upstream_cx_active{envoy_cluster_name=~"cluster-server-.*-tcp"}) or vector(0)`)
	udp, err2 := c.promQuery(`sum(envoy_udp_downstream_sess_active) or vector(0)`)
	rl, err3 := c.promQuery(`sum(increase(envoy_local_rate_limit_rate_limited[5m])) or vector(0)`)
	top, err4 := c.promQueryVector(`topk(5, sum by (envoy_local_rate_limit) (increase(envoy_local_rate_limit_rate_limited[5m])))`)
	if err4 != nil {
		// Older/alternate label for local_ratelimit stat_prefix
		top, err4 = c.promQueryVector(`topk(5, sum by (stat_prefix) (increase(envoy_local_rate_limit_rate_limited[5m])))`)
	}
	if err1 != nil || err2 != nil || err3 != nil {
		var errs []string
		if err1 != nil {
			errs = append(errs, err1.Error())
		}
		if err2 != nil {
			errs = append(errs, err2.Error())
		}
		if err3 != nil {
			errs = append(errs, err3.Error())
		}
		st.Error = strings.Join(errs, "; ")
	}
	st.TCPActiveConnections = tcp
	st.UDPActiveSessions = udp
	st.LocalRateLimited5m = rl
	if err4 == nil {
		st.TopLimitedRules = parseTopLimited(top)
	}
	return st
}

type promSample struct {
	Metric map[string]string
	Value  float64
}

func parseTopLimited(samples []promSample) []RuleRateLimit {
	out := make([]RuleRateLimit, 0, len(samples))
	for _, s := range samples {
		prefix := s.Metric["envoy_local_rate_limit"]
		if prefix == "" {
			prefix = s.Metric["stat_prefix"]
		}
		if prefix == "" {
			continue
		}
		rule := prefix
		if strings.HasPrefix(rule, "rl_") {
			rule = strings.ReplaceAll(strings.TrimPrefix(rule, "rl_"), "_", "-")
		}
		out = append(out, RuleRateLimit{Rule: rule, Prefix: prefix, Hits5m: s.Value})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hits5m > out[j].Hits5m })
	return out
}

func (c *Client) get(u string) (string, error) {
	resp, err := c.HTTP.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return string(b), nil
}

func (c *Client) promQuery(expr string) (float64, error) {
	samples, err := c.promQueryVector(expr)
	if err != nil {
		return 0, err
	}
	if len(samples) == 0 {
		return 0, nil
	}
	return samples[0].Value, nil
}

func (c *Client) promQueryVector(expr string) ([]promSample, error) {
	u := c.Prometheus + "/api/v1/query?query=" + url.QueryEscape(expr)
	resp, err := c.HTTP.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed")
	}
	out := make([]promSample, 0, len(payload.Data.Result))
	for _, r := range payload.Data.Result {
		if len(r.Value) < 2 {
			continue
		}
		s, _ := r.Value[1].(string)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			continue
		}
		out = append(out, promSample{Metric: r.Metric, Value: v})
	}
	return out, nil
}

func findStat(body, key string) (string, bool) {
	prefix := key + ": "
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
		}
	}
	return "", false
}
