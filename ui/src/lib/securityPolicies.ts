/**
 * security.access + security.protections[] model in resources.yaml.
 */

export type SecurityPolicyId =
  | "kernel_syn"
  | "nic_egress_shape"
  | "nic_ingress_police"
  | "firewall_new_conn_limit"
  | "gateway_new_conn_limit"
  | "gateway_conn_limit"
  | "firewall_udp_limit"

export type SecurityPolicyType = SecurityPolicyId

/** Product domain (kernel / nic / firewall / gateway). Not an execution component name. */
export type SecurityPolicyLayer = "kernel" | "nic" | "firewall" | "gateway"

/** Open params bag — keys/values shown raw (no per-field i18n). */
export type PolicyParams = Record<string, string | number | boolean>

export type SecurityPolicy = {
  id: SecurityPolicyId
  type: SecurityPolicyType
  enabled: boolean
  attack_tags: string[]
  params: PolicyParams
}

export type SecurityAccess = {
  enabled: boolean
  deny: string[]
  allow: string[]
}

export type SecurityState = {
  access: SecurityAccess
  protections: SecurityPolicy[]
}

export type SecurityPolicyCatalogEntry = {
  id: SecurityPolicyId
  type: SecurityPolicyType
  layer: SecurityPolicyLayer
  attack_tags: string[]
}

export const SECURITY_POLICY_IDS: SecurityPolicyId[] = [
  "kernel_syn",
  "nic_egress_shape",
  "nic_ingress_police",
  "firewall_new_conn_limit",
  "gateway_new_conn_limit",
  "gateway_conn_limit",
  "firewall_udp_limit",
]

/** Built-in catalog aligned with Go `SecurityPolicyCatalog` (type≡id; one instance per type). */
export const SECURITY_POLICY_CATALOG: SecurityPolicyCatalogEntry[] = [
  { id: "kernel_syn", type: "kernel_syn", layer: "kernel", attack_tags: ["T1"] },
  { id: "nic_egress_shape", type: "nic_egress_shape", layer: "nic", attack_tags: ["T7"] },
  { id: "nic_ingress_police", type: "nic_ingress_police", layer: "nic", attack_tags: ["T7"] },
  {
    id: "firewall_new_conn_limit",
    type: "firewall_new_conn_limit",
    layer: "firewall",
    attack_tags: ["T1", "T4"],
  },
  {
    id: "gateway_new_conn_limit",
    type: "gateway_new_conn_limit",
    layer: "gateway",
    attack_tags: ["T1", "T4"],
  },
  { id: "gateway_conn_limit", type: "gateway_conn_limit", layer: "gateway", attack_tags: ["T2"] },
  {
    id: "firewall_udp_limit",
    type: "firewall_udp_limit",
    layer: "firewall",
    attack_tags: ["T6"],
  },
]

const POLICY_META: Record<
  SecurityPolicyId,
  { type: SecurityPolicyType; layer: SecurityPolicyLayer; attack_tags: string[] }
> = Object.fromEntries(
  SECURITY_POLICY_CATALOG.map((e) => [e.id, { type: e.type, layer: e.layer, attack_tags: e.attack_tags }]),
) as Record<SecurityPolicyId, { type: SecurityPolicyType; layer: SecurityPolicyLayer; attack_tags: string[] }>

const DEFAULT_PARAMS: Record<SecurityPolicyId, PolicyParams> = {
  kernel_syn: {
    tcp_syncookies: 1,
    tcp_max_syn_backlog: 8192,
    tcp_synack_retries: 2,
    tcp_syn_retries: 3,
    tcp_abort_on_overflow: 0,
  },
  nic_egress_shape: { device: "", rate: "3mbit" },
  nic_ingress_police: { device: "", rate: "3mbit" },
  firewall_new_conn_limit: { tcp_per_ip: "30/second", burst: 60 },
  gateway_new_conn_limit: { per_sec: 200, burst: 400 },
  gateway_conn_limit: { max_connections: 1024 },
  firewall_udp_limit: { udp_pps_per_ip: "500/second", udp_burst: 1000 },
}

export function defaultProtection(id: SecurityPolicyId): SecurityPolicy {
  const meta = POLICY_META[id]
  return {
    id,
    type: meta.type,
    // Align with Go DefaultSecurity: nic shaping/police stay off unless operator/profile enables them.
    enabled: id !== "nic_egress_shape" && id !== "nic_ingress_police",
    attack_tags: [...meta.attack_tags],
    params: { ...DEFAULT_PARAMS[id] },
  }
}

