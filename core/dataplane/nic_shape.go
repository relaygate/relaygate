package dataplane

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/relaygate/relaygate/core/resources"
)

// TBF / police burst defaults (not exposed as product params in v1).
const (
	nicTBFBurst    = "32kbit"
	nicTBFLatency  = "400ms"
	nicPoliceBurst = "32kbit"
)

var nicRateTokenNormRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)([kmg]?bit|[kmg]?bps)`)

// ApplyNICShapeFromResources applies enabled nic_* policies (egress TBF + ingress police).
// Disabled policies are skipped (does not remove existing qdisc/filters). Requires root.
func ApplyNICShapeFromResources(root string) error {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return err
	}
	egress := res.Security.EffectiveNICEgressShape()
	ingress := res.Security.EffectiveNICIngressPolice()
	if egress == nil && ingress == nil {
		fmt.Println("网卡：nic_egress_shape / nic_ingress_police 均已关闭，跳过")
		return nil
	}
	if !IsRoot() {
		if handled, err := maybePrivilegedReexec(os.Stdout, os.Stderr, "nic-shape-apply"); handled {
			return err
		}
		return errNeedRootOrHelper()
	}
	if egress != nil {
		if err := applyNICEgress(root, egress); err != nil {
			return err
		}
	} else {
		fmt.Println("网卡：nic_egress_shape 已关闭，跳过出口整形")
	}
	if ingress != nil {
		if err := applyNICIngress(root, ingress); err != nil {
			return err
		}
	} else {
		fmt.Println("网卡：nic_ingress_police 已关闭，跳过入向限速")
	}
	return nil
}

func applyNICEgress(root string, want *resources.NICEgressShapeParams) error {
	dev, err := resolveNICDevice(root, want.Device)
	if err != nil {
		return err
	}
	rate := strings.TrimSpace(want.Rate)
	if rate == "" {
		rate = resources.DefaultNICEgressRate
	}
	if !resources.ValidateNICRate(rate) {
		return fmt.Errorf("rate 无效: %s", rate)
	}
	// replace is idempotent; avoids stacking qdiscs on re-apply.
	if err := RunCmd(root, "tc", "qdisc", "replace", "dev", dev, "root", "handle", "1:",
		"tbf", "rate", rate, "burst", nicTBFBurst, "latency", nicTBFLatency); err != nil {
		return fmt.Errorf("应用网卡出口整形失败（%s）：%w", dev, err)
	}
	fmt.Printf("网卡出口整形已应用：dev=%s rate=%s\n", dev, rate)
	return nil
}

func applyNICIngress(root string, want *resources.NICIngressPoliceParams) error {
	dev, err := resolveNICDevice(root, want.Device)
	if err != nil {
		return err
	}
	rate := strings.TrimSpace(want.Rate)
	if rate == "" {
		rate = resources.DefaultNICIngressRate
	}
	if !resources.ValidateNICRate(rate) {
		return fmt.Errorf("rate 无效: %s", rate)
	}
	// Ingress qdisc handle ffff:; police filter drops excess inbound bytes.
	if err := RunCmd(root, "tc", "qdisc", "replace", "dev", dev, "handle", "ffff:", "ingress"); err != nil {
		return fmt.Errorf("应用网卡入向 qdisc 失败（%s）：%w", dev, err)
	}
	// replace filter is idempotent for the same parent/prio/protocol.
	if err := RunCmd(root, "tc", "filter", "replace", "dev", dev, "parent", "ffff:", "protocol", "all",
		"prio", "1", "u32", "match", "u32", "0", "0",
		"police", "rate", rate, "burst", nicPoliceBurst, "drop", "flowid", ":1"); err != nil {
		return fmt.Errorf("应用网卡入向 police 失败（%s）：%w", dev, err)
	}
	fmt.Printf("网卡入向限速已应用：dev=%s rate=%s\n", dev, rate)
	return nil
}

// VerifyNICShape checks live tc against enabled nic_* policies.
// When all nic policies are disabled, returns nil (nothing to verify).
func VerifyNICShape(root string) error {
	resPath, _, _ := resources.DefaultPaths(root)
	res, err := resources.Load(resPath)
	if err != nil {
		return err
	}
	if want := res.Security.EffectiveNICEgressShape(); want != nil {
		if err := verifyNICEgress(root, want); err != nil {
			return err
		}
	}
	if want := res.Security.EffectiveNICIngressPolice(); want != nil {
		if err := verifyNICIngress(root, want); err != nil {
			return err
		}
	}
	return nil
}

func verifyNICEgress(root string, want *resources.NICEgressShapeParams) error {
	dev, err := resolveNICDevice(root, want.Device)
	if err != nil {
		return err
	}
	rate := strings.TrimSpace(want.Rate)
	if rate == "" {
		rate = resources.DefaultNICEgressRate
	}
	out, err := RunCmdCapture(root, "tc", "qdisc", "show", "dev", dev)
	if err != nil {
		return fmt.Errorf("查询网卡 qdisc 失败（%s）：%v (%s)", dev, err, strings.TrimSpace(out))
	}
	if !nicQdiscMatches(out, rate) {
		return fmt.Errorf("网卡出口整形未按配置生效（%s）：期望 tbf rate≈%s；实际：%s",
			dev, rate, strings.TrimSpace(compactWhitespace(out)))
	}
	return nil
}

func verifyNICIngress(root string, want *resources.NICIngressPoliceParams) error {
	dev, err := resolveNICDevice(root, want.Device)
	if err != nil {
		return err
	}
	rate := strings.TrimSpace(want.Rate)
	if rate == "" {
		rate = resources.DefaultNICIngressRate
	}
	qOut, err := RunCmdCapture(root, "tc", "qdisc", "show", "dev", dev)
	if err != nil {
		return fmt.Errorf("查询网卡 qdisc 失败（%s）：%v (%s)", dev, err, strings.TrimSpace(qOut))
	}
	if !nicIngressQdiscPresent(qOut) {
		return fmt.Errorf("网卡入向限速未按配置生效（%s）：期望 ingress qdisc；实际：%s",
			dev, strings.TrimSpace(compactWhitespace(qOut)))
	}
	fOut, err := RunCmdCapture(root, "tc", "filter", "show", "dev", dev, "parent", "ffff:")
	if err != nil {
		return fmt.Errorf("查询网卡入向 filter 失败（%s）：%v (%s)", dev, err, strings.TrimSpace(fOut))
	}
	if !nicPoliceMatches(fOut, rate) {
		return fmt.Errorf("网卡入向限速未按配置生效（%s）：期望 police rate≈%s；实际：%s",
			dev, rate, strings.TrimSpace(compactWhitespace(fOut)))
	}
	return nil
}

func resolveNICDevice(root, configured string) (string, error) {
	name := strings.TrimSpace(configured)
	if name != "" {
		if !resources.ValidateNICDeviceName(name) {
			return "", fmt.Errorf("device 无效: %q", name)
		}
		return name, nil
	}
	return detectDefaultRouteDevice(root)
}

func detectDefaultRouteDevice(root string) (string, error) {
	out, err := RunCmdCapture(root, "ip", "-o", "route", "show", "default")
	if err != nil || strings.TrimSpace(out) == "" {
		out2, err2 := RunCmdCapture(root, "ip", "route", "show", "default")
		if err2 != nil {
			return "", fmt.Errorf("探测默认路由网卡失败：%v", firstErr(err, err2))
		}
		out = out2
	}
	dev := parseDefaultRouteDevice(out)
	if dev == "" {
		return "", fmt.Errorf("未能从默认路由解析业务口（请在 nic_* .params.device 显式填写）")
	}
	if !resources.ValidateNICDeviceName(dev) {
		return "", fmt.Errorf("探测到的网卡名无效: %q", dev)
	}
	return dev, nil
}

func parseDefaultRouteDevice(routeOut string) string {
	for _, line := range strings.Split(routeOut, "\n") {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "dev" {
				return fields[i+1]
			}
		}
	}
	return ""
}

func nicQdiscMatches(tcOut, wantRate string) bool {
	low := strings.ToLower(tcOut)
	if !strings.Contains(low, "tbf") {
		return false
	}
	return nicRateAppears(low, wantRate)
}

func nicIngressQdiscPresent(tcOut string) bool {
	return strings.Contains(strings.ToLower(tcOut), "ingress")
}

func nicPoliceMatches(filterOut, wantRate string) bool {
	low := strings.ToLower(filterOut)
	if !strings.Contains(low, "police") {
		return false
	}
	return nicRateAppears(low, wantRate)
}

func nicRateAppears(low, wantRate string) bool {
	wantNorm := normalizeRateToken(wantRate)
	if wantNorm == "" {
		return false
	}
	for _, m := range nicRateTokenNormRe.FindAllStringSubmatch(low, -1) {
		if len(m) >= 3 {
			got := normalizeRateToken(m[1] + m[2])
			if got == wantNorm {
				return true
			}
		}
	}
	// Fallback: raw substring (tc may print 3Mbit for 3mbit).
	return strings.Contains(low, strings.ToLower(wantRate))
}

func normalizeRateToken(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	m := nicRateTokenNormRe.FindStringSubmatch(raw)
	if len(m) < 3 {
		return ""
	}
	num := m[1]
	unit := m[2]
	switch unit {
	case "bps":
		unit = "bit"
	case "kbps":
		unit = "kbit"
	case "mbps":
		unit = "mbit"
	case "gbps":
		unit = "gbit"
	}
	return num + unit
}

func compactWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
