/**
 * Unified security.policies[] model in resources.yaml.
 */

export type SecurityPolicyId =
  | "kernel_syn"
  | "firewall_new_conn_limit"
  | "gateway_new_conn_limit"
  | "conn_limit"
  | "allowlist"
  | "udp_limit"

export type SecurityPolicyType =
  | "kernel"
  | "new_conn_limit_firewall"
  | "new_conn_limit_gateway"
  | "conn_limit"
  | "allowlist"
  | "udp_limit"

export type PolicyParams = {
  deny?: string[]
  allow?: string[]
  tcp_per_ip?: string
  burst?: number
  per_sec?: number
  max_connections?: number
  udp_pps_per_ip?: string
  udp_burst?: number
  tcp_syncookies?: number
  tcp_max_syn_backlog?: number
  tcp_synack_retries?: number
  tcp_syn_retries?: number
  tcp_abort_on_overflow?: number
}

export type SecurityPolicy = {
  id: SecurityPolicyId
  type: SecurityPolicyType
  enabled: boolean
  attack_tags: string[]
  params: PolicyParams
}

export type SecurityState = {
  policies: SecurityPolicy[]
}

export const SECURITY_POLICY_IDS: SecurityPolicyId[] = [
  "kernel_syn",
  "firewall_new_conn_limit",
  "gateway_new_conn_limit",
  "conn_limit",
  "allowlist",
  "udp_limit",
]

const POLICY_META: Record<
  SecurityPolicyId,
  { type: SecurityPolicyType; attack_tags: string[] }
> = {
  kernel_syn: { type: "kernel", attack_tags: ["T1"] },
  firewall_new_conn_limit: { type: "new_conn_limit_firewall", attack_tags: ["T1", "T4"] },
  gateway_new_conn_limit: { type: "new_conn_limit_gateway", attack_tags: ["T1", "T4"] },
  conn_limit: { type: "conn_limit", attack_tags: ["T2"] },
  allowlist: { type: "allowlist", attack_tags: ["T5"] },
  udp_limit: { type: "udp_limit", attack_tags: ["T6"] },
}

const DEFAULT_PARAMS: Record<SecurityPolicyId, PolicyParams> = {
  kernel_syn: {
    tcp_syncookies: 1,
    tcp_max_syn_backlog: 8192,
    tcp_synack_retries: 2,
    tcp_syn_retries: 3,
    tcp_abort_on_overflow: 0,
  },
  firewall_new_conn_limit: { tcp_per_ip: "30/second", burst: 60 },
  gateway_new_conn_limit: { per_sec: 200, burst: 400 },
  conn_limit: { max_connections: 1024 },
  allowlist: { deny: [], allow: [] },
  udp_limit: { udp_pps_per_ip: "500/second", udp_burst: 1000 },
}

export function defaultSecurityState(): SecurityState {
  return {
    policies: SECURITY_POLICY_IDS.map((id) => ({
      id,
      type: POLICY_META[id].type,
      enabled: true,
      attack_tags: [...POLICY_META[id].attack_tags],
      params: { ...DEFAULT_PARAMS[id] },
    })),
  }
}

function stripComment(line: string): string {
  let inSingle = false
  let inDouble = false
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (c === "'" && !inDouble) inSingle = !inSingle
    else if (c === '"' && !inSingle) inDouble = !inDouble
    else if (c === "#" && !inSingle && !inDouble) return line.slice(0, i).trimEnd()
  }
  return line
}

function indentOf(line: string): number {
  const m = /^ */.exec(line)
  return m ? m[0].length : 0
}

function isBlankOrComment(line: string): boolean {
  const t = line.trim()
  return t === "" || t.startsWith("#")
}

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

