import {
  normalizeACL,
  normalizeChangeEntries,
  normalizeEnvoy,
  normalizeLifecycle,
  normalizeProfiles,
  normalizeRule,
  normalizeRules,
  normalizeServers,
  normalizeSession,
  normalizeTraffic,
  type ACL,
  type ApplyPreview,
  type ChangeDetail,
  type ChangeEntry,
  type LoginResponse,
  type OpsResult,
  type Profile,
  type RollbackPreview,
  type Rule,
  type Server,
  type ServerLifecycle,
  type Session,
  type EnvoyStatus,
  type TrafficStatus,
} from "@/lib/types"

export class ApiError extends Error {
  status: number
  body: unknown

  constructor(message: string, status: number, body?: unknown) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.body = body
  }
}

/** Prefer ops `output` from error body; fall back to message / fallback. */
export function apiErrorDetail(err: unknown, fallback: string): string {
  if (err instanceof ApiError) {
    const body = err.body as Record<string, unknown> | null
    if (body && typeof body.output === "string" && body.output.trim()) {
      return body.output
    }
    if (err.message) return err.message
  }
  return fallback
}

export function getCookie(name: string): string {
  const match = document.cookie.match(new RegExp(`(?:^|;\\s*)${name}=([^;]+)`))
  return match ? decodeURIComponent(match[1]) : ""
}

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE"

async function request<T>(
  method: Method,
  path: string,
  body?: unknown,
  init?: RequestInit,
): Promise<T> {
  const headers: Record<string, string> = {
    Accept: "application/json",
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json"
  }
  if (method !== "GET") {
    const csrf = getCookie("panel_csrf")
    if (csrf) headers["X-CSRF-Token"] = csrf
  }

  const res = await fetch(path, {
    method,
    credentials: "include",
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    ...init,
  })

  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (!res.ok) {
    const errObj = data as Record<string, unknown> | null
    const message =
      (errObj && typeof errObj.error === "string" && errObj.error) ||
      (typeof data === "string" ? data : res.statusText) ||
      "Request failed"
    throw new ApiError(message, res.status, data)
  }

  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  delete: <T>(path: string, body?: unknown) => request<T>("DELETE", path, body),
}

function pickString(obj: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const v = obj[key]
    if (typeof v === "string") return v
  }
  return ""
}

function normalizeOpsResult(raw: unknown): OpsResult {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    ok: o.ok === true || o.Ok === true,
    output: pickString(o, "output", "Output", "body", "Body") || undefined,
    error: pickString(o, "error", "Error") || undefined,
  }
}

export async function login(password: string): Promise<LoginResponse> {
  return api.post<LoginResponse>("/api/login", { password })
}

export async function logout(): Promise<void> {
  await api.post("/api/logout")
}

export async function getSession(): Promise<Session | null> {
  try {
    const data = await api.get<unknown>("/api/session")
    return normalizeSession(data)
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) return null
    throw err
  }
}

export async function setLang(lang: "en" | "zh-CN"): Promise<void> {
  await api.post("/api/lang", { lang })
}

export async function getServers(): Promise<{
  servers: Server[]
  lifecycle: Record<string, ServerLifecycle>
}> {
  const data = await api.get<unknown>("/api/servers")
  if (Array.isArray(data)) {
    return { servers: normalizeServers(data), lifecycle: {} }
  }
  const obj = (data ?? {}) as Record<string, unknown>
  return {
    servers: normalizeServers(obj.servers ?? obj.Servers),
    lifecycle: normalizeLifecycle(obj.lifecycle ?? obj.Lifecycle),
  }
}

export async function createServer(body: {
  name: string
  address: string
  tcp?: { port: number } | null
  udp?: { port: number } | null
  enabled?: boolean
}): Promise<{ ok: boolean }> {
  const data = await api.post<unknown>("/api/servers", body)
  const o = (data ?? {}) as Record<string, unknown>
  return {
    ok: o.ok === true || o.Ok === true,
  }
}

