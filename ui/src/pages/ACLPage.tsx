import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { PlusIcon } from "lucide-react"

import { IntentSourceNote } from "@/components/layout/IntentSourceNote"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
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
  const [addOpen, setAddOpen] = useState(false)
  const [adding, setAdding] = useState(false)

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
    setAdding(true)
    try {
      const acl = await addACL(list, cidr.trim())
      setDeny(acl.deny)
      setAllow(acl.allow)
      setCidr("")
      setAddOpen(false)
      toast.success(tf("acl.toast_added", cidr.trim()))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setAdding(false)
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
      <Section title={title} className="flex-1">
        {strict !== undefined ? (
          <Badge variant={strict ? "default" : "secondary"} className="w-fit text-[10px]">
            {strict ? t("acl.strict") : t("acl.non_strict")}
          </Badge>
        ) : null}
        {loading ? (
          <div className="flex flex-col gap-2">
            <div className="h-8 animate-pulse rounded-md bg-muted" />
            <div className="h-8 animate-pulse rounded-md bg-muted" />
          </div>
        ) : entries.length === 0 ? (
          <p className="rounded-md border border-dashed border-border/60 bg-muted/10 px-3 py-3 text-sm text-muted-foreground">
            {t("acl.empty")}
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {entries.map((entry) => (
              <li
                key={entry}
                className="flex items-center justify-between gap-2 rounded-md border border-border/60 bg-background/40 px-3 py-2 font-mono text-xs"
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
      <PageHeader
        title={t("acl.title")}
        hint={t("acl.hint")}
        actions={
          <Button
            size="sm"
            disabled={standby}
            onClick={() => {
              setCidr("")
              setList("deny")
              setAddOpen(true)
            }}
          >
            <PlusIcon data-icon="inline-start" />
            {t("acl.add")}
          </Button>
        }
      />
      <IntentSourceNote />

      <div className="grid gap-6 lg:grid-cols-2">
        {renderList("deny", deny)}
        {renderList("allow", allow, allow.length > 0)}
      </div>

      <Dialog
        open={addOpen}
        onOpenChange={(open) => {
          if (!adding) {
            setAddOpen(open)
            if (!open) setCidr("")
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("acl.add_heading")}</DialogTitle>
            <DialogDescription>{t("acl.hint")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={handleAdd} className="flex flex-col gap-4">
            <FieldGroup className="grid gap-3">
              <Field>
                <FieldLabel>{t("acl.list")}</FieldLabel>
                <Select
                  value={list}
                  onValueChange={(v) => setList(v as "deny" | "allow")}
                  disabled={standby || adding}
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
              <Field>
                <FieldLabel htmlFor="acl-cidr">{t("acl.cidr")}</FieldLabel>
                <Input
                  id="acl-cidr"
                  value={cidr}
                  onChange={(e) => setCidr(e.target.value)}
                  placeholder="203.0.113.0/24"
                  disabled={standby || adding}
                  required
                />
              </Field>
            </FieldGroup>
            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={() => setAddOpen(false)}
                disabled={adding}
              >
                {t("ops.cancel")}
              </Button>
              <Button type="submit" disabled={standby || adding || !cidr.trim()}>
                {adding ? <Spinner data-icon="inline-start" /> : null}
                {t("acl.add")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
