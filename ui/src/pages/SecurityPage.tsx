import { useCallback, useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { EyeIcon, PlusIcon, ShieldPlusIcon } from "lucide-react"

import { EmptyState } from "@/components/layout/EmptyState"
import { Page, PageHeader } from "@/components/layout/PageParts"
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
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useStandby } from "@/context/SessionContext"
import {
  ApiError,
  applyFirewall,
  getConfigResources,
  getSecurityProfiles,
  mergeSecurityProfile,
  previewSecurity,
  putConfigResources,
} from "@/lib/api"
import { matchesConfirm } from "@/lib/confirm"
import {
  addProtection,
  availableCatalogEntries,
  cloneSecurityState,
  findInvalidAllowlistEntry,
  normalizeAllowlistEntries,
  parseAllowlistLines,
  parsePolicyParamsJson,
  parseSecurityPolicies,
  patchSecurityPolicies,
  policiesEqual,
  policyLayer,
  securityFromMerge,
  stringifyPolicyParams,
  validateAccess,
  SECURITY_LOCAL_SOURCE,
  type SecurityPolicy,
  type SecurityPolicyId,
  type SecurityState,
} from "@/lib/securityPolicies"
import { buildComponentSummaries } from "@/lib/securityPreviewSummary"
import type { Profile, SecurityPreview } from "@/lib/types"
import { cn } from "@/lib/utils"

