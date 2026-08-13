import { useEffect, useRef, type ReactNode } from "react"

import { opsLineTone, opsToneClass } from "@/lib/opsLog"
import { cn } from "@/lib/utils"

/**
 * Shared monospace log / output pane (ops tools, last-apply, apply result, etc.).
 * Fixed min-height + capped max-height; scrolls inside the panel; auto-scrolls to bottom on new content.
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

  useEffect(() => {
    if (empty) return
    const el = scrollRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [value, empty])

  const shell = cn(
    "overflow-y-auto thin-scrollbar rounded-md border border-border bg-muted/40 text-foreground",
    // Cap height so long logs scroll inside the panel instead of stretching the page.
    fixedHeight ? "min-h-56 max-h-[28rem]" : "max-h-[32rem]",
    error && "border-destructive/50 bg-destructive/5",
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
    <div ref={scrollRef} className={shell}>
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
    </div>
  )
}