export type BatchServerItem = {
  name: string
  address: string
  tcp?: { port: number } | null
  udp?: { port: number } | null
  enabled?: boolean
  entries?: string[]
}

export type BatchServerResult = {
  name: string
  ok: boolean
  error?: string
  rules?: Rule[]
}

export async function createServersBatch(body: {
  servers: BatchServerItem[]
  entries?: string[]
  enable_production?: boolean
}): Promise<{
  ok: boolean
  succeeded: number
  failed: number
  results: BatchServerResult[]
}> {
  const normalize = (data: unknown) => {
    const o = (data ?? {}) as Record<string, unknown>
    const raw = o.results ?? o.Results
    const results: BatchServerResult[] = Array.isArray(raw)
      ? raw.map((item) => {
          const row = (item ?? {}) as Record<string, unknown>
          const rulesRaw = row.rules ?? row.Rules
          return {
            name: String(row.name ?? row.Name ?? ""),
            ok: row.ok === true || row.Ok === true,
            error:
              typeof row.error === "string"
                ? row.error
                : typeof row.Error === "string"
                  ? row.Error
                  : undefined,
            rules: Array.isArray(rulesRaw)
              ? rulesRaw.map((r) => normalizeRule(r))
              : undefined,
          }
        })
      : []
    return {
      ok: o.ok === true || o.Ok === true,
      succeeded: Number(o.succeeded ?? o.Succeeded ?? 0),
      failed: Number(o.failed ?? o.Failed ?? 0),
      results,
    }
  }
  try {
    const data = await api.post<unknown>("/api/servers/batch", body)
    return normalize(data)
  } catch (err) {
    if (err instanceof ApiError && err.status === 400 && err.body) {
      const parsed = normalize(err.body)
      if (parsed.results.length) return parsed
    }
    throw err
  }
}

export async function updateServer(
  name: string,
  body: {
    address: string
    tcp?: { port: number } | null
    udp?: { port: number } | null
    enabled: boolean
  },
): Promise<{ ok: boolean; cascaded_rules?: number }> {
  const data = await api.put<unknown>(`/api/servers/${encodeURIComponent(name)}`, body)
  const o = (data ?? {}) as Record<string, unknown>
  const cascaded = o.cascaded_rules ?? o.CascadedRules
  return {
    ok: o.ok === true || o.Ok === true,
    cascaded_rules: typeof cascaded === "number" ? cascaded : undefined,
  }
}

export async function createRule(body: {
  server: string
  entry: "validation" | "production"
  protocols?: string[]
  enabled?: boolean
}): Promise<{ ok: boolean; rules?: Rule[]; server?: string }> {
  const data = await api.post<unknown>("/api/rules", body)
  const o = (data ?? {}) as Record<string, unknown>
  const rulesRaw = o.rules ?? o.Rules
  return {
    ok: o.ok === true || o.Ok === true,
    rules: Array.isArray(rulesRaw) ? rulesRaw.map((r) => normalizeRule(r)) : undefined,
    server: typeof o.server === "string" ? o.server : undefined,
  }
}

export async function createServerEntries(
  name: string,
  body: {
    entry: "validation" | "production"
    protocols?: string[]
    enabled?: boolean
  },
): Promise<{ ok: boolean; rules?: Rule[] }> {
  const data = await api.post<unknown>(
    `/api/servers/${encodeURIComponent(name)}/entries`,
    body,
  )
  const o = (data ?? {}) as Record<string, unknown>
  const rulesRaw = o.rules ?? o.Rules
  return {
    ok: o.ok === true || o.Ok === true,
    rules: Array.isArray(rulesRaw) ? rulesRaw.map((r) => normalizeRule(r)) : undefined,
  }
}

export async function deleteServer(name: string): Promise<{ removed_rules?: number }> {
  const data = await api.delete<Record<string, unknown>>(`/api/servers/${encodeURIComponent(name)}`)
  const removed = data.removed_rules ?? data.RemovedRules
  return { removed_rules: typeof removed === "number" ? removed : undefined }
}

