export interface ProtoPort {
  port: number
}

export interface Upstream {
  name: string
  address: string
  /** Present with port = TCP enabled; absent = off. */
  tcp?: ProtoPort | null
  /** Present with port = UDP enabled; absent = off. */
  udp?: ProtoPort | null
  enabled: boolean
}

export interface UpstreamLifecycle {
  name: string
  upstream_enabled: boolean
  validation_enabled: boolean
  production_enabled: boolean
  validation_ports: string[]
  production_ports: string[]
  validation_forward_count: number
  production_forward_count: number
  protocols: string[]
}

export interface Forward {
  name: string
  entry: string
  upstream: string
  protocol: string
  listen_port: number
  enabled: boolean
}

export interface ACL {
  deny: string[]
  allow: string[]
}

export interface Session {
  authenticated: boolean
  csrf?: string
  lang?: string
  role?: string
  standby?: boolean
  grafana_enabled?: boolean
}

export interface LoginResponse {
  ok: boolean
  csrf?: string
  lang?: string
  role?: string
  grafana_enabled?: boolean
}

export interface EnvoyStatus {
  ready: boolean
  ready_body?: string
  healthy_clusters: number
  cluster_lines?: string[]
  error?: string
  stats?: Record<string, string>
}

export interface ForwardRateLimit {
  forward: string
  prefix: string
  hits_5m: number
}

export interface TrafficStatus {
  tcp_active_connections: number
  udp_active_sessions: number
  local_rate_limited_5m: number
  top_limited_forwards?: ForwardRateLimit[]
  error?: string
}

export interface ApplyPreview {
  summary: string
  last_apply: string
  needs_reload: boolean
  needs_firewall: boolean
  apply_mode?: "hot" | "hard" | "none"
  confirm_phrase?: string
  bootstrap_migrated?: boolean
  needs_hard_reload?: boolean
}

export interface ChangeEntry {
  stamp: string
  summary: string
  path?: string
}

export interface ChangeDetail {
  stamp: string
  summary: string
}

export interface RollbackPreview {
  stamp: string
  summary: string
  found: boolean
}

export interface Profile {
  name: string
  description: string
  scenario?: string
}

export interface SecurityExecutionStep {
  order: number
  layer: string
  component: string
  action: string
  policies?: string[]
}

export interface SecurityPolicySurface {
  policy_id: string
  layers: string[]
  apply_path: string
  overlap_note?: string
}

export interface SecurityPreview {
  execution_order: SecurityExecutionStep[]
  surfaces: SecurityPolicySurface[]
  kernel?: {
    enabled: boolean
    apply_script?: string
    content?: string
  }
  firewall?: {
    forward_ports: string
    gateway_excerpt: string
  }
  gateway?: {
    max_connections: number
    local_ratelimit_per_sec: number
    local_ratelimit_burst: number
    listeners_with_rate_limit: number
    enabled_tcp_forwards: number
    rate_limit_enabled: boolean
    conn_limit_enabled: boolean
  }
  notes: string[]
}

export interface SecurityProfileMerge {
  name: string
  description: string
  scenario?: string
  policies: unknown[]
}

export interface OpsResult {
  ok?: boolean
  output?: string
  error?: string
}

function pick<T>(obj: Record<string, unknown>, ...keys: string[]): T | undefined {
  for (const key of keys) {
    if (obj[key] !== undefined) return obj[key] as T
  }
  return undefined
}

function asString(v: unknown): string {
  return typeof v === "string" ? v : String(v ?? "")
}

function asNumber(v: unknown): number {
  if (typeof v === "number") return v
  if (typeof v === "string") return Number(v) || 0
  return 0
}

function asBool(v: unknown): boolean {
  return v === true || v === "true" || v === 1
}

function asStringArray(v: unknown): string[] {
  if (!Array.isArray(v)) return []
  return v.map((x) => asString(x))
}

export function normalizeUpstream(raw: unknown): Upstream {
  const o = (raw ?? {}) as Record<string, unknown>
  const tcpRaw = pick(o, "tcp", "TCP")
  const udpRaw = pick(o, "udp", "UDP")
  const parseProto = (v: unknown): ProtoPort | undefined => {
    if (!v || typeof v !== "object") return undefined
    const port = asNumber(pick(v as Record<string, unknown>, "port", "Port"))
    if (port < 1) return undefined
    return { port }
  }
  return {
    name: asString(pick(o, "name", "Name")),
    address: asString(pick(o, "address", "Address")),
    tcp: parseProto(tcpRaw),
    udp: parseProto(udpRaw),
    enabled: asBool(pick(o, "enabled", "Enabled")),
  }
}

