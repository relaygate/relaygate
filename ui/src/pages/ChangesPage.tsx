import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { DiffView } from "@/components/layout/DiffView"
import { Page, PageHeader, Section } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
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
  const [detailLoading, setDetailLoading] = useState(false)
  const [rollbackStamp, setRollbackStamp] = useState<string | null>(null)
  const [rollbackSummary, setRollbackSummary] = useState("")
  const [rollbackConfirm, setRollbackConfirm] = useState("")
  const [rollbackResult, setRollbackResult] = useState("")
  const [rollbackBusy, setRollbackBusy] = useState(false)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setEntries(await getChanges(50))
  }, [])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  async function viewDetail(stamp: string) {
    setSelectedStamp(stamp)
    setDetailLoading(true)
    try {
      const data = await getChangeDetail(stamp)
      setDetail(data.summary)
    } catch (err) {
      setDetail(err instanceof ApiError ? err.message : t("changes.no_summary"))
    } finally {
      setDetailLoading(false)
    }
  }

  async function openRollback(stamp: string) {
    setRollbackStamp(stamp)
    setRollbackConfirm("")
    setRollbackResult("")
    setRollbackSummary(t("common.loading"))
    try {
      const data = await rollbackPreview(stamp)
      setRollbackSummary(data.summary || t("changes.no_summary"))
    } catch (err) {
      setRollbackSummary(err instanceof ApiError ? err.message : t("changes.rollback_err"))
    }
  }

  async function runRollback() {
    if (!rollbackStamp || standby) return
    setRollbackBusy(true)
    try {
      const res = await rollback(rollbackStamp)
      setRollbackResult(res.output ?? t("changes.rollback_ok"))
      setRollbackConfirm("")
      toast.success(t("changes.rollback_ok"))
      setRollbackStamp(null)
      await load()
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : t("changes.rollback_err")
      setRollbackResult(msg)
      toast.error(msg)
    } finally {
      setRollbackBusy(false)
    }
  }

  return (
    <Page className="gap-5">
      <PageHeader title={t("changes.title")} />

      <Section title={t("changes.history")} className="p-0 gap-0 overflow-hidden">
        <div className="border-b border-border/60 px-3.5 py-2.5">
          <h2 className="text-[13px] font-semibold tracking-wide">{t("changes.history")}</h2>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("changes.stamp")}</TableHead>
              <TableHead>{t("changes.summary")}</TableHead>
              <TableHead className="text-right">{t("shell.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell colSpan={3}>
                    <Skeleton className="h-4 w-full" />
                  </TableCell>
                </TableRow>
              ))
            ) : entries.length === 0 ? (
              <TableRow>
                <TableCell colSpan={3} className="py-8 text-center text-muted-foreground">
                  {t("changes.empty")}
                </TableCell>
              </TableRow>
            ) : (
              entries.map((entry) => (
                <TableRow key={entry.stamp} data-state={selectedStamp === entry.stamp ? "selected" : undefined}>
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
                        onClick={() => openRollback(entry.stamp)}
                        disabled={standby}
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
      </Section>

      <Section title={t("changes.detail")}>
        <DiffView
          value={detailLoading ? "" : detail}
          placeholder={
            detailLoading
              ? t("common.loading")
              : selectedStamp
                ? t("changes.no_summary")
                : t("changes.detail_placeholder")
          }
        />
      </Section>

      <Dialog
        open={!!rollbackStamp}
        onOpenChange={(open) => !open && !rollbackBusy && setRollbackStamp(null)}
      >
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>{t("changes.rollback")}</DialogTitle>
            <DialogDescription>
              {t("changes.rollback_to")}:{" "}
              <code className="font-mono text-foreground">{rollbackStamp}</code>
            </DialogDescription>
          </DialogHeader>
          <DiffView value={rollbackSummary} />
          <FieldGroup>
            <Field>
              <FieldLabel>{t("changes.rollback_confirm")}</FieldLabel>
              <Input
                value={rollbackConfirm}
                onChange={(e) => setRollbackConfirm(e.target.value)}
                disabled={standby || rollbackBusy}
                autoComplete="off"
              />
            </Field>
          </FieldGroup>
          {rollbackResult ? <DiffView value={rollbackResult} error /> : null}
          <p className="text-xs text-muted-foreground">{t("changes.rollback_hint")}</p>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setRollbackStamp(null)} disabled={rollbackBusy}>
              {t("ops.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={runRollback}
              disabled={standby || rollbackBusy || rollbackConfirm !== "ROLLBACK"}
            >
              {rollbackBusy ? <Spinner data-icon="inline-start" /> : null}
              {t("changes.rollback_run")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
