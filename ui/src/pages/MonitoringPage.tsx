import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import {
  ExternalLinkIcon,
  MonitorOffIcon,
  ScrollTextIcon,
  SearchIcon,
} from "lucide-react"

import { EmptyState } from "@/components/layout/EmptyState"
import { Page, PageHeader } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useSession } from "@/context/SessionContext"
import {
  TCP_SESSION_DASHBOARD_HREF,
  anomalyFlagsExploreHref,
  buildSessionLogQL,
  sessionLogsExploreHref,
  shortSessionsExploreHref,
} from "@/lib/lokiExplore"

const GRAFANA_HREF = "/grafana/"

export function MonitoringPage() {
  const { t } = useTranslation()
  const { session } = useSession()
  const enabled = session?.grafana_enabled === true

  const [gateway, setGateway] = useState("")
  const [ip, setIp] = useState("")
  const [rule, setRule] = useState("")
  const [from, setFrom] = useState("now-1h")
  const [to, setTo] = useState("now")

  const previewExpr = useMemo(
    () => buildSessionLogQL({ gateway, ip, rule }),
    [gateway, ip, rule],
  )
  const exploreHref = useMemo(
    () => sessionLogsExploreHref({ gateway, ip, rule, from, to }),
    [gateway, ip, rule, from, to],
  )

  return (
    <Page>
      <PageHeader title={t("monitoring.title")} description={t("monitoring.desc")} />
      {enabled ? (
        <div className="flex flex-col gap-6">
          <div className="flex flex-col gap-3 rounded-lg border border-border bg-muted/20 p-6">
            <p className="text-sm text-muted-foreground">{t("monitoring.open_hint")}</p>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                title={t("monitoring.open_window")}
                render={<a href={GRAFANA_HREF} target="_blank" rel="noopener noreferrer" />}
              >
                <ExternalLinkIcon data-icon="inline-start" />
                {t("monitoring.open_window")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                title={t("monitoring.session_dashboard")}
                render={
                  <a href={TCP_SESSION_DASHBOARD_HREF} target="_blank" rel="noopener noreferrer" />
                }
              >
                <ScrollTextIcon data-icon="inline-start" />
                {t("monitoring.session_dashboard")}
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-3 rounded-lg border border-border bg-muted/20 p-6">
            <div>
              <h2 className="text-sm font-medium text-foreground">
                {t("monitoring.session_logs")}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {t("monitoring.session_logs_hint")}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant="secondary"
                render={
                  <a
                    href={sessionLogsExploreHref({ gateway: gateway || undefined, from, to })}
                    target="_blank"
                    rel="noopener noreferrer"
                  />
                }
              >
                <ExternalLinkIcon data-icon="inline-start" />
                {t("monitoring.open_explore")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                render={
                  <a
                    href={anomalyFlagsExploreHref(gateway || undefined, { from, to })}
                    target="_blank"
                    rel="noopener noreferrer"
                  />
                }
              >
                {t("monitoring.preset_flags")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                render={
                  <a
                    href={shortSessionsExploreHref(gateway || undefined, 2000, { from, to })}
                    target="_blank"
                    rel="noopener noreferrer"
                  />
                }
              >
                {t("monitoring.preset_short")}
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-3 rounded-lg border border-border bg-muted/20 p-6">
            <div>
              <h2 className="text-sm font-medium text-foreground">
                {t("monitoring.query_title")}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {t("monitoring.query_hint")}
              </p>
            </div>
            <FieldGroup className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="log-gateway">{t("monitoring.field_gateway")}</FieldLabel>
                <Input
                  id="log-gateway"
                  value={gateway}
                  onChange={(e) => setGateway(e.target.value)}
                  placeholder="gateway-01"
                  autoComplete="off"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="log-ip">{t("monitoring.field_ip")}</FieldLabel>
                <Input
                  id="log-ip"
                  value={ip}
                  onChange={(e) => setIp(e.target.value)}
                  placeholder="203.0.113.9"
                  autoComplete="off"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="log-rule">{t("monitoring.field_rule")}</FieldLabel>
                <Input
                  id="log-rule"
                  value={rule}
                  onChange={(e) => setRule(e.target.value)}
                  placeholder="forward-server-01-production-tcp"
                  autoComplete="off"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="log-from">{t("monitoring.field_from")}</FieldLabel>
                <Input
                  id="log-from"
                  value={from}
                  onChange={(e) => setFrom(e.target.value)}
                  placeholder="now-1h"
                  autoComplete="off"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="log-to">{t("monitoring.field_to")}</FieldLabel>
                <Input
                  id="log-to"
                  value={to}
                  onChange={(e) => setTo(e.target.value)}
                  placeholder="now"
                  autoComplete="off"
                />
              </Field>
            </FieldGroup>
            <p className="font-mono text-[11px] break-all text-muted-foreground">{previewExpr}</p>
            <div>
              <Button
                size="sm"
                render={<a href={exploreHref} target="_blank" rel="noopener noreferrer" />}
              >
                <SearchIcon data-icon="inline-start" />
                {t("monitoring.open_query")}
              </Button>
            </div>
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
