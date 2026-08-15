import { useEffect, useRef, type ReactNode } from "react"

import { opsLineTone, opsToneClass } from "@/lib/opsLog"
import { cn } from "@/lib/utils"

/** Shared pane size: empty and filled use the same min/max so the box does not jump. */
const PANE_SIZE_FIXED = "h-56 min-h-56 max-h-56"
const PANE_SIZE_FLUID = "min-h-56 max-h-[32rem]"

/**
 * Shared monospace log / output pane (ops tools, last-apply, apply result, etc.).
 * Fixed height viewport (empty and filled share the same min/max); scrolls inside; auto-scrolls on new content.
 */
export function OpsLogView({
  value,
  placeholder,
  className,
  error = false,
  fixedHeight = true,
}: {
  value?: string
  placeholder?: ReactNode
  className?: string
  /** Soft destructive chrome when the operation failed. */
  error?: boolean
  /** When true (default), box keeps a stable min-height so layout does not jump. */
  fixedHeight?: boolean
}) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const trimmed = value?.trim() ?? ""
  const empty = !trimmed || trimmed === "尚无" || trimmed === "None" || trimmed === "(no output)"
  const isNodePlaceholder = empty && placeholder != null && typeof placeholder !== "string"

  useEffect(() => {
    if (empty) return
    const el = scrollRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [value, empty])

  const lines = empty ? [] : value!.replace(/\r\n/g, "\n").split("\n")

  return (
    <div
      ref={scrollRef}
      data-slot="ops-log"
      className={cn(
        "overflow-y-auto thin-scrollbar rounded-md border border-border bg-muted/40 text-foreground",
        fixedHeight ? PANE_SIZE_FIXED : PANE_SIZE_FLUID,
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
          {lines.map((line, i) => (
            <div
              key={i}
              className={cn(
                "whitespace-pre-wrap break-all",
                opsToneClass[opsLineTone(line)],
                error && "text-destructive",
              )}
            >
              {line || " "}
            </div>
          ))}
        </pre>
      )}
    </div>
  )
}