function findBlock(
  lines: string[],
  from: number,
  to: number,
  key: string,
  expectedIndent: number,
): { keyLine: number; end: number } | null {
  const re = new RegExp(`^${" ".repeat(expectedIndent)}${escapeRe(key)}:\\s*(.*)$`)
  for (let i = from; i < to; i++) {
    const raw = lines[i]
    if (isBlankOrComment(raw)) continue
    const ind = indentOf(raw)
    if (ind < expectedIndent) return null
    if (ind > expectedIndent) continue
    if (!re.test(stripComment(raw))) continue
    let end = i + 1
    while (end < to) {
      const L = lines[end]
      if (isBlankOrComment(L)) {
        end++
        continue
      }
      if (indentOf(L) <= expectedIndent) break
      end++
    }
    return { keyLine: i, end }
  }
  return null
}

function readScalar(lines: string[], from: number, to: number, key: string, indent: number): string | null {
  const re = new RegExp(`^${" ".repeat(indent)}${escapeRe(key)}:\\s*(.*)$`)
  for (let i = from; i < to; i++) {
    const raw = stripComment(lines[i])
    if (isBlankOrComment(raw)) continue
    if (indentOf(raw) !== indent) continue
    const m = re.exec(raw)
    if (!m) continue
    let v = (m[1] ?? "").trim()
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
      v = v.slice(1, -1)
    }
    return v
  }
  return null
}

function readList(lines: string[], from: number, to: number, key: string, indent: number): string[] {
  const block = findBlock(lines, from, to, key, indent)
  if (!block) return []
  const out: string[] = []
  for (let i = block.keyLine + 1; i < block.end; i++) {
    const raw = stripComment(lines[i])
    const m = /^- (.+)$/.exec(raw.trim())
    if (m) out.push(m[1].trim())
  }
  return out
}

const LEGACY_NEW_CONN_ID = "new_conn_limit"

type ParsedPolicy = {
  id: string
  enabled: boolean
  attack_tags: string[]
  params: PolicyParams & Record<string, unknown>
}

function migrateLegacyNewConnLimit(byId: Map<string, ParsedPolicy>): void {
  const legacy = byId.get(LEGACY_NEW_CONN_ID)
  if (!legacy) return
  const nft =
    byId.get("firewall_new_conn_limit") ??
    ({
      id: "firewall_new_conn_limit",
      enabled: legacy.enabled,
      attack_tags: [...POLICY_META.firewall_new_conn_limit.attack_tags],
      params: { ...DEFAULT_PARAMS.firewall_new_conn_limit },
    } satisfies ParsedPolicy)
  const envoy =
    byId.get("gateway_new_conn_limit") ??
    ({
      id: "gateway_new_conn_limit",
      enabled: legacy.enabled,
      attack_tags: [...POLICY_META.gateway_new_conn_limit.attack_tags],
      params: { ...DEFAULT_PARAMS.gateway_new_conn_limit },
    } satisfies ParsedPolicy)
  if (legacy.params.tcp_per_ip) nft.params.tcp_per_ip = legacy.params.tcp_per_ip
  const legacyNftBurst = legacy.params.nft_burst as number | undefined
  const legacyEnvoyBurst = legacy.params.envoy_burst as number | undefined
  if (legacyNftBurst != null) nft.params.burst = legacyNftBurst
  if (legacy.params.per_sec != null) envoy.params.per_sec = legacy.params.per_sec
  if (legacyEnvoyBurst != null) envoy.params.burst = legacyEnvoyBurst
  nft.enabled = legacy.enabled
  envoy.enabled = legacy.enabled
  byId.set("firewall_new_conn_limit", nft)
  byId.set("gateway_new_conn_limit", envoy)
  byId.delete(LEGACY_NEW_CONN_ID)
}

