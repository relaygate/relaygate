import { useCallback, useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { toast } from "sonner"
import { ArrowRightLeftIcon, DownloadIcon, PlusIcon, ServerIcon } from "lucide-react"

import { EmptyState } from "@/components/layout/EmptyState"
import { Page, PageHeader } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
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
import {
  ApiError,
  createForward,
  exportPortMapCSV,
  getForwards,
  getUpstreams,
  patchForward,
} from "@/lib/api"
import { ENTRIES, entryLabel, type EntryKind } from "@/lib/entry"
import type { Forward, Upstream } from "@/lib/types"
import { tf } from "@/i18n"

type AddForm = {
  upstream: string
  entry: EntryKind
  protocols: string[]
  enabledOverride: boolean | null
}

const emptyAddForm = (): AddForm => ({
  upstream: "",
  entry: "validation",
  protocols: [],
  enabledOverride: null,
})

function upstreamProtocols(upstream: Upstream | undefined): string[] {
  if (!upstream) return []
  const protocols: string[] = []
  if (upstream.tcp?.port) protocols.push("TCP")
  if (upstream.udp?.port) protocols.push("UDP")
  return protocols
}

function upstreamPort(upstream: Upstream | undefined, protocol: string): string {
  if (!upstream) return "—"
  const proto = protocol.toUpperCase()
  if (proto === "TCP") return upstream.tcp?.port ? String(upstream.tcp.port) : "—"
  if (proto === "UDP") return upstream.udp?.port ? String(upstream.udp.port) : "—"
  return "—"
}

export function RulesPage({ embedded = false }: { embedded?: boolean }) {
  const { t } = useTranslation()
  const standby = useStandby()
  const [forwards, setForwards] = useState<Forward[]>([])
  const [upstreams, setUpstreams] = useState<Upstream[]>([])
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState<string | null>(null)
  const [exporting, setExporting] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState<AddForm>(emptyAddForm)

  const load = useCallback(async () => {
    try {
      const [rulesData, serversData] = await Promise.all([getForwards(), getUpstreams()])
      setForwards(rulesData)
      setUpstreams(serversData.upstreams)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("common.toast_load_fail"))
    }
  }, [t])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  const upstreamsByName = useMemo(() => {
    const map = new Map<string, Upstream>()
    for (const s of upstreams) map.set(s.name, s)
    return map
  }, [upstreams])

  const selectedUpstream = useMemo(
    () => upstreamsByName.get(form.upstream),
    [upstreamsByName, form.upstream],
  )

  const availableProtocols = useMemo(
    () => upstreamProtocols(selectedUpstream),
    [selectedUpstream],
  )

  function openAdd() {
    const first = upstreams[0]
    const protocols = upstreamProtocols(first)
    setForm({
      ...emptyAddForm(),
      upstream: first?.name ?? "",
      protocols: [...protocols],
    })
    setAddOpen(true)
  }

  function setUpstream(name: string) {
    const srv = upstreams.find((s) => s.name === name)
    setForm((f) => ({
      ...f,
      upstream: name,
      protocols: upstreamProtocols(srv),
    }))
  }

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (standby) return
    if (!form.upstream) {
      toast.error(t("forwards.need_upstream"))
      return
    }
    if (!form.protocols.length) {
      toast.error(t("upstreams.quick_need_proto"))
      return
    }
    setAdding(true)
    try {
      const enabled =
        form.enabledOverride !== null ? { enabled: form.enabledOverride } : {}
      const res = await createForward({
        upstream: form.upstream,
        entry: form.entry,
        protocols: form.protocols,
        ...enabled,
      })
      await load()
      setAddOpen(false)
      setForm(emptyAddForm())
      if (res.forwards?.length) {
        toast.success(tf("forwards.toast_added", res.forwards.map((r) => r.name).join(", ")))
      } else {
        toast.message(t("forwards.toast_added_none"))
      }
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setAdding(false)
    }
  }

  async function handleExport() {
    setExporting(true)
    try {
      await exportPortMapCSV()
      toast.success(t("forwards.toast_export_ok"))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("forwards.toast_export_fail"))
    } finally {
      setExporting(false)
    }
  }

  async function toggleForward(fwd: Forward, enabled: boolean) {
    if (standby) return
    setToggling(fwd.name)
    setForwards((prev) =>
      prev.map((r) => (r.name === fwd.name ? { ...r, enabled } : r)),
    )
    try {
      await patchForward(fwd.name, enabled)
      toast.success(
        enabled ? tf("forwards.toast_enabled", fwd.name) : tf("forwards.toast_disabled", fwd.name),
      )
    } catch (err) {
      setForwards((prev) =>
        prev.map((r) => (r.name === fwd.name ? { ...r, enabled: fwd.enabled } : r)),
      )
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setToggling(null)
    }
  }

  const headerActions = (
    <>
      <Button size="sm" disabled={standby || loading} title={t("forwards.add")} onClick={openAdd}>
        <PlusIcon data-icon="inline-start" />
        {t("forwards.add")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={exporting || loading}
        title={t("forwards.export")}
        onClick={handleExport}
      >
        {exporting ? <Spinner data-icon="inline-start" /> : <DownloadIcon data-icon="inline-start" />}
        {t("forwards.export")}
      </Button>
    </>
  )

  const main = (
    <>
      {embedded ? <div className="flex flex-wrap justify-end gap-2">{headerActions}</div> : null}
      <div className="rounded-md border border-border/60">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("forwards.col_rule")}</TableHead>
              <TableHead>{t("forwards.col_entry")}</TableHead>
              <TableHead>{t("forwards.col_protocol")}</TableHead>
              <TableHead>{t("forwards.col_upstream")}</TableHead>
              <TableHead>{t("forwards.col_upstream_ip")}</TableHead>
              <TableHead>{t("forwards.col_upstream_port")}</TableHead>
              <TableHead>{t("forwards.col_port")}</TableHead>
              <TableHead>{t("forwards.col_enabled")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 8 }).map((__, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-4 w-full" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : forwards.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={8} className="p-2">
                  <EmptyState
                    icon={ArrowRightLeftIcon}
                    title={t("forwards.empty")}
                    description={t("forwards.empty_hint")}
                  />
                </TableCell>
              </TableRow>
            ) : (
              forwards.map((fwd) => {
                const srv = upstreamsByName.get(fwd.upstream)
                return (
                  <TableRow key={fwd.name}>
                    <TableCell className="font-mono text-xs font-medium">{fwd.name}</TableCell>
                    <TableCell>{entryLabel(fwd.entry)}</TableCell>
                    <TableCell>{fwd.protocol}</TableCell>
                    <TableCell className="font-medium">{fwd.upstream}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {srv?.address || "—"}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {upstreamPort(srv, fwd.protocol)}
                    </TableCell>
                    <TableCell className="font-mono text-xs">{fwd.listen_port}</TableCell>
                    <TableCell>
                      <Switch
                        title={t("forwards.col_enabled")}
                        aria-label={t("forwards.col_enabled")}
                        checked={fwd.enabled}
                        onCheckedChange={(v) => toggleForward(fwd, v)}
                        disabled={standby || toggling === fwd.name}
                      />
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog
        open={addOpen}
        onOpenChange={(open) => {
          if (!adding) {
            setAddOpen(open)
            if (!open) setForm(emptyAddForm())
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("forwards.add_heading")}</DialogTitle>
            <DialogDescription>{t("forwards.add_description")}</DialogDescription>
          </DialogHeader>
          {upstreams.length === 0 ? (
            <div className="flex flex-col gap-4">
              <EmptyState
                icon={ServerIcon}
                title={t("upstreams.empty")}
                description={t("forwards.need_upstream_hint")}
                compact
              />
              <DialogFooter>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setAddOpen(false)}
                >
                  {t("upstreams.cancel")}
                </Button>
                <Button render={<Link to="/upstreams" onClick={() => setAddOpen(false)} />}>
                  {t("forwards.goto_upstreams")}
                </Button>
              </DialogFooter>
            </div>
          ) : (
            <form onSubmit={handleAdd} className="flex flex-col gap-4">
              <Field>
                <FieldLabel>{t("forwards.upstream")}</FieldLabel>
                <Select
                  value={form.upstream || undefined}
                  onValueChange={(v) => {
                    if (v) setUpstream(v)
                  }}
                  disabled={standby || adding}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder={t("upstreams.empty")} />
                  </SelectTrigger>
                  <SelectContent>
                    {upstreams.map((s) => (
                      <SelectItem key={s.name} value={s.name}>
                        {s.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel>{t("forwards.entry")}</FieldLabel>
                <div className="flex h-8 items-center gap-3 text-sm">
                  {ENTRIES.map((entry) => (
                    <label key={entry} className="flex items-center gap-1.5">
                      <input
                        type="radio"
                        name="rule-entry"
                        checked={form.entry === entry}
                        onChange={() =>
                          setForm((f) => ({
                            ...f,
                            entry,
                            enabledOverride: null,
                          }))
                        }
                        disabled={standby || adding}
                      />
                      {entryLabel(entry)}
                    </label>
                  ))}
                </div>
              </Field>

              <Field>
                <FieldLabel>{t("forwards.protocols")}</FieldLabel>
                <div className="flex h-8 items-center gap-3 text-sm">
                  {(["TCP", "UDP"] as const).map((proto) => {
                    const available = availableProtocols.includes(proto)
                    return (
                      <label key={proto} className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={form.protocols.includes(proto)}
                          onChange={(e) =>
                            setForm((f) => ({
                              ...f,
                              protocols: e.target.checked
                                ? [...f.protocols.filter((p) => p !== proto), proto]
                                : f.protocols.filter((p) => p !== proto),
                            }))
                          }
                          disabled={standby || adding || !available}
                        />
                        {proto}
                      </label>
                    )
                  })}
                </div>
              </Field>

              <Field>
                <FieldLabel htmlFor="rule-enabled-override">{t("forwards.col_enabled")}</FieldLabel>
                <div className="flex h-8 items-center gap-2 text-sm">
                  <Switch
                    id="rule-enabled-override"
                    checked={
                      form.enabledOverride ?? (form.entry === "validation")
                    }
                    onCheckedChange={(v) => setForm((f) => ({ ...f, enabledOverride: v }))}
                    disabled={standby || adding}
                    aria-label={t("forwards.col_enabled")}
                  />
                  <span className="text-xs text-muted-foreground">
                    {form.entry === "validation"
                      ? entryLabel("validation")
                      : entryLabel("production")}
                  </span>
                </div>
              </Field>

              <DialogFooter>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setAddOpen(false)}
                  disabled={adding}
                >
                  {t("upstreams.cancel")}
                </Button>
                <Button type="submit" disabled={standby || adding}>
                  {adding ? <Spinner data-icon="inline-start" /> : null}
                  {t("forwards.add")}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>
    </>
  )

  if (embedded) return main

  return (
    <Page>
      <PageHeader title={t("forwards.title")} description={t("forwards.desc")} actions={headerActions} />
      {main}
    </Page>
  )
}
