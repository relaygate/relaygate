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
  createServer,
  createServerEntries,
  createServersBatch,
  deleteServer,
  getServers,
  promoteServer,
  updateServer,
  type BatchServerResult,
} from "@/lib/api"
import { ENTRIES, entryLabel, type EntryKind } from "@/lib/entry"
import type { Server, ServerLifecycle } from "@/lib/types"
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

function serverProtocols(server: Server): string[] {
  const protocols: string[] = []
  if (server.tcp?.port) protocols.push("TCP")
  if (server.udp?.port) protocols.push("UDP")
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
  server: string
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
  const [servers, setServers] = useState<Server[]>([])
  const [lifecycle, setLifecycle] = useState<Record<string, ServerLifecycle>>({})
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
    results: BatchServerResult[]
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
      const data = await getServers()
      setServers(data.servers)
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
      toast.error(t("servers.need_ports"))
      return
    }
    setAdding(true)
    try {
      await createServer({
        name: form.name.trim(),
        address: form.address.trim(),
        tcp: upstreams.tcp,
        udp: upstreams.udp,
        enabled: form.enabled,
      })
      await load()
      setForm(emptyForm)
      setAddOpen(false)
      toast.success(tf("servers.toast_added", form.name.trim()))
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
      toast.error(t("servers.quick_paste_empty"))
      return
    }
    setQuickRows(parsed)
    toast.success(tf("servers.quick_paste_ok", parsed.length))
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
    const servers = quickRows
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
    if (!servers.length) {
      toast.error(t("servers.quick_need_rows"))
      return
    }
    setQuicking(true)
    try {
      const res = await createServersBatch({
        servers,
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
        toast.message(tf("servers.quick_done_summary", res.succeeded, res.failed))
      } else if (res.succeeded === 0) {
        toast.error(tf("servers.quick_done_summary", res.succeeded, res.failed))
      }
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setQuicking(false)
    }
  }

  function startEdit(server: Server) {
    setEditDraft({
      name: server.name,
      address: server.address,
      tcp_port: server.tcp?.port ?? 0,
      udp_port: server.udp?.port ?? 0,
      enabled: server.enabled,
    })
  }

  function startAddEntry(server: Server) {
    const protocols = serverProtocols(server)
    setEntryDraft({
      server: server.name,
      entry: "validation",
      protocols: [...protocols],
    })
  }

  async function saveEdit(e: React.FormEvent) {
    e.preventDefault()
    if (!editDraft || standby) return
    const upstreams = resolveUpstreams(editDraft.tcp_port, editDraft.udp_port)
    if (!upstreams) {
      toast.error(t("servers.need_ports"))
      return
    }
    setSaving(true)
    try {
      await updateServer(editDraft.name, {
        address: editDraft.address,
        tcp: upstreams.tcp,
        udp: upstreams.udp,
        enabled: editDraft.enabled,
      })
      await load()
      toast.success(tf("servers.toast_saved", editDraft.name))
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
      toast.error(t("servers.quick_need_proto"))
      return
    }
    setAddingEntry(true)
    try {
      const res = await createServerEntries(entryDraft.server, {
        entry: entryDraft.entry,
        protocols: entryDraft.protocols,
      })
      await load()
      if (res.rules?.length) {
        toast.success(
          tf(
            "servers.toast_entry_added",
            entryDraft.server,
            res.rules.map((r) => r.name).join(", "),
          ),
        )
      } else {
        toast.message(tf("servers.toast_entry_none", entryDraft.server))
      }
      setEntryDraft(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setAddingEntry(false)
    }
  }

  async function toggleEnabled(server: Server, enabled: boolean) {
    if (standby) return
    setToggling(server.name)
    setServers((prev) =>
      prev.map((s) => (s.name === server.name ? { ...s, enabled } : s)),
    )
    try {
      const res = await updateServer(server.name, {
        address: server.address,
        tcp: server.tcp ?? null,
        udp: server.udp ?? null,
        enabled,
      })
      if (!enabled && typeof res.cascaded_rules === "number") {
        toast.success(tf("servers.toast_cascaded", server.name, res.cascaded_rules))
      } else {
        toast.success(
          enabled
            ? tf("servers.toast_enabled", server.name)
            : tf("servers.toast_disabled", server.name),
        )
      }
    } catch (err) {
      setServers((prev) =>
        prev.map((s) => (s.name === server.name ? { ...s, enabled: server.enabled } : s)),
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
      const res = await deleteServer(deleteTarget)
      await load()
      toast.success(tf("servers.toast_deleted", deleteTarget, res.removed_rules ?? 0))
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
      const res = await promoteServer(promoteTarget)
      await load()
      toast.success(tf("servers.toast_promoted", promoteTarget, res.changed ?? 0))
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
        {lc.validation_rule_count > 0 ? (
          <Badge variant={lc.validation_enabled ? "default" : "secondary"} className="text-[10px]">
            {entryLabel("validation")}
            {lc.validation_enabled && lc.validation_ports.length
              ? ` ${lc.validation_ports.join(",")}`
              : ""}
          </Badge>
        ) : null}
        {lc.production_rule_count > 0 ? (
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
        title={t("servers.add")}
        onClick={() => {
          setForm(emptyForm)
          setAddOpen(true)
        }}
      >
        <PlusIcon data-icon="inline-start" />
        {t("servers.add")}
      </Button>
      <Button
        size="sm"
        disabled={standby}
        title={t("servers.quick")}
        onClick={() => {
          resetQuickForm()
          setQuickOpen(true)
        }}
      >
        <RocketIcon data-icon="inline-start" />
        {t("servers.quick")}
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
              <TableHead>{t("servers.name")}</TableHead>
              <TableHead>{t("servers.address")}</TableHead>
              <TableHead>{t("servers.tcp")}</TableHead>
              <TableHead>{t("servers.udp")}</TableHead>
              <TableHead>{t("servers.enabled")}</TableHead>
              <TableHead>{t("servers.rule_shortcuts")}</TableHead>
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
            ) : servers.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={7} className="p-2">
                  <EmptyState
                    icon={ServerIcon}
                    title={t("servers.empty")}
                    description={t("servers.empty_hint")}
                  />
                </TableCell>
              </TableRow>
            ) : (
              servers.map((srv) => {
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
                        aria-label={t("servers.enabled")}
                        title={t("servers.enabled")}
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
                          {t("servers.edit")}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => startAddEntry(srv)}
                          disabled={standby || !serverProtocols(srv).length}
                          title={t("servers.add_entry")}
                        >
                          {t("servers.add_entry")}
                        </Button>
                        <Button
                          size="sm"
                          variant="caution"
                          onClick={() => setPromoteTarget(srv.name)}
                          disabled={standby}
                          title={t("servers.promote_hint")}
                        >
                          {t("servers.promote")}
                        </Button>
                        <Button
                          size="sm"
                          variant="destructive"
                          onClick={() => setDeleteTarget(srv.name)}
                          disabled={standby}
                        >
                          {t("servers.delete")}
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
                <DialogTitle>{t("servers.quick_done_title")}</DialogTitle>
                <DialogDescription>
                  {tf("servers.quick_done_summary", quickDone.succeeded, quickDone.failed)}.{" "}
                  {t("servers.quick_done_body")}
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
                          {t("servers.quick_failed")}
                        </span>
                      ) : null}
                    </p>
                    {item.ok && item.rules?.length ? (
                      <ul className="mt-2 space-y-1 text-xs">
                        <li className="mb-1 text-muted-foreground">{t("servers.quick_created")}</li>
                        {item.rules.map((rule) => (
                          <li key={rule.name} className="flex flex-wrap gap-x-2 gap-y-0.5 font-mono">
                            <span>{rule.name}</span>
                            <span className="text-muted-foreground">
                              :{rule.listen_port} {rule.protocol} · {entryLabel(rule.entry)}
                              {rule.enabled ? "" : " · off"}
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
                  {t("servers.quick_close")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={quickDone.succeeded === 0}
                  render={
                    <Link
                      to="/rules"
                      onClick={() => {
                        setQuickOpen(false)
                        resetQuickForm()
                      }}
                    />
                  }
                >
                  {t("servers.quick_goto_rules")}
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
                  {t("servers.quick_goto_apply")}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>{t("servers.quick_heading")}</DialogTitle>
                <DialogDescription>{t("servers.quick_description")}</DialogDescription>
              </DialogHeader>
              <form onSubmit={handleQuick} className="flex flex-col gap-4">
                <Field>
                  <FieldLabel htmlFor="quick-paste">{t("servers.quick_paste")}</FieldLabel>
                  <Textarea
                    id="quick-paste"
                    value={quickPaste}
                    onChange={(e) => setQuickPaste(e.target.value)}
                    disabled={standby || quicking}
                    placeholder={"server-01 185.244.208.205:4301\nserver-02 185.244.208.206:4301"}
                    className="min-h-20 font-mono text-xs"
                  />
                  <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                    <p className="text-xs text-muted-foreground">{t("servers.quick_paste_hint")}</p>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={standby || quicking || !quickPaste.trim()}
                      onClick={applyQuickPaste}
                    >
                      {t("servers.quick_paste_apply")}
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
                          <FieldLabel htmlFor={`quick-name-${row.id}`}>{t("servers.alias")}</FieldLabel>
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
                            {t("servers.address")}
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
                          <FieldLabel htmlFor={`quick-tcp-${row.id}`}>{t("servers.tcp")}</FieldLabel>
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
                          <FieldLabel htmlFor={`quick-udp-${row.id}`}>{t("servers.udp")}</FieldLabel>
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
                        title={t("servers.quick_remove_row")}
                        onClick={() =>
                          setQuickRows((rows) => rows.filter((r) => r.id !== row.id))
                        }
                      >
                        <Trash2Icon />
                      </Button>
                    </div>
                  ))}
                  <p className="text-xs text-muted-foreground">{t("servers.port_enable_hint")}</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="self-start"
                    disabled={standby || quicking}
                    onClick={() => setQuickRows((rows) => [...rows, newQuickRow()])}
                  >
                    <PlusIcon data-icon="inline-start" />
                    {t("servers.quick_add_row")}
                  </Button>
                </div>

                <FieldGroup className="grid gap-3 sm:grid-cols-2">
                  <Field>
                    <FieldLabel>{t("servers.quick_entries")}</FieldLabel>
                    <div className="flex flex-col gap-2 text-sm">
                      <label className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={quickDefaults.entries.includes("production")}
                          onChange={(e) => toggleQuickEntry("production", e.target.checked)}
                          disabled={standby || quicking}
                        />
                        {t("servers.quick_entry_production")}
                      </label>
                      <label className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={quickDefaults.entries.includes("validation")}
                          onChange={(e) => toggleQuickEntry("validation", e.target.checked)}
                          disabled={standby || quicking}
                        />
                        {t("servers.quick_entry_validation")}
                      </label>
                    </div>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="quick-enable-prod">
                      {t("servers.quick_enable_production")}
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
                        aria-label={t("servers.quick_enable_production")}
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
                    {t("servers.cancel")}
                  </Button>
                  <Button type="submit" disabled={standby || quicking}>
                    {quicking ? <Spinner data-icon="inline-start" /> : null}
                    {t("servers.quick_submit")}
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
            <DialogTitle>{t("servers.add_heading")}</DialogTitle>
            <DialogDescription>{t("servers.add_description")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={handleAdd} className="flex flex-col gap-4">
            <FieldGroup className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="srv-name">{t("servers.name")}</FieldLabel>
                <Input
                  id="srv-name"
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  disabled={standby || adding}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="srv-addr">{t("servers.address")}</FieldLabel>
                <Input
                  id="srv-addr"
                  value={form.address}
                  onChange={(e) => setForm((f) => ({ ...f, address: e.target.value }))}
                  disabled={standby || adding}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="srv-tcp">{t("servers.tcp_upstream")}</FieldLabel>
                <Input
                  id="srv-tcp"
                  type="number"
                  value={form.tcp_port}
                  onChange={(e) => setForm((f) => ({ ...f, tcp_port: e.target.value }))}
                  disabled={standby || adding}
                  placeholder={t("servers.port_optional")}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="srv-udp">{t("servers.udp_upstream")}</FieldLabel>
                <Input
                  id="srv-udp"
                  type="number"
                  value={form.udp_port}
                  onChange={(e) => setForm((f) => ({ ...f, udp_port: e.target.value }))}
                  disabled={standby || adding}
                  placeholder={t("servers.port_optional")}
                />
              </Field>
              <p className="sm:col-span-2 text-xs text-muted-foreground">
                {t("servers.port_enable_hint")}
              </p>
              {!form.tcp_port.trim() ? (
                <p className="sm:col-span-2 text-xs text-muted-foreground">
                  {t("servers.health_no_tcp")}
                </p>
              ) : null}
              <Field>
                <FieldLabel htmlFor="srv-enabled">{t("servers.enabled")}</FieldLabel>
                <div className="flex h-8 items-center">
                  <Switch
                    id="srv-enabled"
                    checked={form.enabled}
                    onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
                    disabled={standby || adding}
                    aria-label={t("servers.enabled")}
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
                {t("servers.cancel")}
              </Button>
              <Button type="submit" disabled={standby || adding}>
                {adding ? <Spinner data-icon="inline-start" /> : null}
                {t("servers.add")}
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
              {t("servers.edit")}
              {editDraft ? ` — ${editDraft.name}` : ""}
            </DialogTitle>
          </DialogHeader>
          {editDraft ? (
            <form onSubmit={saveEdit} className="flex flex-col gap-4">
              <FieldGroup className="grid gap-3 sm:grid-cols-2">
                <Field className="sm:col-span-2">
                  <FieldLabel htmlFor="edit-addr">{t("servers.address")}</FieldLabel>
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
                  <FieldLabel htmlFor="edit-tcp">{t("servers.tcp_upstream")}</FieldLabel>
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
                    placeholder={t("servers.port_optional")}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="edit-udp">{t("servers.udp_upstream")}</FieldLabel>
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
                    placeholder={t("servers.port_optional")}
                  />
                </Field>
                <p className="sm:col-span-2 text-xs text-muted-foreground">
                  {t("servers.port_enable_hint")}
                </p>
                {!editDraft.tcp_port ? (
                  <p className="sm:col-span-2 text-xs text-muted-foreground">
                    {t("servers.health_no_tcp")}
                  </p>
                ) : null}
                <Field>
                  <FieldLabel htmlFor="edit-enabled">{t("servers.enabled")}</FieldLabel>
                  <div className="flex h-8 items-center">
                    <Switch
                      id="edit-enabled"
                      checked={editDraft.enabled}
                      onCheckedChange={(v) =>
                        setEditDraft((d) => (d ? { ...d, enabled: v } : d))
                      }
                      disabled={standby || saving}
                      aria-label={t("servers.enabled")}
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
                  {t("servers.cancel")}
                </Button>
                <Button type="submit" disabled={standby || saving}>
                  {saving ? <Spinner data-icon="inline-start" /> : null}
                  {t("servers.save")}
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
            <DialogTitle>{t("servers.add_entry_heading")}</DialogTitle>
            <DialogDescription>
              {entryDraft
                ? `${t("servers.add_entry_description")} (${entryDraft.server})`
                : t("servers.add_entry_description")}
            </DialogDescription>
          </DialogHeader>
          {entryDraft ? (
            <form onSubmit={handleAddEntry} className="flex flex-col gap-4">
              <Field>
                <FieldLabel>{t("servers.entry_type")}</FieldLabel>
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
                <FieldLabel>{t("servers.quick_protocols")}</FieldLabel>
                <div className="flex h-8 items-center gap-3 text-sm">
                  {(["TCP", "UDP"] as const).map((proto) => {
                    const available = (() => {
                      const srv = servers.find((s) => s.name === entryDraft.server)
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
                  {t("servers.cancel")}
                </Button>
                <Button type="submit" disabled={standby || addingEntry}>
                  {addingEntry ? <Spinner data-icon="inline-start" /> : null}
                  {t("servers.add_entry")}
                </Button>
              </DialogFooter>
            </form>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={(open) => !open && !deleting && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("servers.confirm")}</DialogTitle>
            <DialogDescription>
              {deleteTarget ? tf("servers.confirm_body", deleteTarget) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)} disabled={deleting}>
              {t("servers.cancel")}
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={standby || deleting}>
              {deleting ? <Spinner data-icon="inline-start" /> : null}
              {t("servers.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!promoteTarget} onOpenChange={(open) => !open && !promoting && setPromoteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("servers.promote_title")}</DialogTitle>
            <DialogDescription>
              {promoteTarget ? tf("servers.promote_confirm", promoteTarget) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPromoteTarget(null)} disabled={promoting}>
              {t("servers.cancel")}
            </Button>
            <Button variant="caution" onClick={confirmPromote} disabled={standby || promoting}>
              {promoting ? <Spinner data-icon="inline-start" /> : null}
              {t("servers.promote")}
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
      <PageHeader title={t("servers.title")} description={t("servers.desc")} actions={headerActions} />
      {main}
    </Page>
  )
}
