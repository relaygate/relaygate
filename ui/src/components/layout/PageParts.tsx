import type { ReactNode } from "react"

import { OpsLogView } from "@/components/layout/OpsLogView"
import { cn } from "@/lib/utils"

export function Page({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("page-enter flex flex-col gap-5", className)}>{children}</div>
}

export function PageHeader({
  title,
  hint,
  description,
  actions,
}: {
  title: string
  hint?: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <header className="flex flex-col gap-2 border-b border-border/60 pb-3 sm:flex-row sm:items-end sm:justify-between sm:gap-4">
      <div className="flex min-w-0 flex-col gap-1">
        <h1 className="text-lg font-semibold tracking-tight">{title}</h1>
        {description ? (
          <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>
        ) : hint ? (
          <p className="max-w-2xl text-sm text-muted-foreground">{hint}</p>
        ) : null}
        {description && hint ? (
          <p className="text-xs text-muted-foreground">{hint}</p>
        ) : null}
      </div>
      {actions ? (
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">{actions}</div>
      ) : null}
    </header>
  )
}

export function StatGrid({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
      {children}
    </div>
  )
}

export function StatBlock({
  label,
  value,
  detail,
  tone,
}: {
  label: string
  value: ReactNode
  detail?: string
  tone?: "ok" | "danger" | "default"
}) {
  return (
    <div className="flex flex-col gap-1 rounded-md border border-border/60 bg-card/50 px-3.5 py-2.5">
      <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <span
        className={cn(
          "text-xl font-semibold tabular-nums",
          tone === "ok" && "text-primary",
          tone === "danger" && "text-destructive",
        )}
      >
        {value}
      </span>
      {detail ? <span className="truncate text-xs text-muted-foreground">{detail}</span> : null}
    </div>
  )
}

/** Plain console-style output (ops logs / errors). Prefer DiffView for change summaries. */
export function OutputPre({
  value,
  placeholder,
  error,
}: {
  value?: string
  placeholder?: string
  error?: boolean
}) {
  return <OpsLogView value={value} placeholder={placeholder} error={error} />
}

export function Section({
  title,
  actions,
  children,
  className,
}: {
  title?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section
      className={cn(
        "flex flex-col gap-2.5 rounded-md border border-border/60 bg-card/30 p-3.5",
        className,
      )}
    >
      {title || actions ? (
        <div className="flex flex-wrap items-center gap-2">
          {title ? (
            <h2 className="text-[13px] font-semibold tracking-wide text-foreground">{title}</h2>
          ) : null}
          {actions}
        </div>
      ) : null}
      {children}
    </section>
  )
}
