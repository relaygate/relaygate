import { useEffect, useRef, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { isLifecycleInfoLine, opsLineTone, opsToneClass } from "@/lib/opsLog"
import { cn } from "@/lib/utils"

function lineKind(line: string): "add" | "del" | "hunk" | "meta" | "ctx" {
  if (
    line.startsWith("+++") ||
    line.startsWith("---") ||
    line.startsWith("diff ") ||
    line.startsWith("index ")
  ) {
    return "meta"
  }
  if (line.startsWith("@@")) return "hunk"
  if (line.startsWith("+")) return "add"
  if (line.startsWith("-")) return "del"
  return "ctx"
}

/** Theme-aware diff line colors (readable in light and dark). */
const diffKindClass: Record<ReturnType<typeof lineKind>, string> = {
  add: "bg-emerald-500/10 text-emerald-800 dark:text-emerald-300",
  del: "bg-red-500/10 text-red-800 dark:text-red-300",
  hunk: "bg-sky-500/10 text-sky-800 dark:text-sky-300",
  meta: "text-muted-foreground",
  ctx: "text-foreground",
}

/** RelayGate change-summary lines: `  + server`, `  - rule`, `  ~ defaults`. */
function summaryKind(line: string): "add" | "del" | "chg" | "head" | "ctx" {
  const s = line.trimStart()
  if (/^(校验通过|Validation)/i.test(s) || isLifecycleInfoLine(s.trim())) {
    return "head"
  }
  // Require whitespace after the type keyword so FormatLifecycle rows like
  // `  - server-01 server=on …` are not treated as `  - server <name>` removals
  // (\b matches between "server" and "-01").
  if (/^\+\s+(server|rule)\s+/i.test(s)) return "add"
  if (/^-\s+(server|rule)\s+/i.test(s)) return "del"
  if (/^~\s+(server|rule|port|defaults|acl)\s+/i.test(s)) return "chg"
  return "ctx"
}

const summaryKindClass: Record<ReturnType<typeof summaryKind>, string> = {
  add: "bg-emerald-500/10 text-emerald-800 dark:text-emerald-300",
  del: "bg-red-500/10 text-red-800 dark:text-red-300",
  chg: "bg-amber-500/10 text-amber-900 dark:text-amber-300",
  head: "font-medium text-muted-foreground",
  ctx: "text-foreground",
}

function looksLikeDiff(lines: string[]): boolean {
  return lines.some((l) => /^(diff |@@|\+\+\+|---|[+-](?![+-]))/.test(l))
}

function looksLikeChangeSummary(lines: string[]): boolean {
  for (const line of lines) {
    const s = line.trimStart()
    // Same `\s+` (not `\b`) rule as summaryKind — see comment there.
    if (/^[+~-]\s+(server|rule|port|defaults|acl)\s+/i.test(s)) return true
  }
  return false
}

/** "no diff" one-liners from change-summary.txt. */
function isNoDiffLine(trimmed: string): boolean {
  if (!trimmed) return false
  if (/^无差异$|^无变更$|^No changes$/i.test(trimmed)) return true
  // （相对上次备份无 upstream/forward/defaults/security 差异） / （无 upstream/forward/defaults/security 差异）
  if (
    /^[（(]?(?:相对上次备份)?无(?:\s*server\/rule\/defaults\/acl\s*)?差异[）)]?$/i.test(
      trimmed,
    )
  ) {
    return true
  }
  return false
}

/**
 * Strip redundant headers from change-summary text
 * ("相对备份 …" / "变更摘要:" etc.) and normalize empty-diff lines.
 */
export function stripChangeSummaryNoise(
  text: string,
  opts?: { noDiffLabel?: string },
): string {
  const noDiffLabel = opts?.noDiffLabel?.trim() || "无差异"
  return text
    .replace(/\r\n/g, "\n")
    .split("\n")
    .flatMap((line) => {
      const trimmed = line.trim()
      if (/^(相对备份|vs backup)\b/i.test(trimmed)) return []
      if (/^(变更摘要|Change summary)\s*:?\s*$/i.test(trimmed)) return []
      const labeled = trimmed.match(/^(?:变更摘要|Change summary)\s*:\s*(.*)$/i)
      if (labeled) {
        const rest = labeled[1]?.trim() ?? ""
        if (!rest) return []
        if (isNoDiffLine(rest)) return [noDiffLabel]
        return [rest]
      }
      if (isNoDiffLine(trimmed)) return [noDiffLabel]
      return [line]
    })
    .join("\n")
    .replace(/^\n+/, "")
    .replace(/\n+$/, "")
}

/**
 * Change-summary / unified-diff viewer.
 * Same pane chrome as OpsLogView: fixed h-56 viewport, in-panel scroll, auto-scroll.
 */
export function DiffView({
  value,
  placeholder,
  error,
  className,
  /** Lock viewport height (default): no collapse when empty, scroll when long. */
  fixedHeight = true,
}: {
  value?: string
  placeholder?: ReactNode
  error?: boolean
  className?: string
  fixedHeight?: boolean
}) {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)
  // Backend may return localized sentinels (e.g. 「尚无」/ "None") instead of "".
  const trimmed = value?.trim() ?? ""
  const text =
    !trimmed || trimmed === "尚无" || trimmed === "None"
      ? ""
      : stripChangeSummaryNoise(value ?? "", { noDiffLabel: t("changes.no_diff") })
  const empty = !text

  useEffect(() => {
    if (empty) return
    const el = scrollRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [text, empty])

  const paneSize = fixedHeight ? "h-56 min-h-56 max-h-56" : "min-h-56 max-h-[32rem]"
  const isNodePlaceholder = empty && placeholder != null && typeof placeholder !== "string"
  const lines = empty ? [] : text.replace(/\r\n/g, "\n").split("\n")
  const asDiff = !empty && looksLikeDiff(lines)
  const asSummary = !empty && !asDiff && looksLikeChangeSummary(lines)

  return (
    <div
      ref={scrollRef}
      className={cn(
        "overflow-y-auto thin-scrollbar rounded-md border border-border bg-muted/40 text-foreground",
        paneSize,
        error && "border-destructive/50 bg-destructive/5",
        empty && "flex border-dashed",
        empty &&
          (isNodePlaceholder
            ? "items-center justify-center bg-transparent p-0"
            : "items-center px-3 py-3 text-sm text-muted-foreground"),
        className,
      )}
    >
      {empty ? (
        (placeholder ?? "")
      ) : (
        <pre className="m-0 p-3 font-mono text-[12px] leading-[1.55]">
          {lines.map((line, i) => {
            const toneClass = asDiff
              ? diffKindClass[lineKind(line)]
              : asSummary
                ? summaryKindClass[summaryKind(line)]
                : opsToneClass[opsLineTone(line)]
            return (
              <div
                key={i}
                className={cn(
                  "whitespace-pre-wrap break-all",
                  toneClass,
                  error && "text-destructive",
                )}
              >
                {line || " "}
              </div>
            )
          })}
        </pre>
      )}
    </div>
  )
}