export function defaultSecurityState(): SecurityState {
  return {
    access: { enabled: true, deny: [], allow: [] },
    protections: SECURITY_POLICY_IDS.map((id) => defaultProtection(id)),
  }
}

export function policyLayer(id: SecurityPolicyId): SecurityPolicyLayer {
  return POLICY_META[id].layer
}

/** Catalog entries not yet present in draft (one instance per type). */
export function availableCatalogEntries(state: SecurityState): SecurityPolicyCatalogEntry[] {
  const present = new Set(state.protections.map((p) => p.id))
  return SECURITY_POLICY_CATALOG.filter((e) => !present.has(e.id))
}

/** Insert a catalog protection with default params; no-op if type already present. */
export function addProtection(state: SecurityState, id: SecurityPolicyId): SecurityState {
  if (policyById(state, id)) return cloneSecurityState(state)
  const next = cloneSecurityState(state)
  next.protections.push(defaultProtection(id))
  next.protections = sortProtections(next.protections)
  return next
}

function sortProtections(list: SecurityPolicy[]): SecurityPolicy[] {
  const order = new Map(SECURITY_POLICY_IDS.map((id, i) => [id, i]))
  return [...list].sort((a, b) => (order.get(a.id) ?? 99) - (order.get(b.id) ?? 99))
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
  // Inline empty list: key: []
  const keyLine = stripComment(lines[block.keyLine])
  if (/:\s*\[\s*\]\s*$/.test(keyLine)) return []
  return out
}

type ParsedPolicy = {
  id: string
  enabled: boolean
  attack_tags: string[]
  params: PolicyParams & Record<string, unknown>
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
    for (let i = pf; i < pt; i++) {
      const raw = stripComment(lines[i])
      if (isBlankOrComment(raw)) continue
      if (indentOf(raw) !== 8) continue
      const m = /^ {8}([A-Za-z0-9_]+):\s*(.*)$/.exec(raw)
      if (!m) continue
      let v = (m[2] ?? "").trim()
      if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
        v = v.slice(1, -1)
      }
      if (v === "true" || v === "false") params[m[1]] = v === "true"
      else if (/^-?\d+$/.test(v)) params[m[1]] = Number.parseInt(v, 10)
      else params[m[1]] = v
    }
  }
  return { id, enabled, attack_tags, params }
}

function parseAccess(lines: string[], from: number, to: number): SecurityAccess {
  const out = defaultSecurityState().access
  const enabledRaw = readScalar(lines, from, to, "enabled", 4)
  if (enabledRaw != null) out.enabled = enabledRaw === "true"
  const deny = readList(lines, from, to, "deny", 4)
  const allow = readList(lines, from, to, "allow", 4)
  // Only overwrite when the key exists
  if (findBlock(lines, from, to, "deny", 4)) out.deny = deny
  if (findBlock(lines, from, to, "allow", 4)) out.allow = allow
  return out
}

/** Parse security.access + security.protections from resources.yaml. */
export function parseSecurityPolicies(content: string): SecurityState {
  const lines = content.split(/\r?\n/)
  const out = defaultSecurityState()
  const sec = findBlock(lines, 0, lines.length, "security", 0)
  if (!sec) return out

  const accessBlock = findBlock(lines, sec.keyLine + 1, sec.end, "access", 2)
  if (accessBlock) {
    out.access = parseAccess(lines, accessBlock.keyLine + 1, accessBlock.end)
  }

  const protections = findBlock(lines, sec.keyLine + 1, sec.end, "protections", 2)
  if (!protections) return out

  const parsed: ParsedPolicy[] = []
  for (let i = protections.keyLine + 1; i < protections.end; i++) {
    const raw = lines[i]
    if (isBlankOrComment(raw)) continue
    if (indentOf(raw) !== 4 || !/^ {4}- /.test(raw)) continue
    let end = i + 1
    while (end < protections.end) {
      const L = lines[end]
      if (isBlankOrComment(L)) {
        end++
        continue
      }
      if (indentOf(L) <= 4) break
      end++
    }
    const item = parsePolicyItem(lines, i, end)
    if (item) parsed.push(item)
    i = end - 1
  }
  // Preserve sparse protections[] (one instance per type); do not invent missing catalog ids.
  const byId = new Map(parsed.map((p) => [p.id as SecurityPolicyId, p]))
  const list = SECURITY_POLICY_IDS.filter((id) => byId.has(id)).map((id) => {
    const item = byId.get(id)!
    return {
      id,
      type: POLICY_META[id].type,
      enabled: item.enabled,
      attack_tags: item.attack_tags.length ? item.attack_tags : [...POLICY_META[id].attack_tags],
      params: { ...DEFAULT_PARAMS[id], ...item.params },
    }
  })
  return { access: out.access, protections: list }
}

