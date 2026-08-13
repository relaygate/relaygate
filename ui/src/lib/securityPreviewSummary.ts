import { policyById, type SecurityState } from "@/lib/securityPolicies"
import type { SecurityPreview } from "@/lib/types"

export type PreviewKv = { key: string; value: string }

export type ComponentPreviewSummary = {
  id: "kernel" | "firewall" | "gateway"
  enabled: boolean
  params: PreviewKv[]
  applyPathKey: string
}

export function parseKernelParams(content: string | undefined): PreviewKv[] {
  if (!content) return []
  const out: PreviewKv[] = []
  for (const line of content.split("\n")) {
    const t = line.trim()
    if (!t || t.startsWith("#")) continue
    const eq = t.indexOf("=")
    if (eq > 0) {
      out.push({ key: t.slice(0, eq).trim(), value: t.slice(eq + 1).trim() })
    }
  }
  return out
}

export function buildComponentSummaries(
  preview: SecurityPreview,
  policies: SecurityState,
  disabledLabel: string,
): ComponentPreviewSummary[] {
  const kernelPolicy = policyById(policies, "kernel_syn")
  const kernelEnabled = Boolean(preview.kernel?.enabled && kernelPolicy?.enabled)
  const kernelParams = kernelEnabled
    ? parseKernelParams(preview.kernel?.content)
    : [{ key: "kernel_syn", value: disabledLabel }]

  const allowlist = policyById(policies, "allowlist")
  const fwLimit = policyById(policies, "firewall_new_conn_limit")
  const udpLimit = policyById(policies, "udp_limit")
  const firewallParams: PreviewKv[] = []
  let firewallEnabled = false

  if (allowlist?.enabled) {
    firewallEnabled = true
    const deny = allowlist.params.deny ?? []
    const allow = allowlist.params.allow ?? []
    firewallParams.push({ key: "acl.deny", value: String(deny.length) })
    firewallParams.push({ key: "acl.allow", value: String(allow.length) })
    if (allow.length > 0) {
      firewallParams.push({ key: "acl.mode", value: "strict" })
    }
  } else {
    firewallParams.push({ key: "allowlist", value: disabledLabel })
  }

  if (fwLimit?.enabled) {
    firewallEnabled = true
    firewallParams.push({ key: "tcp_per_ip", value: fwLimit.params.tcp_per_ip ?? "—" })
    firewallParams.push({ key: "burst", value: String(fwLimit.params.burst ?? 0) })
  } else {
    firewallParams.push({ key: "firewall_new_conn_limit", value: disabledLabel })
  }

  if (udpLimit?.enabled) {
    firewallEnabled = true
    firewallParams.push({ key: "udp_pps_per_ip", value: udpLimit.params.udp_pps_per_ip ?? "—" })
    firewallParams.push({ key: "udp_burst", value: String(udpLimit.params.udp_burst ?? 0) })
  } else {
    firewallParams.push({ key: "udp_limit", value: disabledLabel })
  }

  const gatewayRate = policyById(policies, "gateway_new_conn_limit")
  const connLimit = policyById(policies, "conn_limit")
  const g = preview.gateway
  const gatewayParams: PreviewKv[] = []
  let gatewayEnabled = false

  if (connLimit?.enabled && g?.conn_limit_enabled) {
    gatewayEnabled = true
    gatewayParams.push({ key: "max_connections", value: String(g.max_connections) })
  } else {
    gatewayParams.push({ key: "conn_limit", value: disabledLabel })
  }

  if (gatewayRate?.enabled && g?.rate_limit_enabled) {
    gatewayEnabled = true
    gatewayParams.push({ key: "local_ratelimit", value: `${g.local_ratelimit_per_sec}/s` })
    gatewayParams.push({ key: "burst", value: String(g.local_ratelimit_burst) })
    if (g.listeners_with_rate_limit > 0) {
      gatewayParams.push({ key: "tcp_listeners", value: String(g.listeners_with_rate_limit) })
    }
  } else {
    gatewayParams.push({ key: "gateway_new_conn_limit", value: disabledLabel })
  }

  return [
    {
      id: "kernel",
      enabled: kernelEnabled,
      params: kernelParams,
      applyPathKey: "security.preview_apply_kernel",
    },
    {
      id: "firewall",
      enabled: firewallEnabled,
      params: firewallParams,
      applyPathKey: "security.preview_apply_firewall",
    },
    {
      id: "gateway",
      enabled: gatewayEnabled,
      params: gatewayParams,
      applyPathKey: "security.preview_apply_gateway",
    },
  ]
}
