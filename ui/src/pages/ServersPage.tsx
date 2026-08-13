import { useCallback, useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { PlusIcon, RocketIcon, ServerIcon, Trash2Icon } from "lucide-react"

import { EmptyState } from "@/components/layout/EmptyState"
import { newId } from "@/lib/id"
import { Page, PageHeader } from "@/components/layout/PageParts"
import { Badge } from "@/components/ui/badge"
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
import { Switch } from "@/components/ui/switch"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { useStandby } from "@/context/SessionContext"
import {
  ApiError,
  createUpstream,
  createUpstreamEntries,
  createUpstreamsBatch,
  deleteUpstream,
  getUpstreams,
  promoteUpstream,
  updateUpstream,
  type BatchUpstreamResult,
} from "@/lib/api"
import { ENTRIES, entryLabel, type EntryKind } from "@/lib/entry"
import type { Upstream, UpstreamLifecycle } from "@/lib/types"
import { tf } from "@/i18n"

const emptyForm = {
  name: "",
  address: "",
  tcp_port: "",
  udp_port: "",
  enabled: true,
}

type ResolvedUpstreams = {
  tcp: { port: number } | null
  udp: { port: number } | null
}

/** Port filled → protocol enabled. Empty = protocol off (no mirror placeholders). */
function resolveUpstreams(tcpPort: number, udpPort: number): ResolvedUpstreams | null {
  const wantTcp = tcpPort >= 1
  const wantUdp = udpPort >= 1
  if (!wantTcp && !wantUdp) return null
  return {
    tcp: wantTcp ? { port: tcpPort } : null,
    udp: wantUdp ? { port: udpPort } : null,
  }
}

function upstreamProtocols(upstream: Upstream): string[] {
  const protocols: string[] = []
  if (upstream.tcp?.port) protocols.push("TCP")
  if (upstream.udp?.port) protocols.push("UDP")
  return protocols
}

type EditDraft = {
  name: string
  address: string
  tcp_port: number
  udp_port: number
  enabled: boolean
}

type QuickRow = {
  id: string
  name: string
  address: string
  tcp_port: string
  udp_port: string
}

type QuickDefaults = {
  entries: EntryKind[]
  enable_production: boolean
}

const emptyQuickDefaults: QuickDefaults = {
  entries: ["production"],
  enable_production: false,
}

type EntryDraft = {
  upstream: string
  entry: EntryKind
  protocols: string[]
}

function newQuickRow(partial?: Partial<Omit<QuickRow, "id">>): QuickRow {
  return {
    id: newId(),
    name: "",
    address: "",
    tcp_port: "7777",
    udp_port: "7777",
    ...partial,
  }
}

/** Parse lines like `server-01 185.244.208.205:4301` or `server-01 10.0.0.1 7777`. */
function parseServerList(text: string): QuickRow[] {
  const rows: QuickRow[] = []
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim()
    if (!line || line.startsWith("#")) continue
    const parts = line.split(/\s+/)
    if (parts.length < 2) continue
    const name = parts[0]
    let address = parts[1]
    let port = "7777"
    const colon = address.lastIndexOf(":")
    if (colon > 0) {
      const host = address.slice(0, colon)
      const p = address.slice(colon + 1)
      if (host && /^\d+$/.test(p)) {
        address = host
        port = p
      }
    } else if (parts.length >= 3 && /^\d+$/.test(parts[2])) {
      port = parts[2]
    }
    rows.push(newQuickRow({ name, address, tcp_port: port, udp_port: port }))
  }
  return rows
}

