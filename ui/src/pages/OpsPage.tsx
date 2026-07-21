import { useEffect, useState, type ReactNode } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  ActivityIcon,
  ArrowLeftRightIcon,
  InboxIcon,
  ShieldCheckIcon,
  SlidersHorizontalIcon,
  StethoscopeIcon,
  WrenchIcon,
} from "lucide-react"

import { DiffView } from "@/components/layout/DiffView"
import { EmptyState } from "@/components/layout/EmptyState"
import { Page, PageHeader } from "@/components/layout/PageParts"
import { Button, buttonVariants } from "@/components/ui/button"
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { useStandby } from "@/context/SessionContext"
import {
  ApiError,
  apiErrorDetail,
  getProfiles,
  opsCanary,
  opsDoctor,
  opsDrain,
  opsFirewallCheck,
  opsProfileApply,
  opsProfilePreview,
  opsSmoke,
} from "@/lib/api"
import type { Profile } from "@/lib/types"
import { tf } from "@/i18n"
import { cn } from "@/lib/utils"

function profileShortLabel(
  t: (key: string) => string,
  name: string,
): string {
  const key = `ops.profile_label.${name}`
  const label = t(key)
  return label === key ? name.replace(/-/g, " ") : label
}

type BusyKey =
  | "doctor"
  | "drain"
  | "smoke"
  | "canary"
  | "firewall"
  | "preview"
  | "profile"
  | null

