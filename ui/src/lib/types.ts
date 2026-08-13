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
  nic?: {
    enabled: boolean
    apply_script?: string
    device?: string
    rate?: string
    egress_enabled?: boolean
    ingress_enabled?: boolean
    ingress_device?: string
    ingress_rate?: string
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
    gateway_conn_limit_enabled: boolean
  }
  notes: string[]
}

export interface SecurityProfileMerge {
  name: string
  description: string
  scenario?: string
  access?: unknown
  protections: unknown[]
}

export interface OpsResult {
  ok?: boolean
  output?: string
  error?: string
}

/** Read a snake_case JSON field (API encoding is snake_case only; no PascalCase dual-read). */
function field<T>(obj: Record<string, unknown>, key: string): T | undefined {
  if (obj[key] !== undefined) return obj[key] as T
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
  const parseProto = (v: unknown): ProtoPort | undefined => {
    if (!v || typeof v !== "object") return undefined
    const port = asNumber(field((v as Record<string, unknown>), "port"))
    if (port < 1) return undefined
    return { port }
  }
  return {
    name: asString(field(o, "name")),
    address: asString(field(o, "address")),
    tcp: parseProto(field(o, "tcp")),
    udp: parseProto(field(o, "udp")),
    enabled: asBool(field(o, "enabled")),
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
    name: asString(field(o, "name")),
    upstream_enabled: asBool(field(o, "upstream_enabled")),
    validation_enabled: asBool(field(o, "validation_enabled")),
    production_enabled: asBool(field(o, "production_enabled")),
    validation_ports: asStringArray(field(o, "validation_ports")),
    production_ports: asStringArray(field(o, "production_ports")),
    validation_forward_count: asNumber(field(o, "validation_forward_count")),
    production_forward_count: asNumber(field(o, "production_forward_count")),
    protocols: asStringArray(field(o, "protocols")),
  }
}

export function normalizeForward(raw: unknown): Forward {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    name: asString(field(o, "name")),
    entry: asString(field(o, "entry")),
    upstream: asString(field(o, "upstream")),
    protocol: asString(field(o, "protocol")),
    listen_port: asNumber(field(o, "listen_port")),
    enabled: asBool(field(o, "enabled")),
  }
}

export function normalizeForwards(raw: unknown): Forward[] {
  if (!Array.isArray(raw)) return []
  return raw.map(normalizeForward)
}

export function normalizeACL(raw: unknown): ACL {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    deny: asStringArray(field(o, "deny")),
    allow: asStringArray(field(o, "allow")),
  }
}

export function normalizeEnvoy(raw: unknown): EnvoyStatus {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    ready: asBool(field(o, "ready")),
    ready_body: asString(field(o, "ready_body")),
    healthy_clusters: asNumber(field(o, "healthy_clusters")),
    cluster_lines: asStringArray(field(o, "cluster_lines")),
    error: asString(field(o, "error")) || undefined,
    stats: field<Record<string, string>>(o, "stats"),
  }
}

export function normalizeTraffic(raw: unknown): TrafficStatus {
  const o = (raw ?? {}) as Record<string, unknown>
  const topRaw = field<unknown[]>(o, "top_limited_forwards")
  const top: ForwardRateLimit[] = Array.isArray(topRaw)
    ? topRaw.map((item) => {
        const r = (item ?? {}) as Record<string, unknown>
        return {
          forward: asString(field(r, "forward")),
          prefix: asString(field(r, "prefix")),
          hits_5m: asNumber(field(r, "hits_5m")),
        }
      })
    : []
  return {
    tcp_active_connections: asNumber(field(o, "tcp_active_connections")),
    udp_active_sessions: asNumber(field(o, "udp_active_sessions")),
    local_rate_limited_5m: asNumber(field(o, "local_rate_limited_5m")),
    top_limited_forwards: top,
    error: asString(field(o, "error")) || undefined,
  }
}

export function normalizeSession(raw: unknown): Session {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    authenticated: asBool(field(o, "authenticated")),
    csrf: asString(field(o, "csrf")) || undefined,
    lang: asString(field(o, "lang")) || undefined,
    role: asString(field(o, "role")) || undefined,
    standby: asBool(field(o, "standby")),
    grafana_enabled: asBool(field(o, "grafana_enabled")),
  }
}

