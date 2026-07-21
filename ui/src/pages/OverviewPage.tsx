import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Page, PageHeader, OutputPre, Section, StatBlock, StatGrid } from "@/components/layout/PageParts"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Spinner } from "@/components/ui/spinner"
import { getApplyPreview, getEnvoyStatus, getTrafficStatus } from "@/lib/api"
import type { EnvoyStatus, TrafficStatus } from "@/lib/types"

export function OverviewPage() {
  const { t } = useTranslation()
  const [envoy, setEnvoy] = useState<EnvoyStatus | null>(null)
  const [traffic, setTraffic] = useState<TrafficStatus | null>(null)
  const [lastApply, setLastApply] = useState("")
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([getEnvoyStatus(), getTrafficStatus(), getApplyPreview()])
      .then(([e, tr, preview]) => {
        setEnvoy(e)
        setTraffic(tr)
        setLastApply(preview.last_apply)
      })
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Spinner className="size-6 text-primary" />
      </div>
    )
  }

  return (
    <Page>
      <PageHeader title={t("overview.title")} />
      <StatGrid>
        <StatBlock
          label={t("overview.envoy")}
          value={envoy?.ready ? t("common.ready") : t("common.down")}
          detail={envoy?.ready_body}
          tone={envoy?.ready ? "ok" : "danger"}
        />
        <StatBlock label={t("overview.healthy_clusters")} value={envoy?.healthy_clusters ?? 0} />
        <StatBlock
          label={t("overview.tcp_active")}
          value={Math.round(traffic?.tcp_active_connections ?? 0)}
        />
        <StatBlock
          label={t("overview.udp_sessions")}
          value={Math.round(traffic?.udp_active_sessions ?? 0)}
        />
        <StatBlock
          label={t("overview.rate_limit_5m")}
          value={Math.round(traffic?.local_rate_limited_5m ?? 0)}
          detail={traffic?.error}
        />
      </StatGrid>

      <Section title={t("overview.last_apply")}>
        <OutputPre value={lastApply} placeholder={t("overview.none")} />
      </Section>

      <Section title={t("overview.top_limited")}>
        {traffic?.top_limited_rules?.length ? (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("overview.col_rule")}</TableHead>
                  <TableHead>{t("overview.col_prefix")}</TableHead>
                  <TableHead className="text-right">{t("overview.col_hits")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {traffic.top_limited_rules.map((row) => (
                  <TableRow key={`${row.rule}-${row.prefix}`}>
                    <TableCell className="font-medium">{row.rule}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {row.prefix}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {Math.round(row.hits_5m)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ) : (
          <p className="rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
            {t("overview.top_empty")}
          </p>
        )}
        {envoy?.error ? (
          <Badge variant="outline" className="w-fit text-destructive">
            {envoy.error}
          </Badge>
        ) : null}
      </Section>
    </Page>
  )
}