function parseLegacyPolicyItem(lines: string[], from: number, to: number): ParsedPolicy | null {
  let id: string | null = null
  for (let i = from; i < to; i++) {
    const raw = stripComment(lines[i])
    const listId = /^ {4}- id:\s*(\S+)/.exec(raw)
    if (listId) {
      id = listId[1]
      break
    }
  }
  if (id !== LEGACY_NEW_CONN_ID) return null
  const enabledRaw = readScalar(lines, from, to, "enabled", 6)
  const enabled = enabledRaw == null ? true : enabledRaw === "true"
  const itemBody = from + 1
  const params: PolicyParams & Record<string, unknown> = {}
  const paramsBlock = findBlock(lines, itemBody, to, "params", 6)
  if (paramsBlock) {
    const pf = paramsBlock.keyLine + 1
    const pt = paramsBlock.end
    const tcp =
      readScalar(lines, pf, pt, "nft_tcp_new_conn_per_ip", 8) ?? readScalar(lines, pf, pt, "tcp_per_ip", 8)
    const nftBurst = readScalar(lines, pf, pt, "nft_tcp_burst", 8)
    const perSec = readScalar(lines, pf, pt, "envoy_per_sec", 8) ?? readScalar(lines, pf, pt, "per_sec", 8)
    const envoyBurst = readScalar(lines, pf, pt, "envoy_burst", 8)
    if (tcp) params.tcp_per_ip = tcp
    if (nftBurst != null) params.nft_burst = Number.parseInt(nftBurst, 10) || 0
    if (perSec != null) params.per_sec = Number.parseInt(perSec, 10) || 0
    if (envoyBurst != null) params.envoy_burst = Number.parseInt(envoyBurst, 10) || 0
  }
  return {
    id: LEGACY_NEW_CONN_ID,
    enabled,
    attack_tags: [...POLICY_META.firewall_new_conn_limit.attack_tags],
    params,
  }
}

function parsePolicyItem(lines: string[], from: number, to: number): ParsedPolicy | null {
  let id: SecurityPolicyId | null = null
  for (let i = from; i < to; i++) {
    const raw = stripComment(lines[i])
    const listId = /^ {4}- id:\s*(\S+)/.exec(raw)
    if (listId) {
      id = listId[1] as SecurityPolicyId
      break
    }
    const scalarId = /^ {6}id:\s*(\S+)/.exec(raw)
    if (scalarId) {
      id = scalarId[1] as SecurityPolicyId
      break
    }
  }
  if (!id || !SECURITY_POLICY_IDS.includes(id)) return null
  const enabledRaw = readScalar(lines, from, to, "enabled", 6)
  const enabled = enabledRaw == null ? true : enabledRaw === "true"
  const itemBody = from + 1
  const tagsBlock = findBlock(lines, itemBody, to, "attack_tags", 6)
  let attack_tags = [...POLICY_META[id].attack_tags]
  if (tagsBlock) {
    attack_tags = []
    for (let i = tagsBlock.keyLine + 1; i < tagsBlock.end; i++) {
      const raw = stripComment(lines[i])
      const m = /^- (.+)$/.exec(raw.trim())
      if (m) attack_tags.push(m[1].trim())
    }
  }
  const params: PolicyParams = { ...DEFAULT_PARAMS[id] }
  const paramsBlock = findBlock(lines, itemBody, to, "params", 6)
  if (paramsBlock) {
    const pf = paramsBlock.keyLine + 1
    const pt = paramsBlock.end
    const intField = (k: keyof PolicyParams, legacyKeys?: string[]) => {
      for (const key of legacyKeys ? [k, ...legacyKeys] : [k]) {
        const v = readScalar(lines, pf, pt, key, 8)
        if (v != null) {
          ;(params as Record<string, unknown>)[k] = Number.parseInt(v, 10) || 0
          return
        }
      }
    }
    const strField = (k: keyof PolicyParams, legacyKeys?: string[]) => {
      for (const key of legacyKeys ? [k, ...legacyKeys] : [k]) {
        const v = readScalar(lines, pf, pt, key, 8)
        if (v != null) {
          ;(params as Record<string, unknown>)[k] = v
          return
        }
      }
    }
    strField("tcp_per_ip", ["nft_tcp_new_conn_per_ip"])
    strField("udp_pps_per_ip")
    if (id === "firewall_new_conn_limit") intField("burst", ["nft_tcp_burst"])
    else if (id === "gateway_new_conn_limit") {
      intField("per_sec", ["envoy_per_sec"])
      const envoyBurst = readScalar(lines, pf, pt, "envoy_burst", 8)
      if (envoyBurst != null) params.burst = Number.parseInt(envoyBurst, 10) || 0
      else intField("burst")
    } else {
      intField("burst")
      intField("per_sec")
    }
    intField("max_connections")
    intField("udp_burst")
    intField("tcp_syncookies")
    intField("tcp_max_syn_backlog")
    intField("tcp_synack_retries")
    intField("tcp_syn_retries")
    intField("tcp_abort_on_overflow")
    const deny = readList(lines, pf, pt, "deny", 8)
    const allow = readList(lines, pf, pt, "allow", 8)
    if (deny.length) params.deny = deny
    if (allow.length) params.allow = allow
  }
  return { id, enabled, attack_tags, params }
}