export async function promoteServer(name: string): Promise<{ changed?: number }> {
  const data = await api.post<Record<string, unknown>>(
    `/api/servers/${encodeURIComponent(name)}/promote`,
  )
  const changed = data.changed ?? data.Changed
  return { changed: typeof changed === "number" ? changed : undefined }
}

export async function getRules(): Promise<Rule[]> {
  const data = await api.get<unknown>("/api/rules")
  return normalizeRules(data)
}

export async function patchRule(name: string, enabled: boolean): Promise<void> {
  await api.patch(`/api/rules/${encodeURIComponent(name)}`, { enabled })
}

export async function getACL(): Promise<ACL> {
  const data = await api.get<unknown>("/api/acl")
  return normalizeACL(data)
}

export async function addACL(list: "deny" | "allow", cidr: string): Promise<ACL> {
  const data = await api.post<Record<string, unknown>>("/api/acl", { list, cidr })
  return normalizeACL(data.acl ?? data.ACL ?? data)
}

export async function removeACL(list: "deny" | "allow", cidr: string): Promise<ACL> {
  const data = await api.delete<Record<string, unknown>>("/api/acl", { list, cidr })
  return normalizeACL(data.acl ?? data.ACL ?? data)
}

/** Panel returns localized 「尚无」/ "None" when there is no last apply. */
function normalizeLastApply(raw: string): string {
  const v = raw.trim()
  if (!v || v === "尚无" || v === "None") return ""
  return raw
}

export async function getApplyPreview(): Promise<ApplyPreview> {
  const data = await api.get<Record<string, unknown>>("/api/apply/preview")
  return {
    summary: pickString(data, "summary", "Summary"),
    last_apply: normalizeLastApply(pickString(data, "last_apply", "LastApply")),
    needs_reload: data.needs_reload === true || data.NeedsReload === true,
    needs_firewall: data.needs_firewall === true || data.NeedsFirewall === true,
    apply_mode: normalizeApplyMode(data.apply_mode ?? data.ApplyMode),
    confirm_phrase: pickString(data, "confirm_phrase", "ConfirmPhrase") || undefined,
    bootstrap_migrated:
      data.bootstrap_migrated === true || data.BootstrapMigrated === true
        ? true
        : data.bootstrap_migrated === false || data.BootstrapMigrated === false
          ? false
          : undefined,
    needs_hard_reload: data.needs_hard_reload === true || data.NeedsHardReload === true,
  }
}

function normalizeApplyMode(raw: unknown): ApplyPreview["apply_mode"] {
  const v = String(raw ?? "").toLowerCase()
  if (v === "hot" || v === "hard" || v === "none") return v
  return undefined
}

export async function applyConfig(confirm: string): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/apply", { confirm }))
}

export async function applyFirewall(confirm: string): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/firewall/apply", { confirm }))
}

export async function getEnvoyStatus(): Promise<EnvoyStatus> {
  return normalizeEnvoy(await api.get("/api/status/envoy"))
}

export async function getTrafficStatus(): Promise<TrafficStatus> {
  return normalizeTraffic(await api.get("/api/status/traffic"))
}

export async function getChanges(limit = 50): Promise<ChangeEntry[]> {
  const data = await api.get<unknown>(`/api/changes?limit=${limit}`)
  return normalizeChangeEntries(data)
}

export async function getChangeDetail(stamp: string): Promise<ChangeDetail> {
  const data = await api.get<Record<string, unknown>>(`/api/changes/${encodeURIComponent(stamp)}`)
  return {
    stamp: pickString(data, "stamp", "Stamp") || stamp,
    summary: pickString(data, "summary", "Summary"),
  }
}

