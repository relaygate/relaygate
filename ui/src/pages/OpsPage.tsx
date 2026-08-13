import { useEffect, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  ActivityIcon,
  ArrowLeftRightIcon,
  InboxIcon,
  SlidersHorizontalIcon,
  StethoscopeIcon,
  WrenchIcon,
} from "lucide-react"

import { OpsLogView } from "@/components/layout/OpsLogView"
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
  apiErrorOutput,
  getProfiles,
  opsCanary,
  opsDoctor,
  opsDrain,
  opsProfileApply,
  opsProfilePreview,
  opsSmoke,
} from "@/lib/api"
import type { Profile } from "@/lib/types"
import { tf } from "@/i18n"
import { cn } from "@/lib/utils"
import { matchesConfirm } from "@/lib/confirm"

function profileShortLabel(t: (key: string) => string, name: string): string {
  const key = `ops.profile_label.${name}`
  const label = t(key)
  return label === key ? name.replace(/-/g, " ") : label
}

type BusyKey =
  | "doctor"
  | "drain"
  | "smoke"
  | "canary"
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
  const { t, i18n } = useTranslation()
  const standby = useStandby()
  const [busy, setBusy] = useState<BusyKey>(null)
  const [doctorOut, setDoctorOut] = useState("")
  const [doctorErr, setDoctorErr] = useState(false)
  const [drainOut, setDrainOut] = useState("")
  const [drainErr, setDrainErr] = useState(false)
  const [probeOut, setProbeOut] = useState("")
  const [probeErr, setProbeErr] = useState(false)
  const [host, setHost] = useState("127.0.0.1")
  const [drainAction, setDrainAction] = useState<"fail" | "ok" | null>(null)
  const [drainConfirm, setDrainConfirm] = useState("")
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profileName, setProfileName] = useState("")
  const [profileOut, setProfileOut] = useState("")
  const [profileErr, setProfileErr] = useState(false)
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
      setDoctorErr(false)
      setDoctorOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_doctor_ok"))
    } catch (err) {
      const msg = apiErrorDetail(err, t("ops.toast_doctor_err"))
      setDoctorErr(true)
      setDoctorOut(apiErrorOutput(err, msg))
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runDrain(action: "status" | "fail" | "ok") {
    if (action !== "status") {
      if (!matchesConfirm(drainConfirm)) return
    }
    const actionLabel =
      action === "fail"
        ? t("ops.btn_drain_fail")
        : action === "ok"
          ? t("ops.btn_drain_ok")
          : t("ops.btn_drain_status")
    setBusy("drain")
    try {
      const res = await opsDrain(action, action === "status" ? undefined : drainConfirm.trim())
      setDrainErr(false)
      setDrainOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(tf("ops.toast_drain_ok", actionLabel))
      setDrainAction(null)
      setDrainConfirm("")
    } catch (err) {
      const msg = apiErrorDetail(err, tf("ops.toast_drain_err", actionLabel))
      setDrainErr(true)
      setDrainOut(apiErrorOutput(err, msg))
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runProbe(kind: "smoke" | "canary") {
    setBusy(kind)
    try {
      const res = kind === "smoke" ? await opsSmoke(host) : await opsCanary(host)
      setProbeErr(false)
      setProbeOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(kind === "smoke" ? t("ops.toast_smoke_ok") : t("ops.toast_canary_ok"))
    } catch (err) {
      const fallback =
        kind === "smoke" ? t("ops.toast_smoke_err") : t("ops.toast_canary_err")
      const msg = apiErrorDetail(err, fallback)
      setProbeErr(true)
      setProbeOut(apiErrorOutput(err, msg))
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
      setProfileErr(false)
      setProfileOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_preview_ok"))
    } catch (err) {
      const msg = apiErrorDetail(err, t("ops.toast_preview_err"))
      setProfileErr(true)
      setProfileOut(apiErrorOutput(err, msg))
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runProfileApply() {
    if (!profileName || !matchesConfirm(profileConfirm)) return
    setBusy("profile")
    try {
      const res = await opsProfileApply(profileName, profileConfirm.trim())
      setProfileErr(false)
      setProfileOut(res.output ?? t("error.no_output"))
      toast.success(t("ops.toast_profile_ok"))
      setProfileApplyOpen(false)
      setProfileConfirm("")
    } catch (err) {
      const msg = apiErrorDetail(err, t("ops.toast_profile_err"))
      setProfileErr(true)
      setProfileOut(apiErrorOutput(err, msg))
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
            <Button size="sm" variant="outline" onClick={runDoctor} disabled={anyBusy}>
              {busy === "doctor" ? <Spinner data-icon="inline-start" /> : null}
              {t("ops.run")}
            </Button>
          }
        >
          <OpsLogView
            value={doctorOut}
            error={doctorErr}
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
              <Button size="sm" variant="outline" onClick={() => runDrain("status")} disabled={anyBusy}>
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
                variant="caution"
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
          <OpsLogView
            value={drainOut}
            error={drainErr}
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

        <div className="grid gap-3 lg:col-span-2 lg:grid-cols-2">
          <OpsCard
            icon={<ActivityIcon className="size-4" />}
            title={t("ops.smoke_canary")}
            description={t("ops.probe_desc")}
            className="min-w-0"
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
                <Button size="sm" variant="outline" onClick={() => runProbe("smoke")} disabled={anyBusy}>
                  {busy === "smoke" ? <Spinner data-icon="inline-start" /> : null}
                  {t("ops.btn_smoke")}
                </Button>
                <Button size="sm" variant="outline" onClick={() => runProbe("canary")} disabled={anyBusy}>
                  {busy === "canary" ? <Spinner data-icon="inline-start" /> : null}
                  {t("ops.btn_canary")}
                </Button>
              </>
            }
          >
            <OpsLogView
              value={probeOut}
              error={probeErr}
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
            className="min-w-0"
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
                    variant="outline"
                    onClick={runProfilePreview}
                    disabled={anyBusy || !profileName}
                  >
                    {busy === "preview" ? <Spinner data-icon="inline-start" /> : null}
                    {t("ops.profile_preview")}
                  </Button>
                  <Button
                    size="sm"
                    variant="caution"
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
            <OpsLogView
              value={profileOut}
              error={profileErr}
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
            <DialogDescription asChild>
              <div className="space-y-2 text-sm text-muted-foreground">
                <p>
                  {drainAction === "fail" ? t("ops.drain_confirm_fail") : t("ops.drain_confirm_ok")}
                </p>
                {drainAction === "fail" ? (
                  <p className="text-destructive">{t("ops.drain_confirm_fail_disconnect")}</p>
                ) : null}
              </div>
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="drain-confirm">{t("common.confirm_typed_label")}</FieldLabel>
              <Input
                id="drain-confirm"
                value={drainConfirm}
                onChange={(e) => setDrainConfirm(e.target.value)}
                disabled={busy === "drain"}
                autoComplete="off"
                placeholder={i18n.language.toLowerCase().startsWith("zh") ? "确认" : "Confirm"}
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
              variant={drainAction === "fail" ? "destructive" : "caution"}
              onClick={() => drainAction && runDrain(drainAction)}
              disabled={
                standby ||
                busy === "drain" ||
                !drainAction ||
                !matchesConfirm(drainConfirm)
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
            <DialogDescription asChild>
              <div className="space-y-2 text-sm text-muted-foreground">
                {profileName ? (
                  <p>
                    {t("ops.profile_select")}: {profileShortLabel(t, profileName)}{" "}
                    <code className="font-mono text-foreground">({profileName})</code>
                  </p>
                ) : null}
                <p>{t("ops.profile_confirm_body")}</p>
                <p className="text-destructive">{t("ops.profile_confirm_disconnect")}</p>
              </div>
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="profile-confirm">{t("common.confirm_typed_label")}</FieldLabel>
              <Input
                id="profile-confirm"
                value={profileConfirm}
                onChange={(e) => setProfileConfirm(e.target.value)}
                disabled={standby || busy === "profile"}
                autoComplete="off"
                placeholder={i18n.language.toLowerCase().startsWith("zh") ? "确认" : "Confirm"}
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
              variant="caution"
              onClick={runProfileApply}
              disabled={standby || busy === "profile" || !matchesConfirm(profileConfirm)}
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