/** Parse security.policies from resources.yaml. */
export function parseSecurityPolicies(content: string): SecurityState {
  const lines = content.split(/\r?\n/)
  const out = defaultSecurityState()
  const sec = findBlock(lines, 0, lines.length, "security", 0)
  if (!sec) return out
  const policies = findBlock(lines, sec.keyLine + 1, sec.end, "policies", 2)
  if (!policies) return out

  const parsed: ParsedPolicy[] = []
  for (let i = policies.keyLine + 1; i < policies.end; i++) {
    const raw = lines[i]
    if (isBlankOrComment(raw)) continue
    if (indentOf(raw) !== 4 || !/^ {4}- /.test(raw)) continue
    let end = i + 1
    while (end < policies.end) {
      const L = lines[end]
      if (isBlankOrComment(L)) {
        end++
        continue
      }
      if (indentOf(L) <= 4) break
      end++
    }
    const legacy = parseLegacyPolicyItem(lines, i, end)
    if (legacy) parsed.push(legacy)
    else {
      const item = parsePolicyItem(lines, i, end)
      if (item) parsed.push(item)
    }
    i = end - 1
  }
  if (parsed.length === 0) return out
  const byId = new Map(parsed.map((p) => [p.id, p]))
  migrateLegacyNewConnLimit(byId)
  return {
    policies: SECURITY_POLICY_IDS.map((id) => {
      const item = byId.get(id)
      if (!item) return out.policies.find((p) => p.id === id)!
      return {
        id,
        type: POLICY_META[id].type,
        enabled: item.enabled,
        attack_tags: item.attack_tags.length ? item.attack_tags : [...POLICY_META[id].attack_tags],
        params: { ...DEFAULT_PARAMS[id], ...item.params },
      }
    }),
  }
}

function yamlList(items: string[], indent: number): string[] {
  const pad = " ".repeat(indent)
  if (items.length === 0) return [`${pad}[]`]
  return items.map((v) => `${pad}- ${v}`)
}

