/**
 * Line-oriented helpers for defaults rate-limit fields in resources.yaml.
 * Avoids a YAML dependency; preserves comments and unrelated keys.
 */

export type RateLimitDefaults = {
  tcpLocalRateLimitPerSec: number
  tcpLocalRateLimitBurst: number
  nftTcpNewConnPerIp: string
  nftUdpPpsPerIp: string
  nftTcpBurst: number
  nftUdpBurst: number
}

export const RATE_LIMIT_PRODUCT_DEFAULTS: RateLimitDefaults = {
  tcpLocalRateLimitPerSec: 200,
  tcpLocalRateLimitBurst: 400,
  nftTcpNewConnPerIp: "30/second",
  nftUdpPpsPerIp: "500/second",
  nftTcpBurst: 60,
  nftUdpBurst: 1000,
}

const YAML_KEY: Record<keyof RateLimitDefaults, string> = {
  tcpLocalRateLimitPerSec: "tcp_local_rate_limit_per_sec",
  tcpLocalRateLimitBurst: "tcp_local_rate_limit_burst",
  nftTcpNewConnPerIp: "tcp_new_conn_per_ip",
  nftUdpPpsPerIp: "udp_pps_per_ip",
  nftTcpBurst: "tcp_burst",
  nftUdpBurst: "udp_burst",
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

/** Inclusive key line, exclusive end of child lines. */
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

function readScalar(
  lines: string[],
  blockFrom: number,
  blockTo: number,
  key: string,
  expectedIndent: number,
): string | null {
  const re = new RegExp(`^${" ".repeat(expectedIndent)}${escapeRe(key)}:\\s*(.*)$`)
  for (let i = blockFrom; i < blockTo; i++) {
    const raw = lines[i]
    if (isBlankOrComment(raw)) continue
    const ind = indentOf(raw)
    if (ind < expectedIndent) return null
    if (ind > expectedIndent) continue
    const m = re.exec(stripComment(raw))
    if (!m) continue
    let v = (m[1] ?? "").trim()
    if (
      (v.startsWith('"') && v.endsWith('"')) ||
      (v.startsWith("'") && v.endsWith("'"))
    ) {
      v = v.slice(1, -1)
    }
    return v
  }
  return null
}

function setOrInsertScalar(
  lines: string[],
  blockKeyLine: number,
  blockEnd: number,
  key: string,
  expectedIndent: number,
  value: string,
): string[] {
  const pad = " ".repeat(expectedIndent)
  const re = new RegExp(`^${pad}${escapeRe(key)}:\\s*`)
  for (let i = blockKeyLine + 1; i < blockEnd; i++) {
    const raw = lines[i]
    if (isBlankOrComment(raw)) continue
    const ind = indentOf(raw)
    if (ind < expectedIndent) break
    if (ind > expectedIndent) continue
    if (!re.test(stripComment(raw))) continue
    const next = [...lines]
    next[i] = `${pad}${key}: ${value}`
    return next
  }
  const next = [...lines]
  next.splice(blockEnd, 0, `${pad}${key}: ${value}`)
  return next
}

function ensureNamedBlock(
  lines: string[],
  parentKeyLine: number,
  parentEnd: number,
  key: string,
  expectedIndent: number,
): { lines: string[]; keyLine: number; end: number } {
  const found = findBlock(lines, parentKeyLine + 1, parentEnd, key, expectedIndent)
  if (found) return { lines, keyLine: found.keyLine, end: found.end }
  const pad = " ".repeat(expectedIndent)
  const next = [...lines]
  next.splice(parentEnd, 0, `${pad}${key}:`)
  return { lines: next, keyLine: parentEnd, end: parentEnd + 1 }
}

function ensureDefaults(lines: string[]): { lines: string[]; keyLine: number; end: number } {
  const found = findBlock(lines, 0, lines.length, "defaults", 0)
  if (found) return { lines, keyLine: found.keyLine, end: found.end }
  const next = [...lines, "defaults:"]
  return { lines: next, keyLine: next.length - 1, end: next.length }
}

function parseIntSafe(raw: string | null, fallback: number): number {
  if (raw == null || raw === "") return fallback
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) ? n : fallback
}

