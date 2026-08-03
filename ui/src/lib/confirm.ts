/** Unified typed confirmation phrases for sensitive Panel ops. */

export const CONFIRM_PHRASE_ZH = "确认"
export const CONFIRM_PHRASE_EN = "Confirm"

/** Accepts Chinese「确认」or English Confirm (case-sensitive). */
export function matchesConfirm(value: string): boolean {
  const v = value.trim()
  return v === CONFIRM_PHRASE_ZH || v === CONFIRM_PHRASE_EN
}

/** Locale-facing prompt word (UI label / placeholder). Backend still accepts both. */
export function confirmPhraseForLocale(lang: string): string {
  return lang.toLowerCase().startsWith("zh") ? CONFIRM_PHRASE_ZH : CONFIRM_PHRASE_EN
}