function renderPolicy(p: SecurityPolicy): string[] {
  const lines = [
    `    - id: ${p.id}`,
    `      type: ${p.type}`,
    `      enabled: ${p.enabled ? "true" : "false"}`,
    `      attack_tags: [${p.attack_tags.join(", ")}]`,
    `      params:`,
  ]
  const par = p.params
  switch (p.id) {
    case "kernel_syn":
      lines.push(
        `        tcp_syncookies: ${par.tcp_syncookies ?? 1}`,
        `        tcp_max_syn_backlog: ${par.tcp_max_syn_backlog ?? 8192}`,
        `        tcp_synack_retries: ${par.tcp_synack_retries ?? 2}`,
        `        tcp_syn_retries: ${par.tcp_syn_retries ?? 3}`,
        `        tcp_abort_on_overflow: ${par.tcp_abort_on_overflow ?? 0}`,
      )
      break
    case "firewall_new_conn_limit":
      lines.push(
        `        tcp_per_ip: ${par.tcp_per_ip ?? "30/second"}`,
        `        burst: ${par.burst ?? 60}`,
      )
      break
    case "gateway_new_conn_limit":
      lines.push(`        per_sec: ${par.per_sec ?? 200}`, `        burst: ${par.burst ?? 400}`)
      break
    case "conn_limit":
      lines.push(`        max_connections: ${par.max_connections ?? 1024}`)
      break
    case "allowlist":
      lines.push(`        deny:`)
      lines.push(...yamlList(par.deny ?? [], 10))
      lines.push(`        allow:`)
      lines.push(...yamlList(par.allow ?? [], 10))
      break
    case "udp_limit":
      lines.push(
        `        udp_pps_per_ip: ${par.udp_pps_per_ip ?? "500/second"}`,
        `        udp_burst: ${par.udp_burst ?? 1000}`,
      )
      break
  }
  return lines
}

/** Replace or insert security.policies block. */
export function patchSecurityPolicies(content: string, next: SecurityState): string {
  const blockLines = ["security:", "  policies:", ...next.policies.flatMap(renderPolicy)]
  const lines = content.split(/\r?\n/)
  const sec = findBlock(lines, 0, lines.length, "security", 0)
  if (sec) {
    const nextLines = [...lines.slice(0, sec.keyLine), ...blockLines, ...lines.slice(sec.end)]
    return nextLines.join("\n")
  }
  return [...lines, "", ...blockLines].join("\n")
}

export function policyById(state: SecurityState, id: SecurityPolicyId): SecurityPolicy | undefined {
  return state.policies.find((p) => p.id === id)
}

export function cloneSecurityState(state: SecurityState): SecurityState {
  return {
    policies: state.policies.map((p) => ({
      ...p,
      attack_tags: [...p.attack_tags],
      params: {
        ...p.params,
        deny: p.params.deny ? [...p.params.deny] : undefined,
        allow: p.params.allow ? [...p.params.allow] : undefined,
      },
    })),
  }
}

function paramsEqual(a: PolicyParams, b: PolicyParams): boolean {
  const keys = new Set([
    ...Object.keys(a),
    ...Object.keys(b),
  ]) as Set<keyof PolicyParams>
  for (const key of keys) {
    const av = a[key]
    const bv = b[key]
    if (Array.isArray(av) && Array.isArray(bv)) {
      if (av.length !== bv.length) return false
      for (let i = 0; i < av.length; i++) {
        if (av[i] !== bv[i]) return false
      }
      continue
    }
    if (av !== bv) return false
  }
  return true
}

export function policiesEqual(a: SecurityState, b: SecurityState): boolean {
  for (const id of SECURITY_POLICY_IDS) {
    const pa = policyById(a, id)
    const pb = policyById(b, id)
    if (!pa || !pb) return false
    if (pa.enabled !== pb.enabled) return false
    if (!paramsEqual(pa.params, pb.params)) return false
  }
  return true
}

/** Pick first defined value among keys (snake_case API + legacy PascalCase). */
function pickRaw(obj: Record<string, unknown>, ...keys: string[]): unknown {
  for (const key of keys) {
    if (obj[key] !== undefined && obj[key] !== null) return obj[key]
  }
  return undefined
}

function pickStr(obj: Record<string, unknown>, ...keys: string[]): string | undefined {
  const v = pickRaw(obj, ...keys)
  if (v == null) return undefined
  const s = String(v).trim()
  return s === "" ? undefined : s
}

function pickNum(obj: Record<string, unknown>, ...keys: string[]): number | undefined {
  const v = pickRaw(obj, ...keys)
  if (v == null || v === "") return undefined
  const n = typeof v === "number" ? v : Number(v)
  return Number.isFinite(n) ? n : undefined
}

