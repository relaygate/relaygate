import { policyById, type SecurityState } from "@/lib/securityPolicies"
import type { SecurityPreview } from "@/lib/types"

export type PreviewKv = { key: string; value: string }

export type ComponentPreviewSummary = {
  id: "kernel" | "nic" | "firewall" | "gateway"
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
  state: SecurityState,
  disabledLabel: string,
): ComponentPreviewSummary[] {
  const kernelPolicy = policyById(state, "kernel_syn")
  const kernelEnabled = Boolean(preview.kernel?.enabled && kernelPolicy?.enabled)
  const kernelParams = kernelEnabled
    ? parseKernelParams(preview.kernel?.content)
    : [{ key: "kernel_syn", value: disabledLabel }]

  const egressPolicy = policyById(state, "nic_egress_shape")
  const ingressPolicy = policyById(state, "nic_ingress_police")
  const egressOn = Boolean(preview.nic?.egress_enabled && egressPolicy?.enabled)
  const ingressOn = Boolean(preview.nic?.ingress_enabled && ingressPolicy?.enabled)
  const nicEnabled = Boolean(preview.nic?.enabled && (egressOn || ingressOn))
  const nicParams: PreviewKv[] = []
  if (egressOn) {
    nicParams.push({
      key: "egress.device",
      value: String(preview.nic?.device ?? egressPolicy?.params.device ?? "auto"),
    })
    nicParams.push({
      key: "egress.rate",
      value: String(preview.nic?.rate ?? egressPolicy?.params.rate ?? "—"),
    })
  } else {
    nicParams.push({ key: "nic_egress_shape", value: disabledLabel })
  }
  if (ingressOn) {
    nicParams.push({
      key: "ingress.device",
      value: String(preview.nic?.ingress_device ?? ingressPolicy?.params.device ?? "auto"),
    })
    nicParams.push({
      key: "ingress.rate",
      value: String(preview.nic?.ingress_rate ?? ingressPolicy?.params.rate ?? "—"),
    })
  } else {
    nicParams.push({ key: "nic_ingress_police", value: disabledLabel })
  }

  const access = state.access
  const fwLimit = policyById(state, "firewall_new_conn_limit")
  const udpLimit = policyById(state, "firewall_udp_limit")
  const firewallParams: PreviewKv[] = []
  let firewallEnabled = false

  if (access?.enabled) {
    firewallEnabled = true
    const deny = access.deny ?? []
    const allow = access.allow ?? []
    firewallParams.push({ key: "acl.deny", value: String(deny.length) })
    firewallParams.push({ key: "acl.allow", value: String(allow.length) })
    if (allow.length > 0) {
      firewallParams.push({ key: "acl.mode", value: "strict" })
    }
  } else {
    firewallParams.push({ key: "access", value: disabledLabel })
  }

  if (fwLimit?.enabled) {
    firewallEnabled = true
    firewallParams.push({ key: "tcp_per_ip", value: String(fwLimit.params.tcp_per_ip ?? "—") })
    firewallParams.push({ key: "burst", value: String(fwLimit.params.burst ?? 0) })
  } else {
    firewallParams.push({ key: "firewall_new_conn_limit", value: disabledLabel })
  }

  if (udpLimit?.enabled) {
    firewallEnabled = true
    firewallParams.push({ key: "udp_pps_per_ip", value: String(udpLimit.params.udp_pps_per_ip ?? "—") })
    firewallParams.push({ key: "udp_burst", value: String(udpLimit.params.udp_burst ?? 0) })
  } else {
    firewallParams.push({ key: "firewall_udp_limit", value: disabledLabel })
  }

  const gatewayRate = policyById(state, "gateway_new_conn_limit")
  const connLimit = policyById(state, "gateway_conn_limit")
  const g = preview.gateway
  const gatewayParams: PreviewKv[] = []
  let gatewayEnabled = false

  if (connLimit?.enabled && g?.gateway_conn_limit_enabled) {
    gatewayEnabled = true
    gatewayParams.push({ key: "max_connections", value: String(g.max_connections) })
  } else {
    gatewayParams.push({ key: "gateway_conn_limit", value: disabledLabel })
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
      id: "nic",
      enabled: nicEnabled,
      params: nicParams,
      applyPathKey: "security.preview_apply_nic",
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