export async function rollbackPreview(stamp: string): Promise<RollbackPreview> {
  const data = await api.post<Record<string, unknown>>("/api/rollback/preview", { stamp })
  return {
    stamp: pickString(data, "stamp", "Stamp") || stamp,
    summary: pickString(data, "summary", "Summary"),
    found: data.found === true || data.Found === true,
  }
}

export async function rollback(stamp: string, confirm: string): Promise<OpsResult> {
  return normalizeOpsResult(
    await api.post("/api/rollback", { stamp, confirm }),
  )
}

export async function getProfiles(): Promise<Profile[]> {
  const data = await api.get<unknown>("/api/profiles")
  return normalizeProfiles(data)
}

export async function opsDoctor(): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/ops/doctor"))
}

export async function opsDrain(action: "status" | "fail" | "ok", confirm?: string): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/ops/drain", { action, confirm }))
}

export async function opsSmoke(host: string): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/ops/smoke", { host }))
}

export async function opsCanary(host: string): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/ops/canary", { host }))
}

export async function opsFirewallCheck(): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/ops/firewall-check"))
}

export async function opsProfilePreview(name: string): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/ops/profile-preview", { name }))
}

export async function opsProfileApply(name: string, confirm: string): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/ops/profile-apply", { name, confirm }))
}

export type FleetNode = {
  name: string
  role?: string
  applied_version?: string
  last_heartbeat?: string
}

export type FleetNodeStatus = {
  name: string
  role?: string
  status: "aligned" | "drifted" | "offline" | "unauthorized" | "unknown"
  applied_version?: string
  published_version?: string
  last_heartbeat?: string
}

export type FleetPublishMeta = {
  version?: string
  published_at?: string
}

export type FleetStatusOverview = {
  published_version: string
  published?: FleetPublishMeta
  nodes: FleetNodeStatus[]
}

export type FleetOverview = {
  nodes: FleetNode[]
  hints: string[]
  published: FleetPublishMeta | null
}

export async function getFleetOverview(): Promise<FleetOverview> {
  const data = await api.get<unknown>("/api/ops/fleet")
  if (!data || typeof data !== "object") {
    return { nodes: [], hints: [], published: null }
  }
  const o = data as Record<string, unknown>
  const nodes = Array.isArray(o.nodes)
    ? o.nodes.map((n) => {
        const row = n as Record<string, unknown>
        return {
          name: String(row.name ?? ""),
          role: row.role ? String(row.role) : undefined,
          applied_version: row.applied_version ? String(row.applied_version) : undefined,
          last_heartbeat: row.last_heartbeat ? String(row.last_heartbeat) : undefined,
        }
      })
    : []
  const hints = Array.isArray(o.hints) ? o.hints.map((h) => String(h)) : []
  const pub = o.published
  let published: FleetPublishMeta | null = null
  if (pub && typeof pub === "object") {
    const p = pub as Record<string, unknown>
    published = {
      version: p.version ? String(p.version) : undefined,
      published_at: p.published_at ? String(p.published_at) : undefined,
    }
  }
  return { nodes, hints, published }
}

export async function getFleetStatus(): Promise<FleetStatusOverview> {
  const data = await api.get<Record<string, unknown>>("/api/ops/fleet/status")
  const nodesRaw = data.nodes ?? data.Nodes
  const nodes: FleetNodeStatus[] = Array.isArray(nodesRaw)
    ? nodesRaw.map((n) => {
        const row = (n ?? {}) as Record<string, unknown>
        const status = String(row.status ?? row.Status ?? "unknown")
        return {
          name: String(row.name ?? row.Name ?? ""),
          role: row.role ? String(row.role) : undefined,
          status: status as FleetNodeStatus["status"],
          applied_version: row.applied_version ? String(row.applied_version) : undefined,
          published_version: row.published_version ? String(row.published_version) : undefined,
          last_heartbeat: row.last_heartbeat ? String(row.last_heartbeat) : undefined,
        }
      })
    : []
  const pub = data.published
  let published: FleetPublishMeta | undefined
  if (pub && typeof pub === "object") {
    const p = pub as Record<string, unknown>
    published = {
      version: p.version ? String(p.version) : undefined,
      published_at: p.published_at ? String(p.published_at) : undefined,
    }
  }
  return {
    published_version: String(data.published_version ?? published?.version ?? ""),
    published,
    nodes,
  }
}