function pickBool(obj: Record<string, unknown>, ...keys: string[]): boolean | undefined {
  const v = pickRaw(obj, ...keys)
  if (typeof v === "boolean") return v
  return undefined
}

function pickStrList(obj: Record<string, unknown>, ...keys: string[]): string[] | undefined {
  const v = pickRaw(obj, ...keys)
  if (!Array.isArray(v)) return undefined
  return v.map((x) => String(x))
}

/** Parse policies[] from profile-apply API response. */
export function policiesFromMerge(raw: unknown[]): SecurityPolicy[] {
  const byId = new Map<string, ParsedPolicy>(
    defaultSecurityState().policies.map((p) => [
      p.id,
      { id: p.id, enabled: p.enabled, attack_tags: [...p.attack_tags], params: { ...p.params } },
    ]),
  )
  for (const item of raw) {
    const o = (item ?? {}) as Record<string, unknown>
    const id = String(pickRaw(o, "id", "ID") ?? "")
    if (!SECURITY_POLICY_IDS.includes(id as SecurityPolicyId) && id !== LEGACY_NEW_CONN_ID) continue
    const params = (pickRaw(o, "params", "Params") ?? {}) as Record<string, unknown>
    const enabled = pickBool(o, "enabled", "Enabled")
    const tagsRaw = pickRaw(o, "attack_tags", "AttackTags")
    const parsed: ParsedPolicy = {
      id,
      enabled: enabled !== false,
      attack_tags: Array.isArray(tagsRaw) ? tagsRaw.map((x) => String(x)) : [],
      params: {
        deny: pickStrList(params, "deny", "Deny"),
        allow: pickStrList(params, "allow", "Allow"),
        tcp_per_ip: pickStr(params, "tcp_per_ip", "TCPPerIP", "nft_tcp_new_conn_per_ip"),
        per_sec: pickNum(params, "per_sec", "PerSec", "envoy_per_sec"),
        burst: pickNum(params, "burst", "Burst", "nft_tcp_burst"),
        max_connections: pickNum(params, "max_connections", "MaxConnections"),
        udp_pps_per_ip: pickStr(params, "udp_pps_per_ip", "UDPPPSPerIP"),
        udp_burst: pickNum(params, "udp_burst", "UDPBurst"),
        tcp_syncookies: pickNum(params, "tcp_syncookies", "TcpSyncookies"),
        tcp_max_syn_backlog: pickNum(params, "tcp_max_syn_backlog", "TcpMaxSynBacklog"),
        tcp_synack_retries: pickNum(params, "tcp_synack_retries", "TcpSynackRetries"),
        tcp_syn_retries: pickNum(params, "tcp_syn_retries", "TcpSynRetries"),
        tcp_abort_on_overflow: pickNum(params, "tcp_abort_on_overflow", "TcpAbortOnOverflow"),
      },
    }
    if (id === LEGACY_NEW_CONN_ID) {
      const nftBurst = pickNum(params, "nft_tcp_burst", "nft_burst")
      if (nftBurst != null) parsed.params.nft_burst = nftBurst
      const envoyBurst = pickNum(params, "envoy_burst")
      if (envoyBurst != null && envoyBurst > 0) parsed.params.envoy_burst = envoyBurst
    } else if (id === "gateway_new_conn_limit") {
      const envoyBurst = pickNum(params, "envoy_burst")
      if (envoyBurst != null && envoyBurst > 0) parsed.params.burst = envoyBurst
    }
    byId.set(id, parsed)
  }
  migrateLegacyNewConnLimit(byId)
  return SECURITY_POLICY_IDS.map((pid) => {
    const item = byId.get(pid)!
    return {
      id: pid,
      type: POLICY_META[pid].type,
      enabled: item.enabled,
      attack_tags: item.attack_tags.length ? item.attack_tags : [...POLICY_META[pid].attack_tags],
      params: { ...DEFAULT_PARAMS[pid], ...item.params },
    }
  })
}

