import type { LucideIcon } from "lucide-react"

import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { cn } from "@/lib/utils"

export function EmptyState({
  icon: Icon,
  title,
  description,
  className,
  compact = false,
}: {
  icon: LucideIcon
  title: string
  description?: string
  className?: string
  /** Smaller footprint for nested panels (e.g. ACL lists). */
  compact?: boolean
}) {
  return (
    <Empty
      className={cn(
        "border border-dashed border-border/60 bg-muted/10",
        compact ? "min-h-28 gap-2 p-4" : "min-h-44 gap-3 p-6",
        className,
      )}
    >
      <EmptyHeader className={cn(compact && "gap-1.5")}>
        <EmptyMedia variant="icon">
          <Icon />
        </EmptyMedia>
        <EmptyTitle className={cn(compact && "text-xs")}>{title}</EmptyTitle>
        {description ? (
          <EmptyDescription className={cn(compact && "text-xs")}>{description}</EmptyDescription>
        ) : null}
      </EmptyHeader>
    </Empty>
  )
}
