import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { Page, PageHeader, Section } from "@/components/layout/PageParts"
import { Button } from "@/components/ui/button"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useStandby } from "@/context/SessionContext"
import { addACL, ApiError, getACL, removeACL } from "@/lib/api"
import { tf } from "@/i18n"

export function ACLPage() {
  const { t } = useTranslation()
  const standby = useStandby()
  const [deny, setDeny] = useState<string[]>([])
  const [allow, setAllow] = useState<string[]>([])
  const [list, setList] = useState<"deny" | "allow">("deny")
  const [cidr, setCidr] = useState("")
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    const acl = await getACL()
    setDeny(acl.deny)
    setAllow(acl.allow)
  }, [])

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [load])

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (standby || !cidr.trim()) return
    try {
      const acl = await addACL(list, cidr.trim())
      setDeny(acl.deny)
      setAllow(acl.allow)
      setCidr("")
      toast.success(tf("acl.toast_added", cidr.trim()))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    }
  }

  async function handleRemove(listName: "deny" | "allow", entry: string) {
    if (standby) return
    try {
      const acl = await removeACL(listName, entry)
      setDeny(acl.deny)
      setAllow(acl.allow)
      toast.success(tf("acl.toast_removed", entry))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    }
  }

  function renderList(name: "deny" | "allow", entries: string[], strict?: boolean) {
    const title = name === "deny" ? "ACL_DENY" : "ACL_ALLOW"
    return (
      <Section
        title={`${title} ${strict ? t("acl.strict") : t("acl.non_strict")}`}
        className="flex-1"
      >
        {loading ? (
          <p className="text-sm text-muted-foreground">…</p>
        ) : entries.length === 0 ? (
          <p className="rounded-lg border border-border bg-muted/20 p-3 text-sm text-muted-foreground">
            {t("acl.empty")}
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {entries.map((entry) => (
              <li
                key={entry}
                className="flex items-center justify-between gap-2 rounded-md border border-border bg-card/30 px-3 py-2 font-mono text-xs"
              >
                <span>{entry}</span>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => handleRemove(name, entry)}
                  disabled={standby}
                >
                  {t("acl.remove")}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Section>
    )
  }

  return (
    <Page>
      <PageHeader title={t("acl.title")} hint={t("acl.hint")} />

      <form
        onSubmit={handleAdd}
        className="flex flex-col gap-4 rounded-lg border border-border bg-card/30 p-4"
      >
        <h2 className="text-sm font-semibold">{t("acl.add_heading")}</h2>
        <FieldGroup className="grid gap-4 md:grid-cols-3">
          <Field>
            <FieldLabel>{t("acl.list")}</FieldLabel>
            <Select
              value={list}
              onValueChange={(v) => setList(v as "deny" | "allow")}
              disabled={standby}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="deny">deny</SelectItem>
                <SelectItem value="allow">allow</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field className="md:col-span-2">
            <FieldLabel htmlFor="acl-cidr">{t("acl.cidr")}</FieldLabel>
            <Input
              id="acl-cidr"
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              placeholder="203.0.113.0/24"
              disabled={standby}
            />
          </Field>
        </FieldGroup>
        <Button type="submit" disabled={standby || !cidr.trim()} className="w-fit">
          {t("acl.add")}
        </Button>
      </form>

      <div className="grid gap-6 lg:grid-cols-2">
        {renderList("deny", deny)}
        {renderList("allow", allow, allow.length > 0)}
      </div>
    </Page>
  )
}
