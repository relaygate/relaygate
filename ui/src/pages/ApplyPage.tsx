import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Page, PageHeader, OutputPre, Section } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import { useStandby } from "@/context/SessionContext"
import { ApiError, applyConfig, getApplyPreview } from "@/lib/api"

export function ApplyPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [summary, setSummary] = useState("")
  const [lastApply, setLastApply] = useState("")
  const [result, setResult] = useState("")
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)
  const [applying, setApplying] = useState(false)

  async function loadPreview() {
    try {
      const preview = await getApplyPreview()
      setSummary(preview.summary)
      setLastApply(preview.last_apply)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_load_fail"))
    }
  }

  useEffect(() => {
    loadPreview().finally(() => setLoading(false))
  }, [])

  async function handleApply() {
    if (standby) return
    setApplying(true)
    setError(false)
    try {
      const res = await applyConfig()
      const out = res.output ?? t("apply.toast_ok")
      setResult(out)
      toast.success(t("apply.toast_ok"))
      await loadPreview()
    } catch (err) {
      setError(true)
      const msg = err instanceof ApiError ? err.message : t("apply.toast_fail")
      const body = err instanceof ApiError ? (err.body as Record<string, unknown>) : null
      const out = typeof body?.output === "string" ? body.output : msg
      setResult(out)
      toast.error(msg)
    } finally {
      setApplying(false)
    }
  }

  return (
    <Page>
      <PageHeader title={t("apply.title")} />
      <Section title={t("apply.summary")}>
        <OutputPre
          value={loading ? "" : summary}
          placeholder={loading ? "…" : t("apply.none")}
        />
      </Section>
      <Section title={t("overview.last_apply")}>
        <OutputPre value={lastApply} placeholder={t("apply.none")} />
      </Section>
      <div className="flex items-center gap-3">
        <Button onClick={handleApply} disabled={standby || applying}>
          {t("apply.submit")}
        </Button>
      </div>
      {result ? (
        <Section title="Result">
          <OutputPre value={result} error={error} />
        </Section>
      ) : null}
    </Page>
  )
}
