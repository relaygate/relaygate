import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Page, PageHeader } from "@/components/layout/PageParts"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useStandby } from "@/context/SessionContext"
import { ApiError, getRules, patchRule } from "@/lib/api"
import type { Rule } from "@/lib/types"
import { tf } from "@/i18n"

export function RulesPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [rules, setRules] = useState<Rule[]>([])
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState<string | null>(null)

  const load = useCallback(async () => {
    setRules(await getRules())
  }, [])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  async function toggleRule(rule: Rule, enabled: boolean) {
    if (standby) return
    setToggling(rule.name)
    setRules((prev) =>
      prev.map((r) => (r.name === rule.name ? { ...r, enabled } : r)),
    )
    try {
      await patchRule(rule.name, enabled)
      toast.success(
        enabled ? tf("rules.toast_enabled", rule.name) : tf("rules.toast_disabled", rule.name),
      )
    } catch (err) {
      setRules((prev) =>
        prev.map((r) => (r.name === rule.name ? { ...r, enabled: rule.enabled } : r)),
      )
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setToggling(null)
    }
  }

  return (
    <Page>
      <PageHeader title={t("rules.title")} hint={t("rules.hint")} />
      <div className="rounded-md border border-border/60">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("rules.col_name")}</TableHead>
              <TableHead>{t("rules.col_kind")}</TableHead>
              <TableHead>{t("rules.col_protocol")}</TableHead>
              <TableHead>{t("rules.col_listen")}</TableHead>
              <TableHead>{t("rules.col_server")}</TableHead>
              <TableHead>{t("rules.col_enabled")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 6 }).map((__, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-4 w-full" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : rules.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                  {t("rules.empty")}
                </TableCell>
              </TableRow>
            ) : (
              rules.map((rule) => (
                <TableRow key={rule.name}>
                  <TableCell className="font-medium">{rule.name}</TableCell>
                  <TableCell>{rule.kind}</TableCell>
                  <TableCell>{rule.protocol}</TableCell>
                  <TableCell className="font-mono text-xs">{rule.listen_port}</TableCell>
                  <TableCell>{rule.server}</TableCell>
                  <TableCell>
                    <Switch
                      checked={rule.enabled}
                      onCheckedChange={(v) => toggleRule(rule, v)}
                      disabled={standby || toggling === rule.name}
                    />
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </Page>
  )
}