export type FleetPublishResponse = OpsResult & {
  version?: string
}

export async function opsFleetPublish(confirm: string): Promise<FleetPublishResponse> {
  try {
    const data = await api.post<Record<string, unknown>>("/api/ops/fleet/publish", { confirm })
    return {
      ok: data.ok === true,
      output: pickString(data, "output", "Output") || undefined,
      error: pickString(data, "error", "Error") || undefined,
      version: data.version ? String(data.version) : undefined,
    }
  } catch (err) {
    if (err instanceof ApiError && err.body && typeof err.body === "object") {
      const data = err.body as Record<string, unknown>
      return {
        ok: false,
        output: pickString(data, "output", "Output") || undefined,
        error: pickString(data, "error", "Error") || err.message,
      }
    }
    throw err
  }
}

export type FleetJoinResponse = OpsResult & {
  name?: string
  token?: string
  bootstrap_hint?: string
  join_command?: string
  manual_hints?: string[]
}

export async function opsFleetJoin(body: {
  name: string
  primary_url?: string
}): Promise<FleetJoinResponse> {
  try {
    const data = await api.post<Record<string, unknown>>("/api/ops/fleet/join", body)
    const hintsRaw = data.manual_hints
    return {
      ok: data.ok === true,
      error: pickString(data, "error", "Error") || undefined,
      name: data.name ? String(data.name) : undefined,
      token: data.token ? String(data.token) : undefined,
      bootstrap_hint: data.bootstrap_hint ? String(data.bootstrap_hint) : undefined,
      join_command: data.join_command ? String(data.join_command) : undefined,
      manual_hints: Array.isArray(hintsRaw) ? hintsRaw.map((h) => String(h)) : undefined,
    }
  } catch (err) {
    if (err instanceof ApiError && err.body && typeof err.body === "object") {
      const data = err.body as Record<string, unknown>
      return { ok: false, error: pickString(data, "error", "Error") || err.message }
    }
    throw err
  }
}

export type FleetLeaveResponse = OpsResult & {
  name?: string
  manual_hints?: string[]
}

export async function opsFleetLeave(body: {
  confirm: string
  name: string
}): Promise<FleetLeaveResponse> {
  try {
    const data = await api.post<Record<string, unknown>>("/api/ops/fleet/leave", body)
    const hintsRaw = data.manual_hints
    return {
      ok: data.ok === true,
      error: pickString(data, "error", "Error") || undefined,
      name: data.name ? String(data.name) : undefined,
      manual_hints: Array.isArray(hintsRaw) ? hintsRaw.map((h) => String(h)) : undefined,
    }
  } catch (err) {
    if (err instanceof ApiError && err.body && typeof err.body === "object") {
      const data = err.body as Record<string, unknown>
      return { ok: false, error: pickString(data, "error", "Error") || err.message }
    }
    throw err
  }
}

export type ConfigResources = {
  content: string
  mtime: string
  etag: string
}

export type ConfigYAMLError = {
  line?: number
  path?: string
  msg: string
}

export type ConfigValidateResult = {
  ok: boolean
  errors?: ConfigYAMLError[]
  diff?: string
}

export type ConfigPutResult = {
  ok: boolean
  mtime: string
  etag: string
  diff?: string
  message?: string
}

export async function getConfigResources(): Promise<ConfigResources> {
  const data = await api.get<Record<string, unknown>>("/api/config/resources")
  return {
    content: pickString(data, "content", "Content"),
    mtime: pickString(data, "mtime", "Mtime"),
    etag: pickString(data, "etag", "ETag", "Etag"),
  }
}