export function normalizeChangeEntries(raw: unknown): ChangeEntry[] {
  const arr = Array.isArray(raw)
    ? raw
    : field<unknown[]>(raw as Record<string, unknown>, "entries") ?? []
  return arr.map((item) => {
    const o = (item ?? {}) as Record<string, unknown>
    return {
      stamp: asString(field(o, "stamp")),
      summary: asString(field(o, "summary")),
      path: asString(field(o, "path")) || undefined,
    }
  })
}

export function normalizeProfiles(raw: unknown): Profile[] {
  const arr = Array.isArray(raw)
    ? raw
    : field<unknown[]>(raw as Record<string, unknown>, "profiles") ?? []
  return arr.map((item) => {
    const o = (item ?? {}) as Record<string, unknown>
    return {
      name: asString(field(o, "name")),
      description: asString(field(o, "description")),
      scenario: asString(field(o, "scenario")) || undefined,
    }
  })
}

export function normalizeSecurityPreview(raw: unknown): SecurityPreview {
  const o = (raw ?? {}) as Record<string, unknown>
  const orderRaw = field<unknown[]>(o, "execution_order") ?? []
  const surfacesRaw = field<unknown[]>(o, "surfaces") ?? []
  const kernelRaw = field<Record<string, unknown>>(o, "kernel")
  const nicRaw = field<Record<string, unknown>>(o, "nic")
  const firewallRaw = field<Record<string, unknown>>(o, "firewall")
  const gatewayRaw = field<Record<string, unknown>>(o, "gateway")
  return {
    execution_order: orderRaw.map((item) => {
      const r = (item ?? {}) as Record<string, unknown>
      return {
        order: asNumber(field(r, "order")),
        layer: asString(field(r, "layer")),
        component: asString(field(r, "component")),
        action: asString(field(r, "action")),
        policies: asStringArray(field(r, "policies")),
      }
    }),
    surfaces: surfacesRaw.map((item) => {
      const r = (item ?? {}) as Record<string, unknown>
      return {
        policy_id: asString(field(r, "policy_id")),
        layers: asStringArray(field(r, "layers")),
        apply_path: asString(field(r, "apply_path")),
        overlap_note: asString(field(r, "overlap_note")) || undefined,
      }
    }),
    kernel: kernelRaw
      ? {
          enabled: asBool(field(kernelRaw, "enabled")),
          apply_script: asString(field(kernelRaw, "apply_script")) || undefined,
          content: asString(field(kernelRaw, "content")) || undefined,
        }
      : undefined,
    nic: nicRaw
      ? {
          enabled: asBool(field(nicRaw, "enabled")),
          apply_script: asString(field(nicRaw, "apply_script")) || undefined,
          device: asString(field(nicRaw, "device")) || undefined,
          rate: asString(field(nicRaw, "rate")) || undefined,
          egress_enabled: asBool(field(nicRaw, "egress_enabled")),
          ingress_enabled: asBool(field(nicRaw, "ingress_enabled")),
          ingress_device: asString(field(nicRaw, "ingress_device")) || undefined,
          ingress_rate: asString(field(nicRaw, "ingress_rate")) || undefined,
        }
      : undefined,
    firewall: firewallRaw
      ? {
          forward_ports: asString(field(firewallRaw, "forward_ports")),
          gateway_excerpt: asString(field(firewallRaw, "gateway_excerpt")),
        }
      : undefined,
    gateway: gatewayRaw
      ? {
          max_connections: asNumber(field(gatewayRaw, "max_connections")),
          local_ratelimit_per_sec: asNumber(field(gatewayRaw, "local_ratelimit_per_sec")),
          local_ratelimit_burst: asNumber(field(gatewayRaw, "local_ratelimit_burst")),
          listeners_with_rate_limit: asNumber(field(gatewayRaw, "listeners_with_rate_limit")),
          enabled_tcp_forwards: asNumber(field(gatewayRaw, "enabled_tcp_forwards")),
          rate_limit_enabled: asBool(field(gatewayRaw, "rate_limit_enabled")),
          gateway_conn_limit_enabled: asBool(field(gatewayRaw, "gateway_conn_limit_enabled")),
        }
      : undefined,
    notes: asStringArray(field(o, "notes")),
  }
}
