export interface Server {
  name: string
  address: string
  tcp_port: number
  udp_port: number
  health_check_port: number
  enabled: boolean
}

export interface ServerLifecycle {
  name: string
  server_enabled: boolean
  canary_enabled: boolean
  production_enabled: boolean
  canary_ports: string[]
  production_ports: string[]
  canary_rule_count: number
  production_rule_count: number
}

export interface Rule {
  name: string
  kind: string
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
  return {
    name: asString(pick(o, "name", "Name")),
    address: asString(pick(o, "address", "Address")),
    tcp_port: asNumber(pick(o, "tcp_port", "TCPPort")),
    udp_port: asNumber(pick(o, "udp_port", "UDPPort")),
    health_check_port: asNumber(pick(o, "health_check_port", "HealthCheckPort")),
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
    canary_enabled: asBool(pick(o, "canary_enabled", "CanaryEnabled")),
    production_enabled: asBool(pick(o, "production_enabled", "ProductionEnabled")),
    canary_ports: asStringArray(pick(o, "canary_ports", "CanaryPorts")),
    production_ports: asStringArray(pick(o, "production_ports", "ProductionPorts")),
    canary_rule_count: asNumber(pick(o, "canary_rule_count", "CanaryRuleCount")),
    production_rule_count: asNumber(pick(o, "production_rule_count", "ProductionRuleCount")),
  }
}

export function normalizeRule(raw: unknown): Rule {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    name: asString(pick(o, "name", "Name")),
    kind: asString(pick(o, "kind", "Kind")),
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
