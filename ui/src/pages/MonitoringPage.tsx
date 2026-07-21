import { useTranslation } from "react-i18next"
import { ExternalLinkIcon, MonitorOffIcon } from "lucide-react"

import { EmptyState } from "@/components/layout/EmptyState"
import { Page, PageHeader } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import { useSession } from "@/context/SessionContext"

const GRAFANA_HREF = "/grafana/"

export function MonitoringPage() {
  const { t } = useTranslation()
  const { session } = useSession()
  const enabled = session?.grafana_enabled === true

  return (
    <Page>
      <PageHeader title={t("monitoring.title")} description={t("monitoring.desc")} />
      {enabled ? (
        <div className="flex flex-col gap-3 rounded-lg border border-border bg-muted/20 p-6">
          <p className="text-sm text-muted-foreground">{t("monitoring.open_hint")}</p>
          <div>
            <Button
              size="sm"
              title={t("monitoring.open_window")}
              render={<a href={GRAFANA_HREF} target="_blank" rel="noopener noreferrer" />}
            >
              <ExternalLinkIcon data-icon="inline-start" />
              {t("monitoring.open_window")}
            </Button>
          </div>
        </div>
      ) : (
        <EmptyState
          icon={MonitorOffIcon}
          title={t("monitoring.disabled_title")}
          description={t("monitoring.disabled")}
        />
      )}
    </Page>
  )
}
