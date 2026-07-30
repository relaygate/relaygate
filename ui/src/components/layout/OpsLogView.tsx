import type { ReactNode } from "react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { ChevronDownIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { tf } from "@/i18n"
import {
  opsLineTone,
  opsLogOutcome,
  opsToneClass,
  parseOpsLog,
  type OpsStep,
  type OpsStepStatus,
} from "@/lib/opsLog"
import { cn } from "@/lib/utils"

function stepBadgeVariant(
  status: OpsStepStatus,
): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case "fail":
      return "destructive"
    case "ok":
      return "default"
    case "warn":
      return "secondary"
    default:
      return "outline"
  }
}

function stepLabel(t: (k: string) => string, status: OpsStepStatus): string {
  switch (status) {
    case "ok":
      return t("ops.log_status_ok")
    case "fail":
      return t("ops.log_status_fail")
    case "warn":
      return t("ops.log_status_warn")
    case "start":
      return t("ops.log_status_start")
    default:
      return t("ops.log_status_info")
  }
}

function outcomeBadge(
  t: (k: string) => string,
  outcome: ReturnType<typeof opsLogOutcome>,
) {
  if (outcome === "fail") {
    return <Badge variant="destructive">{t("ops.log_outcome_fail")}</Badge>
  }
  if (outcome === "warn") {
    return (
      <Badge
        variant="secondary"
        className="border-amber-500/40 bg-amber-500/10 text-amber-950 dark:text-amber-200"
      >
        {t("ops.log_outcome_warn")}
      </Badge>
    )
  }
  if (outcome === "ok") {
    return (
      <Badge
        variant="outline"
        className="border-emerald-500/40 bg-emerald-500/10 text-emerald-900 dark:text-emerald-300"
      >
        {t("ops.log_outcome_ok")}
      </Badge>
    )
  }
  return null
}

function StepsList({ steps }: { steps: OpsStep[] }) {
  const { t } = useTranslation()
  if (!steps.length) return null
  return (
    <ul className="flex flex-col gap-1 border-b border-border/60 px-2.5 py-2">
      {steps.map((step) => (
        <li
          key={step.name}
          className="flex min-w-0 items-start gap-2 text-[12px] leading-snug"
        >
          <Badge
            variant={stepBadgeVariant(step.status)}
            className={cn(
              "mt-0.5 shrink-0 rounded-md",
              step.status === "warn" &&
                "border-amber-500/40 bg-amber-500/10 text-amber-950 dark:text-amber-200",
              step.status === "ok" &&
                "border-emerald-500/30 bg-emerald-500/10 text-emerald-900 dark:text-emerald-300",
            )}
          >
            {stepLabel(t, step.status)}
          </Badge>
          <div className="min-w-0 flex-1">
            <div className="font-medium text-foreground">{step.name}</div>
            {step.detail ? (
              <div className="mt-0.5 break-all text-muted-foreground">{step.detail}</div>
            ) : null}
          </div>
        </li>
      ))}
    </ul>
  )
}

