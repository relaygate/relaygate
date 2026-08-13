import { useCallback, useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { EyeIcon } from "lucide-react"

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
  cloneSecurityState,
  defaultSecurityState,
  findInvalidAllowlistEntry,
  normalizeAllowlistEntries,
  parseAllowlistLines,
  parseSecurityPolicies,
  patchSecurityPolicies,
  policiesEqual,
  policiesFromMerge,
  policyById,
  SECURITY_LOCAL_SOURCE,
  SECURITY_POLICY_IDS,
  validatePolicyParams,
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
  const [applyingNft, setApplyingNft] = useState(false)
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

  const dirty = useMemo(
    () =>
      savedPolicies !== null &&
      draftPolicies !== null &&
      !policiesEqual(savedPolicies, draftPolicies),
    [savedPolicies, draftPolicies],
  )

  const loadSaved = useCallback(async () => {
    const data = await getConfigResources()
    const parsed = cloneSecurityState(parseSecurityPolicies(data.content))
    setSavedPolicies(parsed)
    setDraftPolicies(cloneSecurityState(parsed))
    const allowlist = parsed.policies.find((p) => p.id === "allowlist")
    setDenyText((allowlist?.params.deny ?? []).join("\n"))
    setAllowText((allowlist?.params.allow ?? []).join("\n"))
    setSelectedSource(SECURITY_LOCAL_SOURCE)
    setPreview(null)
    setPreviewOpen(false)
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
        policies: prev.policies.map((p) =>
          p.id === id ? { ...p, ...patch, params: { ...p.params, ...patch.params } } : p,
        ),
      }
    })
    setPreview(null)
    setPreviewOpen(false)
  }

  function patchParam(id: SecurityPolicyId, key: string, value: string | number) {
    setDraftPolicies((prev) => {
      if (!prev) return prev
      return {
        policies: prev.policies.map((p) =>
          p.id === id ? { ...p, params: { ...p.params, [key]: value } } : p,
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
        policies: prev.policies.map((p) =>
          p.id === "allowlist" ? { ...p, params: { ...p.params, [list]: entries } } : p,
        ),
      }
    })
    setPreview(null)
    setPreviewOpen(false)
  }

  async function savePolicies() {
    if (!draftPolicies || standby || !dirty) return
    const normalizedPolicies = draftPolicies.policies.map((p) => {
      if (p.id !== "allowlist") return p
      return {
        ...p,
        params: {
          ...p.params,
          deny: normalizeAllowlistEntries(p.params.deny ?? []),
          allow: normalizeAllowlistEntries(p.params.allow ?? []),
        },
      }
    })
    const nextState = { policies: normalizedPolicies }
    const allowlist = normalizedPolicies.find((p) => p.id === "allowlist")
    setDenyText((allowlist?.params.deny ?? []).join("\n"))
    setAllowText((allowlist?.params.allow ?? []).join("\n"))
    setDraftPolicies(nextState)
    for (const p of normalizedPolicies) {
      const invalid = validatePolicyParams(p)
      if (invalid) {
        if (invalid.startsWith("allowlist:")) {
          toast.error(t("security.toast_invalid_cidr", { entry: invalid.slice("allowlist:".length) }))
        } else {
          toast.error(t("security.toast_invalid_param", { field: invalid }))
        }
        return
      }
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

  async function handleApplyNft() {
    if (standby || !matchesConfirm(applyConfirm)) return
    setApplyingNft(true)
    try {
      await applyFirewall(applyConfirm.trim())
      toast.success(t("security.toast_firewall_ok"))
      setApplyOpen(false)
      setApplyConfirm("")
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : t("security.toast_firewall_fail"))
    } finally {
      setApplyingNft(false)
    }
  }

  async function handlePreview() {
    if (!draftPolicies) return
    setPreviewOpen(true)
    setPreviewing(true)
    try {
      const result = await previewSecurity(draftPolicies.policies)
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
        const policies = policiesFromMerge(merged.policies)
        const next = cloneSecurityState({ policies })
        setDraftPolicies(next)
        const allowlist = policyById(next, "allowlist")
        setDenyText((allowlist?.params.deny ?? []).join("\n"))
        setAllowText((allowlist?.params.allow ?? []).join("\n"))
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
    const p = policyById(draftPolicies, "allowlist")
    if (!p) return
    const normalized = normalizeAllowlistEntries(p.params[list] ?? [])
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

  function renderParams(p: SecurityPolicy) {
    const par = p.params
    switch (p.id) {
      case "kernel_syn":
        return (
          <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            <Field>
              <FieldLabel>{t("security.param.tcp_syncookies")}</FieldLabel>
              <div className="flex h-9 items-center gap-2">
                <Switch
                  checked={(par.tcp_syncookies ?? 1) === 1}
                  disabled={standby || !p.enabled}
                  onCheckedChange={(v) => patchParam(p.id, "tcp_syncookies", v ? 1 : 0)}
                  aria-label={t("security.param.tcp_syncookies")}
                />
                <span className="text-xs text-muted-foreground">
                  {(par.tcp_syncookies ?? 1) === 1 ? t("security.param.on") : t("security.param.off")}
                </span>
              </div>
            </Field>
            <Field>
              <FieldLabel>{t("security.param.tcp_max_syn_backlog")}</FieldLabel>
              <Input
                type="number"
                value={par.tcp_max_syn_backlog ?? 8192}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) =>
                  patchParam(p.id, "tcp_max_syn_backlog", Number.parseInt(e.target.value, 10) || 0)
                }
              />
            </Field>
            <Field>
              <FieldLabel>{t("security.param.tcp_synack_retries")}</FieldLabel>
              <Input
                type="number"
                value={par.tcp_synack_retries ?? 2}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) =>
                  patchParam(p.id, "tcp_synack_retries", Number.parseInt(e.target.value, 10) || 0)
                }
              />
            </Field>
            <Field>
              <FieldLabel>{t("security.param.tcp_syn_retries")}</FieldLabel>
              <Input
                type="number"
                value={par.tcp_syn_retries ?? 3}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) =>
                  patchParam(p.id, "tcp_syn_retries", Number.parseInt(e.target.value, 10) || 0)
                }
              />
            </Field>
            <Field>
              <FieldLabel>{t("security.param.tcp_abort_on_overflow")}</FieldLabel>
              <div className="flex h-9 items-center gap-2">
                <Switch
                  checked={(par.tcp_abort_on_overflow ?? 0) === 1}
                  disabled={standby || !p.enabled}
                  onCheckedChange={(v) => patchParam(p.id, "tcp_abort_on_overflow", v ? 1 : 0)}
                  aria-label={t("security.param.tcp_abort_on_overflow")}
                />
                <span className="text-xs text-muted-foreground">
                  {(par.tcp_abort_on_overflow ?? 0) === 1 ? t("security.param.on") : t("security.param.off")}
                </span>
              </div>
            </Field>
          </div>
        )
      case "firewall_new_conn_limit":
        return (
          <div className="mt-2 grid gap-2 sm:grid-cols-2">
            <Field>
              <FieldLabel>{t("security.param.tcp_per_ip")}</FieldLabel>
              <Input
                value={par.tcp_per_ip ?? ""}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) => patchParam(p.id, "tcp_per_ip", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>{t("security.param.burst")}</FieldLabel>
              <Input
                type="number"
                value={par.burst ?? 0}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) => patchParam(p.id, "burst", Number.parseInt(e.target.value, 10) || 0)}
              />
            </Field>
          </div>
        )
      case "gateway_new_conn_limit":
        return (
          <div className="mt-2 grid gap-2 sm:grid-cols-2">
            <Field>
              <FieldLabel>{t("security.param.per_sec")}</FieldLabel>
              <Input
                type="number"
                value={par.per_sec ?? 0}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) => patchParam(p.id, "per_sec", Number.parseInt(e.target.value, 10) || 0)}
              />
            </Field>
            <Field>
              <FieldLabel>{t("security.param.burst")}</FieldLabel>
              <Input
                type="number"
                value={par.burst ?? 0}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) => patchParam(p.id, "burst", Number.parseInt(e.target.value, 10) || 0)}
              />
            </Field>
          </div>
        )
      case "conn_limit":
        return (
          <Field className="mt-2 max-w-xs">
            <FieldLabel>{t("security.param.max_connections")}</FieldLabel>
            <Input
              type="number"
              value={par.max_connections ?? 0}
              disabled={standby || !p.enabled}
              className="font-mono text-xs"
              onChange={(e) => patchParam(p.id, "max_connections", Number.parseInt(e.target.value, 10) || 0)}
            />
          </Field>
        )
      case "udp_limit":
        return (
          <div className="mt-2 grid gap-2 sm:grid-cols-2">
            <Field>
              <FieldLabel>{t("security.param.udp_pps")}</FieldLabel>
              <Input
                value={par.udp_pps_per_ip ?? ""}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) => patchParam(p.id, "udp_pps_per_ip", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>{t("security.param.udp_burst")}</FieldLabel>
              <Input
                type="number"
                value={par.udp_burst ?? 0}
                disabled={standby || !p.enabled}
                className="font-mono text-xs"
                onChange={(e) => patchParam(p.id, "udp_burst", Number.parseInt(e.target.value, 10) || 0)}
              />
            </Field>
          </div>
        )
      default:
        return null
    }
  }

  const previewSummaries = useMemo(() => {
    if (!preview || !draftPolicies) return []
    return buildComponentSummaries(preview, draftPolicies, t("security.preview_disabled"))
  }, [preview, draftPolicies, t])

  const allowlistPolicy = draftPolicies ? policyById(draftPolicies, "allowlist") : null
  const denyEntries = allowlistPolicy?.params.deny ?? []
  const allowEntries = allowlistPolicy?.params.allow ?? []

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
                {sourceLoading ? (
                  <Spinner className="size-3.5" />
                ) : (
                  <SelectValue />
                )}
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
              onClick={() => handlePreview()}
              disabled={loading || !draftPolicies || previewing || sourceLoading}
            >
              {previewing ? <Spinner data-icon="inline-start" /> : <EyeIcon data-icon="inline-start" />}
              {t("security.preview_strategy")}
            </Button>
            <Button
              size="sm"
              onClick={savePolicies}
              disabled={standby || loading || saving || applyingNft || sourceLoading || !dirty}
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
              disabled={standby || loading || saving || applyingNft || sourceLoading}
            >
              {applyingNft ? <Spinner data-icon="inline-start" /> : null}
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
          {SECURITY_POLICY_IDS.map((id) => {
            const p = policyById(draftPolicies, id) ?? defaultSecurityState().policies[0]
            return (
              <li key={id} className="px-3 py-3">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-[13px] font-medium text-foreground">
                        {t(`security.policy.${id}`)}
                      </p>
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
                    onCheckedChange={(v) => patchPolicy(id, { enabled: v })}
                    aria-label={t(`security.policy.${id}`)}
                  />
                </div>
                {renderParams(p)}
                {id === "allowlist" && p.enabled ? (
                  <div className={cn("mt-3 grid gap-4 lg:grid-cols-2")}>
                    {renderAllowlistTextarea("deny", denyText, setDenyText, denyEntries)}
                    {renderAllowlistTextarea("allow", allowText, setAllowText, allowEntries)}
                  </div>
                ) : null}
              </li>
            )
          })}
        </ul>
      )}

      <p className="text-xs text-muted-foreground">{t("security.save_hint")}</p>

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
          if (!applyingNft) {
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
                disabled={standby || applyingNft}
                autoComplete="off"
                placeholder={confirmPlaceholder}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setApplyOpen(false)} disabled={applyingNft}>
              {t("ops.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleApplyNft}
              disabled={standby || applyingNft || !matchesConfirm(applyConfirm)}
            >
              {applyingNft ? <Spinner data-icon="inline-start" /> : null}
              {t("security.apply_firewall")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Page>
  )
}