export function normalizeUpstreams(raw: unknown): Upstream[] {
  if (!Array.isArray(raw)) return []
  return raw.map(normalizeUpstream)
}

export function normalizeLifecycle(raw: unknown): Record<string, UpstreamLifecycle> {
  if (!raw || typeof raw !== "object") return {}
  const out: Record<string, UpstreamLifecycle> = {}
  if (Array.isArray(raw)) {
    for (const item of raw) {
      const lc = normalizeLifecycleEntry(item)
      out[lc.name] = lc
    }
    return out
  }
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    const lc = normalizeLifecycleEntry(value)
    out[lc.name || key] = lc
  }
  return out
}

function normalizeLifecycleEntry(raw: unknown): UpstreamLifecycle {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    name: asString(pick(o, "name", "Name")),
    upstream_enabled: asBool(pick(o, "upstream_enabled", "UpstreamEnabled", "server_enabled", "ServerEnabled")),
    validation_enabled: asBool(pick(o, "validation_enabled", "ValidationEnabled")),
    production_enabled: asBool(pick(o, "production_enabled", "ProductionEnabled")),
    validation_ports: asStringArray(pick(o, "validation_ports", "ValidationPorts")),
    production_ports: asStringArray(pick(o, "production_ports", "ProductionPorts")),
    validation_forward_count: asNumber(pick(o, "validation_forward_count", "ValidationForwardCount", "validation_rule_count", "ValidationRuleCount")),
    production_forward_count: asNumber(pick(o, "production_forward_count", "ProductionForwardCount", "production_rule_count", "ProductionRuleCount")),
    protocols: asStringArray(pick(o, "protocols", "Protocols")),
  }
}

export function normalizeForward(raw: unknown): Forward {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    name: asString(pick(o, "name", "Name")),
    entry: asString(pick(o, "entry", "Entry")),
    upstream: asString(pick(o, "upstream", "Upstream", "server", "Server")),
    protocol: asString(pick(o, "protocol", "Protocol")),
    listen_port: asNumber(pick(o, "listen_port", "ListenPort")),
    enabled: asBool(pick(o, "enabled", "Enabled")),
  }
}

export function normalizeForwards(raw: unknown): Forward[] {
  if (!Array.isArray(raw)) return []
  return raw.map(normalizeForward)
}

export function normalizeACL(raw: unknown): ACL {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    deny: asStringArray(pick(o, "deny", "Deny")),
    allow: asStringArray(pick(o, "allow", "Allow")),
  }
}

export function normalizeEnvoy(raw: unknown): EnvoyStatus {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    ready: asBool(pick(o, "ready", "Ready")),
    ready_body: asString(pick(o, "ready_body", "ReadyBody")),
    healthy_clusters: asNumber(pick(o, "healthy_clusters", "HealthyClusters")),
    cluster_lines: asStringArray(pick(o, "cluster_lines", "ClusterLines")),
    error: asString(pick(o, "error", "Error")) || undefined,
    stats: (pick(o, "stats", "Stats") as Record<string, string>) ?? undefined,
  }
}

export function normalizeTraffic(raw: unknown): TrafficStatus {
  const o = (raw ?? {}) as Record<string, unknown>
  const topRaw = pick<unknown[]>(o, "top_limited_forwards", "TopLimitedRules")
  const top: ForwardRateLimit[] = Array.isArray(topRaw)
    ? topRaw.map((item) => {
        const r = (item ?? {}) as Record<string, unknown>
        return {
          forward: asString(pick(r, "forward", "Forward", "rule", "Rule")),
          prefix: asString(pick(r, "prefix", "Prefix")),
          hits_5m: asNumber(pick(r, "hits_5m", "Hits5m")),
        }
      })
    : []
  return {
    tcp_active_connections: asNumber(pick(o, "tcp_active_connections", "TCPActiveConnections")),
    udp_active_sessions: asNumber(pick(o, "udp_active_sessions", "UDPActiveSessions")),
    local_rate_limited_5m: asNumber(pick(o, "local_rate_limited_5m", "LocalRateLimited5m")),
    top_limited_forwards: top,
    error: asString(pick(o, "error", "Error")) || undefined,
  }
}

export function normalizeSession(raw: unknown): Session {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    authenticated: asBool(pick(o, "authenticated", "Authenticated")),
    csrf: asString(pick(o, "csrf", "CSRF")) || undefined,
    lang: asString(pick(o, "lang", "Lang")) || undefined,
    role: asString(pick(o, "role", "Role")) || undefined,
    standby: asBool(pick(o, "standby", "Standby")),
    grafana_enabled: asBool(pick(o, "grafana_enabled", "GrafanaEnabled")),
  }
}

