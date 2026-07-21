import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { FileClockIcon } from "lucide-react"

import { DiffView } from "@/components/layout/DiffView"
import { EmptyState } from "@/components/layout/EmptyState"
import { Page, PageHeader, Section } from "@/components/layout/PageParts"
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

const FW_CONFIRM = "YES_FLUSH_NFTABLES"

export function ApplyPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [summary, setSummary] = useState("")
  const [lastApply, setLastApply] = useState("")
  const [needsReload, setNeedsReload] = useState(false)
  const [needsFirewall, setNeedsFirewall] = useState(false)
  const [result, setResult] = useState("")
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)
  const [applyingConfig, setApplyingConfig] = useState(false)
  const [applyingFirewall, setApplyingFirewall] = useState(false)
  const [fwOpen, setFwOpen] = useState(false)
  const [fwConfirm, setFwConfirm] = useState("")

  async function loadPreview() {
    try {
      const preview = await getApplyPreview()
      setSummary(preview.summary)
      setLastApply(preview.last_apply)
      setNeedsReload(preview.needs_reload)
      setNeedsFirewall(preview.needs_firewall)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_load_fail"))
    }
  }

  useEffect(() => {
    loadPreview().finally(() => setLoading(false))
  }, [])

  async function handleApplyConfig() {
    if (standby || !needsReload) return
    setApplyingConfig(true)
    setError(false)
    try {
      const res = await applyConfig()
      const out = res.output ?? t("apply.toast_config_ok")
      setResult(out)
      toast.success(t("apply.toast_config_ok"))
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

  async function handleApplyFirewall() {
    if (standby || fwConfirm !== FW_CONFIRM) return
    setApplyingFirewall(true)
    setError(false)
    try {
      const res = await applyFirewall(FW_CONFIRM)
      const out = res.output ?? t("apply.toast_fw_ok")
      setResult(out)
      toast.success(t("apply.toast_fw_ok"))
      setFwOpen(false)
      setFwConfirm("")
      await loadPreview()
    } catch (err) {
      setError(true)
      const msg = err instanceof ApiError ? err.message : t("apply.toast_fw_fail")
      const body = err instanceof ApiError ? (err.body as Record<string, unknown>) : null
      const out = typeof body?.output === "string" ? body.output : msg
      setResult(out)
      toast.error(msg)
    } finally {
      setApplyingFirewall(false)
    }
  }

  const configPrimary = needsReload && !needsFirewall
  const firewallPrimary = needsFirewall && !needsReload
  const mixed = needsReload && needsFirewall
  const busy = applyingConfig || applyingFirewall

  const summaryBadges = (
    <>
      {needsReload ? <Badge variant="default">{t("apply.tag_config")}</Badge> : null}
      {needsFirewall ? <Badge variant="default">{t("apply.tag_firewall")}</Badge> : null}
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
              variant={configPrimary || mixed ? "default" : "outline"}
              onClick={handleApplyConfig}
              disabled={standby || busy || !needsReload}
              title={t("apply.submit_config")}
            >
              {applyingConfig ? <Spinner data-icon="inline-start" /> : null}
              {applyingConfig ? t("common.working") : t("apply.submit_config")}
            </Button>
            <Button
              variant={firewallPrimary ? "default" : "outline"}
              onClick={() => {
                setFwConfirm("")
                setFwOpen(true)
              }}
              disabled={standby || busy || !needsFirewall}
              title={t("apply.submit_firewall")}
            >
              {t("apply.submit_firewall")}
            </Button>
          </div>
        }
      />

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
        <DiffView
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
          <DiffView value={result} error={error} />
        </Section>
      ) : null}

      <Dialog
        open={fwOpen}
        onOpenChange={(open) => {
          if (!applyingFirewall) {
            setFwOpen(open)
            if (!open) setFwConfirm("")
          }
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("apply.fw_confirm_title")}</DialogTitle>
            <DialogDescription>{t("apply.fw_confirm_body")}</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>{t("apply.fw_confirm_label")}</FieldLabel>
              <Input
                value={fwConfirm}
                onChange={(e) => setFwConfirm(e.target.value)}
                disabled={standby || applyingFirewall}
                autoComplete="off"
                className="font-mono"
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setFwOpen(false)} disabled={applyingFirewall}>
              {t("ops.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleApplyFirewall}
              disabled={standby || applyingFirewall || fwConfirm !== FW_CONFIRM}
            >
              {applyingFirewall ? <Spinner data-icon="inline-start" /> : null}
              {t("apply.submit_firewall")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
