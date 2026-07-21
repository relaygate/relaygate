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
  tcp_port: number
  udp_port: number
  health_check_port: number
  enabled?: boolean
}): Promise<{ ok: boolean; rules?: Rule[] }> {
  const data = await api.post<unknown>("/api/servers", body)
  const o = (data ?? {}) as Record<string, unknown>
  const rulesRaw = o.rules ?? o.Rules
  return {
    ok: o.ok === true || o.Ok === true,
    rules: Array.isArray(rulesRaw) ? rulesRaw.map((r) => normalizeRule(r)) : undefined,
  }
}

export async function updateServer(
  name: string,
  body: {
    address: string
    tcp_port: number
    udp_port: number
    health_check_port: number
    enabled: boolean
  },
): Promise<void> {
  await api.put(`/api/servers/${encodeURIComponent(name)}`, body)
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

export async function getApplyPreview(): Promise<ApplyPreview> {
  const data = await api.get<Record<string, unknown>>("/api/apply/preview")
  return {
    summary: pickString(data, "summary", "Summary"),
    last_apply: pickString(data, "last_apply", "LastApply"),
  }
}

export async function applyConfig(): Promise<OpsResult> {
  return normalizeOpsResult(await api.post("/api/apply"))
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

export async function rollback(stamp: string): Promise<OpsResult> {
  return normalizeOpsResult(
    await api.post("/api/rollback", { stamp, confirm: "ROLLBACK" }),
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
