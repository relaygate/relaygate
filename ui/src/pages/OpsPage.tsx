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

export function OpsPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [doctorOut, setDoctorOut] = useState("")
  const [drainOut, setDrainOut] = useState("")
  const [drainConfirm, setDrainConfirm] = useState("")
  const [drainAction, setDrainAction] = useState<"fail" | "ok" | null>(null)
  const [host, setHost] = useState("127.0.0.1")
  const [smokeOut, setSmokeOut] = useState("")
  const [canaryOut, setCanaryOut] = useState("")
  const [firewallOut, setFirewallOut] = useState("")
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profileName, setProfileName] = useState("")
  const [profileOut, setProfileOut] = useState("")
  const [profileConfirm, setProfileConfirm] = useState("")
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

  async function runSmoke() {
    setBusy("smoke")
    try {
      const res = await opsSmoke(host.trim() || "127.0.0.1")
      setSmokeOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_smoke_ok"))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : t("ops.toast_smoke_err")
      setSmokeOut(msg)
      toast.error(msg)
    } finally {
      setBusy(null)
    }
  }

  async function runCanary() {
    setBusy("canary")
    try {
      const res = await opsCanary(host.trim() || "127.0.0.1")
      setCanaryOut(res.output ?? res.error ?? t("error.no_output"))
      toast.success(t("ops.toast_canary_ok"))
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : t("ops.toast_canary_err")
      setCanaryOut(msg)
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
            onClick={() => runDrain("status")}
            disabled={busy === "drain"}
          >
            {busy === "drain" ? <Spinner data-icon="inline-start" /> : null}
            status
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              setDrainAction("fail")
              setDrainConfirm("")
            }}
            disabled={standby}
          >
            fail
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              setDrainAction("ok")
              setDrainConfirm("")
            }}
            disabled={standby}
          >
            ok
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">{t("ops.drain_hint")}</p>
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
        <FieldGroup className="max-w-md">
          <Field>
            <FieldLabel htmlFor="ops-host">{t("ops.host")}</FieldLabel>
            <Input id="ops-host" value={host} onChange={(e) => setHost(e.target.value)} />
          </Field>
          <div className="flex gap-2">
            <Button variant="outline" onClick={runSmoke} disabled={busy === "smoke"}>
              {busy === "smoke" ? <Spinner data-icon="inline-start" /> : null}
              smoke
            </Button>
            <Button variant="outline" onClick={runCanary} disabled={busy === "canary"}>
              {busy === "canary" ? <Spinner data-icon="inline-start" /> : null}
              canary
            </Button>
          </div>
        </FieldGroup>
        <DiffView value={smokeOut || canaryOut} placeholder={t("error.no_output")} />
      </Section>

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
            </div>
            <Field>
              <FieldLabel>{t("ops.profile_confirm")}</FieldLabel>
              <Input
                value={profileConfirm}
                onChange={(e) => setProfileConfirm(e.target.value)}
                disabled={standby}
              />
            </Field>
            <Button
              onClick={runProfileApply}
              disabled={standby || busy === "profile" || profileConfirm !== "APPLY_PROFILE"}
            >
              {busy === "profile" ? <Spinner data-icon="inline-start" /> : null}
              Apply profile
            </Button>
          </FieldGroup>
        )}
        <DiffView value={profileOut} placeholder={t("error.no_output")} />
      </Section>
    </Page>
  )
}
