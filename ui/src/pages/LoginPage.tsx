import { useState } from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ApiError } from "@/lib/api"
import { useSession } from "@/context/SessionContext"
import { tf } from "@/i18n"

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const { login } = useSession()
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [submitting, setSubmitting] = useState(false)

  const from = (location.state as { from?: string } | null)?.from ?? "/"

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError("")
    setSubmitting(true)
    try {
      await login(password)
      navigate(from, { replace: true })
    } catch (err) {
      let msg = t("login.error_password")
      if (err instanceof ApiError) {
        const body = err.body as Record<string, unknown> | undefined
        if (typeof body?.retry_after === "number") {
          msg = tf("login.error_rate_limit", body.retry_after)
        } else if (err.message) {
          msg = err.message
        }
      }
      setError(msg)
      toast.error(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="page-enter flex min-h-svh items-center justify-center bg-background p-6">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center gap-3">
          <img src="/favicon.svg" alt="" className="size-10 rounded-lg" />
          <div>
            <h1 className="text-lg font-semibold tracking-wide">RelayGate</h1>
            <p className="text-sm text-muted-foreground">{t("login.title")}</p>
          </div>
        </div>
        <form onSubmit={handleSubmit} className="flex flex-col gap-5 rounded-lg border border-border bg-card/50 p-6">
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="password">{t("login.password")}</FieldLabel>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoFocus
              />
            </Field>
          </FieldGroup>
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
          <Button type="submit" disabled={submitting || !password}>
            {t("login.submit")}
          </Button>
        </form>
      </div>
    </div>
  )
}