function LifecycleBlock({
  count,
  rows,
}: {
  count: number
  rows: { name: string; server: string; validation: string; production: string }[]
}) {
  const { t } = useTranslation()
  return (
    <div className="border-b border-border/60 bg-muted/30 px-2.5 py-2">
      <div className="mb-1.5 flex items-center gap-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
        <span>{t("ops.log_lifecycle")}</span>
        <span className="rounded bg-muted px-1.5 py-px font-mono text-[10px] normal-case text-foreground/80">
          {tf("ops.log_lifecycle_count", count)}
        </span>
      </div>
      {rows.length === 0 ? (
        <p className="text-[12px] text-muted-foreground">{t("ops.log_lifecycle_empty")}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[20rem] border-collapse text-left text-[11px] leading-snug">
            <thead>
              <tr className="text-muted-foreground">
                <th className="pr-2 pb-1 font-medium">{t("ops.log_col_upstream")}</th>
                <th className="pr-2 pb-1 font-medium">server</th>
                <th className="pr-2 pb-1 font-medium">validation</th>
                <th className="pb-1 font-medium">production</th>
              </tr>
            </thead>
            <tbody className="font-mono text-foreground">
              {rows.map((row) => (
                <tr key={row.name} className="align-top">
                  <td className="pr-2 py-0.5 whitespace-nowrap">{row.name}</td>
                  <td className="pr-2 py-0.5 whitespace-nowrap">{row.server}</td>
                  <td className="pr-2 py-0.5 break-all">{row.validation}</td>
                  <td className="py-0.5 break-all">{row.production}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function DetailLog({ lines }: { lines: string[] }) {
  return (
    <pre className="m-0 p-0 font-mono text-[12px] leading-[1.55]">
      {lines.map((line, i) => (
        <div
          key={i}
          className={cn(
            "whitespace-pre-wrap break-all px-3 py-px",
            opsToneClass[opsLineTone(line)],
          )}
        >
          {line || " "}
        </div>
      ))}
    </pre>
  )
}

/**
 * Ops-tool output panel: structured steps + lifecycle matrix + detail log.
 * Accepts legacy plain-text `output` from `/api/ops/*` (backward compatible).
 */
export function OpsLogView({
  value,
  placeholder,
  className,
  fixedHeight = true,
}: {
  value?: string
  placeholder?: ReactNode
  className?: string
  fixedHeight?: boolean
}) {
  const { t } = useTranslation()
  const [detailsOpen, setDetailsOpen] = useState(true)

  const trimmed = value?.trim() ?? ""
  const empty = !trimmed || trimmed === "尚无" || trimmed === "None" || trimmed === "(no output)"

  const parsed = useMemo(
    () => (empty ? null : parseOpsLog(value ?? "")),
    [empty, value],
  )

  const shell = cn(
    "overflow-hidden rounded-md border border-border bg-muted/50 text-foreground",
    fixedHeight && "flex h-64 flex-col",
    !fixedHeight && "max-h-[32rem]",
    className,
  )

  if (empty || !parsed) {
    const isNode = placeholder != null && typeof placeholder !== "string"
    return (
      <div
        className={cn(
          shell,
          isNode
            ? "flex min-h-44 items-center justify-center border-dashed bg-transparent p-0"
            : "border-dashed px-3 py-3 text-sm text-muted-foreground",
        )}
      >
        {placeholder ?? ""}
      </div>
    )
  }

  const outcome = opsLogOutcome(parsed.steps, parsed.detailLines)
  const hasStructure = Boolean(parsed.lifecycle || parsed.steps.length > 0)

  return (
    <div className={shell}>
      <div className="flex shrink-0 items-center justify-between gap-2 border-b border-border/60 px-2.5 py-1.5">
        <span className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
          {t("ops.log_title")}
        </span>
        {outcomeBadge(t, outcome)}
      </div>

      {parsed.lifecycle ? (
        <LifecycleBlock count={parsed.lifecycle.count} rows={parsed.lifecycle.rows} />
      ) : null}

      {parsed.steps.length > 0 ? <StepsList steps={parsed.steps} /> : null}

      <div className="flex min-h-0 flex-1 flex-col">
        {hasStructure ? (
          <button
            type="button"
            className="flex shrink-0 items-center gap-1 px-2.5 py-1 text-[11px] text-muted-foreground hover:text-foreground"
            onClick={() => setDetailsOpen((v) => !v)}
            aria-expanded={detailsOpen}
          >
            <ChevronDownIcon
              className={cn("size-3.5 transition-transform", !detailsOpen && "-rotate-90")}
            />
            {t("ops.log_details")}
          </button>
        ) : null}
        {detailsOpen || !hasStructure ? (
          <ScrollArea className={cn("min-h-0 flex-1", fixedHeight && "h-full")}>
            <DetailLog lines={parsed.detailLines} />
          </ScrollArea>
        ) : null}
      </div>
    </div>
  )
}