function OpsCard({
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

export function OpsPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [busy, setBusy] = useState<BusyKey>(null)
  const [doctorOut, setDoctorOut] = useState("")
  const [drainOut, setDrainOut] = useState("")
  const [probeOut, setProbeOut] = useState("")
  const [firewallOut, setFirewallOut] = useState("")
  const [host, setHost] = useState("127.0.0.1")
  const [drainAction, setDrainAction] = useState<"fail" | "ok" | null>(null)
  const [drainConfirm, setDrainConfirm] = useState("")
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profileName, setProfileName] = useState("")
  const [profileOut, setProfileOut] = useState("")
  const [profileConfirm, setProfileConfirm] = useState("")
  const [profileApplyOpen, setProfileApplyOpen] = useState(false)

  useEffect(() => {
    getProfiles()
      .then((list) => {
        setProfiles(list)
        if (list.length && !profileName) setProfileName(list[0].name)
      })
      .catch((err) => {
        setProfiles([])
        toast.error(err instanceof ApiError ? err.message : t("common.toast_load_fail"))
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- load once
  }, [])

  async function runDoctor() {
    setBusy("doctor")
    try {
      const res = await opsDoctor()
      setDoctorOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_doctor_ok"))
    } catch (err) {
      const msg = apiErrorDetail(err, t("ops.toast_doctor_err"))
      setDoctorOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runDrain(action: "status" | "fail" | "ok") {
    if (action !== "status") {
      const expected = `DRAIN_${action.toUpperCase()}`
      if (drainConfirm !== expected) return
    }
    setBusy("drain")
    try {
      const res = await opsDrain(action, action === "status" ? undefined : drainConfirm)
      setDrainOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(tf("ops.toast_drain_ok", action))
      setDrainAction(null)
      setDrainConfirm("")
    } catch (err) {
      const msg = apiErrorDetail(err, tf("ops.toast_drain_err", action))
      setDrainOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runProbe(kind: "smoke" | "canary") {
    setBusy(kind)
    try {
      const res = kind === "smoke" ? await opsSmoke(host) : await opsCanary(host)
      setProbeOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(kind === "smoke" ? t("ops.toast_smoke_ok") : t("ops.toast_canary_ok"))
    } catch (err) {
      const msg = apiErrorDetail(
        err,
        kind === "smoke" ? t("ops.toast_smoke_err") : t("ops.toast_canary_err"),
      )
      setProbeOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runFirewall() {
    setBusy("firewall")
    try {
      const res = await opsFirewallCheck()
      setFirewallOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_fw_ok"))
    } catch (err) {
      const msg = apiErrorDetail(err, t("ops.toast_fw_err"))
      setFirewallOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runProfilePreview() {
    if (!profileName) return
    setBusy("preview")
    try {
      const res = await opsProfilePreview(profileName)
      setProfileOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_preview_ok"))
    } catch (err) {
      const msg = apiErrorDetail(err, t("ops.toast_preview_err"))
      setProfileOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runProfileApply() {
    if (!profileName || profileConfirm !== "APPLY_PROFILE") return
    setBusy("profile")
    try {
      const res = await opsProfileApply(profileName, profileConfirm)
      // Backend already appends ops.profile_applied_body to output
      setProfileOut(res.output ?? t("error.no_output"))
      toast.success(t("ops.toast_profile_ok"))
      setProfileApplyOpen(false)
      setProfileConfirm("")
    } catch (err) {
      const msg = apiErrorDetail(err, t("ops.toast_profile_err"))
      setProfileOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  const anyBusy = busy !== null

  return (
    <Page>
      <PageHeader title={t("ops.title")} description={t("ops.desc")} />

      <div className="grid gap-3 lg:grid-cols-2">
        <OpsCard
          icon={<StethoscopeIcon className="size-4" />}
          title={t("ops.doctor")}
          description={t("ops.doctor_desc")}
          actions={
            <Button size="sm" onClick={runDoctor} disabled={anyBusy}>
              {busy === "doctor" ? <Spinner data-icon="inline-start" /> : null}
              {t("ops.run")}
            </Button>
          }
        >
          <DiffView
            fixedHeight
            value={doctorOut}
            placeholder={
              <EmptyState
                compact
                icon={StethoscopeIcon}
                title={t("ops.doctor_placeholder")}
                className="w-full border-0 bg-transparent"
              />
            }
          />
        </OpsCard>

        <OpsCard
          icon={<ArrowLeftRightIcon className="size-4" />}
          title={t("ops.drain")}
          description={t("ops.drain_hint")}
          actions={
            <>
              <Button size="sm" onClick={() => runDrain("status")} disabled={anyBusy}>
                {busy === "drain" ? <Spinner data-icon="inline-start" /> : null}
                {t("ops.btn_drain_status")}
              </Button>
              <Button
                size="sm"
                variant="destructive"
                onClick={() => {
                  setDrainAction("fail")
                  setDrainConfirm("")
                }}
                disabled={standby || anyBusy}
              >
                {t("ops.btn_drain_fail")}
              </Button>
              <Button
                size="sm"
                onClick={() => {
                  setDrainAction("ok")
                  setDrainConfirm("")
                }}
                disabled={standby || anyBusy}
              >
                {t("ops.btn_drain_ok")}
              </Button>
            </>
          }
        >
          <DiffView
            fixedHeight
            value={drainOut}
            placeholder={
              <EmptyState
                compact
                icon={InboxIcon}
                title={t("error.no_output")}
                className="w-full border-0 bg-transparent"
              />
            }
          />
        </OpsCard>

        <OpsCard
          icon={<ActivityIcon className="size-4" />}
          title={t("ops.smoke_canary")}
          description={t("ops.probe_desc")}
          actions={
            <>
              <Input
                id="ops-host"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                disabled={anyBusy}
                className="h-7 w-36 font-mono text-xs"
                placeholder="127.0.0.1"
                aria-label={t("ops.host")}
              />
              <Button size="sm" onClick={() => runProbe("smoke")} disabled={anyBusy}>
                {busy === "smoke" ? <Spinner data-icon="inline-start" /> : null}
                {t("ops.btn_smoke")}
              </Button>
              <Button size="sm" onClick={() => runProbe("canary")} disabled={anyBusy}>
                {busy === "canary" ? <Spinner data-icon="inline-start" /> : null}
                {t("ops.btn_canary")}
              </Button>
            </>
          }
        >
          <DiffView
            fixedHeight
            value={probeOut}
            placeholder={
              <EmptyState
                compact
                icon={InboxIcon}
                title={t("error.no_output")}
                className="w-full border-0 bg-transparent"
              />
            }
          />
        </OpsCard>

        <OpsCard
          icon={<ShieldCheckIcon className="size-4" />}
          title={t("ops.firewall")}
          description={t("ops.fw_desc")}
          actions={
            <>
              <Button size="sm" onClick={runFirewall} disabled={anyBusy}>
                {busy === "firewall" ? <Spinner data-icon="inline-start" /> : null}
                {t("ops.fw_check")}
              </Button>
              <Link
                to="/apply"
                className={buttonVariants({ size: "sm", variant: "secondary" })}
              >
                {t("ops.fw_apply_link")}
              </Link>
            </>
          }
        >
          <DiffView
            fixedHeight
            value={firewallOut}
            placeholder={
              <EmptyState
                compact
                icon={InboxIcon}
                title={t("error.no_output")}
                className="w-full border-0 bg-transparent"
              />
            }
          />
        </OpsCard>

        <OpsCard
          icon={<WrenchIcon className="size-4" />}
          title={t("ops.profile")}
          description={t("ops.profile_desc")}
          className="lg:col-span-2"
          actions={
            profiles.length === 0 ? null : (
              <>
                <Select
                  value={profileName}
                  onValueChange={(v) => setProfileName(v ?? "")}
                  disabled={anyBusy}
                >
                  <SelectTrigger className="h-7 w-28 min-w-0 max-w-[7.5rem] text-[0.8rem]" size="sm">
                    <SelectValue placeholder={t("ops.profile_select")} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger className="min-w-0 w-(--anchor-width)">
                    {profiles.map((p) => (
                      <SelectItem key={p.name} value={p.name} title={p.description || p.name}>
                        <span className="truncate">{profileShortLabel(t, p.name)}</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={runProfilePreview}
                  disabled={anyBusy || !profileName}
                >
                  {busy === "preview" ? <Spinner data-icon="inline-start" /> : null}
                  {t("ops.profile_preview")}
                </Button>
                <Button
                  size="sm"
                  onClick={() => {
                    setProfileConfirm("")
                    setProfileApplyOpen(true)
                  }}
                  disabled={standby || anyBusy || !profileName}
                >
                  {t("ops.profile_apply")}
                </Button>
              </>
            )
          }
        >
          {profiles.length === 0 ? (
            <EmptyState
              compact
              icon={SlidersHorizontalIcon}
              title={t("ops.profile_empty")}
              description={t("ops.profile_empty_hint")}
            />
          ) : null}
          <DiffView
            fixedHeight
            value={profileOut}
            placeholder={
              <EmptyState
                compact
                icon={InboxIcon}
                title={t("error.no_output")}
                className="w-full border-0 bg-transparent"
              />
            }
          />
        </OpsCard>
      </div>

      <Dialog
        open={!!drainAction}
        onOpenChange={(open) => !open && busy !== "drain" && setDrainAction(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {drainAction === "fail" ? t("ops.btn_drain_fail") : t("ops.btn_drain_ok")}
            </DialogTitle>
            <DialogDescription>
              {drainAction === "fail" ? t("ops.drain_label_fail") : t("ops.drain_label_ok")}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="drain-confirm">
                {drainAction === "fail" ? "DRAIN_FAIL" : "DRAIN_OK"}
              </FieldLabel>
              <Input
                id="drain-confirm"
                value={drainConfirm}
                onChange={(e) => setDrainConfirm(e.target.value)}
                disabled={busy === "drain"}
                autoComplete="off"
                className="font-mono"
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => setDrainAction(null)}
              disabled={busy === "drain"}
            >
              {t("ops.cancel")}
            </Button>
            <Button
              variant={drainAction === "fail" ? "destructive" : "default"}
              onClick={() => drainAction && runDrain(drainAction)}
              disabled={
                standby ||
                busy === "drain" ||
                !drainAction ||
                drainConfirm !== `DRAIN_${drainAction.toUpperCase()}`
              }
            >
              {busy === "drain" ? <Spinner data-icon="inline-start" /> : null}
              {t("ops.run")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={profileApplyOpen}
        onOpenChange={(open) => {
          if (!open && busy !== "profile") {
            setProfileApplyOpen(false)
            setProfileConfirm("")
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("ops.profile_apply")}</DialogTitle>
            <DialogDescription>
              {profileName ? (
                <>
                  {t("ops.profile_select")}: {profileShortLabel(t, profileName)}{" "}
                  <code className="font-mono text-foreground">({profileName})</code>
                </>
              ) : (
                t("ops.profile_confirm")
              )}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="profile-confirm">{t("ops.profile_confirm")}</FieldLabel>
              <Input
                id="profile-confirm"
                value={profileConfirm}
                onChange={(e) => setProfileConfirm(e.target.value)}
                disabled={standby || busy === "profile"}
                autoComplete="off"
                className="font-mono"
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => setProfileApplyOpen(false)}
              disabled={busy === "profile"}
            >
              {t("ops.cancel")}
            </Button>
            <Button
              onClick={runProfileApply}
              disabled={standby || busy === "profile" || profileConfirm !== "APPLY_PROFILE"}
            >
              {busy === "profile" ? <Spinner data-icon="inline-start" /> : null}
              {t("ops.profile_apply")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
