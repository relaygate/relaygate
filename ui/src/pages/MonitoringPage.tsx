import { useEffect } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { ExternalLinkIcon, MonitorOffIcon } from "lucide-react"

import { EmptyState } from "@/components/layout/EmptyState"
import { Page, PageHeader } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import { useSession } from "@/context/SessionContext"

const GRAFANA_HREF = "/grafana/"

/** Fallback when `/monitoring` is opened directly; sidebar opens Grafana in a new tab. */
export function MonitoringPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { session } = useSession()
  const enabled = session?.grafana_enabled === true

  useEffect(() => {
    if (!enabled) return
    window.open(GRAFANA_HREF, "_blank", "noopener,noreferrer")
    navigate("/", { replace: true })
  }, [enabled, navigate])

  if (!enabled) {
    return (
      <Page>
        <PageHeader title={t("nav.monitoring")} />
        <EmptyState
          icon={MonitorOffIcon}
          title={t("monitoring.disabled_title")}
          description={t("monitoring.disabled")}
        />
      </Page>
    )
  }

  return (
    <Page>
      <PageHeader title={t("nav.monitoring")} description={t("monitoring.use_sidebar")} />
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          render={<a href={GRAFANA_HREF} target="_blank" rel="noopener noreferrer" />}
        >
          <ExternalLinkIcon data-icon="inline-start" />
          {t("monitoring.open_window")}
        </Button>
      </div>
    </Page>
  )
}
