import type { ReactNode } from "react"

import { opsLineTone, opsToneClass } from "@/lib/opsLog"
import { cn } from "@/lib/utils"

/**
 * Ops-tool output panel: plain monospace log (scrollable, fixed min-height).
 * Accepts plain-text `output` from `/api/ops/*`.
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
  /** When true (default), box keeps a stable min-height so layout does not jump. */
  fixedHeight?: boolean
}) {
  const trimmed = value?.trim() ?? ""
  const empty = !trimmed || trimmed === "尚无" || trimmed === "None" || trimmed === "(no output)"

  const shell = cn(
    "overflow-hidden rounded-md border border-border bg-muted/40 text-foreground",
    fixedHeight && "min-h-56",
    !fixedHeight && "max-h-[32rem]",
    className,
  )

  if (empty) {
    const isNode = placeholder != null && typeof placeholder !== "string"
    return (
      <div
        className={cn(
          shell,
          isNode
            ? "flex min-h-56 items-center justify-center border-dashed bg-transparent p-0"
            : "flex min-h-56 items-center border-dashed px-3 py-3 text-sm text-muted-foreground",
        )}
      >
        {placeholder ?? ""}
      </div>
    )
  }

  const lines = value!.replace(/\r\n/g, "\n").split("\n")

  return (
    <div className={cn(shell, "overflow-y-auto")}>
      <pre className="m-0 min-h-56 p-3 font-mono text-[12px] leading-[1.55]">
        {lines.map((line, i) => (
          <div
            key={i}
            className={cn("whitespace-pre-wrap break-all", opsToneClass[opsLineTone(line)])}
          >
            {line || " "}
          </div>
        ))}
      </pre>
    </div>
  )
}