/** Read rate-limit defaults from resources.yaml text. Missing keys use product defaults. */
export function parseRateLimitDefaults(content: string): RateLimitDefaults {
  const lines = content.split(/\r?\n/)
  const root = findBlock(lines, 0, lines.length, "defaults", 0)
  const out: RateLimitDefaults = { ...RATE_LIMIT_PRODUCT_DEFAULTS }
  if (!root) return out

  const tcpRate = readScalar(lines, root.keyLine + 1, root.end, YAML_KEY.tcpLocalRateLimitPerSec, 2)
  const tcpBurst = readScalar(lines, root.keyLine + 1, root.end, YAML_KEY.tcpLocalRateLimitBurst, 2)
  out.tcpLocalRateLimitPerSec = parseIntSafe(tcpRate, out.tcpLocalRateLimitPerSec)
  out.tcpLocalRateLimitBurst = parseIntSafe(tcpBurst, out.tcpLocalRateLimitBurst)

  const nft = findBlock(lines, root.keyLine + 1, root.end, "nftables", 2)
  if (!nft) return out

  const tcpNew = readScalar(lines, nft.keyLine + 1, nft.end, YAML_KEY.nftTcpNewConnPerIp, 4)
  const udpPps = readScalar(lines, nft.keyLine + 1, nft.end, YAML_KEY.nftUdpPpsPerIp, 4)
  const nftTcpBurst = readScalar(lines, nft.keyLine + 1, nft.end, YAML_KEY.nftTcpBurst, 4)
  const nftUdpBurst = readScalar(lines, nft.keyLine + 1, nft.end, YAML_KEY.nftUdpBurst, 4)
  if (tcpNew) out.nftTcpNewConnPerIp = tcpNew
  if (udpPps) out.nftUdpPpsPerIp = udpPps
  out.nftTcpBurst = parseIntSafe(nftTcpBurst, out.nftTcpBurst)
  out.nftUdpBurst = parseIntSafe(nftUdpBurst, out.nftUdpBurst)
  return out
}

/** Patch rate-limit fields in-place; creates defaults/nftables blocks if missing. */
export function patchRateLimitDefaults(content: string, next: RateLimitDefaults): string {
  let lines = content.split(/\r?\n/)

  const ensured = ensureDefaults(lines)
  lines = ensured.lines
  let defaults: { keyLine: number; end: number } = {
    keyLine: ensured.keyLine,
    end: ensured.end,
  }

  const tcpRate = String(next.tcpLocalRateLimitPerSec)
  const tcpBurst = String(next.tcpLocalRateLimitBurst)
  lines = setOrInsertScalar(
    lines,
    defaults.keyLine,
    defaults.end,
    YAML_KEY.tcpLocalRateLimitPerSec,
    2,
    tcpRate,
  )
  defaults = findBlock(lines, 0, lines.length, "defaults", 0)!
  lines = setOrInsertScalar(
    lines,
    defaults.keyLine,
    defaults.end,
    YAML_KEY.tcpLocalRateLimitBurst,
    2,
    tcpBurst,
  )

  defaults = findBlock(lines, 0, lines.length, "defaults", 0)!
  const nftEnsured = ensureNamedBlock(lines, defaults.keyLine, defaults.end, "nftables", 2)
  lines = nftEnsured.lines
  let nft: { keyLine: number; end: number } = {
    keyLine: nftEnsured.keyLine,
    end: nftEnsured.end,
  }

  const nftWrites: Array<[keyof RateLimitDefaults, string]> = [
    ["nftTcpNewConnPerIp", String(next.nftTcpNewConnPerIp).trim()],
    ["nftUdpPpsPerIp", String(next.nftUdpPpsPerIp).trim()],
    ["nftTcpBurst", String(next.nftTcpBurst)],
    ["nftUdpBurst", String(next.nftUdpBurst)],
  ]
  for (const [field, value] of nftWrites) {
    defaults = findBlock(lines, 0, lines.length, "defaults", 0)!
    nft = findBlock(lines, defaults.keyLine + 1, defaults.end, "nftables", 2)!
    lines = setOrInsertScalar(lines, nft.keyLine, nft.end, YAML_KEY[field], 4, value)
  }

  return lines.join("\n")
}

/** Returns the invalid field id, or null when ok. */
export function validateRateLimitDefaults(v: RateLimitDefaults): string | null {
  if (!Number.isInteger(v.tcpLocalRateLimitPerSec) || v.tcpLocalRateLimitPerSec < 0) {
    return "tcp_local_rate_limit_per_sec"
  }
  if (!Number.isInteger(v.tcpLocalRateLimitBurst) || v.tcpLocalRateLimitBurst < 0) {
    return "tcp_local_rate_limit_burst"
  }
  if (!Number.isInteger(v.nftTcpBurst) || v.nftTcpBurst < 0) {
    return "nftables.tcp_burst"
  }
  if (!Number.isInteger(v.nftUdpBurst) || v.nftUdpBurst < 0) {
    return "nftables.udp_burst"
  }
  if (!v.nftTcpNewConnPerIp.trim()) return "nftables.tcp_new_conn_per_ip"
  if (!v.nftUdpPpsPerIp.trim()) return "nftables.udp_pps_per_ip"
  return null
}
