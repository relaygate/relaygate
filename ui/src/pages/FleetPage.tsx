import { useEffect, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { NetworkIcon, UserPlusIcon } from "lucide-react"

import { tf } from "@/i18n"

import { EmptyState } from "@/components/layout/EmptyState"
import { OpsLogView } from "@/components/layout/OpsLogView"
import { Page, PageHeader } from "@/components/layout/PageParts"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import { Spinner } from "@/components/ui/spinner"
import { useStandby } from "@/context/SessionContext"
import {
  ApiError,
  apiErrorDetail,
  getFleetOverview,
  getFleetStatus,
  opsFleetJoin,
  opsFleetLeave,
  type FleetNode,
  type FleetNodeStatus,
  type FleetOverview,
  type FleetStatusOverview,
} from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { matchesConfirm } from "@/lib/confirm"
import { cn } from "@/lib/utils"

type BusyKey = "join" | "leave" | null

function fleetStatusLabel(t: (key: string) => string, status: FleetNodeStatus["status"]): string {
  switch (status) {
    case "aligned":
      return t("fleet.status_aligned")
    case "drifted":
      return t("fleet.status_drifted")
    case "offline":
      return t("fleet.status_offline")
    case "unauthorized":
      return t("fleet.status_unauthorized")
    default:
      return t("fleet.status_unknown")
  }
}

function fleetStatusClass(status: FleetNodeStatus["status"]): string {
  switch (status) {
    case "aligned":
      return "text-emerald-600 dark:text-emerald-400"
    case "drifted":
      return "text-amber-600 dark:text-amber-400"
    case "offline":
    case "unauthorized":
      return "text-destructive"
    default:
      return "text-muted-foreground"
  }
}

function fleetRoleLabel(t: (key: string) => string, role?: string): string {
  switch (role) {
    case "control":
      return t("fleet.role_control")
    case "node":
      return t("fleet.role_node")
    default:
      return role || "—"
  }
}

function isRetirableGateway(node: FleetNode): boolean {
  return node.role !== "control"
}

function FleetCard({
  icon,
  title,
  description,
  actions,
  children,
  className,
}: {
  icon: ReactNode
  title: string
  description?: string
  actions?: ReactNode
  children?: ReactNode
  className?: string
}) {
  return (
    <section
      className={cn(
        "flex flex-col gap-3 rounded-md border border-border/60 bg-card/40 p-3.5",
        className,
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex min-w-0 items-start gap-2.5">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border border-border/60 bg-muted/40 text-muted-foreground">
            {icon}
          </div>
          <div className="min-w-0">
            <h2 className="text-[13px] font-semibold tracking-wide text-foreground">{title}</h2>
            {description ? (
              <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
            ) : null}
          </div>
        </div>
        {actions ? (
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">{actions}</div>
        ) : null}
      </div>
      {children}
    </section>
  )
}

export function FleetPage() {
  const { t, i18n } = useTranslation()
  const standby = useStandby()
  const [busy, setBusy] = useState<BusyKey>(null)
  const [fleet, setFleet] = useState<FleetOverview | null>(null)
  const [status, setStatus] = useState<FleetStatusOverview | null>(null)
  const [fleetBusy, setFleetBusy] = useState(false)

  const [joinOpen, setJoinOpen] = useState(false)
  const [joinName, setJoinName] = useState("gateway-02")
  const [joinHint, setJoinHint] = useState("")
  const [joinCommand, setJoinCommand] = useState("")
  const [joinToken, setJoinToken] = useState("")

  const [leaveName, setLeaveName] = useState("")
  const [leaveConfirm, setLeaveConfirm] = useState("")
  const [leaveOpen, setLeaveOpen] = useState(false)
  const [leaveHints, setLeaveHints] = useState<string[]>([])

  async function loadFleet() {
    setFleetBusy(true)
    try {
      const overview = await getFleetOverview()
      setFleet(overview)
      try {
        setStatus(await getFleetStatus())
      } catch {
        setStatus(null)
      }
    } catch (err) {
      setFleet({ nodes: [], hints: [], published: null })
      setStatus(null)
      toast.error(err instanceof ApiError ? err.message : t("common.toast_load_fail"))
    } finally {
      setFleetBusy(false)
    }
  }

  useEffect(() => {
    void loadFleet()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- load once
  }, [])

  function openJoin() {
    setJoinHint("")
    setJoinCommand("")
    setJoinToken("")
    setJoinOpen(true)
  }

  async function runJoin() {
    if (!joinName.trim()) return
    setBusy("join")
    setJoinHint("")
    setJoinCommand("")
    setJoinToken("")
    try {
      const res = await opsFleetJoin({ name: joinName.trim() })
      if (res.ok) {
        setJoinCommand(res.join_command ?? "")
        setJoinHint(res.bootstrap_hint ?? "")
        setJoinToken(res.token ?? "")
        toast.success(t("fleet.toast_join_ok"))
        await loadFleet()
      } else {
        toast.error(apiErrorDetail(res, t("fleet.toast_join_err")))
      }
    } catch (err) {
      toast.error(apiErrorDetail(err, t("fleet.toast_join_err")))
    } finally {
      setBusy(null)
    }
  }

  async function copyJoinCommand() {
    if (!joinCommand) return
    if (await copyText(joinCommand)) {
      toast.success(t("fleet.toast_copy_ok"))
    } else {
      toast.error(t("fleet.toast_copy_err"))
    }
  }

  function openLeave(name: string) {
    setLeaveName(name)
    setLeaveConfirm("")
    setLeaveOpen(true)
  }

  async function runLeave() {
    if (!matchesConfirm(leaveConfirm) || !leaveName) return
    setBusy("leave")
    setLeaveHints([])
    try {
      const res = await opsFleetLeave({ confirm: leaveConfirm.trim(), name: leaveName })
      if (res.ok) {
        setLeaveHints(res.manual_hints ?? [])
        toast.success(t("fleet.toast_leave_ok"))
        setLeaveOpen(false)
        setLeaveConfirm("")
        setLeaveName("")
        await loadFleet()
      } else {
        toast.error(apiErrorDetail(res, t("fleet.toast_leave_err")))
      }
    } catch (err) {
      toast.error(apiErrorDetail(err, t("fleet.toast_leave_err")))
    } finally {
      setBusy(null)
    }
  }

  const anyBusy = busy !== null
  const statusByName: Record<string, FleetNodeStatus> = {}
  for (const n of status?.nodes ?? []) {
    statusByName[n.name] = n
  }
  const confirmPlaceholder = i18n.language.toLowerCase().startsWith("zh") ? "确认" : "Confirm"

  return (
    <Page>
      <PageHeader
        title={t("fleet.title")}
        description={t("fleet.desc")}
        actions={
          <Button size="sm" onClick={openJoin} disabled={standby || anyBusy}>
            <UserPlusIcon data-icon="inline-start" />
            {t("fleet.join_run")}
          </Button>
        }
      />

      {standby ? (
        <Alert>
          <AlertTitle>{t("fleet.standby_title")}</AlertTitle>
          <AlertDescription>{t("fleet.standby_body")}</AlertDescription>
        </Alert>
      ) : null}

      <FleetCard
        icon={<NetworkIcon className="size-4" />}
        title={t("fleet.list_title")}
        description={t("fleet.list_desc")}
        actions={
          <Button size="sm" variant="outline" onClick={() => void loadFleet()} disabled={fleetBusy}>
            {fleetBusy ? <Spinner data-icon="inline-start" /> : null}
            {t("fleet.refresh")}
          </Button>
        }
      >
        {fleet && fleet.nodes.length === 0 ? (
          <EmptyState
            compact
            icon={NetworkIcon}
            title={t("fleet.empty")}
            description={t("fleet.empty_hint")}
          />
        ) : fleet && fleet.nodes.length > 0 ? (
          <div className="overflow-x-auto rounded-md border border-border/50">
            <table className="w-full text-left text-xs">
              <thead className="bg-muted/30 text-muted-foreground">
                <tr>
                  <th className="px-2 py-1.5 font-medium">{t("fleet.col_name")}</th>
                  <th className="px-2 py-1.5 font-medium">{t("fleet.col_role")}</th>
                  <th className="px-2 py-1.5 font-medium">{t("fleet.col_align")}</th>
                  <th className="px-2 py-1.5 font-medium">{t("fleet.col_version")}</th>
                  <th className="px-2 py-1.5 text-right font-medium">{t("fleet.col_actions")}</th>
                </tr>
              </thead>
              <tbody>
                {fleet.nodes.map((node: FleetNode) => {
                  const st = statusByName[node.name]
                  const canRetire = isRetirableGateway(node)
                  return (
                    <tr key={node.name} className="border-t border-border/40">
                      <td className="px-2 py-1.5 font-mono">{node.name}</td>
                      <td className="px-2 py-1.5">{fleetRoleLabel(t, node.role)}</td>
                      <td className={cn("px-2 py-1.5 font-medium", fleetStatusClass(st?.status ?? "unknown"))}>
                        {fleetStatusLabel(t, st?.status ?? "unknown")}
                      </td>
                      <td className="px-2 py-1.5 font-mono text-muted-foreground">
                        {st?.applied_version || node.applied_version || "—"}
                      </td>
                      <td className="px-2 py-1.5 text-right">
                        {canRetire ? (
                          <Button
                            size="sm"
                            variant="destructive"
                            onClick={() => openLeave(node.name)}
                            disabled={standby || anyBusy}
                          >
                            {busy === "leave" && leaveName === node.name ? (
                              <Spinner data-icon="inline-start" />
                            ) : null}
                            {t("fleet.leave_run")}
                          </Button>
                        ) : null}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <EmptyState compact icon={NetworkIcon} title={t("common.loading")} />
        )}

        {leaveHints.length > 0 ? (
          <div className="rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-800 dark:text-amber-200">
            <p className="mb-1 font-medium">{t("fleet.manual_nlb")}</p>
            {leaveHints.map((h) => (
              <p key={h}>{h}</p>
            ))}
          </div>
        ) : null}
      </FleetCard>

      <Dialog
        open={joinOpen}
        onOpenChange={(open) => {
          if (!open && busy !== "join") {
            setJoinOpen(false)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("fleet.join_title")}</DialogTitle>
            <DialogDescription>{t("fleet.join_desc")}</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="join-name">{t("fleet.field_name")}</FieldLabel>
              <Input
                id="join-name"
                className="font-mono text-xs"
                value={joinName}
                onChange={(e) => setJoinName(e.target.value)}
                disabled={anyBusy}
                autoComplete="off"
                placeholder="gateway-02"
              />
              <p className="mt-1 text-xs text-muted-foreground">{t("fleet.join_name_hint")}</p>
            </Field>
          </FieldGroup>
          {joinCommand ? (
            <div className="space-y-2">
              <div className="flex items-center justify-between gap-2">
                <p className="text-xs font-medium text-foreground">{t("fleet.join_command_label")}</p>
                <Button size="sm" variant="outline" onClick={() => void copyJoinCommand()}>
                  {t("fleet.copy_command")}
                </Button>
              </div>
              <OpsLogView value={joinCommand} />
              <p className="text-xs text-muted-foreground">{t("fleet.join_command_hint")}</p>
            </div>
          ) : joinHint ? (
            <OpsLogView value={`${joinHint}${joinToken ? `\nTOKEN=${joinToken}` : ""}`} />
          ) : null}
          <DialogFooter>
            <Button variant="ghost" onClick={() => setJoinOpen(false)} disabled={busy === "join"}>
              {t("ops.cancel")}
            </Button>
            <Button
              onClick={() => void runJoin()}
              disabled={standby || anyBusy || !joinName.trim()}
            >
              {busy === "join" ? <Spinner data-icon="inline-start" /> : null}
              {t("fleet.join_run")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={leaveOpen}
        onOpenChange={(open) => {
          if (!open && busy !== "leave") {
            setLeaveOpen(false)
            setLeaveConfirm("")
            setLeaveName("")
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("fleet.leave_run")}</DialogTitle>
            <DialogDescription>
              {leaveName ? tf("fleet.leave_confirm_body", leaveName) : t("fleet.leave_confirm_body_generic")}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="leave-confirm">{t("common.confirm_typed_label")}</FieldLabel>
              <Input
                id="leave-confirm"
                value={leaveConfirm}
                onChange={(e) => setLeaveConfirm(e.target.value)}
                disabled={standby || busy === "leave"}
                autoComplete="off"
                placeholder={confirmPlaceholder}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => {
                setLeaveOpen(false)
                setLeaveConfirm("")
                setLeaveName("")
              }}
              disabled={busy === "leave"}
            >
              {t("ops.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => void runLeave()}
              disabled={standby || busy === "leave" || !matchesConfirm(leaveConfirm)}
            >
              {busy === "leave" ? <Spinner data-icon="inline-start" /> : null}
              {t("fleet.leave_run")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
