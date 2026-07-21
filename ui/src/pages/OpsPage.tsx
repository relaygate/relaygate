import { useEffect, useState } from "react"
import { Link } from "react-router-dom"
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

type BusyKey =
  | "doctor"
  | "drain"
  | "smoke"
  | "canary"
  | "firewall"
  | "preview"
  | "profile"
  | null

type ProbeKind = "smoke" | "canary" | null

export function OpsPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [doctorOut, setDoctorOut] = useState("")
  const [drainOut, setDrainOut] = useState("")
  const [drainConfirm, setDrainConfirm] = useState("")
  const [drainAction, setDrainAction] = useState<"fail" | "ok" | null>(null)
  const [host, setHost] = useState("127.0.0.1")
  const [probeKind, setProbeKind] = useState<ProbeKind>(null)
  const [smokeOut, setSmokeOut] = useState("")
  const [canaryOut, setCanaryOut] = useState("")
  const [firewallOut, setFirewallOut] = useState("")
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profileName, setProfileName] = useState("")
  const [profileOut, setProfileOut] = useState("")
  const [profileConfirm, setProfileConfirm] = useState("")
  const [profileApplyOpen, setProfileApplyOpen] = useState(false)
  const [busy, setBusy] = useState<BusyKey>(null)

  useEffect(() => {
    getProfiles()
      .then((items) => {
        setProfiles(items)
        if (items[0]) setProfileName(items[0].name)
      })
      .catch(() => setProfiles([]))
  }, [])

  async function runDoctor() {
    setBusy("doctor")
    try {
      const res = await opsDoctor()
      setDoctorOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_doctor_ok"))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : t("ops.toast_doctor_err")
      setDoctorOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runDrain(action: "status" | "fail" | "ok") {
    setBusy("drain")
    try {
      const confirm =
        action === "fail" || action === "ok" ? drainConfirm.trim() : undefined
      const res = await opsDrain(action, confirm)
      setDrainOut(res.output ?? res.error ?? t("error.no_output"))
      setDrainAction(null)
      setDrainConfirm("")
      toast.success(tf("ops.toast_drain_ok", action))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : tf("ops.toast_drain_err", action)
      setDrainOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runProbe() {
    if (!probeKind) return
    setBusy(probeKind)
    try {
      const target = host.trim() || "127.0.0.1"
      if (probeKind === "smoke") {
        const res = await opsSmoke(target)
        setSmokeOut(res.output ?? res.error ?? t("error.no_output"))
        toast.success(t("ops.toast_smoke_ok"))
      } else {
        const res = await opsCanary(target)
        setCanaryOut(res.output ?? res.error ?? t("error.no_output"))
        toast.success(t("ops.toast_canary_ok"))
      }
      setProbeKind(null)
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : probeKind === "smoke"
            ? t("ops.toast_smoke_err")
            : t("ops.toast_canary_err")
      if (probeKind === "smoke") setSmokeOut(msg)
      else setCanaryOut(msg)
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
      const msg = err instanceof ApiError ? err.message : t("ops.toast_fw_err")
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
      setProfileOut(res.output ?? t("error.no_output"))
      toast.success(t("ops.toast_preview_ok"))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : t("ops.toast_preview_err")
      setProfileOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runProfileApply() {
    if (!profileName) return
    setBusy("profile")
    try {
      const res = await opsProfileApply(profileName, profileConfirm.trim())
      setProfileOut((res.output ?? "") + t("ops.profile_applied_body"))
      setProfileConfirm("")
      setProfileApplyOpen(false)
      toast.success(t("ops.toast_profile_ok"))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : t("ops.toast_profile_err")
      setProfileOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  return (
    <Page className="gap-5">
      <PageHeader title={t("ops.title")} />

      <Section title={t("ops.doctor")}>
        <Button onClick={runDoctor} disabled={busy === "doctor"}>
          {busy === "doctor" ? <Spinner data-icon="inline-start" /> : null}
          {t("ops.run")}
        </Button>
        <DiffView value={doctorOut} placeholder={t("ops.doctor_placeholder")} />
      </Section>

      <Section title={t("ops.drain")}>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            title={t("ops.btn_drain_status")}
            onClick={() => runDrain("status")}
            disabled={busy === "drain"}
          >
            {busy === "drain" ? <Spinner data-icon="inline-start" /> : null}
            {t("ops.btn_drain_status")}
          </Button>
          <Button
            variant="outline"
            title={t("ops.btn_drain_fail")}
            onClick={() => {
              setDrainAction("fail")
              setDrainConfirm("")
            }}
            disabled={standby}
          >
            {t("ops.btn_drain_fail")}
          </Button>
          <Button
            variant="outline"
            title={t("ops.btn_drain_ok")}
            onClick={() => {
              setDrainAction("ok")
              setDrainConfirm("")
            }}
            disabled={standby}
          >
            {t("ops.btn_drain_ok")}
          </Button>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">{t("ops.drain_hint")}</p>
        <DiffView value={drainOut} placeholder={t("error.no_output")} />
      </Section>

      <Dialog
        open={!!drainAction}
        onOpenChange={(open) => !open && busy !== "drain" && setDrainAction(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("ops.drain")}</DialogTitle>
            <DialogDescription>
              {drainAction === "fail" ? t("ops.drain_label_fail") : t("ops.drain_label_ok")}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>
                {drainAction === "fail" ? "DRAIN_FAIL" : "DRAIN_OK"}
              </FieldLabel>
              <Input
                value={drainConfirm}
                onChange={(e) => setDrainConfirm(e.target.value)}
                autoComplete="off"
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDrainAction(null)} disabled={busy === "drain"}>
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

      <Section title={t("ops.smoke_canary")}>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            onClick={() => setProbeKind("smoke")}
            disabled={busy === "smoke" || busy === "canary"}
          >
            {t("ops.btn_smoke")}
          </Button>
          <Button
            variant="outline"
            onClick={() => setProbeKind("canary")}
            disabled={busy === "smoke" || busy === "canary"}
          >
            {t("ops.btn_canary")}
          </Button>
        </div>
        <DiffView value={smokeOut || canaryOut} placeholder={t("error.no_output")} />
      </Section>

      <Dialog
        open={!!probeKind}
        onOpenChange={(open) => {
          if (!open && busy !== "smoke" && busy !== "canary") setProbeKind(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {probeKind === "canary" ? t("ops.btn_canary") : t("ops.btn_smoke")}
            </DialogTitle>
            <DialogDescription>{t("ops.smoke_canary")}</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="ops-host">{t("ops.host")}</FieldLabel>
              <Input
                id="ops-host"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                disabled={busy === "smoke" || busy === "canary"}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => setProbeKind(null)}
              disabled={busy === "smoke" || busy === "canary"}
            >
              {t("ops.cancel")}
            </Button>
            <Button onClick={runProbe} disabled={busy === "smoke" || busy === "canary"}>
              {busy === "smoke" || busy === "canary" ? (
                <Spinner data-icon="inline-start" />
              ) : null}
              {t("ops.run")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Section title={t("ops.firewall")}>
        <p className="text-xs text-muted-foreground">
          {t("ops.fw_check_label")} <code className="font-mono">sudo relaygate firewall check</code>
        </p>
        <p className="text-xs text-muted-foreground">
          {t("ops.fw_apply_label")}{" "}
          <Link to="/apply" className="underline underline-offset-2">
            {t("ops.fw_apply_link")}
          </Link>
        </p>
        <Button variant="outline" onClick={runFirewall} disabled={busy === "firewall"}>
          {busy === "firewall" ? <Spinner data-icon="inline-start" /> : null}
          {t("ops.run")} check
        </Button>
        <DiffView value={firewallOut} placeholder={t("error.no_output")} />
      </Section>

      <Section title={t("ops.profile")}>
        {profiles.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("ops.profile_empty")}</p>
        ) : (
          <FieldGroup className="max-w-lg gap-3">
            <Field>
              <FieldLabel>{t("ops.profile_select")}</FieldLabel>
              <Select value={profileName} onValueChange={(v) => setProfileName(v ?? "")}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {profiles.map((p) => (
                    <SelectItem key={p.name} value={p.name}>
                      {p.name}
                      {p.description ? ` — ${p.description}` : ""}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                onClick={runProfilePreview}
                disabled={busy === "preview"}
              >
                {busy === "preview" ? <Spinner data-icon="inline-start" /> : null}
                {t("ops.profile_preview")}
              </Button>
              <Button
                onClick={() => {
                  setProfileConfirm("")
                  setProfileApplyOpen(true)
                }}
                disabled={standby || !profileName}
              >
                {t("ops.profile_apply")}
              </Button>
            </div>
          </FieldGroup>
        )}
        <DiffView value={profileOut} placeholder={t("error.no_output")} />
      </Section>

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
                  {t("ops.profile_select")}:{" "}
                  <code className="font-mono text-foreground">{profileName}</code>
                </>
              ) : null}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>{t("ops.profile_confirm")}</FieldLabel>
              <Input
                value={profileConfirm}
                onChange={(e) => setProfileConfirm(e.target.value)}
                disabled={standby || busy === "profile"}
                autoComplete="off"
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
