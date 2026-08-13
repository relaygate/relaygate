import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { FileClockIcon, TriangleAlertIcon } from "lucide-react"

import { DiffView } from "@/components/layout/DiffView"
import { EmptyState } from "@/components/layout/EmptyState"
import { OpsLogView } from "@/components/layout/OpsLogView"
import { Page, PageHeader, Section } from "@/components/layout/PageParts"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import { Spinner } from "@/components/ui/spinner"
import { useStandby } from "@/context/SessionContext"
import { ApiError, applyConfig, applyFirewall, getApplyPreview } from "@/lib/api"
import { matchesConfirm } from "@/lib/confirm"

export function ApplyPage() {
  const { t, i18n } = useTranslation()
  const standby = useStandby()
  const [summary, setSummary] = useState("")
  const [lastApply, setLastApply] = useState("")
  const [needsReload, setNeedsReload] = useState(false)
  const [needsFirewall, setNeedsFirewall] = useState(false)
  const [applyMode, setApplyMode] = useState<"hot" | "hard" | "none" | undefined>()
  const [bootstrapMigrated, setBootstrapMigrated] = useState<boolean | undefined>()
  const [result, setResult] = useState("")
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)
  const [applyingConfig, setApplyingConfig] = useState(false)
  const [applyingSecurityFirewall, setApplyingSecurityFirewall] = useState(false)
  const [fwOpen, setFwOpen] = useState(false)
  const [fwConfirm, setFwConfirm] = useState("")
  const [reloadOpen, setReloadOpen] = useState(false)
  const [reloadConfirm, setReloadConfirm] = useState("")

  async function loadPreview() {
    try {
      const preview = await getApplyPreview()
      setSummary(preview.summary)
      setLastApply(preview.last_apply)
      setNeedsReload(preview.needs_reload)
      setNeedsFirewall(preview.needs_firewall)
      setApplyMode(preview.apply_mode)
      setBootstrapMigrated(preview.bootstrap_migrated)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_load_fail"))
    }
  }

  useEffect(() => {
    loadPreview().finally(() => setLoading(false))
  }, [])

  const configConfirmTitle =
    applyMode === "hot" ? t("apply.hot_confirm_title") : t("apply.reload_confirm_title")
  const configConfirmBody =
    applyMode === "hot" ? t("apply.hot_confirm_body") : t("apply.reload_confirm_body")
  const configConfirmDisconnect =
    applyMode === "hot"
      ? t("apply.hot_confirm_disconnect")
      : t("apply.reload_confirm_disconnect")
  const configHint =
    applyMode === "hot"
      ? t("apply.hint_config_hot")
      : applyMode === "hard"
        ? t("apply.hint_config_hard")
        : t("apply.hint_config")
  const confirmLabel = t("common.confirm_typed_label")
  const confirmPlaceholder = i18n.language.toLowerCase().startsWith("zh") ? "确认" : "Confirm"

  async function handleApplyConfig() {
    if (standby || !needsReload || !matchesConfirm(reloadConfirm)) return
    setApplyingConfig(true)
    setError(false)
    try {
      const res = await applyConfig(reloadConfirm.trim())
      const out = res.output ?? t("apply.toast_config_ok")
      setResult(out)
      toast.success(t("apply.toast_config_ok"))
      setReloadOpen(false)
      setReloadConfirm("")
      await loadPreview()
    } catch (err) {
      setError(true)
      const msg = err instanceof ApiError ? err.message : t("apply.toast_config_fail")
      const body = err instanceof ApiError ? (err.body as Record<string, unknown>) : null
      const out = typeof body?.output === "string" ? body.output : msg
      setResult(out)
      toast.error(msg)
    } finally {
      setApplyingConfig(false)
    }
  }

  async function handleApplySecurityFirewall() {
    if (standby || !matchesConfirm(fwConfirm)) return
    setApplyingSecurityFirewall(true)
    setError(false)
    try {
      const res = await applyFirewall(fwConfirm.trim())
      const out = res.output ?? t("apply.toast_security_firewall_ok")
      setResult(out)
      toast.success(t("apply.toast_security_firewall_ok"))
      setFwOpen(false)
      setFwConfirm("")
      await loadPreview()
    } catch (err) {
      setError(true)
      const msg = err instanceof ApiError ? err.message : t("apply.toast_security_firewall_fail")
      const body = err instanceof ApiError ? (err.body as Record<string, unknown>) : null
      const out = typeof body?.output === "string" ? body.output : msg
      setResult(out)
      toast.error(msg)
    } finally {
      setApplyingSecurityFirewall(false)
    }
  }

  const busy = applyingConfig || applyingSecurityFirewall
  const configBtnVariant = applyMode === "hard" ? "destructive" : "caution"
  const configDialogVariant = applyMode === "hard" ? "destructive" : "caution"

  const summaryBadges = (
    <>
      {needsReload ? <Badge variant="default">{t("apply.tag_config")}</Badge> : null}
      {needsFirewall ? <Badge variant="default">{t("apply.tag_security_firewall")}</Badge> : null}
      {needsReload && applyMode === "hot" ? (
        <Badge variant="secondary">{t("apply.tag_hot")}</Badge>
      ) : null}
      {needsReload && applyMode === "hard" ? (
        <Badge variant="destructive">{t("apply.tag_hard")}</Badge>
      ) : null}
      {!needsReload && !needsFirewall ? (
        <Badge variant="secondary">{t("apply.tag_none")}</Badge>
      ) : null}
    </>
  )

  return (
    <Page>
      <PageHeader
        title={t("apply.title")}
        description={t("apply.desc")}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant={configBtnVariant}
              onClick={() => {
                setReloadConfirm("")
                setReloadOpen(true)
              }}
              disabled={standby || busy || !needsReload}
              title={configHint}
            >
              {applyingConfig ? <Spinner data-icon="inline-start" /> : null}
              {applyingConfig ? t("common.working") : t("apply.submit_config")}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setFwConfirm("")
                setFwOpen(true)
              }}
              disabled={standby || busy || !needsFirewall}
              title={t("apply.hint_security_firewall")}
            >
              {t("apply.submit_security_firewall")}
            </Button>
          </div>
        }
      />

      {bootstrapMigrated === false ? (
        <Alert>
          <TriangleAlertIcon />
          <AlertTitle>{t("apply.migrate_title")}</AlertTitle>
          <AlertDescription>{t("apply.migrate_body")}</AlertDescription>
        </Alert>
      ) : null}

      <Section title={t("apply.summary")} actions={summaryBadges}>
        <DiffView
          value={loading ? "" : summary}
          placeholder={
            loading ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Spinner />
                {t("common.loading")}
              </div>
            ) : (
              <EmptyState
                icon={FileClockIcon}
                title={t("apply.none")}
                description={t("apply.none_hint")}
                className="w-full border-0 bg-transparent"
              />
            )
          }
        />
      </Section>

      <Section title={t("overview.last_apply")}>
        <OpsLogView
          value={loading ? "" : lastApply}
          placeholder={
            loading ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Spinner />
                {t("common.loading")}
              </div>
            ) : (
              <EmptyState
                icon={FileClockIcon}
                title={t("apply.none")}
                description={t("apply.none_hint")}
                className="w-full border-0 bg-transparent"
              />
            )
          }
        />
      </Section>

      {result ? (
        <Section title={t("apply.result")}>
          <OpsLogView value={result} error={error} />
        </Section>
      ) : null}

      <Dialog
        open={reloadOpen}
        onOpenChange={(open) => {
          if (!applyingConfig) {
            setReloadOpen(open)
            if (!open) setReloadConfirm("")
          }
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{configConfirmTitle}</DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-2 text-sm text-muted-foreground">
                <p>{configConfirmBody}</p>
                <p className="text-destructive">{configConfirmDisconnect}</p>
              </div>
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>{confirmLabel}</FieldLabel>
              <Input
                value={reloadConfirm}
                onChange={(e) => setReloadConfirm(e.target.value)}
                disabled={standby || applyingConfig}
                autoComplete="off"
                placeholder={confirmPlaceholder}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setReloadOpen(false)} disabled={applyingConfig}>
              {t("ops.cancel")}
            </Button>
            <Button
              variant={configDialogVariant}
              onClick={handleApplyConfig}
              disabled={standby || applyingConfig || !matchesConfirm(reloadConfirm)}
            >
              {applyingConfig ? <Spinner data-icon="inline-start" /> : null}
              {t("apply.submit_config")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={fwOpen}
        onOpenChange={(open) => {
          if (!applyingSecurityFirewall) {
            setFwOpen(open)
            if (!open) setFwConfirm("")
          }
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("apply.security_firewall_confirm_title")}</DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-2 text-sm text-muted-foreground">
                <p>{t("apply.security_firewall_confirm_body")}</p>
                <p className="text-destructive">{t("apply.security_firewall_confirm_disconnect")}</p>
              </div>
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>{confirmLabel}</FieldLabel>
              <Input
                value={fwConfirm}
                onChange={(e) => setFwConfirm(e.target.value)}
                disabled={standby || applyingSecurityFirewall}
                autoComplete="off"
                placeholder={confirmPlaceholder}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setFwOpen(false)} disabled={applyingSecurityFirewall}>
              {t("ops.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleApplySecurityFirewall}
              disabled={standby || applyingSecurityFirewall || !matchesConfirm(fwConfirm)}
            >
              {applyingSecurityFirewall ? <Spinner data-icon="inline-start" /> : null}
              {t("apply.submit_security_firewall")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
