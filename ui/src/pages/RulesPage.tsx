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
  createRule,
  exportPortMapCSV,
  getRules,
  getServers,
  patchRule,
} from "@/lib/api"
import { ENTRIES, entryLabel, type EntryKind } from "@/lib/entry"
import type { Rule, Server } from "@/lib/types"
import { tf } from "@/i18n"

type AddForm = {
  server: string
  entry: EntryKind
  protocols: string[]
  enabledOverride: boolean | null
}

const emptyAddForm = (): AddForm => ({
  server: "",
  entry: "validation",
  protocols: [],
  enabledOverride: null,
})

function serverProtocols(server: Server | undefined): string[] {
  if (!server) return []
  const protocols: string[] = []
  if (server.tcp?.port) protocols.push("TCP")
  if (server.udp?.port) protocols.push("UDP")
  return protocols
}

function upstreamPort(server: Server | undefined, protocol: string): string {
  if (!server) return "—"
  const proto = protocol.toUpperCase()
  if (proto === "TCP") return server.tcp?.port ? String(server.tcp.port) : "—"
  if (proto === "UDP") return server.udp?.port ? String(server.udp.port) : "—"
  return "—"
}

export function RulesPage({ embedded = false }: { embedded?: boolean }) {
  const { t } = useTranslation()
  const standby = useStandby()
  const [rules, setRules] = useState<Rule[]>([])
  const [servers, setServers] = useState<Server[]>([])
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState<string | null>(null)
  const [exporting, setExporting] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState<AddForm>(emptyAddForm)

  const load = useCallback(async () => {
    try {
      const [rulesData, serversData] = await Promise.all([getRules(), getServers()])
      setRules(rulesData)
      setServers(serversData.servers)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("common.toast_load_fail"))
    }
  }, [t])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  const serversByName = useMemo(() => {
    const map = new Map<string, Server>()
    for (const s of servers) map.set(s.name, s)
    return map
  }, [servers])

  const selectedServer = useMemo(
    () => serversByName.get(form.server),
    [serversByName, form.server],
  )

  const availableProtocols = useMemo(
    () => serverProtocols(selectedServer),
    [selectedServer],
  )

  function openAdd() {
    const first = servers[0]
    const protocols = serverProtocols(first)
    setForm({
      ...emptyAddForm(),
      server: first?.name ?? "",
      protocols: [...protocols],
    })
    setAddOpen(true)
  }

  function setServer(name: string) {
    const srv = servers.find((s) => s.name === name)
    setForm((f) => ({
      ...f,
      server: name,
      protocols: serverProtocols(srv),
    }))
  }

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (standby) return
    if (!form.server) {
      toast.error(t("rules.need_upstream"))
      return
    }
    if (!form.protocols.length) {
      toast.error(t("servers.quick_need_proto"))
      return
    }
    setAdding(true)
    try {
      const enabled =
        form.enabledOverride !== null ? { enabled: form.enabledOverride } : {}
      const res = await createRule({
        server: form.server,
        entry: form.entry,
        protocols: form.protocols,
        ...enabled,
      })
      await load()
      setAddOpen(false)
      setForm(emptyAddForm())
      if (res.rules?.length) {
        toast.success(tf("rules.toast_added", res.rules.map((r) => r.name).join(", ")))
      } else {
        toast.message(t("rules.toast_added_none"))
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
      toast.success(t("rules.toast_export_ok"))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("rules.toast_export_fail"))
    } finally {
      setExporting(false)
    }
  }

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

  const headerActions = (
    <>
      <Button size="sm" disabled={standby || loading} title={t("rules.add")} onClick={openAdd}>
        <PlusIcon data-icon="inline-start" />
        {t("rules.add")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={exporting || loading}
        title={t("rules.export")}
        onClick={handleExport}
      >
        {exporting ? <Spinner data-icon="inline-start" /> : <DownloadIcon data-icon="inline-start" />}
        {t("rules.export")}
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
              <TableHead>{t("rules.col_rule")}</TableHead>
              <TableHead>{t("rules.col_entry")}</TableHead>
              <TableHead>{t("rules.col_protocol")}</TableHead>
              <TableHead>{t("rules.col_upstream")}</TableHead>
              <TableHead>{t("rules.col_upstream_ip")}</TableHead>
              <TableHead>{t("rules.col_upstream_port")}</TableHead>
              <TableHead>{t("rules.col_port")}</TableHead>
              <TableHead>{t("rules.col_enabled")}</TableHead>
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
            ) : rules.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={8} className="p-2">
                  <EmptyState
                    icon={ArrowRightLeftIcon}
                    title={t("rules.empty")}
                    description={t("rules.empty_hint")}
                  />
                </TableCell>
              </TableRow>
            ) : (
              rules.map((rule) => {
                const srv = serversByName.get(rule.server)
                return (
                  <TableRow key={rule.name}>
                    <TableCell className="font-mono text-xs font-medium">{rule.name}</TableCell>
                    <TableCell>{entryLabel(rule.entry)}</TableCell>
                    <TableCell>{rule.protocol}</TableCell>
                    <TableCell className="font-medium">{rule.server}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {srv?.address || "—"}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {upstreamPort(srv, rule.protocol)}
                    </TableCell>
                    <TableCell className="font-mono text-xs">{rule.listen_port}</TableCell>
                    <TableCell>
                      <Switch
                        title={t("rules.col_enabled")}
                        aria-label={t("rules.col_enabled")}
                        checked={rule.enabled}
                        onCheckedChange={(v) => toggleRule(rule, v)}
                        disabled={standby || toggling === rule.name}
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
            <DialogTitle>{t("rules.add_heading")}</DialogTitle>
            <DialogDescription>{t("rules.add_description")}</DialogDescription>
          </DialogHeader>
          {servers.length === 0 ? (
            <div className="flex flex-col gap-4">
              <EmptyState
                icon={ServerIcon}
                title={t("servers.empty")}
                description={t("rules.need_upstream_hint")}
                compact
              />
              <DialogFooter>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setAddOpen(false)}
                >
                  {t("servers.cancel")}
                </Button>
                <Button render={<Link to="/servers" onClick={() => setAddOpen(false)} />}>
                  {t("rules.goto_servers")}
                </Button>
              </DialogFooter>
            </div>
          ) : (
            <form onSubmit={handleAdd} className="flex flex-col gap-4">
              <Field>
                <FieldLabel>{t("rules.server")}</FieldLabel>
                <Select
                  value={form.server || undefined}
                  onValueChange={(v) => {
                    if (v) setServer(v)
                  }}
                  disabled={standby || adding}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder={t("servers.empty")} />
                  </SelectTrigger>
                  <SelectContent>
                    {servers.map((s) => (
                      <SelectItem key={s.name} value={s.name}>
                        {s.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel>{t("rules.entry")}</FieldLabel>
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
                <FieldLabel>{t("rules.protocols")}</FieldLabel>
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
                <FieldLabel htmlFor="rule-enabled-override">{t("rules.col_enabled")}</FieldLabel>
                <div className="flex h-8 items-center gap-2 text-sm">
                  <Switch
                    id="rule-enabled-override"
                    checked={
                      form.enabledOverride ?? (form.entry === "validation")
                    }
                    onCheckedChange={(v) => setForm((f) => ({ ...f, enabledOverride: v }))}
                    disabled={standby || adding}
                    aria-label={t("rules.col_enabled")}
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
                  {t("servers.cancel")}
                </Button>
                <Button type="submit" disabled={standby || adding}>
                  {adding ? <Spinner data-icon="inline-start" /> : null}
                  {t("rules.add")}
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
      <PageHeader title={t("rules.title")} description={t("rules.desc")} actions={headerActions} />
      {main}
    </Page>
  )
}