export function normalizeChangeEntries(raw: unknown): ChangeEntry[] {
  const arr = Array.isArray(raw) ? raw : pick<unknown[]>(raw as Record<string, unknown>, "entries", "Entries") ?? []
  return arr.map((item) => {
    const o = (item ?? {}) as Record<string, unknown>
    return {
      stamp: asString(pick(o, "stamp", "Stamp")),
      summary: asString(pick(o, "summary", "Summary")),
      path: asString(pick(o, "path", "Path")) || undefined,
    }
  })
}

export function normalizeProfiles(raw: unknown): Profile[] {
  const arr = Array.isArray(raw)
    ? raw
    : pick<unknown[]>(raw as Record<string, unknown>, "profiles", "Profiles") ?? []
  return arr.map((item) => {
    const o = (item ?? {}) as Record<string, unknown>
    return {
      name: asString(pick(o, "name", "Name")),
      description: asString(pick(o, "description", "Description")),
      scenario: asString(pick(o, "scenario", "Scenario")) || undefined,
    }
  })
}

export function normalizeSecurityPreview(raw: unknown): SecurityPreview {
  const o = (raw ?? {}) as Record<string, unknown>
  const orderRaw = pick<unknown[]>(o, "execution_order", "ExecutionOrder") ?? []
  const surfacesRaw = pick<unknown[]>(o, "surfaces", "Surfaces") ?? []
  const kernelRaw = pick(o, "kernel", "Kernel") as Record<string, unknown> | undefined
  const firewallRaw = pick(o, "firewall", "Firewall") as Record<string, unknown> | undefined
  const gatewayRaw = pick(o, "gateway", "Gateway") as Record<string, unknown> | undefined
  return {
    execution_order: orderRaw.map((item) => {
      const r = (item ?? {}) as Record<string, unknown>
      return {
        order: asNumber(pick(r, "order", "Order")),
        layer: asString(pick(r, "layer", "Layer")),
        component: asString(pick(r, "component", "Component")),
        action: asString(pick(r, "action", "Action")),
        policies: asStringArray(pick(r, "policies", "Policies")),
      }
    }),
    surfaces: surfacesRaw.map((item) => {
      const r = (item ?? {}) as Record<string, unknown>
      return {
        policy_id: asString(pick(r, "policy_id", "PolicyID")),
        layers: asStringArray(pick(r, "layers", "Layers")),
        apply_path: asString(pick(r, "apply_path", "ApplyPath")),
        overlap_note: asString(pick(r, "overlap_note", "OverlapNote")) || undefined,
      }
    }),
    kernel: kernelRaw
      ? {
          enabled: asBool(pick(kernelRaw, "enabled", "Enabled")),
          apply_script: asString(pick(kernelRaw, "apply_script", "ApplyScript")) || undefined,
          content: asString(pick(kernelRaw, "content", "Content")) || undefined,
        }
      : undefined,
    firewall: firewallRaw
      ? {
          forward_ports: asString(pick(firewallRaw, "forward_ports", "ForwardPorts")),
          gateway_excerpt: asString(pick(firewallRaw, "gateway_excerpt", "GatewayExcerpt")),
        }
      : undefined,
    gateway: gatewayRaw
      ? {
          max_connections: asNumber(pick(gatewayRaw, "max_connections", "MaxConnections")),
          local_ratelimit_per_sec: asNumber(
            pick(gatewayRaw, "local_ratelimit_per_sec", "LocalRateLimitPerSec"),
          ),
          local_ratelimit_burst: asNumber(
            pick(gatewayRaw, "local_ratelimit_burst", "LocalRateLimitBurst"),
          ),
          listeners_with_rate_limit: asNumber(
            pick(gatewayRaw, "listeners_with_rate_limit", "ListenersWithRateLimit"),
          ),
          enabled_tcp_forwards: asNumber(pick(gatewayRaw, "enabled_tcp_forwards", "EnabledTCPForwards")),
          rate_limit_enabled: asBool(pick(gatewayRaw, "rate_limit_enabled", "RateLimitEnabled")),
          conn_limit_enabled: asBool(pick(gatewayRaw, "conn_limit_enabled", "ConnLimitEnabled")),
        }
      : undefined,
    notes: asStringArray(pick(o, "notes", "Notes")),
  }
}
