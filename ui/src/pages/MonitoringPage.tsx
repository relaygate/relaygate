import { useTranslation } from "react-i18next"

import { Page, PageHeader } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import { useSession } from "@/context/SessionContext"

export function MonitoringPage() {
  const { t } = useTranslation()
  const { session } = useSession()
  const enabled = session?.grafana_enabled === true

  if (!enabled) {
    return (
      <Page>
        <PageHeader title={t("monitoring.title")} />
        <p className="rounded-lg border border-border bg-muted/20 p-6 text-sm text-muted-foreground">
          {t("monitoring.disabled")}
        </p>
      </Page>
    )
  }

  return (
    <div className="page-enter flex min-h-[calc(100svh-7rem)] flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold tracking-tight">{t("monitoring.title")}</h1>
        <Button variant="outline" size="sm" render={<a href="/grafana/" target="_blank" rel="noreferrer" />}>
          {t("monitoring.open_window")}
        </Button>
      </div>
      <iframe
        title="Grafana"
        src="/grafana/"
        className="min-h-0 flex-1 rounded-lg border border-border bg-background"
      />
    </div>
  )
}