export function ServersPage({ embedded = false }: { embedded?: boolean }) {
  const { t } = useTranslation()
  const standby = useStandby()
  const [upstreams, setUpstreams] = useState<Upstream[]>([])
  const [lifecycle, setLifecycle] = useState<Record<string, UpstreamLifecycle>>({})
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState(emptyForm)
  const [addOpen, setAddOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [quickOpen, setQuickOpen] = useState(false)
  const [quickDefaults, setQuickDefaults] = useState<QuickDefaults>(emptyQuickDefaults)
  const [quickRows, setQuickRows] = useState<QuickRow[]>(() => [newQuickRow()])
  const [quickPaste, setQuickPaste] = useState("")
  const [quicking, setQuicking] = useState(false)
  const [quickDone, setQuickDone] = useState<{
    succeeded: number
    failed: number
    results: BatchUpstreamResult[]
  } | null>(null)
  const [editDraft, setEditDraft] = useState<EditDraft | null>(null)
  const [saving, setSaving] = useState(false)
  const [toggling, setToggling] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [promoteTarget, setPromoteTarget] = useState<string | null>(null)
  const [promoting, setPromoting] = useState(false)
  const [entryDraft, setEntryDraft] = useState<EntryDraft | null>(null)
  const [addingEntry, setAddingEntry] = useState(false)

  const load = useCallback(async () => {
    try {
      const data = await getUpstreams()
      setUpstreams(data.upstreams)
      setLifecycle(data.lifecycle)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("common.toast_load_fail"))
    }
  }, [t])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (standby) return
    const upstreams = resolveUpstreams(Number(form.tcp_port) || 0, Number(form.udp_port) || 0)
    if (!upstreams) {
      toast.error(t("upstreams.need_ports"))
      return
    }
    setAdding(true)
    try {
      await createUpstream({
        name: form.name.trim(),
        address: form.address.trim(),
        tcp: upstreams.tcp,
        udp: upstreams.udp,
        enabled: form.enabled,
      })
      await load()
      setForm(emptyForm)
      setAddOpen(false)
      toast.success(tf("upstreams.toast_added", form.name.trim()))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setAdding(false)
    }
  }

  function resetQuickForm() {
    setQuickDefaults(emptyQuickDefaults)
    setQuickRows([newQuickRow()])
    setQuickPaste("")
    setQuickDone(null)
  }

  function applyQuickPaste() {
    const parsed = parseServerList(quickPaste)
    if (!parsed.length) {
      toast.error(t("upstreams.quick_paste_empty"))
      return
    }
    setQuickRows(parsed)
    toast.success(tf("upstreams.quick_paste_ok", parsed.length))
  }

  function toggleQuickEntry(entry: EntryKind, checked: boolean) {
    setQuickDefaults((f) => {
      const next = checked
        ? ENTRIES.filter((e) => f.entries.includes(e) || e === entry)
        : f.entries.filter((e) => e !== entry)
      return { ...f, entries: next }
    })
  }

  async function handleQuick(e: React.FormEvent) {
    e.preventDefault()
    if (standby) return
    const batchUpstreams = quickRows
      .map((row) => {
        const name = row.name.trim()
        const address = row.address.trim()
        const upstreams = resolveUpstreams(Number(row.tcp_port) || 0, Number(row.udp_port) || 0)
        if (!name || !address || !upstreams) return null
        return {
          name,
          address,
          tcp: upstreams.tcp,
          udp: upstreams.udp,
          enabled: true,
        }
      })
      .filter((s): s is NonNullable<typeof s> => s !== null)
    if (!batchUpstreams.length) {
      toast.error(t("upstreams.quick_need_rows"))
      return
    }
    setQuicking(true)
    try {
      const res = await createUpstreamsBatch({
        upstreams: batchUpstreams,
        entries: quickDefaults.entries.length ? quickDefaults.entries : undefined,
        enable_production: quickDefaults.enable_production || undefined,
      })
      await load()
      setQuickDone({
        succeeded: res.succeeded,
        failed: res.failed,
        results: res.results,
      })
      setQuickRows([newQuickRow()])
      setQuickPaste("")
      if (res.failed > 0 && res.succeeded > 0) {
        toast.message(tf("upstreams.quick_done_summary", res.succeeded, res.failed))
      } else if (res.succeeded === 0) {
        toast.error(tf("upstreams.quick_done_summary", res.succeeded, res.failed))
      }
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setQuicking(false)
    }
  }

  function startEdit(upstream: Upstream) {
    setEditDraft({
      name: upstream.name,
      address: upstream.address,
      tcp_port: upstream.tcp?.port ?? 0,
      udp_port: upstream.udp?.port ?? 0,
      enabled: upstream.enabled,
    })
  }

  function startAddEntry(upstream: Upstream) {
    const protocols = upstreamProtocols(upstream)
    setEntryDraft({
      upstream: upstream.name,
      entry: "validation",
      protocols: [...protocols],
    })
  }

  async function saveEdit(e: React.FormEvent) {
    e.preventDefault()
    if (!editDraft || standby) return
    const upstreams = resolveUpstreams(editDraft.tcp_port, editDraft.udp_port)
    if (!upstreams) {
      toast.error(t("upstreams.need_ports"))
      return
    }
    setSaving(true)
    try {
      await updateUpstream(editDraft.name, {
        address: editDraft.address,
        tcp: upstreams.tcp,
        udp: upstreams.udp,
        enabled: editDraft.enabled,
      })
      await load()
      toast.success(tf("upstreams.toast_saved", editDraft.name))
      setEditDraft(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setSaving(false)
    }
  }

  async function handleAddEntry(e: React.FormEvent) {
    e.preventDefault()
    if (!entryDraft || standby) return
    if (!entryDraft.protocols.length) {
      toast.error(t("upstreams.quick_need_proto"))
      return
    }
    setAddingEntry(true)
    try {
      const res = await createUpstreamEntries(entryDraft.upstream, {
        entry: entryDraft.entry,
        protocols: entryDraft.protocols,
      })
      await load()
      if (res.forwards?.length) {
        toast.success(
          tf(
            "servers.toast_entry_added",
            entryDraft.upstream,
            res.forwards.map((r) => r.name).join(", "),
          ),
        )
      } else {
        toast.message(tf("upstreams.toast_entry_none", entryDraft.upstream))
      }
      setEntryDraft(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setAddingEntry(false)
    }
  }

  async function toggleEnabled(upstream: Upstream, enabled: boolean) {
    if (standby) return
    setToggling(upstream.name)
    setUpstreams((prev) =>
      prev.map((s) => (s.name === upstream.name ? { ...s, enabled } : s)),
    )
    try {
      const res = await updateUpstream(upstream.name, {
        address: upstream.address,
        tcp: upstream.tcp ?? null,
        udp: upstream.udp ?? null,
        enabled,
      })
      if (!enabled && typeof res.cascaded_forwards === "number") {
        toast.success(tf("upstreams.toast_cascaded", upstream.name, res.cascaded_forwards))
      } else {
        toast.success(
          enabled
            ? tf("upstreams.toast_enabled", upstream.name)
            : tf("upstreams.toast_disabled", upstream.name),
        )
      }
    } catch (err) {
      setUpstreams((prev) =>
        prev.map((s) => (s.name === upstream.name ? { ...s, enabled: upstream.enabled } : s)),
      )
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setToggling(null)
    }
  }

  async function confirmDelete() {
    if (!deleteTarget || standby) return
    setDeleting(true)
    try {
      const res = await deleteUpstream(deleteTarget)
      await load()
      toast.success(tf("upstreams.toast_deleted", deleteTarget, res.removed_forwards ?? 0))
      setDeleteTarget(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setDeleting(false)
    }
  }

  async function confirmPromote() {
    if (!promoteTarget || standby) return
    setPromoting(true)
    try {
      const res = await promoteUpstream(promoteTarget)
      await load()
      toast.success(tf("upstreams.toast_promoted", promoteTarget, res.changed ?? 0))
      setPromoteTarget(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setPromoting(false)
    }
  }

  function lifecycleBadges(name: string) {
    const lc = lifecycle[name]
    if (!lc) return null
    return (
      <div className="flex flex-wrap gap-1">
        {lc.validation_forward_count > 0 ? (
          <Badge variant={lc.validation_enabled ? "default" : "secondary"} className="text-[10px]">
            {entryLabel("validation")}
            {lc.validation_enabled && lc.validation_ports.length
              ? ` ${lc.validation_ports.join(",")}`
              : ""}
          </Badge>
        ) : null}
        {lc.production_forward_count > 0 ? (
          <Badge variant={lc.production_enabled ? "default" : "outline"} className="text-[10px]">
            {entryLabel("production")}
            {lc.production_enabled && lc.production_ports.length
              ? ` ${lc.production_ports.join(",")}`
              : ""}
          </Badge>
        ) : null}
      </div>
    )
  }

  const headerActions = (
    <>
      <Button
        size="sm"
        variant="outline"
        disabled={standby}
        title={t("upstreams.add")}
        onClick={() => {
          setForm(emptyForm)
          setAddOpen(true)
        }}
      >
        <PlusIcon data-icon="inline-start" />
        {t("upstreams.add")}
      </Button>
      <Button
        size="sm"
        disabled={standby}
        title={t("upstreams.quick")}
        onClick={() => {
          resetQuickForm()
          setQuickOpen(true)
        }}
      >
        <RocketIcon data-icon="inline-start" />
        {t("upstreams.quick")}
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
              <TableHead>{t("upstreams.name")}</TableHead>
              <TableHead>{t("upstreams.address")}</TableHead>
              <TableHead>{t("upstreams.tcp")}</TableHead>
              <TableHead>{t("upstreams.udp")}</TableHead>
              <TableHead>{t("upstreams.enabled")}</TableHead>
              <TableHead>{t("upstreams.rule_shortcuts")}</TableHead>
              <TableHead className="text-right">{t("shell.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 4 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 7 }).map((__, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-4 w-full" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : upstreams.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={7} className="p-2">
                  <EmptyState
                    icon={ServerIcon}
                    title={t("upstreams.empty")}
                    description={t("upstreams.empty_hint")}
                  />
                </TableCell>
              </TableRow>
            ) : (
              upstreams.map((srv) => {
                const tcpPort = srv.tcp?.port
                const udpPort = srv.udp?.port
                return (
                  <TableRow key={srv.name}>
                    <TableCell className="font-medium">{srv.name}</TableCell>
                    <TableCell>{srv.address}</TableCell>
                    <TableCell>{tcpPort ?? "—"}</TableCell>
                    <TableCell>{udpPort ?? "—"}</TableCell>
                    <TableCell>
                      <Switch
                        checked={srv.enabled}
                        disabled={standby || toggling === srv.name}
                        onCheckedChange={(v) => toggleEnabled(srv, v)}
                        aria-label={t("upstreams.enabled")}
                        title={t("upstreams.enabled")}
                      />
                    </TableCell>
                    <TableCell>{lifecycleBadges(srv.name)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex flex-wrap justify-end gap-1">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => startEdit(srv)}
                          disabled={standby}
                        >
                          {t("upstreams.edit")}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => startAddEntry(srv)}
                          disabled={standby || !upstreamProtocols(srv).length}
                          title={t("upstreams.add_entry")}
                        >
                          {t("upstreams.add_entry")}
                        </Button>
                        <Button
                          size="sm"
                          variant="caution"
                          onClick={() => setPromoteTarget(srv.name)}
                          disabled={standby}
                          title={t("upstreams.promote_hint")}
                        >
                          {t("upstreams.promote")}
                        </Button>
                        <Button
                          size="sm"
                          variant="destructive"
                          onClick={() => setDeleteTarget(srv.name)}
                          disabled={standby}
                        >
                          {t("upstreams.delete")}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog
        open={quickOpen}
        onOpenChange={(open) => {
          if (!quicking) {
            setQuickOpen(open)
            if (!open) resetQuickForm()
          }
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          {quickDone ? (
            <>
              <DialogHeader>
                <DialogTitle>{t("upstreams.quick_done_title")}</DialogTitle>
                <DialogDescription>
                  {tf("upstreams.quick_done_summary", quickDone.succeeded, quickDone.failed)}.{" "}
                  {t("upstreams.quick_done_body")}
                </DialogDescription>
              </DialogHeader>
              <div className="flex max-h-[50vh] flex-col gap-3 overflow-y-auto text-sm">
                {quickDone.results.map((item) => (
                  <div
                    key={`${item.name}-${item.ok ? "ok" : "err"}`}
                    className="rounded-md border border-border/60 bg-muted/30 p-3"
                  >
                    <p className="font-medium">
                      {item.name || "—"}
                      {!item.ok ? (
                        <span className="ml-2 text-xs font-normal text-destructive">
                          {t("upstreams.quick_failed")}
                        </span>
                      ) : null}
                    </p>
                    {item.ok && item.forwards?.length ? (
                      <ul className="mt-2 space-y-1 text-xs">
                        <li className="mb-1 text-muted-foreground">{t("upstreams.quick_created")}</li>
                        {item.forwards.map((fwd) => (
                          <li key={fwd.name} className="flex flex-wrap gap-x-2 gap-y-0.5 font-mono">
                            <span>{fwd.name}</span>
                            <span className="text-muted-foreground">
                              :{fwd.listen_port} {fwd.protocol} · {entryLabel(fwd.entry)}
                              {fwd.enabled ? "" : " · off"}
                            </span>
                          </li>
                        ))}
                      </ul>
                    ) : null}
                    {!item.ok && item.error ? (
                      <p className="mt-1 text-xs text-destructive">{item.error}</p>
                    ) : null}
                  </div>
                ))}
              </div>
              <DialogFooter className="flex-col gap-2 sm:flex-row sm:justify-end">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => {
                    setQuickOpen(false)
                    resetQuickForm()
                  }}
                >
                  {t("upstreams.quick_close")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={quickDone.succeeded === 0}
                  render={
                    <Link
                      to="/forwards"
                      onClick={() => {
                        setQuickOpen(false)
                        resetQuickForm()
                      }}
                    />
                  }
                >
                  {t("upstreams.quick_goto_forwards")}
                </Button>
                <Button
                  render={
                    <Link
                      to="/apply"
                      onClick={() => {
                        setQuickOpen(false)
                        resetQuickForm()
                      }}
                    />
                  }
                >
                  {t("upstreams.quick_goto_apply")}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>{t("upstreams.quick_heading")}</DialogTitle>
                <DialogDescription>{t("upstreams.quick_description")}</DialogDescription>
              </DialogHeader>
              <form onSubmit={handleQuick} className="flex flex-col gap-4">
                <Field>
                  <FieldLabel htmlFor="quick-paste">{t("upstreams.quick_paste")}</FieldLabel>
                  <Textarea
                    id="quick-paste"
                    value={quickPaste}
                    onChange={(e) => setQuickPaste(e.target.value)}
                    disabled={standby || quicking}
                    placeholder={"server-01 185.244.208.205:4301\nserver-02 185.244.208.206:4301"}
                    className="min-h-20 font-mono text-xs"
                  />
                  <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                    <p className="text-xs text-muted-foreground">{t("upstreams.quick_paste_hint")}</p>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={standby || quicking || !quickPaste.trim()}
                      onClick={applyQuickPaste}
                    >
                      {t("upstreams.quick_paste_apply")}
                    </Button>
                  </div>
                </Field>

                <div className="flex flex-col gap-2">
                  {quickRows.map((row, idx) => (
                    <div
                      key={row.id}
                      className="grid grid-cols-[1fr_1fr_4.5rem_4.5rem_auto] items-end gap-2"
                    >
                      <Field>
                        {idx === 0 ? (
                          <FieldLabel htmlFor={`quick-name-${row.id}`}>{t("upstreams.alias")}</FieldLabel>
                        ) : null}
                        <Input
                          id={`quick-name-${row.id}`}
                          value={row.name}
                          onChange={(e) =>
                            setQuickRows((rows) =>
                              rows.map((r) =>
                                r.id === row.id ? { ...r, name: e.target.value } : r,
                              ),
                            )
                          }
                          disabled={standby || quicking}
                          placeholder="server-03"
                        />
                      </Field>
                      <Field>
                        {idx === 0 ? (
                          <FieldLabel htmlFor={`quick-addr-${row.id}`}>
                            {t("upstreams.address")}
                          </FieldLabel>
                        ) : null}
                        <Input
                          id={`quick-addr-${row.id}`}
                          value={row.address}
                          onChange={(e) =>
                            setQuickRows((rows) =>
                              rows.map((r) =>
                                r.id === row.id ? { ...r, address: e.target.value } : r,
                              ),
                            )
                          }
                          disabled={standby || quicking}
                          placeholder="10.0.0.13"
                        />
                      </Field>
                      <Field>
                        {idx === 0 ? (
                          <FieldLabel htmlFor={`quick-tcp-${row.id}`}>{t("upstreams.tcp")}</FieldLabel>
                        ) : null}
                        <Input
                          id={`quick-tcp-${row.id}`}
                          type="number"
                          value={row.tcp_port}
                          onChange={(e) =>
                            setQuickRows((rows) =>
                              rows.map((r) =>
                                r.id === row.id ? { ...r, tcp_port: e.target.value } : r,
                              ),
                            )
                          }
                          disabled={standby || quicking}
                          placeholder="—"
                        />
                      </Field>
                      <Field>
                        {idx === 0 ? (
                          <FieldLabel htmlFor={`quick-udp-${row.id}`}>{t("upstreams.udp")}</FieldLabel>
                        ) : null}
                        <Input
                          id={`quick-udp-${row.id}`}
                          type="number"
                          value={row.udp_port}
                          onChange={(e) =>
                            setQuickRows((rows) =>
                              rows.map((r) =>
                                r.id === row.id ? { ...r, udp_port: e.target.value } : r,
                              ),
                            )
                          }
                          disabled={standby || quicking}
                          placeholder="—"
                        />
                      </Field>
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        className="mb-0.5"
                        disabled={standby || quicking || quickRows.length <= 1}
                        title={t("upstreams.quick_remove_row")}
                        onClick={() =>
                          setQuickRows((rows) => rows.filter((r) => r.id !== row.id))
                        }
                      >
                        <Trash2Icon />
                      </Button>
                    </div>
                  ))}
                  <p className="text-xs text-muted-foreground">{t("upstreams.port_enable_hint")}</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="self-start"
                    disabled={standby || quicking}
                    onClick={() => setQuickRows((rows) => [...rows, newQuickRow()])}
                  >
                    <PlusIcon data-icon="inline-start" />
                    {t("upstreams.quick_add_row")}
                  </Button>
                </div>

                <FieldGroup className="grid gap-3 sm:grid-cols-2">
                  <Field>
                    <FieldLabel>{t("upstreams.quick_entries")}</FieldLabel>
                    <div className="flex flex-col gap-2 text-sm">
                      <label className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={quickDefaults.entries.includes("production")}
                          onChange={(e) => toggleQuickEntry("production", e.target.checked)}
                          disabled={standby || quicking}
                        />
                        {t("upstreams.quick_entry_production")}
                      </label>
                      <label className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={quickDefaults.entries.includes("validation")}
                          onChange={(e) => toggleQuickEntry("validation", e.target.checked)}
                          disabled={standby || quicking}
                        />
                        {t("upstreams.quick_entry_validation")}
                      </label>
                    </div>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="quick-enable-prod">
                      {t("upstreams.quick_enable_production")}
                    </FieldLabel>
                    <div className="flex h-8 items-center">
                      <Switch
                        id="quick-enable-prod"
                        checked={quickDefaults.enable_production}
                        onCheckedChange={(v) =>
                          setQuickDefaults((f) => ({ ...f, enable_production: v }))
                        }
                        disabled={
                          standby || quicking || !quickDefaults.entries.includes("production")
                        }
                        aria-label={t("upstreams.quick_enable_production")}
                      />
                    </div>
                  </Field>
                </FieldGroup>
                <DialogFooter>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => setQuickOpen(false)}
                    disabled={quicking}
                  >
                    {t("upstreams.cancel")}
                  </Button>
                  <Button type="submit" disabled={standby || quicking}>
                    {quicking ? <Spinner data-icon="inline-start" /> : null}
                    {t("upstreams.quick_submit")}
                  </Button>
                </DialogFooter>
              </form>
            </>
          )}
        </DialogContent>
      </Dialog>

      <Dialog
        open={addOpen}
        onOpenChange={(open) => {
          if (!adding) {
            setAddOpen(open)
            if (!open) setForm(emptyForm)
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("upstreams.add_heading")}</DialogTitle>
            <DialogDescription>{t("upstreams.add_description")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={handleAdd} className="flex flex-col gap-4">
            <FieldGroup className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="srv-name">{t("upstreams.name")}</FieldLabel>
                <Input
                  id="srv-name"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  disabled={standby || adding}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="srv-addr">{t("upstreams.address")}</FieldLabel>
                <Input
                  id="srv-addr"
                  value={form.address}
                  onChange={(e) => setForm((f) => ({ ...f, address: e.target.value }))}
                  disabled={standby || adding}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="srv-tcp">{t("upstreams.tcp_upstream")}</FieldLabel>
                <Input
                  id="srv-tcp"
                  type="number"
                  value={form.tcp_port}
                  onChange={(e) => setForm((f) => ({ ...f, tcp_port: e.target.value }))}
                  disabled={standby || adding}
                  placeholder={t("upstreams.port_optional")}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="srv-udp">{t("upstreams.udp_upstream")}</FieldLabel>
                <Input
                  id="srv-udp"
                  type="number"
                  value={form.udp_port}
                  onChange={(e) => setForm((f) => ({ ...f, udp_port: e.target.value }))}
                  disabled={standby || adding}
                  placeholder={t("upstreams.port_optional")}
                />
              </Field>
              <p className="sm:col-span-2 text-xs text-muted-foreground">
                {t("upstreams.port_enable_hint")}
              </p>
              {!form.tcp_port.trim() ? (
                <p className="sm:col-span-2 text-xs text-muted-foreground">
                  {t("upstreams.health_no_tcp")}
                </p>
              ) : null}
              <Field>
                <FieldLabel htmlFor="srv-enabled">{t("upstreams.enabled")}</FieldLabel>
                <div className="flex h-8 items-center">
                  <Switch
                    id="srv-enabled"
                    checked={form.enabled}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
                    disabled={standby || adding}
                    aria-label={t("upstreams.enabled")}
                  />
                </div>
              </Field>
            </FieldGroup>
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
                {t("upstreams.add")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!editDraft}
        onOpenChange={(open) => {
          if (!saving && !open) setEditDraft(null)
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {t("upstreams.edit")}
              {editDraft ? ` — ${editDraft.name}` : ""}
            </DialogTitle>
          </DialogHeader>
          {editDraft ? (
            <form onSubmit={saveEdit} className="flex flex-col gap-4">
              <FieldGroup className="grid gap-3 sm:grid-cols-2">
                <Field className="sm:col-span-2">
                  <FieldLabel htmlFor="edit-addr">{t("upstreams.address")}</FieldLabel>
                  <Input
                    id="edit-addr"
                    value={editDraft.address}
                    onChange={(e) =>
                      setEditDraft((d) => (d ? { ...d, address: e.target.value } : d))
                    }
                    disabled={standby || saving}
                    required
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="edit-tcp">{t("upstreams.tcp_upstream")}</FieldLabel>
                  <Input
                    id="edit-tcp"
                    type="number"
                    value={editDraft.tcp_port || ""}
                    onChange={(e) =>
                      setEditDraft((d) =>
                        d ? { ...d, tcp_port: Number(e.target.value) || 0 } : d,
                      )
                    }
                    disabled={standby || saving}
                    placeholder={t("upstreams.port_optional")}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="edit-udp">{t("upstreams.udp_upstream")}</FieldLabel>
                  <Input
                    id="edit-udp"
                    type="number"
                    value={editDraft.udp_port || ""}
                    onChange={(e) =>
                      setEditDraft((d) =>
                        d ? { ...d, udp_port: Number(e.target.value) || 0 } : d,
                      )
                    }
                    disabled={standby || saving}
                    placeholder={t("upstreams.port_optional")}
                  />
                </Field>
                <p className="sm:col-span-2 text-xs text-muted-foreground">
                  {t("upstreams.port_enable_hint")}
                </p>
                {!editDraft.tcp_port ? (
                  <p className="sm:col-span-2 text-xs text-muted-foreground">
                    {t("upstreams.health_no_tcp")}
                  </p>
                ) : null}
                <Field>
                  <FieldLabel htmlFor="edit-enabled">{t("upstreams.enabled")}</FieldLabel>
                  <div className="flex h-8 items-center">
                    <Switch
                      id="edit-enabled"
                      checked={editDraft.enabled}
                      onCheckedChange={(v) =>
                        setEditDraft((d) => (d ? { ...d, enabled: v } : d))
                      }
                      disabled={standby || saving}
                      aria-label={t("upstreams.enabled")}
                    />
                  </div>
                </Field>
              </FieldGroup>
              <DialogFooter>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setEditDraft(null)}
                  disabled={saving}
                >
                  {t("upstreams.cancel")}
                </Button>
                <Button type="submit" disabled={standby || saving}>
                  {saving ? <Spinner data-icon="inline-start" /> : null}
                  {t("upstreams.save")}
                </Button>
              </DialogFooter>
            </form>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!entryDraft}
        onOpenChange={(open) => {
          if (!addingEntry && !open) setEntryDraft(null)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("upstreams.add_entry_heading")}</DialogTitle>
            <DialogDescription>
              {entryDraft
                ? `${t("upstreams.add_entry_description")} (${entryDraft.upstream})`
                : t("upstreams.add_entry_description")}
            </DialogDescription>
          </DialogHeader>
          {entryDraft ? (
            <form onSubmit={handleAddEntry} className="flex flex-col gap-4">
              <Field>
                <FieldLabel>{t("upstreams.entry_type")}</FieldLabel>
                <div className="flex h-8 items-center gap-3 text-sm">
                  {ENTRIES.map((entry) => (
                    <label key={entry} className="flex items-center gap-1.5">
                      <input
                        type="radio"
                        name="add-entry-type"
                        checked={entryDraft.entry === entry}
                        onChange={() => setEntryDraft((d) => (d ? { ...d, entry } : d))}
                        disabled={standby || addingEntry}
                      />
                      {entryLabel(entry)}
                    </label>
                  ))}
                </div>
              </Field>
              <Field>
                <FieldLabel>{t("upstreams.quick_protocols")}</FieldLabel>
                <div className="flex h-8 items-center gap-3 text-sm">
                  {(["TCP", "UDP"] as const).map((proto) => {
                    const available = (() => {
                      const srv = upstreams.find((s) => s.name === entryDraft.upstream)
                      if (!srv) return false
                      return proto === "TCP" ? !!srv.tcp?.port : !!srv.udp?.port
                    })()
                    return (
                      <label key={proto} className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={entryDraft.protocols.includes(proto)}
                          onChange={(e) =>
                            setEntryDraft((d) => {
                              if (!d) return d
                              const protocols = e.target.checked
                                ? [...d.protocols.filter((p) => p !== proto), proto]
                                : d.protocols.filter((p) => p !== proto)
                              return { ...d, protocols }
                            })
                          }
                          disabled={standby || addingEntry || !available}
                        />
                        {proto}
                      </label>
                    )
                  })}
                </div>
              </Field>
              <DialogFooter>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setEntryDraft(null)}
                  disabled={addingEntry}
                >
                  {t("upstreams.cancel")}
                </Button>
                <Button type="submit" disabled={standby || addingEntry}>
                  {addingEntry ? <Spinner data-icon="inline-start" /> : null}
                  {t("upstreams.add_entry")}
                </Button>
              </DialogFooter>
            </form>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={(open) => !open && !deleting && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("upstreams.confirm")}</DialogTitle>
            <DialogDescription>
              {deleteTarget ? tf("upstreams.confirm_body", deleteTarget) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)} disabled={deleting}>
              {t("upstreams.cancel")}
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={standby || deleting}>
              {deleting ? <Spinner data-icon="inline-start" /> : null}
              {t("upstreams.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!promoteTarget} onOpenChange={(open) => !open && !promoting && setPromoteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("upstreams.promote_title")}</DialogTitle>
            <DialogDescription>
              {promoteTarget ? tf("upstreams.promote_confirm", promoteTarget) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPromoteTarget(null)} disabled={promoting}>
              {t("upstreams.cancel")}
            </Button>
            <Button variant="caution" onClick={confirmPromote} disabled={standby || promoting}>
              {promoting ? <Spinner data-icon="inline-start" /> : null}
              {t("upstreams.promote")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )

  if (embedded) {
    return <div className="flex flex-col gap-3">{main}</div>
  }

  return (
    <Page>
      <PageHeader title={t("upstreams.title")} description={t("upstreams.desc")} actions={headerActions} />
      {main}
    </Page>
  )
}
