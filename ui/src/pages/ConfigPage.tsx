import { useCallback, useEffect, useRef, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  DownloadIcon,
  FileCodeIcon,
  PackageIcon,
  PencilIcon,
  SaveIcon,
} from "lucide-react"

import { IntentSourceNote } from "@/components/layout/IntentSourceNote"
import { Page, PageHeader, OutputPre, Section } from "@/components/layout/PageParts"
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Textarea } from "@/components/ui/textarea"
import { useStandby } from "@/context/SessionContext"
import {
  ApiError,
  exportConfigPack,
  exportConfigYAML,
  getConfigResources,
  putConfigResources,
  validateConfigResources,
  type ConfigValidateResult,
} from "@/lib/api"

export function ConfigPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const fileRef = useRef<HTMLInputElement>(null)

  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState(false)
  const [content, setContent] = useState("")
  const [mtime, setMtime] = useState("")
  const [etag, setEtag] = useState("")
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [result, setResult] = useState<ConfigValidateResult | null>(null)
  const [saveDiff, setSaveDiff] = useState("")

  const load = useCallback(async () => {
    const data = await getConfigResources()
    setContent(data.content)
    setMtime(data.mtime)
    setEtag(data.etag)
    setDirty(false)
    setResult(null)
    setSaveDiff("")
  }, [])

  useEffect(() => {
    load()
      .catch((err) => {
        toast.error(err instanceof ApiError ? err.message : t("config.toast_load_fail"))
      })
      .finally(() => setLoading(false))
  }, [load, t])

  function enterEdit() {
    setEditing(true)
    setResult(null)
    setSaveDiff("")
  }

  function cancelEdit() {
    setEditing(false)
    setConfirmOpen(false)
    load().catch(() => {})
  }

  /** Validate then open confirm; save itself re-validates on the server. */
  async function requestSave() {
    if (standby || !dirty) return
    setSaving(true)
    try {
      const res = await validateConfigResources(content)
      setResult(res)
      if (!res.ok) {
        toast.error(t("config.toast_validate_fail"))
        return
      }
      setConfirmOpen(true)
    } catch (err) {
      if (err instanceof ApiError && err.body && typeof err.body === "object") {
        const body = err.body as ConfigValidateResult
        if (Array.isArray(body.errors)) {
          setResult(body)
          toast.error(t("config.toast_validate_fail"))
          return
        }
      }
      toast.error(err instanceof ApiError ? err.message : t("config.toast_validate_fail"))
    } finally {
      setSaving(false)
    }
  }

  async function handleSave() {
    if (standby) return
    setSaving(true)
    try {
      const res = await putConfigResources({ content, etag, mtime })
      setEtag(res.etag)
      setMtime(res.mtime)
      setSaveDiff(res.diff || "")
      setResult({ ok: true, diff: res.diff })
      setDirty(false)
      setEditing(false)
      setConfirmOpen(false)
      toast.success(t("config.toast_save_ok"))
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        const body = err.body as { content?: string; etag?: string; mtime?: string }
        toast.error(t("config.toast_conflict"))
        if (typeof body?.content === "string") {
          setContent(body.content)
          setEtag(body.etag ?? "")
          setMtime(body.mtime ?? "")
          setDirty(false)
        }
        setConfirmOpen(false)
        return
      }
      if (err instanceof ApiError && err.body && typeof err.body === "object") {
        const body = err.body as ConfigValidateResult
        if (Array.isArray(body.errors)) {
          setResult(body)
          setConfirmOpen(false)
          toast.error(t("config.toast_validate_fail"))
          return
        }
      }
      toast.error(err instanceof ApiError ? err.message : t("config.toast_save_fail"))
    } finally {
      setSaving(false)
    }
  }

  async function handleExportYAML() {
    try {
      await exportConfigYAML()
      toast.success(t("config.toast_export_ok"))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("config.toast_export_fail"))
    }
  }

  async function handleExportZip() {
    try {
      await exportConfigPack()
      toast.success(t("config.toast_export_ok"))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("config.toast_export_fail"))
    }
  }

  function onImportFile(file: File | null) {
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const text = typeof reader.result === "string" ? reader.result : ""
      setContent(text)
      setDirty(true)
      setEditing(true)
      setResult(null)
      toast.message(t("config.import_ready"))
    }
    reader.readAsText(file)
  }

  const errorText =
    result && !result.ok && result.errors?.length
      ? result.errors
          .map((e) => {
            const loc = [e.line != null ? `L${e.line}` : "", e.path].filter(Boolean).join(" ")
            return loc ? `${loc}: ${e.msg}` : e.msg
          })
          .join("\n")
      : ""

  const headerActions = (
    <div className="flex flex-wrap items-center gap-2">
      {!editing ? (
        <Button variant="outline" size="sm" disabled={standby || loading} onClick={enterEdit}>
          <PencilIcon data-icon="inline-start" />
          {t("common.edit")}
        </Button>
      ) : (
        <>
          <Button
            size="sm"
            disabled={standby || saving || loading || !dirty}
            onClick={requestSave}
          >
            <SaveIcon data-icon="inline-start" />
            {saving && !confirmOpen ? t("common.working") : t("config.save")}
          </Button>
          <Button variant="ghost" size="sm" disabled={saving} onClick={cancelEdit}>
            {t("config.cancel")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={standby || saving}
            onClick={() => fileRef.current?.click()}
          >
            <FileCodeIcon data-icon="inline-start" />
            {t("config.import")}
          </Button>
        </>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="outline" size="sm" disabled={loading} />}
        >
          <DownloadIcon data-icon="inline-start" />
          {t("config.export")}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={handleExportYAML}>
              <DownloadIcon data-icon="inline-start" />
              {t("config.export")} YAML
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleExportZip}>
              <PackageIcon data-icon="inline-start" />
              {t("config.export_pack")}
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      <input
        ref={fileRef}
        type="file"
        accept=".yaml,.yml,text/yaml,text/plain"
        className="hidden"
        onChange={(e) => {
          onImportFile(e.target.files?.[0] ?? null)
          e.target.value = ""
        }}
      />
    </div>
  )

  return (
    <Page>
      <PageHeader
        title={t("config.title")}
        hint={mtime ? `${t("config.hint")} · mtime ${mtime}` : t("config.hint")}
        actions={headerActions}
      />
      <IntentSourceNote />

      {editing ? (
        <Alert>
          <PencilIcon />
          <AlertTitle>{t("config.edit_title")}</AlertTitle>
          <AlertDescription>{t("config.edit_warning")}</AlertDescription>
        </Alert>
      ) : null}

      <Section title="resources.yaml">
        <Textarea
          value={loading ? "" : content}
          readOnly={!editing}
          spellCheck={false}
          onChange={(e) => {
            setContent(e.target.value)
            setDirty(true)
          }}
          className="min-h-[28rem] resize-y rounded-lg border border-border bg-muted/30 font-mono text-xs leading-relaxed"
          placeholder={loading ? t("common.loading") : ""}
        />
      </Section>

      {errorText ? (
        <Section title={t("config.errors")}>
          <OutputPre value={errorText} error />
        </Section>
      ) : null}

      {result?.ok && result.diff ? (
        <Section title={t("config.diff")}>
          <OutputPre value={result.diff} />
        </Section>
      ) : null}

      {saveDiff ? (
        <Alert>
          <AlertTitle>{t("config.saved_title")}</AlertTitle>
          <AlertDescription>
            {t("config.saved_hint")}{" "}
            <Link to="/apply" className="underline underline-offset-2">
              {t("nav.apply")}
            </Link>
          </AlertDescription>
        </Alert>
      ) : null}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("config.save_confirm_title")}</DialogTitle>
            <DialogDescription>{t("config.save_confirm_body")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={saving}>
              {t("config.cancel")}
            </Button>
            <Button onClick={handleSave} disabled={saving || standby}>
              {saving ? t("common.working") : t("config.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
