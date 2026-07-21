import { cn } from "@/lib/utils"

function lineKind(line: string): "add" | "del" | "hunk" | "meta" | "ctx" {
  if (line.startsWith("+++") || line.startsWith("---") || line.startsWith("diff ") || line.startsWith("index ")) {
    return "meta"
  }
  if (line.startsWith("@@")) return "hunk"
  if (line.startsWith("+")) return "add"
  if (line.startsWith("-")) return "del"
  return "ctx"
}

const kindClass: Record<ReturnType<typeof lineKind>, string> = {
  add: "bg-emerald-500/10 text-emerald-300",
  del: "bg-red-500/10 text-red-300",
  hunk: "bg-sky-500/10 text-sky-300",
  meta: "text-muted-foreground",
  ctx: "text-foreground/85",
}

export function DiffView({
  value,
  placeholder,
  error,
  className,
}: {
  value?: string
  placeholder?: string
  error?: boolean
  className?: string
}) {
  const text = value?.trim() ? value : ""
  if (!text) {
    return (
      <div
        className={cn(
          "rounded-md border border-border/60 bg-card/40 px-3 py-3 font-mono text-xs text-muted-foreground",
          className,
        )}
      >
        {placeholder ?? ""}
      </div>
    )
  }

  const lines = text.replace(/\r\n/g, "\n").split("\n")
  const looksLikeDiff = lines.some((l) => /^(diff |@@|\+\+\+|---|[+-](?![+-]))/.test(l))

  return (
    <div
      className={cn(
        "max-h-[28rem] overflow-auto rounded-md border border-border/60 bg-[#0c1016] font-mono text-[12px] leading-[1.55]",
        error && "border-destructive/40",
        className,
      )}
    >
      {looksLikeDiff ? (
        <pre className="m-0 p-0">
          {lines.map((line, i) => {
            const kind = lineKind(line)
            return (
              <div
                key={i}
                className={cn(
                  "whitespace-pre-wrap break-all px-3 py-px",
                  kindClass[kind],
                  error && "text-destructive",
                )}
              >
                {line || " "}
              </div>
            )
          })}
        </pre>
      ) : (
        <pre
          className={cn(
            "m-0 whitespace-pre-wrap break-all px-3 py-3 text-foreground/85",
            error && "text-destructive",
          )}
        >
          {text}
        </pre>
      )}
    </div>
  )
}
