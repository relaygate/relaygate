import { useCallback, useEffect, useState } from "react"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { DownloadIcon, PencilIcon, SaveIcon } from "lucide-react"

import { Page, PageHeader, OutputPre, Section } from "@/components/layout/PageParts"
import { YamlEditor } from "@/components/layout/YamlEditor"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { useStandby } from "@/context/SessionContext"
import {
  ApiError,
  exportConfigYAML,
  getConfigResources,
  putConfigResources,
  validateConfigResources,
  type ConfigValidateResult,
} from "@/lib/api"

/** Local wall time for display; truncate sub-second noise. Full raw stays in title. */
function formatConfigMtime(raw: string): string {
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) {
    return raw.replace(/(\.\d{3})\d+/, "$1")
  }
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function ConfigPage({ embedded = false }: { embedded?: boolean }) {
  const { t } = useTranslation()
  const standby = useStandby()

  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState(false)
  const [content, setContent] = useState("")
  const [mtime, setMtime] = useState("")
  const [etag, setEtag] = useState("")
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
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
    load().catch(() => {})
  }

  async function handleSave() {
    if (standby || !dirty) return
    setSaving(true)
    try {
      const validated = await validateConfigResources(content)
      setResult(validated)
      if (!validated.ok) {
        toast.error(t("config.toast_validate_fail"))
        return
      }
      const res = await putConfigResources({ content, etag, mtime })
      setEtag(res.etag)
      setMtime(res.mtime)
      setSaveDiff(res.diff || "")
      setResult({ ok: true, diff: res.diff })
      setDirty(false)
      setEditing(false)
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
        return
      }
      if (err instanceof ApiError && err.body && typeof err.body === "object") {
        const body = err.body as ConfigValidateResult
        if (Array.isArray(body.errors)) {
          setResult(body)
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
        <>
          <Button
            variant="outline"
            size="sm"
            disabled={standby || loading}
            onClick={enterEdit}
            title={t("common.edit")}
          >
            <PencilIcon data-icon="inline-start" />
            {t("common.edit")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={loading}
            onClick={handleExportYAML}
            title={t("config.export")}
          >
            <DownloadIcon data-icon="inline-start" />
            {t("config.export")}
          </Button>
        </>
      ) : (
        <>
          <Button
            size="sm"
            disabled={standby || saving || loading || !dirty}
            onClick={handleSave}
            title={t("config.save")}
          >
            <SaveIcon data-icon="inline-start" />
            {saving ? t("common.working") : t("config.save")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={saving}
            onClick={cancelEdit}
            title={t("config.cancel")}
          >
            {t("config.cancel")}
          </Button>
        </>
      )}
    </div>
  )

  const main = (
    <>
      {embedded ? <div className="flex justify-end">{headerActions}</div> : null}
      {editing ? (
        <Alert>
          <PencilIcon />
          <AlertTitle>{t("config.edit_title")}</AlertTitle>
          <AlertDescription>{t("config.edit_warning")}</AlertDescription>
        </Alert>
      ) : null}
      <Section
        title={t("config.section_title")}
        actions={
          mtime ? (
            <Badge
              variant="outline"
              title={mtime}
              className="h-5 font-normal tabular-nums text-muted-foreground"
            >
              {formatConfigMtime(mtime)}
            </Badge>
          ) : null
        }
      >
        <YamlEditor
          value={loading ? "" : content}
          readOnly={!editing}
          placeholder={loading ? t("common.loading") : ""}
          onChange={(v) => {
            setContent(v)
            setDirty(true)
          }}
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
    </>
  )

  if (embedded) {
    return <div className="flex flex-col gap-3">{main}</div>
  }

  return (
    <Page>
      <PageHeader
        title={t("config.title")}
        description={t("config.desc")}
        actions={headerActions}
      />
      {main}
    </Page>
  )
}