function yamlList(items: string[], indent: number): string[] {
  const pad = " ".repeat(indent)
  if (items.length === 0) return [`${pad}[]`]
  return items.map((v) => `${pad}- ${v}`)
}

function renderAccess(a: SecurityAccess): string[] {
  return [
    `  access:`,
    `    enabled: ${a.enabled ? "true" : "false"}`,
    `    deny:`,
    ...yamlList(a.deny, 6),
    `    allow:`,
    ...yamlList(a.allow, 6),
  ]
}

function formatYamlScalar(v: string | number | boolean): string {
  if (typeof v === "boolean" || typeof v === "number") return String(v)
  const s = String(v)
  if (s === "" || /[:#{}[\],&*?|>!%@`]/.test(s) || /^\s|\s$/.test(s)) {
    return JSON.stringify(s)
  }
  return s
}

function renderPolicy(p: SecurityPolicy): string[] {
  const lines = [
    `    - id: ${p.id}`,
    `      type: ${p.type}`,
    `      enabled: ${p.enabled ? "true" : "false"}`,
    `      attack_tags: [${p.attack_tags.join(", ")}]`,
    `      params:`,
  ]
  const entries = Object.entries(p.params ?? {})
  if (entries.length === 0) {
    lines[lines.length - 1] = `      params: {}`
  } else {
    for (const [k, v] of entries) {
      lines.push(`        ${k}: ${formatYamlScalar(v)}`)
    }
  }
  return lines
}

/** Replace or insert security.access + security.protections block. */
export function patchSecurityPolicies(content: string, next: SecurityState): string {
  const blockLines = [
    "security:",
    ...renderAccess(next.access),
    `  protections:`,
    ...next.protections.flatMap(renderPolicy),
  ]
  const lines = content.split(/\r?\n/)
  const sec = findBlock(lines, 0, lines.length, "security", 0)
  if (sec) {
    const nextLines = [...lines.slice(0, sec.keyLine), ...blockLines, ...lines.slice(sec.end)]
    return nextLines.join("\n")
  }
  return [...lines, "", ...blockLines].join("\n")
}

export function policyById(state: SecurityState, id: SecurityPolicyId): SecurityPolicy | undefined {
  return state.protections.find((p) => p.id === id)
}

export function cloneSecurityState(state: SecurityState): SecurityState {
  return {
    access: {
      enabled: state.access.enabled,
      deny: [...state.access.deny],
      allow: [...state.access.allow],
    },
    protections: state.protections.map((p) => ({
      ...p,
      attack_tags: [...p.attack_tags],
      params: { ...p.params },
    })),
  }
}

function paramsEqual(a: PolicyParams, b: PolicyParams): boolean {
  const ak = Object.keys(a ?? {}).sort()
  const bk = Object.keys(b ?? {}).sort()
  if (ak.length !== bk.length) return false
  for (let i = 0; i < ak.length; i++) {
    if (ak[i] !== bk[i]) return false
    if (a[ak[i]] !== b[bk[i]]) return false
  }
  return true
}

function listEqual(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

export function policiesEqual(a: SecurityState, b: SecurityState): boolean {
  if (a.access.enabled !== b.access.enabled) return false
  if (!listEqual(a.access.deny, b.access.deny)) return false
  if (!listEqual(a.access.allow, b.access.allow)) return false
  for (const id of SECURITY_POLICY_IDS) {
    const pa = policyById(a, id)
    const pb = policyById(b, id)
    if (!pa && !pb) continue
    if (!pa || !pb) return false
    if (pa.enabled !== pb.enabled) return false
    if (!paramsEqual(pa.params, pb.params)) return false
  }
  return true
}

/** Read first defined snake_case API value. */
function pickRaw(obj: Record<string, unknown>, ...keys: string[]): unknown {
  for (const key of keys) {
    if (obj[key] !== undefined && obj[key] !== null) return obj[key]
  }
  return undefined
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

function coerceParams(raw: Record<string, unknown>): PolicyParams {
  const out: PolicyParams = {}
  for (const [k, v] of Object.entries(raw)) {
    if (typeof v === "string" || typeof v === "number" || typeof v === "boolean") out[k] = v
  }
  return out
}

/** Parse access + protections from profile-apply API response. */
export function securityFromMerge(raw: {
  access?: unknown
  protections?: unknown[]
}): SecurityState {
  const base = defaultSecurityState()
  if (raw.access && typeof raw.access === "object") {
    const o = raw.access as Record<string, unknown>
    const enabled = pickBool(o, "enabled")
    if (enabled !== undefined) base.access.enabled = enabled
    const deny = pickStrList(o, "deny")
    const allow = pickStrList(o, "allow")
    if (deny) base.access.deny = deny
    if (allow) base.access.allow = allow
  }
  const list = Array.isArray(raw.protections) ? raw.protections : []
  const byId = new Map<string, ParsedPolicy>(
    base.protections.map((p) => [
      p.id,
      { id: p.id, enabled: p.enabled, attack_tags: [...p.attack_tags], params: { ...p.params } },
    ]),
  )
  for (const item of list) {
    const o = (item ?? {}) as Record<string, unknown>
    const id = String(pickRaw(o, "id") ?? "")
    if (!SECURITY_POLICY_IDS.includes(id as SecurityPolicyId)) continue
    const paramsRaw = (pickRaw(o, "params") ?? {}) as Record<string, unknown>
    const enabled = pickBool(o, "enabled")
    const tagsRaw = pickRaw(o, "attack_tags")
    byId.set(id, {
      id,
      enabled: enabled !== false,
      attack_tags: Array.isArray(tagsRaw) ? tagsRaw.map((x) => String(x)) : [],
      params: coerceParams(paramsRaw),
    })
  }
  // Profile-apply / EnsureSecurityDefaults always yield the full catalog set.
  return {
    access: base.access,
    protections: SECURITY_POLICY_IDS.map((pid) => {
      const item = byId.get(pid)!
      return {
        id: pid,
        type: POLICY_META[pid].type,
        enabled: item.enabled,
        attack_tags: item.attack_tags.length ? item.attack_tags : [...POLICY_META[pid].attack_tags],
        params: { ...DEFAULT_PARAMS[pid], ...item.params },
      }
    }),
  }
}

/** @deprecated use securityFromMerge */
export function policiesFromMerge(raw: unknown[]): SecurityPolicy[] {
  return securityFromMerge({ protections: raw }).protections
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

export function validateAccess(access: SecurityAccess): string | null {
  const invalid =
    findInvalidAllowlistEntry(access.deny) ?? findInvalidAllowlistEntry(access.allow)
  if (invalid) return `access:${invalid}`
  return null
}

/** @deprecated Params are open key/value bags; UI does not field-validate. Always null. */
export function validatePolicyParams(_p: SecurityPolicy): string | null {
  return null
}

/** One editable params row (key/value as plain text; keys are not i18n'd). */
export type PolicyParamRow = { key: string; value: string }

export function formatParamValue(v: string | number | boolean): string {
  if (typeof v === "boolean") return v ? "true" : "false"
  return String(v)
}

/** Coerce a row value to a scalar (bool / int / float / string). */
export function parseParamValue(raw: string): string | number | boolean {
  const t = raw.trim()
  if (t === "true") return true
  if (t === "false") return false
  if (/^-?\d+$/.test(t)) return Number.parseInt(t, 10)
  if (/^-?\d+\.\d+$/.test(t)) return Number.parseFloat(t)
  return raw
}

export function paramsToRows(params: PolicyParams): PolicyParamRow[] {
  return Object.entries(params ?? {}).map(([key, value]) => ({
    key,
    value: formatParamValue(value),
  }))
}

/**
 * Build PolicyParams from rows. Empty keys are skipped.
 * On duplicate keys, keeps the first and reports `duplicateKey`.
 */
export function rowsToParams(rows: PolicyParamRow[]): {
  params: PolicyParams
  duplicateKey: string | null
} {
  const params: PolicyParams = {}
  const seen = new Set<string>()
  let duplicateKey: string | null = null
  for (const row of rows) {
    const key = row.key.trim()
    if (!key) continue
    if (seen.has(key)) {
      if (duplicateKey == null) duplicateKey = key
      continue
    }
    seen.add(key)
    params[key] = parseParamValue(row.value)
  }
  return { params, duplicateKey }
}
