import i18n from "i18next"
import { initReactI18next } from "react-i18next"

import en from "@/i18n/en.json"
import zhCN from "@/i18n/zh-CN.json"
import { setLang } from "@/lib/api"

export type PanelLang = "en" | "zh-CN"

const resources = {
  en: { translation: en },
  "zh-CN": { translation: zhCN },
}

i18n.use(initReactI18next).init({
  resources,
  lng: "zh-CN",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
})

export default i18n

/** Simple sprintf-style formatter for %.0f / %s / %d placeholders in JSON strings. */
export function tf(key: string, ...args: (string | number)[]): string {
  const template = i18n.t(key)
  if (args.length === 0) return template

  let i = 0
  return template.replace(/%(\.0f|s|d)/g, (_match, spec: string) => {
    const arg = args[i++]
    if (arg === undefined) return ""
    if (spec === ".0f") return String(Math.round(Number(arg)))
    return String(arg)
  })
}

export async function syncLangFromSession(lang?: string): Promise<void> {
  if (lang === "en" || lang === "zh-CN") {
    await i18n.changeLanguage(lang)
  }
}

export async function changePanelLang(lang: PanelLang): Promise<void> {
  await setLang(lang)
  await i18n.changeLanguage(lang)
}
