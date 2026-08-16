/**
 * Ops / doctor / smoke log conventions (plain-text API `output`).
 *
 * Steps (preferred):
 *   ==> [stage] start
 *   ==> [stage] ok [(duration)]
 *   ==> [stage] FAIL: reason
 * WARN: … / FAIL: … / OK / TCP OK / TCP FAIL
 *
 * Lifecycle matrix (info, never error-colored):
 *   ## 入口状态: N 台上游
 *     · name server=on|off validation=… production=…
 *
 * Also accepts doctor sections `-- name --` + OK/FAIL/WARN.
 */

export type OpsTone = "error" | "warn" | "ok" | "meta" | "step" | "ctx"

export type OpsLifecycleRow = {
  name: string
  server: string
  validation: string
  production: string
}

export type OpsStepStatus = "ok" | "fail" | "warn" | "start" | "info"

export type OpsStep = {
  name: string
  status: OpsStepStatus
  detail?: string
}

export type ParsedOpsLog = {
  lifecycle: { count: number; rows: OpsLifecycleRow[] } | null
  steps: OpsStep[]
  detailLines: string[]
}

const LIFECYCLE_HEADER =
  /^(?:#{1,3}\s*)?入口状态\s*[:：]\s*(\d+)\s*台上游\s*$|^(?:#{1,3}\s*)?Entry status\s*:\s*(\d+)/i

const LIFECYCLE_ROW =
  /^[-·*]\s+(\S+)\s+server=(\S+)\s+validation=(.*?)\s+production=(.*)$/i

/** True for FormatLifecycle header / matrix rows — informational only. */
export function isLifecycleInfoLine(s: string): boolean {
  const t = s.trim()
  // No \b after CJK: JS word boundaries only apply to [A-Za-z0-9_].
  if (LIFECYCLE_HEADER.test(t)) return true
  if (LIFECYCLE_ROW.test(t)) return true
  return false
}

/**
 * Severity tone for a single ops log line.
 * Prefer explicit FAIL/ERROR/WARN prefixes over loose substring matches.
 */
export function opsLineTone(line: string): OpsTone {
  const s = line.trim()
  if (!s) return "ctx"
  if (isLifecycleInfoLine(s)) return "meta"

  if (/^#{1,3}\s|^={3,}|^-{3,}\s*$|^\*{3,}/.test(s)) return "meta"

  // Stage lines may embed severity: ==> [tcp] FAIL: … / ==> [x] ok
  if (/^==>\s*\[[^\]]+\]\s+FAIL\b/i.test(s) || /^==>\s*.*\bFAIL\b/.test(s)) {
    return "error"
  }
  if (/^==>\s*\[[^\]]+\]\s+ok\b/i.test(s)) return "ok"
  if (/^==>\s*\[[^\]]+\]\s+start\b/i.test(s) || /^==>\s/.test(s)) return "step"
  if (/^--\s+.+\s+--\s*$/.test(s)) return "step"

  // RelayGate ops: uppercase FAIL ("TCP FAIL", "FAIL:", "==> [x] FAIL").
  // Avoid bare /\bfail\b/i so "drain fail" / "healthcheck/fail" stay neutral.
  if (
    /\bFAIL\b/.test(s) ||
    /^ERROR\b/i.test(s) ||
    /\bERROR\s*:/.test(s) ||
    /\b(fatal|panic|critical|failed|failure|denied|refused)\b/i.test(s) ||
    /^\[\s*(error|err|fatal|crit)\s*\]/i.test(s)
  ) {
    return "error"
  }

  if (
    /^WARN\b/i.test(s) ||
    /\bWARN\s*:/.test(s) ||
    /\b(warning|caution)\b/i.test(s) ||
    /^\[\s*warn(ing)?\s*\]/i.test(s)
  ) {
    return "warn"
  }

  if (
    /^(OK|TCP OK|UDP OK|stats OK|label OK|smoke OK\b)/i.test(s) ||
    /\b(success|succeeded|healthy|pass(ed)?)\b/i.test(s) ||
    /^\[\s*(ok|pass)\s*\]/i.test(s)
  ) {
    return "ok"
  }

  if (/^INFO\b/i.test(s) || /^\[\s*info\s*\]/i.test(s)) return "meta"

  return "ctx"
}

export const opsToneClass: Record<OpsTone, string> = {
  error: "text-red-800 dark:text-red-300",
  warn: "text-amber-900 dark:text-amber-300",
  ok: "text-emerald-800 dark:text-emerald-300",
  meta: "text-muted-foreground",
  step: "font-medium text-sky-900 dark:text-sky-300",
  ctx: "text-foreground",
}

