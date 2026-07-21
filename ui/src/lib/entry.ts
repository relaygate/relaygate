import i18n from "@/i18n"

/** YAML/API entry identifiers. Keep English; UI shows localized labels. */
export const ENTRIES = ["validation", "production"] as const
export type EntryKind = (typeof ENTRIES)[number]

const ENTRY_LABEL_KEYS: Record<EntryKind, string> = {
  validation: "entry.validation",
  production: "entry.production",
}

export function isEntryKind(entry: string): entry is EntryKind {
  return entry === "validation" || entry === "production"
}

/** Localized display name for a forward-rule entry type. Unknown values pass through. */
export function entryLabel(entry: string): string {
  if (!isEntryKind(entry)) return entry
  return i18n.t(ENTRY_LABEL_KEYS[entry])
}
