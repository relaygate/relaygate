import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { ScrollArea } from "@/components/ui/scroll-area"
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

/** Legacy / canonical "no diff" one-liners from change-summary.txt. */
function isNoDiffLine(trimmed: string): boolean {
  if (!trimmed) return false
  if (/^无差异$|^无变更$|^No changes$/i.test(trimmed)) return true
  // （相对上次备份无 server/rule/defaults/acl 差异） / （无 server/rule/defaults/acl 差异）
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
 * Strip legacy redundant headers from change-summary text:
 * "相对备份 …" / "vs backup …" (incl. stamp lines) and bare
 * "变更摘要:" / "Change summary:" labels. Normalizes empty-diff lines
 * to a short label (default 无差异). Keeps historical files readable.
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
      // Legacy: "相对备份 20260720-091113" / "vs backup 20260720-091113"
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

export function DiffView({
  value,
  placeholder,
  error,
  className,
  /** Lock viewport height (ops logs): no collapse when empty, scroll when long. */
  fixedHeight = false,
}: {
  value?: string
  placeholder?: ReactNode
  error?: boolean
  className?: string
  fixedHeight?: boolean
}) {
  const { t } = useTranslation()
  // Backend may return localized sentinels (e.g. 「尚无」/ "None") instead of "".
  const trimmed = value?.trim() ?? ""
  const text =
    !trimmed || trimmed === "尚无" || trimmed === "None"
      ? ""
      : stripChangeSummaryNoise(value ?? "", { noDiffLabel: t("changes.no_diff") })
  const shell = cn(
    "rounded-md border border-border bg-muted/50 font-mono text-[12px] leading-[1.55] text-foreground",
    fixedHeight && "h-44",
    !fixedHeight && "max-h-[28rem]",
    error && "border-destructive/50 bg-destructive/5",
    className,
  )

  if (!text) {
    const isNode = placeholder != null && typeof placeholder !== "string"
    return (
      <div
        className={cn(
          shell,
          isNode
            ? "flex min-h-44 items-center justify-center border-dashed bg-transparent p-0"
            : "border-dashed px-3 py-3 text-muted-foreground",
          fixedHeight && !isNode && "flex items-start",
        )}
      >
        {placeholder ?? ""}
      </div>
    )
  }

  const lines = text.replace(/\r\n/g, "\n").split("\n")
  const asDiff = looksLikeDiff(lines)
  const asSummary = !asDiff && looksLikeChangeSummary(lines)

  return (
    <ScrollArea className={shell}>
      <pre className="m-0 p-0">
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
                "whitespace-pre-wrap break-all px-3 py-px",
                toneClass,
                error && "text-destructive",
              )}
            >
              {line || " "}
            </div>
          )
        })}
      </pre>
    </ScrollArea>
  )
}