function parseLifecycleBlock(lines: string[]): {
  lifecycle: ParsedOpsLog["lifecycle"]
  rest: string[]
} {
  const rest: string[] = []
  let lifecycle: ParsedOpsLog["lifecycle"] = null
  let i = 0
  while (i < lines.length) {
    const trimmed = lines[i]!.trim()
    const hm = trimmed.match(LIFECYCLE_HEADER)
    if (!hm) {
      rest.push(lines[i]!)
      i++
      continue
    }
    const count = Number(hm[1] || hm[2] || 0)
    const rows: OpsLifecycleRow[] = []
    i++
    while (i < lines.length) {
      const rowTrim = lines[i]!.trim()
      if (!rowTrim) {
        i++
        break
      }
      const rm = rowTrim.match(LIFECYCLE_ROW)
      if (!rm) break
      rows.push({
        name: rm[1]!,
        server: rm[2]!,
        validation: rm[3]!.trim(),
        production: rm[4]!.trim(),
      })
      i++
    }
    lifecycle = { count: count || rows.length, rows }
  }
  return { lifecycle, rest }
}

const STAGE_LINE =
  /^==>\s*\[([^\]]+)\]\s+(start|ok|FAIL)(?:\s*\(([^)]*)\))?(?:\s*:\s*(.*))?$/i

const SECTION_HEAD = /^--\s+(.+?)\s+--\s*$/

function upsertStep(steps: OpsStep[], name: string, status: OpsStepStatus, detail?: string) {
  const idx = steps.findIndex((s) => s.name === name)
  if (idx >= 0) {
    const prev = steps[idx]!
    steps[idx] = {
      name,
      status,
      detail: detail ?? prev.detail,
    }
    return
  }
  steps.push({ name, status, detail })
}

function parseSteps(lines: string[]): OpsStep[] {
  const steps: OpsStep[] = []
  let section: string | null = null
  let sectionWarn = false
  let sectionFail: string | undefined

  const flushSection = (fallback: OpsStepStatus = "info") => {
    if (!section) return
    let status: OpsStepStatus = fallback
    let detail: string | undefined
    if (sectionFail) {
      status = "fail"
      detail = sectionFail
    } else if (sectionWarn) {
      status = "warn"
    }
    upsertStep(steps, section, status, detail)
    section = null
    sectionWarn = false
    sectionFail = undefined
  }

  for (const raw of lines) {
    const s = raw.trim()
    if (!s) continue

    const stage = s.match(STAGE_LINE)
    if (stage) {
      flushSection()
      const name = stage[1]!.trim()
      const kind = stage[2]!.toLowerCase()
      const dur = stage[3]?.trim()
      const reason = stage[4]?.trim()
      if (kind === "start") {
        upsertStep(steps, name, "start")
      } else if (kind === "fail") {
        upsertStep(steps, name, "fail", reason || dur)
      } else {
        upsertStep(steps, name, "ok", dur)
      }
      continue
    }

    const head = s.match(SECTION_HEAD)
    if (head) {
      flushSection("ok")
      section = head[1]!.trim()
      sectionWarn = false
      sectionFail = undefined
      continue
    }

    if (section) {
      if (/^FAIL\s*:/i.test(s)) {
        sectionFail = s.replace(/^FAIL\s*:\s*/i, "").trim()
        continue
      }
      if (/^(TCP|UDP)\s+FAIL\b/i.test(s) || /^FAIL\b/i.test(s)) {
        sectionFail = s.replace(/^FAIL\s*:\s*/i, "").trim() || s
        continue
      }
      if (/^WARN\b/i.test(s)) {
        sectionWarn = true
        continue
      }
      if (/^OK\s*$/i.test(s) || /^(TCP|UDP|stats|label)\s+OK\b/i.test(s)) {
        // keep ok unless already failed
        continue
      }
    }

    // Standalone probe results when no open section
    if (/^TCP\s+OK\b/i.test(s)) upsertStep(steps, "TCP", "ok")
    else if (/^TCP\s+FAIL\b/i.test(s)) upsertStep(steps, "TCP", "fail", s)
    else if (/^UDP\s+OK\b/i.test(s)) upsertStep(steps, "UDP", "ok")
    else if (/^smoke\s+OK\b/i.test(s)) upsertStep(steps, "smoke", "ok", s)
  }
  flushSection("info")
  return steps
}

/** Split ops plain-text output into lifecycle matrix, step summary, and detail lines. */
export function parseOpsLog(text: string): ParsedOpsLog {
  const rawLines = text.replace(/\r\n/g, "\n").split("\n")
  const { lifecycle, rest } = parseLifecycleBlock(rawLines)
  const steps = parseSteps(rest)
  return {
    lifecycle,
    steps,
    detailLines: rest,
  }
}

/** Overall outcome for badge strip. */
export function opsLogOutcome(
  steps: OpsStep[],
  detailLines: string[],
): "ok" | "fail" | "warn" | "neutral" {
  if (steps.some((s) => s.status === "fail")) return "fail"
  if (detailLines.some((l) => opsLineTone(l) === "error")) return "fail"
  if (steps.some((s) => s.status === "warn")) return "warn"
  if (detailLines.some((l) => opsLineTone(l) === "warn")) return "warn"
  if (steps.some((s) => s.status === "ok") || detailLines.some((l) => opsLineTone(l) === "ok")) {
    return "ok"
  }
  return "neutral"
}