/** Default select value for saved local policies. */
export const SECURITY_LOCAL_SOURCE = "__local__"

function isValidIPv4(ip: string): boolean {
  const parts = ip.split(".")
  if (parts.length !== 4) return false
  return parts.every((part) => {
    if (!/^(0|[1-9]\d*)$/.test(part)) return false
    const n = Number(part)
    return n >= 0 && n <= 255
  })
}

function isValidIPv6(ip: string): boolean {
  if (!/^[0-9a-fA-F:.]+$/.test(ip)) return false
  if ((ip.match(/::/g) ?? []).length > 1) return false
  const parts = ip.split(":")
  if (parts.length < 2 || parts.length > 8) return false
  for (const part of parts) {
    if (part === "") continue
    if (!/^[0-9a-fA-F]{1,4}$/.test(part)) return false
  }
  return true
}

/** True when raw is a valid IP or CIDR (matches backend NormalizeCIDR acceptance). */
export function isValidCIDR(raw: string): boolean {
  const s = raw.trim()
  if (!s) return false
  if (!s.includes("/")) {
    return isValidIPv4(s) || isValidIPv6(s)
  }
  const slash = s.lastIndexOf("/")
  const ip = s.slice(0, slash)
  const prefixStr = s.slice(slash + 1)
  if (!/^\d+$/.test(prefixStr)) return false
  const prefix = Number(prefixStr)
  if (isValidIPv4(ip)) return prefix >= 0 && prefix <= 32
  if (isValidIPv6(ip)) return prefix >= 0 && prefix <= 128
  return false
}

/** Split textarea content into trimmed non-empty lines. */
export function parseAllowlistLines(text: string): string[] {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
}

/** Trim, drop empty lines, dedupe, sort (save / blur normalization). */
export function normalizeAllowlistEntries(entries: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of entries) {
    const trimmed = raw.trim()
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    out.push(trimmed)
  }
  out.sort()
  return out
}

/** First invalid entry, or null when all valid. */
export function findInvalidAllowlistEntry(entries: string[]): string | null {
  for (const entry of entries) {
    if (!isValidCIDR(entry)) return entry
  }
  return null
}

export function validatePolicyParams(p: SecurityPolicy): string | null {
  if (p.id === "firewall_new_conn_limit" && !p.params.tcp_per_ip?.trim()) {
    return "tcp_per_ip"
  }
  if (p.id === "gateway_new_conn_limit") {
    const par = p.params
    if ((par.per_sec ?? 0) < 0 || (par.burst ?? 0) < 0) return "per_sec"
  }
  if (p.id === "conn_limit" && (p.params.max_connections ?? 0) < 1) {
    return "max_connections"
  }
  if (p.id === "udp_limit" && !p.params.udp_pps_per_ip?.trim()) {
    return "udp_pps_per_ip"
  }
  if (p.id === "kernel_syn") {
    const par = p.params
    const syncookies = par.tcp_syncookies ?? 1
    if (syncookies !== 0 && syncookies !== 1) return "tcp_syncookies"
    if ((par.tcp_max_syn_backlog ?? 0) < 1) return "tcp_max_syn_backlog"
    if ((par.tcp_synack_retries ?? 0) < 0) return "tcp_synack_retries"
    if ((par.tcp_syn_retries ?? 0) < 0) return "tcp_syn_retries"
    const abort = par.tcp_abort_on_overflow ?? 0
    if (abort !== 0 && abort !== 1) return "tcp_abort_on_overflow"
  }
  if (p.id === "allowlist") {
    const invalid =
      findInvalidAllowlistEntry(p.params.deny ?? []) ??
      findInvalidAllowlistEntry(p.params.allow ?? [])
    if (invalid) return `allowlist:${invalid}`
  }
  return null
}
