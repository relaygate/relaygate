export interface ProtoPort {
  port: number
}

export interface Server {
  name: string
  address: string
  /** Present with port = TCP enabled; absent = off. */
  tcp?: ProtoPort | null
  /** Present with port = UDP enabled; absent = off. */
  udp?: ProtoPort | null
  enabled: boolean
}

export interface ServerLifecycle {
  name: string
  server_enabled: boolean
  validation_enabled: boolean
  production_enabled: boolean
  validation_ports: string[]
  production_ports: string[]
  validation_rule_count: number
  production_rule_count: number
  protocols: string[]
}

export interface Rule {
  name: string
  entry: string
  server: string
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

export interface RuleRateLimit {
  rule: string
  prefix: string
  hits_5m: number
}

export interface TrafficStatus {
  tcp_active_connections: number
  udp_active_sessions: number
  local_rate_limited_5m: number
  top_limited_rules?: RuleRateLimit[]
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

export function normalizeServer(raw: unknown): Server {
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

export function normalizeServers(raw: unknown): Server[] {
  if (!Array.isArray(raw)) return []
  return raw.map(normalizeServer)
}

export function normalizeLifecycle(raw: unknown): Record<string, ServerLifecycle> {
  if (!raw || typeof raw !== "object") return {}
  const out: Record<string, ServerLifecycle> = {}
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

function normalizeLifecycleEntry(raw: unknown): ServerLifecycle {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    name: asString(pick(o, "name", "Name")),
    server_enabled: asBool(pick(o, "server_enabled", "ServerEnabled")),
    validation_enabled: asBool(pick(o, "validation_enabled", "ValidationEnabled")),
    production_enabled: asBool(pick(o, "production_enabled", "ProductionEnabled")),
    validation_ports: asStringArray(pick(o, "validation_ports", "ValidationPorts")),
    production_ports: asStringArray(pick(o, "production_ports", "ProductionPorts")),
    validation_rule_count: asNumber(pick(o, "validation_rule_count", "ValidationRuleCount")),
    production_rule_count: asNumber(pick(o, "production_rule_count", "ProductionRuleCount")),
    protocols: asStringArray(pick(o, "protocols", "Protocols")),
  }
}

export function normalizeRule(raw: unknown): Rule {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    name: asString(pick(o, "name", "Name")),
    entry: asString(pick(o, "entry", "Entry")),
    server: asString(pick(o, "server", "Server")),
    protocol: asString(pick(o, "protocol", "Protocol")),
    listen_port: asNumber(pick(o, "listen_port", "ListenPort")),
    enabled: asBool(pick(o, "enabled", "Enabled")),
  }
}

export function normalizeRules(raw: unknown): Rule[] {
  if (!Array.isArray(raw)) return []
  return raw.map(normalizeRule)
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
  const topRaw = pick<unknown[]>(o, "top_limited_rules", "TopLimitedRules")
  const top: RuleRateLimit[] = Array.isArray(topRaw)
    ? topRaw.map((item) => {
        const r = (item ?? {}) as Record<string, unknown>
        return {
          rule: asString(pick(r, "rule", "Rule")),
          prefix: asString(pick(r, "prefix", "Prefix")),
          hits_5m: asNumber(pick(r, "hits_5m", "Hits5m")),
        }
      })
    : []
  return {
    tcp_active_connections: asNumber(pick(o, "tcp_active_connections", "TCPActiveConnections")),
    udp_active_sessions: asNumber(pick(o, "udp_active_sessions", "UDPActiveSessions")),
    local_rate_limited_5m: asNumber(pick(o, "local_rate_limited_5m", "LocalRateLimited5m")),
    top_limited_rules: top,
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
    }
  })
}