export function SecurityPage() {
  const { t, i18n } = useTranslation()
  const standby = useStandby()
  const [savedPolicies, setSavedPolicies] = useState<SecurityState | null>(null)
  const [draftPolicies, setDraftPolicies] = useState<SecurityState | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [applyingFirewall, setApplyingFirewall] = useState(false)
  const [sourceLoading, setSourceLoading] = useState(false)
  const [denyText, setDenyText] = useState("")
  const [allowText, setAllowText] = useState("")
  const [applyOpen, setApplyOpen] = useState(false)
  const [applyConfirm, setApplyConfirm] = useState("")
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [profilesLoading, setProfilesLoading] = useState(true)
  const [selectedSource, setSelectedSource] = useState(SECURITY_LOCAL_SOURCE)
  const [preview, setPreview] = useState<SecurityPreview | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
  const [addSelected, setAddSelected] = useState<SecurityPolicyId | null>(null)
  /** Local JSON text per protection id (open params editor). */
  const [paramsText, setParamsText] = useState<Record<string, string>>({})
  const [paramsInvalid, setParamsInvalid] = useState<Record<string, boolean>>({})

  const dirty = useMemo(
    () =>
      savedPolicies !== null &&
      draftPolicies !== null &&
      !policiesEqual(savedPolicies, draftPolicies),
    [savedPolicies, draftPolicies],
  )

  const catalogAvailable = useMemo(
    () => (draftPolicies ? availableCatalogEntries(draftPolicies) : []),
    [draftPolicies],
  )

  function syncParamsEditors(state: SecurityState) {
    const texts: Record<string, string> = {}
    const invalid: Record<string, boolean> = {}
    for (const p of state.protections) {
      texts[p.id] = stringifyPolicyParams(p.params)
      invalid[p.id] = false
    }
    setParamsText(texts)
    setParamsInvalid(invalid)
  }

  const loadSaved = useCallback(async () => {
    const data = await getConfigResources()
    const parsed = cloneSecurityState(parseSecurityPolicies(data.content))
    setSavedPolicies(parsed)
    setDraftPolicies(cloneSecurityState(parsed))
    setDenyText((parsed.access.deny ?? []).join("\n"))
    setAllowText((parsed.access.allow ?? []).join("\n"))
    setSelectedSource(SECURITY_LOCAL_SOURCE)
    setPreview(null)
    setPreviewOpen(false)
    syncParamsEditors(parsed)
    return parsed
  }, [])

  useEffect(() => {
    loadSaved()
      .catch((err) => {
        toast.error(err instanceof ApiError ? err.message : t("security.toast_load_fail"))
      })
      .finally(() => setLoading(false))
  }, [loadSaved, t])

  useEffect(() => {
    setProfilesLoading(true)
    getSecurityProfiles()
      .then(setProfiles)
      .catch((err) => {
        setProfiles([])
        toast.error(err instanceof ApiError ? err.message : t("security.toast_profiles_fail"))
      })
      .finally(() => setProfilesLoading(false))
  }, [t])

  function patchPolicy(id: SecurityPolicyId, patch: Partial<SecurityPolicy>) {
    setDraftPolicies((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        protections: prev.protections.map((p) =>
          p.id === id
            ? {
                ...p,
                ...patch,
                params: patch.params ? { ...patch.params } : { ...p.params },
              }
            : p,
        ),
      }
    })
    setPreview(null)
    setPreviewOpen(false)
  }

  function patchAllowlist(list: "deny" | "allow", entries: string[]) {
    setDraftPolicies((prev) => {
      if (!prev) return prev
      return {
        ...prev,
        access: { ...prev.access, [list]: entries },
      }
    })
    setPreview(null)
    setPreviewOpen(false)
  }

  function patchAccessEnabled(enabled: boolean) {
    setDraftPolicies((prev) => {
      if (!prev) return prev
      return { ...prev, access: { ...prev.access, enabled } }
    })
    setPreview(null)
    setPreviewOpen(false)
  }

  function handleParamsTextChange(id: SecurityPolicyId, text: string) {
    setParamsText((prev) => ({ ...prev, [id]: text }))
    const parsed = parsePolicyParamsJson(text)
    if (!parsed) {
      setParamsInvalid((prev) => ({ ...prev, [id]: true }))
      return
    }
    setParamsInvalid((prev) => ({ ...prev, [id]: false }))
    patchPolicy(id, { params: parsed })
  }

  function openAddModal() {
    const first = catalogAvailable[0]?.id ?? null
    setAddSelected(first)
    setAddOpen(true)
  }

  function confirmAddProtection() {
    if (!draftPolicies || standby || !addSelected) return
    const next = addProtection(draftPolicies, addSelected)
    setDraftPolicies(next)
    syncParamsEditors(next)
    setPreview(null)
    setPreviewOpen(false)
    setAddOpen(false)
    setAddSelected(null)
    toast.success(t("security.add_protection_toast"))
  }

  async function savePolicies() {
    if (!draftPolicies || standby || !dirty) return
    const badJson = draftPolicies.protections.find((p) => paramsInvalid[p.id])
    if (badJson) {
      toast.error(t("security.params_json_invalid"))
      return
    }
    const nextState: SecurityState = {
      access: {
        ...draftPolicies.access,
        deny: normalizeAllowlistEntries(draftPolicies.access.deny ?? []),
        allow: normalizeAllowlistEntries(draftPolicies.access.allow ?? []),
      },
      protections: draftPolicies.protections.map((p) => ({ ...p, params: { ...p.params } })),
    }
    setDenyText(nextState.access.deny.join("\n"))
    setAllowText(nextState.access.allow.join("\n"))
    setDraftPolicies(nextState)
    syncParamsEditors(nextState)
    const accessInvalid = validateAccess(nextState.access)
    if (accessInvalid) {
      toast.error(t("security.toast_invalid_cidr", { entry: accessInvalid.slice("access:".length) }))
      return
    }
    setSaving(true)
    try {
      const current = await getConfigResources()
      const content = patchSecurityPolicies(current.content, nextState)
      await putConfigResources({ content, etag: current.etag, mtime: current.mtime })
      const saved = cloneSecurityState(nextState)
      setSavedPolicies(saved)
      setPreview(null)
      setPreviewOpen(false)
      toast.success(t("security.toast_saved"))
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("apply.toast_fail"))
    } finally {
      setSaving(false)
    }
  }

  async function handleApplyFirewall() {
    if (standby || !matchesConfirm(applyConfirm)) return
    setApplyingFirewall(true)
    try {
      await applyFirewall(applyConfirm.trim())
      toast.success(t("security.toast_firewall_ok"))
      setApplyOpen(false)
      setApplyConfirm("")
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("security.toast_firewall_fail"))
    } finally {
      setApplyingFirewall(false)
    }
  }

  async function handlePreview() {
    if (!draftPolicies) return
    setPreviewOpen(true)
    setPreviewing(true)
    try {
      const result = await previewSecurity({
        access: draftPolicies.access,
        protections: draftPolicies.protections,
      })
      setPreview(result)
    } catch (err) {
      setPreviewOpen(false)
      toast.error(err instanceof ApiError ? err.message : t("security.toast_preview_fail"))
    } finally {
      setPreviewing(false)
    }
  }

  function scenarioLabel(name: string): string {
    return t(`security.scenario_label.${name}`, { defaultValue: name })
  }

  async function handleSourceChange(value: string | null) {
    if (!value || standby || sourceLoading || value === selectedSource) return
    const prevSource = selectedSource
    setSelectedSource(value)
    setSourceLoading(true)
    try {
      if (value === SECURITY_LOCAL_SOURCE) {
        await loadSaved()
        toast.success(t("security.toast_local_loaded"))
      } else {
        const merged = await mergeSecurityProfile(value)
        const next = cloneSecurityState(
          securityFromMerge({ access: merged.access, protections: merged.protections }),
        )
        setDraftPolicies(next)
        setDenyText((next.access.deny ?? []).join("\n"))
        setAllowText((next.access.allow ?? []).join("\n"))
        syncParamsEditors(next)
        setPreview(null)
        setPreviewOpen(false)
        toast.success(t("security.toast_scenario_loaded", { name: scenarioLabel(value) }))
      }
    } catch (err) {
      setSelectedSource(prevSource)
      toast.error(err instanceof ApiError ? err.message : t("security.toast_scenario_fail"))
    } finally {
      setSourceLoading(false)
    }
  }

  function handleAllowlistBlur(list: "deny" | "allow") {
    if (!draftPolicies) return
    const normalized = normalizeAllowlistEntries(draftPolicies.access[list] ?? [])
    patchAllowlist(list, normalized)
    const text = normalized.join("\n")
    if (list === "deny") setDenyText(text)
    else setAllowText(text)
  }

  function renderAllowlistTextarea(
    list: "deny" | "allow",
    text: string,
    setText: (value: string) => void,
    entries: string[],
  ) {
    const title = list === "deny" ? "DENY" : "ALLOW"
    const invalid = findInvalidAllowlistEntry(parseAllowlistLines(text))
    return (
      <Field className="flex-1" data-invalid={invalid ? true : undefined}>
        <div className="flex flex-wrap items-center gap-2">
          <FieldLabel htmlFor={`sec-acl-${list}`}>{title}</FieldLabel>
          {list === "allow" ? (
            <Badge variant={entries.length > 0 ? "default" : "secondary"} className="text-[10px]">
              {entries.length > 0 ? t("security.acl_strict") : t("security.acl_open")}
            </Badge>
          ) : null}
        </div>
        <FieldDescription>{t("security.acl_lines_hint")}</FieldDescription>
        <Textarea
          id={`sec-acl-${list}`}
          value={text}
          disabled={standby}
          placeholder="203.0.113.0/24"
          className="min-h-24 font-mono text-xs"
          aria-invalid={invalid ? true : undefined}
          onChange={(e) => {
            const value = e.target.value
            setText(value)
            patchAllowlist(list, parseAllowlistLines(value))
          }}
          onBlur={() => handleAllowlistBlur(list)}
        />
        {invalid ? <FieldError>{t("security.acl_invalid", { entry: invalid })}</FieldError> : null}
      </Field>
    )
  }

  function renderParamsJson(p: SecurityPolicy) {
    const text = paramsText[p.id] ?? stringifyPolicyParams(p.params)
    const invalid = Boolean(paramsInvalid[p.id])
    return (
      <Field className="mt-2" data-invalid={invalid ? true : undefined}>
        <FieldLabel htmlFor={`sec-params-${p.id}`}>{t("security.params_json_label")}</FieldLabel>
        <FieldDescription>{t("security.params_json_hint")}</FieldDescription>
        <Textarea
          id={`sec-params-${p.id}`}
          value={text}
          disabled={standby || saving || !p.enabled}
          className="min-h-28 font-mono text-xs"
          aria-invalid={invalid ? true : undefined}
          spellCheck={false}
          onChange={(e) => handleParamsTextChange(p.id, e.target.value)}
        />
        {invalid ? <FieldError>{t("security.params_json_invalid")}</FieldError> : null}
      </Field>
    )
  }

  const previewSummaries = useMemo(() => {
    if (!preview || !draftPolicies) return []
    return buildComponentSummaries(preview, draftPolicies, t("security.preview_disabled"))
  }, [preview, draftPolicies, t])

  const denyEntries = draftPolicies?.access.deny ?? []
  const allowEntries = draftPolicies?.access.allow ?? []

  const confirmPlaceholder = i18n.language.toLowerCase().startsWith("zh") ? "确认" : "Confirm"

  return (
    <Page>
      <PageHeader
        title={t("security.title")}
        description={t("security.desc")}
        actions={
          <>
            <Select
              value={selectedSource}
              onValueChange={handleSourceChange}
              disabled={standby || loading || profilesLoading || sourceLoading}
            >
              <SelectTrigger className="h-7 w-44 min-w-0 max-w-[12rem] text-[0.8rem]" size="sm">
                {sourceLoading ? <Spinner className="size-3.5" /> : <SelectValue />}
              </SelectTrigger>
              <SelectContent alignItemWithTrigger className="z-[100] min-w-0 w-(--anchor-width)">
                <SelectItem value={SECURITY_LOCAL_SOURCE}>
                  <span className="truncate">{t("security.scenario_local")}</span>
                </SelectItem>
                {profiles.map((p) => (
                  <SelectItem key={p.name} value={p.name} title={p.description || p.name}>
                    <span className="truncate">{scenarioLabel(p.name)}</span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              size="sm"
              variant="outline"
              onClick={openAddModal}
              disabled={standby || loading || saving || sourceLoading || !draftPolicies}
            >
              <PlusIcon data-icon="inline-start" />
              {t("security.add_protection")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => handlePreview()}
              disabled={loading || !draftPolicies || previewing || sourceLoading}
            >
              {previewing ? <Spinner data-icon="inline-start" /> : <EyeIcon data-icon="inline-start" />}
              {t("security.preview_strategy")}
            </Button>
            <Button
              size="sm"
              onClick={savePolicies}
              disabled={standby || loading || saving || applyingFirewall || sourceLoading || !dirty}
            >
              {saving ? <Spinner data-icon="inline-start" /> : null}
              {t("security.save")}
            </Button>
            <Button
              size="sm"
              variant="destructive"
              onClick={() => {
                setApplyConfirm("")
                setApplyOpen(true)
              }}
              disabled={standby || loading || saving || applyingFirewall || sourceLoading}
            >
              {applyingFirewall ? <Spinner data-icon="inline-start" /> : null}
              {t("security.apply_firewall")}
            </Button>
          </>
        }
      />

      {loading || !draftPolicies ? (
        <div className="grid gap-2">
          <div className="h-14 animate-pulse rounded-md bg-muted" />
          <div className="h-14 animate-pulse rounded-md bg-muted" />
        </div>
      ) : (
        <ul className="divide-y divide-border/50 rounded-md border border-border/40">
          <li className="px-3 py-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-medium text-foreground">{t("security.policy.access")}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">{t("security.access_hint")}</p>
              </div>
              <Switch
                checked={draftPolicies.access.enabled}
                disabled={standby || saving}
                onCheckedChange={(v) => patchAccessEnabled(v)}
                aria-label={t("security.policy.access")}
              />
            </div>
            {draftPolicies.access.enabled ? (
              <div className={cn("mt-3 grid gap-4 lg:grid-cols-2")}>
                {renderAllowlistTextarea("deny", denyText, setDenyText, denyEntries)}
                {renderAllowlistTextarea("allow", allowText, setAllowText, allowEntries)}
              </div>
            ) : null}
          </li>
          {draftPolicies.protections.map((p) => (
            <li key={p.id} className="px-3 py-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="text-[13px] font-medium text-foreground">
                      {t(`security.policy.${p.id}`)}
                    </p>
                    <Badge variant="secondary" className="text-[10px]">
                      {t(`security.preview_component_${policyLayer(p.id)}`)}
                    </Badge>
                    {p.attack_tags.map((tag) => (
                      <Badge key={tag} variant="outline" className="text-[10px] font-mono">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </div>
                <Switch
                  checked={p.enabled}
                  disabled={standby || saving}
                  onCheckedChange={(v) => patchPolicy(p.id, { enabled: v })}
                  aria-label={t(`security.policy.${p.id}`)}
                />
              </div>
              {renderParamsJson(p)}
            </li>
          ))}
        </ul>
      )}

      <p className="text-xs text-muted-foreground">{t("security.save_hint")}</p>

      <Dialog
        open={addOpen}
        onOpenChange={(open) => {
          setAddOpen(open)
          if (!open) setAddSelected(null)
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("security.add_protection_title")}</DialogTitle>
            <DialogDescription>{t("security.add_protection_desc")}</DialogDescription>
          </DialogHeader>
          {catalogAvailable.length === 0 ? (
            <EmptyState
              compact
              icon={ShieldPlusIcon}
              title={t("security.add_protection_full")}
              description={t("security.add_protection_full_hint")}
            />
          ) : (
            <ul className="max-h-[min(50vh,20rem)] space-y-2 overflow-y-auto">
              {catalogAvailable.map((entry) => {
                const selected = addSelected === entry.id
                return (
                  <li key={entry.id}>
                    <button
                      type="button"
                      disabled={standby}
                      onClick={() => setAddSelected(entry.id)}
                      className={cn(
                        "flex w-full flex-col gap-1 rounded-md border px-3 py-2.5 text-left transition-colors",
                        selected
                          ? "border-foreground/40 bg-muted/40"
                          : "border-border/60 hover:bg-muted/20",
                      )}
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-medium">{t(`security.policy.${entry.id}`)}</span>
                        <Badge variant="secondary" className="text-[10px]">
                          {t(`security.preview_component_${entry.layer}`)}
                        </Badge>
                      </div>
                      <span className="font-mono text-[11px] text-muted-foreground">{entry.type}</span>
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
          <DialogFooter>
            <Button variant="ghost" onClick={() => setAddOpen(false)}>
              {t("ops.cancel")}
            </Button>
            <Button
              variant="outline"
              onClick={confirmAddProtection}
              disabled={standby || catalogAvailable.length === 0 || !addSelected}
            >
              {t("security.add_protection_confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={previewOpen}
        onOpenChange={(open) => {
          if (!previewing) setPreviewOpen(open)
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("security.preview_title")}</DialogTitle>
            <DialogDescription>{t("security.save_hint")}</DialogDescription>
          </DialogHeader>
          {preview && previewSummaries.length > 0 ? (
            <div className="max-h-[min(70vh,28rem)] space-y-3 overflow-y-auto pr-1">
              {previewSummaries.map((section) => (
                <div
                  key={section.id}
                  className="rounded-md border border-border/60 bg-muted/20 px-3 py-2.5"
                >
                  <div className="mb-2 flex items-center gap-2">
                    <span className="font-mono text-xs font-semibold">
                      [{t(`security.preview_component_${section.id}`)}]
                    </span>
                    {!section.enabled ? (
                      <Badge variant="secondary" className="text-[10px]">
                        {t("security.preview_disabled")}
                      </Badge>
                    ) : null}
                  </div>
                  <dl className="space-y-1">
                    {section.params.map((row) => (
                      <div
                        key={`${section.id}-${row.key}`}
                        className="grid grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)] gap-x-3 text-xs"
                      >
                        <dt className="truncate font-mono text-muted-foreground">{row.key}</dt>
                        <dd
                          className={cn(
                            "truncate text-right font-mono",
                            row.value === t("security.preview_disabled") && "text-muted-foreground",
                          )}
                        >
                          {row.value}
                        </dd>
                      </div>
                    ))}
                  </dl>
                  <p className="mt-2 border-t border-border/40 pt-2 text-[11px] text-muted-foreground">
                    {t("security.preview_apply_label")}：{t(section.applyPathKey)}
                  </p>
                </div>
              ))}
            </div>
          ) : previewing ? (
            <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
              <Spinner className="size-4" />
              {t("common.loading")}
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="ghost" onClick={() => setPreviewOpen(false)} disabled={previewing}>
              {t("ops.cancel")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={applyOpen}
        onOpenChange={(open) => {
          if (!applyingFirewall) {
            setApplyOpen(open)
            if (!open) setApplyConfirm("")
          }
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("security.firewall_confirm_title")}</DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-2 text-sm text-muted-foreground">
                <p>{t("security.firewall_confirm_body")}</p>
                <p className="text-destructive">{t("security.firewall_confirm_disconnect")}</p>
              </div>
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>{t("common.confirm_typed_label")}</FieldLabel>
              <Input
                value={applyConfirm}
                onChange={(e) => setApplyConfirm(e.target.value)}
                disabled={standby || applyingFirewall}
                autoComplete="off"
                placeholder={confirmPlaceholder}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setApplyOpen(false)} disabled={applyingFirewall}>
              {t("ops.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleApplyFirewall}
              disabled={standby || applyingFirewall || !matchesConfirm(applyConfirm)}
            >
              {applyingFirewall ? <Spinner data-icon="inline-start" /> : null}
              {t("security.apply_firewall")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