export async function validateConfigResources(content: string): Promise<ConfigValidateResult> {
  const data = await api.post<Record<string, unknown>>("/api/config/resources/validate", { content })
  return normalizeValidateResult(data)
}

export async function putConfigResources(body: {
  content: string
  etag: string
  mtime?: string
}): Promise<ConfigPutResult> {
  const data = await api.put<Record<string, unknown>>("/api/config/resources", body)
  return {
    ok: data.ok === true || data.Ok === true,
    mtime: pickString(data, "mtime", "Mtime"),
    etag: pickString(data, "etag", "ETag", "Etag"),
    diff: pickString(data, "diff", "Diff") || undefined,
    message: pickString(data, "message", "Message") || undefined,
  }
}

function normalizeValidateResult(raw: unknown): ConfigValidateResult {
  const o = (raw ?? {}) as Record<string, unknown>
  const errorsRaw = o.errors ?? o.Errors
  const errors: ConfigYAMLError[] = []
  if (Array.isArray(errorsRaw)) {
    for (const item of errorsRaw) {
      const e = (item ?? {}) as Record<string, unknown>
      const line = e.line ?? e.Line
      errors.push({
        line: typeof line === "number" ? line : undefined,
        path: pickString(e, "path", "Path") || undefined,
        msg: pickString(e, "msg", "Msg", "message", "Message") || "error",
      })
    }
  }
  return {
    ok: o.ok === true || o.Ok === true,
    errors: errors.length ? errors : undefined,
    diff: pickString(o, "diff", "Diff") || undefined,
  }
}

async function downloadBlob(path: string, fallbackName: string): Promise<void> {
  const res = await fetch(path, { method: "GET", credentials: "include", headers: { Accept: "*/*" } })
  if (!res.ok) {
    const text = await res.text()
    let message = res.statusText
    try {
      const data = JSON.parse(text) as { error?: string }
      if (data.error) message = data.error
    } catch {
      if (text) message = text
    }
    throw new ApiError(message, res.status, text)
  }
  const blob = await res.blob()
  const dispo = res.headers.get("Content-Disposition") || ""
  const match = dispo.match(/filename="?([^";]+)"?/i)
  const name = match?.[1] || fallbackName
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

export async function exportConfigYAML(): Promise<void> {
  await downloadBlob("/api/config/export", "resources.yaml")
}

export async function exportConfigPack(): Promise<void> {
  await downloadBlob("/api/config/export?pack=zip", "relaygate-config.zip")
}

export type PortMapRow = {
  server: string
  entry: string
  protocol: string
  listen_port: number
  backend_address: string
  backend_port: number
  enabled: boolean
  rule_name: string
}

export async function getPortMap(): Promise<{
  gateway_public_ip: string
  rows: PortMapRow[]
}> {
  const data = await api.get<Record<string, unknown>>("/api/port-map")
  const rowsRaw = data.rows ?? data.Rows
  const rows: PortMapRow[] = Array.isArray(rowsRaw)
    ? rowsRaw.map((item) => {
        const o = (item ?? {}) as Record<string, unknown>
        return {
          server: String(o.server ?? o.Server ?? ""),
          entry: String(o.entry ?? o.Entry ?? ""),
          protocol: String(o.protocol ?? o.Protocol ?? ""),
          listen_port: Number(o.listen_port ?? o.ListenPort ?? 0) || 0,
          backend_address: String(o.backend_address ?? o.BackendAddress ?? ""),
          backend_port: Number(o.backend_port ?? o.BackendPort ?? 0) || 0,
          enabled: o.enabled === true || o.Enabled === true,
          rule_name: String(o.rule_name ?? o.RuleName ?? ""),
        }
      })
    : []
  return {
    gateway_public_ip: String(data.gateway_public_ip ?? data.GatewayPublicIP ?? ""),
    rows,
  }
}

export async function exportPortMapCSV(): Promise<void> {
  await downloadBlob("/api/port-map?format=csv", "port-map.csv")
}
