import {
  normalizeChangeEntries,
  normalizeEnvoy,
  normalizeLifecycle,
  normalizeProfiles,
  normalizeSecurityPreview,
  normalizeForward,
  normalizeForwards,
  normalizeUpstreams,
  normalizeSession,
  normalizeTraffic,
  type ApplyPreview,
  type ChangeDetail,
  type ChangeEntry,
  type LoginResponse,
  type OpsResult,
  type Profile,
  type RollbackPreview,
  type SecurityPreview,
  type SecurityProfileMerge,
  type Forward,
  type Upstream,
  type UpstreamLifecycle,
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

/**
 * Short actionable text for toast / dialogs.
 * Prefer API `error` (ApiError.message); never dump long CLI `output`.
 */
export function apiErrorDetail(err: unknown, fallback: string): string {
  if (err instanceof ApiError && err.message.trim()) {
    return err.message
  }
  return fallback
}

/**
 * Full ops CLI / log text for OpsLogView.
 * Prefer body.output, then short error, then fallback.
 */
export function apiErrorOutput(err: unknown, fallback: string): string {
  if (err instanceof ApiError) {
    const body = err.body as Record<string, unknown> | null
    if (body && typeof body.output === "string" && body.output.trim()) {
      return body.output
    }
    if (err.message.trim()) return err.message
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

/** snake_case API fields only (no PascalCase dual-read). */
function pickString(obj: Record<string, unknown>, key: string): string {
  const v = obj[key]
  return typeof v === "string" ? v : ""
}

function normalizeOpsResult(raw: unknown): OpsResult {
  const o = (raw ?? {}) as Record<string, unknown>
  return {
    ok: o.ok === true,
    output: (pickString(o, "output") || pickString(o, "body")) || undefined,
    error: pickString(o, "error") || undefined,
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

export async function getUpstreams(): Promise<{
  upstreams: Upstream[]
  lifecycle: Record<string, UpstreamLifecycle>
}> {
  const data = await api.get<unknown>("/api/upstreams")
  if (Array.isArray(data)) {
    return { upstreams: normalizeUpstreams(data), lifecycle: {} }
  }
  const obj = (data ?? {}) as Record<string, unknown>
  return {
    upstreams: normalizeUpstreams(obj.upstreams),
    lifecycle: normalizeLifecycle(obj.lifecycle),
  }
}

export async function createUpstream(body: {
  name: string
  address: string
  tcp?: { port: number } | null
  udp?: { port: number } | null
  enabled?: boolean
}): Promise<{ ok: boolean }> {
  const data = await api.post<unknown>("/api/upstreams", body)
  const o = (data ?? {}) as Record<string, unknown>
  return {
    ok: o.ok === true,
  }
}

export type BatchUpstreamItem = {
  name: string
  address: string
  tcp?: { port: number } | null
  udp?: { port: number } | null
  enabled?: boolean
  entries?: string[]
}

export type BatchUpstreamResult = {
  name: string
  ok: boolean
  error?: string
  forwards?: Forward[]
}

export async function createUpstreamsBatch(body: {
  upstreams: BatchUpstreamItem[]
  entries?: string[]
  enable_production?: boolean
}): Promise<{
  ok: boolean
  succeeded: number
  failed: number
  results: BatchUpstreamResult[]
}> {
  const normalize = (data: unknown) => {
    const o = (data ?? {}) as Record<string, unknown>
    const raw = o.results
    const results: BatchUpstreamResult[] = Array.isArray(raw)
      ? raw.map((item) => {
          const row = (item ?? {}) as Record<string, unknown>
          const rulesRaw = row.forwards
          return {
            name: String(row.name ?? ""),
            ok: row.ok === true,
            error:
              typeof row.error === "string"
                ? row.error
                : typeof row.Error === "string"
                  ? row.Error
                  : undefined,
            forwards: Array.isArray(rulesRaw)
              ? rulesRaw.map((r) => normalizeForward(r))
              : undefined,
          }
        })
      : []
    return {
      ok: o.ok === true,
      succeeded: Number(o.succeeded ?? 0),
      failed: Number(o.failed ?? 0),
      results,
    }
  }
  try {
    const data = await api.post<unknown>("/api/upstreams/batch", body)
    return normalize(data)
  } catch (err) {
    if (err instanceof ApiError && err.status === 400 && err.body) {
      const parsed = normalize(err.body)
      if (parsed.results.length) return parsed
    }
    throw err
  }
}

export async function updateUpstream(
  name: string,
  body: {
    address: string
    tcp?: { port: number } | null
    udp?: { port: number } | null
    enabled: boolean
  },
): Promise<{ ok: boolean; cascaded_forwards?: number }> {
  const data = await api.put<unknown>(`/api/upstreams/${encodeURIComponent(name)}`, body)
  const o = (data ?? {}) as Record<string, unknown>
  const cascaded = o.cascaded_forwards
  return {
    ok: o.ok === true,
    cascaded_forwards: typeof cascaded === "number" ? cascaded : undefined,
  }
}

export async function createForward(body: {
  upstream: string
  entry: "validation" | "production"
  protocols?: string[]
  enabled?: boolean
}): Promise<{ ok: boolean; forwards?: Forward[]; upstream?: string }> {
  const data = await api.post<unknown>("/api/forwards", body)
  const o = (data ?? {}) as Record<string, unknown>
  const forwardsRaw = o.forwards
  return {
    ok: o.ok === true,
    forwards: Array.isArray(forwardsRaw) ? forwardsRaw.map((r) => normalizeForward(r)) : undefined,
    upstream: typeof o.upstream === "string" ? o.upstream : undefined,
  }
}

export async function createUpstreamEntries(
  name: string,
  body: {
    entry: "validation" | "production"
    protocols?: string[]
    enabled?: boolean
  },
): Promise<{ ok: boolean; forwards?: Forward[] }> {
  const data = await api.post<unknown>(
    `/api/upstreams/${encodeURIComponent(name)}/entries`,
    body,
  )
  const o = (data ?? {}) as Record<string, unknown>
  const forwardsRaw = o.forwards
  return {
    ok: o.ok === true,
    forwards: Array.isArray(forwardsRaw) ? forwardsRaw.map((r) => normalizeForward(r)) : undefined,
  }
}

export async function deleteUpstream(name: string): Promise<{ removed_forwards?: number }> {
  const data = await api.delete<Record<string, unknown>>(`/api/upstreams/${encodeURIComponent(name)}`)
  const removed = data.removed_forwards
  return { removed_forwards: typeof removed === "number" ? removed : undefined }
}

export async function promoteUpstream(name: string): Promise<{ changed?: number }> {
  const data = await api.post<Record<string, unknown>>(
    `/api/upstreams/${encodeURIComponent(name)}/promote`,
  )
  const changed = data.changed
  return { changed: typeof changed === "number" ? changed : undefined }
}

export async function getForwards(): Promise<Forward[]> {
  const data = await api.get<unknown>("/api/forwards")
  return normalizeForwards(data)
}

export async function patchForward(name: string, enabled: boolean): Promise<void> {
  await api.patch(`/api/forwards/${encodeURIComponent(name)}`, { enabled })
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
    summary: pickString(data, "summary"),
    last_apply: normalizeLastApply(pickString(data, "last_apply")),
    needs_reload: data.needs_reload === true,
    needs_firewall: data.needs_firewall === true,
    apply_mode: normalizeApplyMode(data.apply_mode),
    confirm_phrase: pickString(data, "confirm_phrase") || undefined,
    bootstrap_migrated:
      data.bootstrap_migrated === true
        ? true
        : data.bootstrap_migrated === false
          ? false
          : undefined,
    needs_hard_reload: data.needs_hard_reload === true,
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
    stamp: pickString(data, "stamp") || stamp,
    summary: pickString(data, "summary"),
  }
}

export async function rollbackPreview(stamp: string): Promise<RollbackPreview> {
  const data = await api.post<Record<string, unknown>>("/api/rollback/preview", { stamp })
  return {
    stamp: pickString(data, "stamp") || stamp,
    summary: pickString(data, "summary"),
    found: data.found === true,
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

export async function getSecurityProfiles(): Promise<Profile[]> {
  const data = await api.get<unknown>("/api/security/profiles")
  return normalizeProfiles(data)
}

export async function previewSecurity(draft?: {
  access?: unknown
  protections?: unknown[]
}): Promise<SecurityPreview> {
  if (draft && (draft.access || (draft.protections && draft.protections.length > 0))) {
    return normalizeSecurityPreview(
      await api.post("/api/security/preview", {
        access: draft.access,
        protections: draft.protections,
      }),
    )
  }
  return normalizeSecurityPreview(await api.get("/api/security/preview"))
}

export async function mergeSecurityProfile(name: string): Promise<SecurityProfileMerge> {
  const data = await api.post<Record<string, unknown>>("/api/security/profile-apply", { name })
  return {
    name: pickString(data, "name") || name,
    description: pickString(data, "description"),
    scenario: pickString(data, "scenario") || undefined,
    access: data.access,
    protections: (data.protections as unknown[]) ?? [],
  }
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
  sync_pending?: boolean
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
  const nodesRaw = data.nodes
  const nodes: FleetNodeStatus[] = Array.isArray(nodesRaw)
    ? nodesRaw.map((n) => {
        const row = (n ?? {}) as Record<string, unknown>
        const status = String(row.status ?? "unknown")
        return {
          name: String(row.name ?? ""),
          role: row.role ? String(row.role) : undefined,
          status: status as FleetNodeStatus["status"],
          applied_version: row.applied_version ? String(row.applied_version) : undefined,
          published_version: row.published_version ? String(row.published_version) : undefined,
          last_heartbeat: row.last_heartbeat ? String(row.last_heartbeat) : undefined,
          sync_pending: row.sync_pending === true,
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

export type FleetJoinResponse = OpsResult & {
  name?: string
  token?: string
  bootstrap_hint?: string
  join_command?: string
  manual_hints?: string[]
}

export async function opsFleetJoin(body: {
  name: string
  control_url?: string
}): Promise<FleetJoinResponse> {
  try {
    const data = await api.post<Record<string, unknown>>("/api/ops/fleet/join", body)
    const hintsRaw = data.manual_hints
    return {
      ok: data.ok === true,
      error: pickString(data, "error") || undefined,
      name: data.name ? String(data.name) : undefined,
      token: data.token ? String(data.token) : undefined,
      bootstrap_hint: data.bootstrap_hint ? String(data.bootstrap_hint) : undefined,
      join_command: data.join_command ? String(data.join_command) : undefined,
      manual_hints: Array.isArray(hintsRaw) ? hintsRaw.map((h) => String(h)) : undefined,
    }
  } catch (err) {
    if (err instanceof ApiError && err.body && typeof err.body === "object") {
      const data = err.body as Record<string, unknown>
      return { ok: false, error: pickString(data, "error") || err.message }
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
      error: pickString(data, "error") || undefined,
      name: data.name ? String(data.name) : undefined,
      manual_hints: Array.isArray(hintsRaw) ? hintsRaw.map((h) => String(h)) : undefined,
    }
  } catch (err) {
    if (err instanceof ApiError && err.body && typeof err.body === "object") {
      const data = err.body as Record<string, unknown>
      return { ok: false, error: pickString(data, "error") || err.message }
    }
    throw err
  }
}

export type FleetSyncResponse = OpsResult & {
  name?: string
  sync_requested_at?: string
}

export async function opsFleetSync(body: {
  confirm: string
  name: string
}): Promise<FleetSyncResponse> {
  try {
    const data = await api.post<Record<string, unknown>>("/api/ops/fleet/sync", body)
    return {
      ok: data.ok === true,
      output: pickString(data, "output") || undefined,
      error: pickString(data, "error") || undefined,
      name: data.name ? String(data.name) : undefined,
      sync_requested_at: data.sync_requested_at ? String(data.sync_requested_at) : undefined,
    }
  } catch (err) {
    if (err instanceof ApiError && err.body && typeof err.body === "object") {
      const data = err.body as Record<string, unknown>
      return { ok: false, error: pickString(data, "error") || err.message }
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
    content: pickString(data, "content"),
    mtime: pickString(data, "mtime"),
    etag: pickString(data, "etag"),
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
    ok: data.ok === true,
    mtime: pickString(data, "mtime"),
    etag: pickString(data, "etag"),
    diff: pickString(data, "diff") || undefined,
    message: pickString(data, "message") || undefined,
  }
}

function normalizeValidateResult(raw: unknown): ConfigValidateResult {
  const o = (raw ?? {}) as Record<string, unknown>
  const errorsRaw = o.errors
  const errors: ConfigYAMLError[] = []
  if (Array.isArray(errorsRaw)) {
    for (const item of errorsRaw) {
      const e = (item ?? {}) as Record<string, unknown>
      const line = e.line
      errors.push({
        line: typeof line === "number" ? line : undefined,
        path: pickString(e, "path") || undefined,
        msg: (pickString(e, "msg") || pickString(e, "message")) || "error",
      })
    }
  }
  return {
    ok: o.ok === true,
    errors: errors.length ? errors : undefined,
    diff: pickString(o, "diff") || undefined,
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
  upstream: string
  entry: string
  protocol: string
  listen_port: number
  upstream_address: string
  upstream_port: number
  enabled: boolean
  forward_name: string
}

export async function getPortMap(): Promise<{
  gateway_public_ip: string
  rows: PortMapRow[]
}> {
  const data = await api.get<Record<string, unknown>>("/api/port-map")
  const rowsRaw = data.rows
  const rows: PortMapRow[] = Array.isArray(rowsRaw)
    ? rowsRaw.map((item) => {
        const o = (item ?? {}) as Record<string, unknown>
        return {
          upstream: String(o.upstream ?? ""),
          entry: String(o.entry ?? ""),
          protocol: String(o.protocol ?? ""),
          listen_port: Number(o.listen_port ?? 0) || 0,
          upstream_address: String(o.upstream_address ?? ""),
          upstream_port: Number(o.upstream_port ?? 0) || 0,
          enabled: o.enabled === true,
          forward_name: String(o.forward_name ?? ""),
        }
      })
    : []
  return {
    gateway_public_ip: String(data.gateway_public_ip ?? ""),
    rows,
  }
}

export async function exportPortMapCSV(): Promise<void> {
  await downloadBlob("/api/port-map?format=csv", "port-map.csv")
}
