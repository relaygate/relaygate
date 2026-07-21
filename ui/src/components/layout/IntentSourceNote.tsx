import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { FileCodeIcon } from "lucide-react"

/** Declares UI/file homology: forms and Config share one resources.yaml. */
export function IntentSourceNote({ className }: { className?: string }) {
  const { t } = useTranslation()
  return (
    <p
      className={
        className ??
        "flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 text-xs text-muted-foreground"
      }
    >
      <FileCodeIcon className="size-3.5 shrink-0 translate-y-px opacity-70" aria-hidden />
      <span>{t("intent.note")}</span>
      <Link to="/config" className="font-medium text-foreground underline-offset-2 hover:underline">
        {t("intent.open_config")}
      </Link>
      <span className="text-border">·</span>
      <Link to="/apply" className="font-medium text-foreground underline-offset-2 hover:underline">
        {t("intent.open_apply")}
      </Link>
    </p>
  )
}
