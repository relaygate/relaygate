import type { ReactNode } from "react"

import { cn } from "@/lib/utils"

export function Page({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("page-enter flex flex-col gap-6", className)}>{children}</div>
}

export function PageHeader({ title, hint }: { title: string; hint?: string }) {
  return (
    <header className="flex flex-col gap-1 border-b border-border pb-4">
      <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
      {hint ? <p className="text-sm text-muted-foreground">{hint}</p> : null}
    </header>
  )
}

export function StatGrid({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
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
    <div className="flex flex-col gap-1 rounded-lg border border-border bg-card/40 px-4 py-3">
      <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <span
        className={cn(
          "text-2xl font-semibold tabular-nums",
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

export function OutputPre({
  value,
  placeholder,
  error,
}: {
  value?: string
  placeholder?: string
  error?: boolean
}) {
  const text = value?.trim() ? value : placeholder ?? ""
  return (
    <pre
      className={cn(
        "max-h-[28rem] overflow-auto rounded-lg border border-border bg-muted/30 p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap",
        error && "border-destructive/40 text-destructive",
        !value?.trim() && "text-muted-foreground",
      )}
    >
      {text}
    </pre>
  )
}

export function Section({
  title,
  children,
  className,
}: {
  title: string
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn("flex flex-col gap-3", className)}>
      <h2 className="text-sm font-semibold tracking-wide text-foreground">{title}</h2>
      {children}
    </section>
  )
}
