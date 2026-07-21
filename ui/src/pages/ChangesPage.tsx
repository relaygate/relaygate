import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Page, PageHeader, OutputPre, Section } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useStandby } from "@/context/SessionContext"
import {
  ApiError,
  getChangeDetail,
  getChanges,
  rollback,
  rollbackPreview,
} from "@/lib/api"
import type { ChangeEntry } from "@/lib/types"

export function ChangesPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [entries, setEntries] = useState<ChangeEntry[]>([])
  const [selectedStamp, setSelectedStamp] = useState<string | null>(null)
  const [detail, setDetail] = useState("")
  const [rollbackStamp, setRollbackStamp] = useState<string | null>(null)
  const [rollbackSummary, setRollbackSummary] = useState("")
  const [rollbackConfirm, setRollbackConfirm] = useState("")
  const [rollbackResult, setRollbackResult] = useState("")
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setEntries(await getChanges(50))
  }, [])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  async function viewDetail(stamp: string) {
    setSelectedStamp(stamp)
    try {
      const data = await getChangeDetail(stamp)
      setDetail(data.summary)
    } catch (err) {
      setDetail(err instanceof ApiError ? err.message : t("changes.no_summary"))
    }
  }

  async function previewRollback(stamp: string) {
    setRollbackStamp(stamp)
    try {
      const data = await rollbackPreview(stamp)
      setRollbackSummary(data.summary || t("changes.no_summary"))
    } catch (err) {
      setRollbackSummary(err instanceof ApiError ? err.message : t("changes.rollback_err"))
    }
  }

  async function runRollback() {
    if (!rollbackStamp || standby) return
    try {
      const res = await rollback(rollbackStamp)
      setRollbackResult(res.output ?? t("changes.rollback_ok"))
      setRollbackConfirm("")
      toast.success(t("changes.rollback_ok"))
      await load()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : t("changes.rollback_err")
      setRollbackResult(msg)
      toast.error(msg)
    }
  }

  return (
    <Page className="gap-8">
      <PageHeader title={t("changes.title")} />

      <Section title={t("changes.history")}>
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("changes.stamp")}</TableHead>
                <TableHead>{t("changes.summary")}</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={3} className="text-muted-foreground">
                    …
                  </TableCell>
                </TableRow>
              ) : entries.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={3} className="text-muted-foreground">
                    {t("changes.empty")}
                  </TableCell>
                </TableRow>
              ) : (
                entries.map((entry) => (
                  <TableRow key={entry.stamp}>
                    <TableCell className="font-mono text-xs">{entry.stamp}</TableCell>
                    <TableCell className="max-w-md truncate text-xs text-muted-foreground">
                      {entry.summary.split("\n")[0]}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button size="sm" variant="outline" onClick={() => viewDetail(entry.stamp)}>
                          {t("changes.view")}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => previewRollback(entry.stamp)}
                        >
                          {t("changes.rollback_ellipsis")}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </Section>

      <Section title={t("changes.detail")}>
        <OutputPre
          value={selectedStamp ? detail : ""}
          placeholder={t("changes.detail_placeholder")}
        />
      </Section>

      {rollbackStamp ? (
        <Section title={t("changes.rollback")}>
          <p className="text-sm text-muted-foreground">
            {t("changes.rollback_to")}: <code className="font-mono">{rollbackStamp}</code>
          </p>
          <OutputPre value={rollbackSummary} />
          <FieldGroup className="max-w-md gap-4">
            <Field>
              <FieldLabel>{t("changes.rollback_confirm")}</FieldLabel>
              <Input
                value={rollbackConfirm}
                onChange={(e) => setRollbackConfirm(e.target.value)}
                disabled={standby}
              />
            </Field>
            <Button
              onClick={runRollback}
              disabled={standby || rollbackConfirm !== "ROLLBACK"}
              variant="destructive"
            >
              {t("changes.rollback_run")}
            </Button>
          </FieldGroup>
          {rollbackResult ? <OutputPre value={rollbackResult} /> : null}
          <p className="text-xs text-muted-foreground">{t("changes.rollback_hint")}</p>
        </Section>
      ) : null}
    </Page>
  )
}
