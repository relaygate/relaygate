import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Page, PageHeader } from "@/components/layout/PageParts"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
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
  createServer,
  deleteServer,
  getServers,
  promoteServer,
  updateServer,
} from "@/lib/api"
import type { Server, ServerLifecycle } from "@/lib/types"
import { tf } from "@/i18n"

const emptyForm = {
  name: "",
  address: "",
  tcp_port: "",
  udp_port: "",
  health_check_port: "",
  enabled: true,
}

export function ServersPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [servers, setServers] = useState<Server[]>([])
  const [lifecycle, setLifecycle] = useState<Record<string, ServerLifecycle>>({})
  const [loading, setLoading] = useState(true)
  const [form, setForm] = useState(emptyForm)
  const [editing, setEditing] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState<Server | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [promoteTarget, setPromoteTarget] = useState<string | null>(null)

  const load = useCallback(async () => {
    const data = await getServers()
    setServers(data.servers)
    setLifecycle(data.lifecycle)
  }, [])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (standby) return
    try {
      const res = await createServer({
        name: form.name.trim(),
        address: form.address.trim(),
        tcp_port: Number(form.tcp_port) || 0,
        udp_port: Number(form.udp_port) || 0,
        health_check_port: Number(form.health_check_port) || 0,
        enabled: form.enabled,
      })
      await load()
      setForm(emptyForm)
      if (res.rules?.length) {
        toast.success(
          tf(
            "servers.toast_added_rules",
            form.name.trim(),
            res.rules.map((r) => r.name).join(", "),
          ),
        )
      } else {
        toast.success(tf("servers.toast_added", form.name.trim()))
      }
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    }
  }

  function startEdit(server: Server) {
    setEditing(server.name)
    setEditDraft({ ...server })
  }

  async function saveEdit() {
    if (!editDraft || standby) return
    try {
      await updateServer(editDraft.name, {
        address: editDraft.address,
        tcp_port: editDraft.tcp_port,
        udp_port: editDraft.udp_port,
        health_check_port: editDraft.health_check_port,
        enabled: editDraft.enabled,
      })
      await load()
      setEditing(null)
      setEditDraft(null)
      toast.success(tf("servers.toast_saved", editDraft.name))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    }
  }

  async function confirmDelete() {
    if (!deleteTarget || standby) return
    try {
      const res = await deleteServer(deleteTarget)
      await load()
      toast.success(tf("servers.toast_deleted", deleteTarget, res.removed_rules ?? 0))
      setDeleteTarget(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    }
  }

  async function confirmPromote() {
    if (!promoteTarget || standby) return
    try {
      const res = await promoteServer(promoteTarget)
      await load()
      toast.success(tf("servers.toast_promoted", promoteTarget, res.changed ?? 0))
      setPromoteTarget(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    }
  }

  function lifecycleBadges(name: string) {
    const lc = lifecycle[name]
    if (!lc) return null
    return (
      <div className="flex flex-wrap gap-1">
        {lc.canary_rule_count > 0 ? (
          <Badge variant={lc.canary_enabled ? "default" : "secondary"} className="text-[10px]">
            {t("servers.canary")}
            {lc.canary_enabled && lc.canary_ports.length
              ? ` ${lc.canary_ports.join(",")}`
              : ""}
          </Badge>
        ) : null}
        {lc.production_rule_count > 0 ? (
          <Badge variant={lc.production_enabled ? "default" : "outline"} className="text-[10px]">
            {t("servers.production")}
            {lc.production_enabled && lc.production_ports.length
              ? ` ${lc.production_ports.join(",")}`
              : ""}
          </Badge>
        ) : null}
      </div>
    )
  }

  return (
    <Page>
      <PageHeader title={t("servers.title")} hint={t("servers.hint")} />

      <form onSubmit={handleAdd} className="flex flex-col gap-4 rounded-lg border border-border bg-card/30 p-4">
        <h2 className="text-sm font-semibold">{t("servers.add_heading")}</h2>
        <FieldGroup className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          <Field>
            <FieldLabel htmlFor="srv-name">{t("servers.name")}</FieldLabel>
            <Input
              id="srv-name"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              disabled={standby}
              required
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="srv-addr">{t("servers.address")}</FieldLabel>
            <Input
              id="srv-addr"
              value={form.address}
              onChange={(e) => setForm((f) => ({ ...f, address: e.target.value }))}
              disabled={standby}
              required
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="srv-tcp">{t("servers.tcp")}</FieldLabel>
            <Input
              id="srv-tcp"
              type="number"
              value={form.tcp_port}
              onChange={(e) => setForm((f) => ({ ...f, tcp_port: e.target.value }))}
              disabled={standby}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="srv-udp">{t("servers.udp")}</FieldLabel>
            <Input
              id="srv-udp"
              type="number"
              value={form.udp_port}
              onChange={(e) => setForm((f) => ({ ...f, udp_port: e.target.value }))}
              disabled={standby}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="srv-health">{t("servers.health")}</FieldLabel>
            <Input
              id="srv-health"
              type="number"
              value={form.health_check_port}
              onChange={(e) => setForm((f) => ({ ...f, health_check_port: e.target.value }))}
              disabled={standby}
            />
          </Field>
          <Field orientation="horizontal" className="items-end">
            <Checkbox
              checked={form.enabled}
              onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v === true }))}
              disabled={standby}
            />
            <FieldLabel>{t("servers.enabled")}</FieldLabel>
          </Field>
        </FieldGroup>
        <Button type="submit" disabled={standby} className="w-fit">
          {t("servers.add")}
        </Button>
      </form>

      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("servers.name")}</TableHead>
              <TableHead>{t("servers.address")}</TableHead>
              <TableHead>{t("servers.tcp")}</TableHead>
              <TableHead>{t("servers.udp")}</TableHead>
              <TableHead>{t("servers.health")}</TableHead>
              <TableHead>{t("servers.enabled")}</TableHead>
              <TableHead>Stage</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={8} className="text-muted-foreground">
                  …
                </TableCell>
              </TableRow>
            ) : servers.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className="text-muted-foreground">
                  {t("servers.empty")}
                </TableCell>
              </TableRow>
            ) : (
              servers.map((srv) => {
                const isEditing = editing === srv.name && editDraft
                const row = isEditing ? editDraft : srv
                return (
                  <TableRow key={srv.name}>
                    <TableCell className="font-medium">{srv.name}</TableCell>
                    <TableCell>
                      {isEditing ? (
                        <Input
                          value={row.address}
                          onChange={(e) =>
                            setEditDraft((d) => (d ? { ...d, address: e.target.value } : d))
                          }
                          className="h-7"
                        />
                      ) : (
                        row.address
                      )}
                    </TableCell>
                    <TableCell>
                      {isEditing ? (
                        <Input
                          type="number"
                          value={row.tcp_port}
                          onChange={(e) =>
                            setEditDraft((d) =>
                              d ? { ...d, tcp_port: Number(e.target.value) || 0 } : d,
                            )
                          }
                          className="h-7 w-20"
                        />
                      ) : (
                        row.tcp_port
                      )}
                    </TableCell>
                    <TableCell>
                      {isEditing ? (
                        <Input
                          type="number"
                          value={row.udp_port}
                          onChange={(e) =>
                            setEditDraft((d) =>
                              d ? { ...d, udp_port: Number(e.target.value) || 0 } : d,
                            )
                          }
                          className="h-7 w-20"
                        />
                      ) : (
                        row.udp_port
                      )}
                    </TableCell>
                    <TableCell>
                      {isEditing ? (
                        <Input
                          type="number"
                          value={row.health_check_port}
                          onChange={(e) =>
                            setEditDraft((d) =>
                              d
                                ? { ...d, health_check_port: Number(e.target.value) || 0 }
                                : d,
                            )
                          }
                          className="h-7 w-20"
                        />
                      ) : (
                        row.health_check_port
                      )}
                    </TableCell>
                    <TableCell>
                      {isEditing ? (
                        <Checkbox
                          checked={row.enabled}
                          onCheckedChange={(v) =>
                            setEditDraft((d) => (d ? { ...d, enabled: v === true } : d))
                          }
                        />
                      ) : row.enabled ? (
                        t("common.on")
                      ) : (
                        t("common.off")
                      )}
                    </TableCell>
                    <TableCell>{lifecycleBadges(srv.name)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        {isEditing ? (
                          <>
                            <Button size="sm" onClick={saveEdit} disabled={standby}>
                              {t("servers.save")}
                            </Button>
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => {
                                setEditing(null)
                                setEditDraft(null)
                              }}
                            >
                              {t("servers.cancel")}
                            </Button>
                          </>
                        ) : (
                          <>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => startEdit(srv)}
                              disabled={standby}
                            >
                              {t("servers.save")}
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setPromoteTarget(srv.name)}
                              disabled={standby}
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
                          </>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("servers.confirm")}</DialogTitle>
            <DialogDescription>{deleteTarget}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)}>
              {t("servers.cancel")}
            </Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={standby}>
              {t("servers.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!promoteTarget} onOpenChange={(open) => !open && setPromoteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("servers.promote_title")}</DialogTitle>
            <DialogDescription>
              {promoteTarget ? tf("servers.promote_confirm", promoteTarget) : ""}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPromoteTarget(null)}>
              {t("servers.cancel")}
            </Button>
            <Button onClick={confirmPromote} disabled={standby}>
              {t("servers.promote")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
