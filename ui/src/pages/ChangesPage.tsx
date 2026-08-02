import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { FileTextIcon, HistoryIcon } from "lucide-react"

import { DiffView, stripChangeSummaryNoise } from "@/components/layout/DiffView"
import { EmptyState } from "@/components/layout/EmptyState"
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

export function ChangesPage({ embedded = false }: { embedded?: boolean }) {
  const { t } = useTranslation()
  const standby = useStandby()
  const [entries, setEntries] = useState<ChangeEntry[]>([])
  const [selectedStamp, setSelectedStamp] = useState<string | null>(null)
  const [detail, setDetail] = useState("")
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailLoading, setDetailLoading] = useState(false)
  const [rollbackStamp, setRollbackStamp] = useState<string | null>(null)
  const [rollbackSummary, setRollbackSummary] = useState("")
  const [rollbackConfirm, setRollbackConfirm] = useState("")
  const [rollbackResult, setRollbackResult] = useState("")
  const [rollbackBusy, setRollbackBusy] = useState(false)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    try {
      setEntries(await getChanges(50))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("common.toast_load_fail"))
    }
  }, [t])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  async function viewDetail(stamp: string) {
    setSelectedStamp(stamp)
    setDetailOpen(true)
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
      {!embedded ? (
        <PageHeader title={t("changes.title")} description={t("changes.desc")} />
      ) : null}

      <Section className="gap-0 overflow-hidden p-0">
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
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={3} className="p-2">
                  <EmptyState
                    icon={HistoryIcon}
                    title={t("changes.empty")}
                    description={t("changes.empty_hint")}
                  />
                </TableCell>
              </TableRow>
            ) : (
              entries.map((entry) => (
                <TableRow
                  key={entry.stamp}
                  data-state={selectedStamp === entry.stamp ? "selected" : undefined}
                >
                  <TableCell className="font-mono text-xs">{entry.stamp}</TableCell>
                  <TableCell className="max-w-md truncate text-xs text-muted-foreground">
                    {stripChangeSummaryNoise(entry.summary, {
                      noDiffLabel: t("changes.no_diff"),
                    }).split("\n")[0] || entry.summary.split("\n")[0]}
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

      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t("changes.detail")}</DialogTitle>
            <DialogDescription>
              {selectedStamp ? (
                <code className="font-mono text-foreground">{selectedStamp}</code>
              ) : null}
            </DialogDescription>
          </DialogHeader>
          <DiffView
            value={detailLoading ? "" : detail}
            placeholder={
              detailLoading ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Spinner />
                  {t("common.loading")}
                </div>
              ) : (
                <EmptyState
                  icon={FileTextIcon}
                  title={
                    selectedStamp ? t("changes.no_summary") : t("changes.detail_placeholder")
                  }
                  className="w-full border-0 bg-transparent"
                />
              )
            }
          />
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDetailOpen(false)}>
              {t("ops.cancel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

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
              <br />
              {t("changes.rollback_body")}
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
