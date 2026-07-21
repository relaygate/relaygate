import i18n from "@/i18n"

/** YAML/API stage identifiers (kind). Keep English; UI shows localized labels. */
export const STAGES = ["canary", "production"] as const
export type StageKind = (typeof STAGES)[number]

const STAGE_LABEL_KEYS: Record<StageKind, string> = {
  canary: "stage.canary",
  production: "stage.production",
}

export function isStageKind(kind: string): kind is StageKind {
  return kind === "canary" || kind === "production"
}

/** Localized display name for a forward-rule stage (kind). Unknown kinds pass through. */
export function stageLabel(kind: string): string {
  if (!isStageKind(kind)) return kind
  return i18n.t(STAGE_LABEL_KEYS[kind])
}
