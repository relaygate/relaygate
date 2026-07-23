/** Grafana Explore deep links for Loki (uid=loki), via Panel /grafana reverse proxy. */

export type LokiExploreRange = {
  from?: string
  to?: string
}

export type SessionLogQuery = {
  gateway?: string
  ip?: string
  rule?: string
  upstream?: string
  flagsOnly?: boolean
  shortMs?: number
} & LokiExploreRange

function baseSelector(gateway?: string): string {
  const g = gateway?.trim()
  if (g) {
    return `{job="envoy-tcp-access", gateway="${escapeLabel(g)}"}`
  }
  return `{job="envoy-tcp-access"}`
}

function escapeLabel(v: string): string {
  return v.replace(/\\/g, "\\\\").replace(/"/g, '\\"')
}

/** Build LogQL from thin ops form fields. Client IP stays a line filter (not a label). */
export function buildSessionLogQL(q: SessionLogQuery): string {
  let expr = baseSelector(q.gateway)
  const ip = q.ip?.trim()
  if (ip) {
    // Match Envoy JSON field downstream:"ip:port" or downstream:"ip"
    expr += ` |= \`"downstream":"${ip.replace(/"/g, "")}\``
  }
  const parsers: string[] = []
  const rule = q.rule?.trim()
  const upstream = q.upstream?.trim()
  const needJson =
    !!rule || !!upstream || q.flagsOnly === true || (q.shortMs != null && q.shortMs > 0)
  if (needJson) {
    parsers.push("| json")
  }
  if (rule) {
    parsers.push(`| rule="${escapeLabel(rule)}"`)
  }
  if (upstream) {
    parsers.push(`| upstream=~"${escapeLabel(upstream)}.*"`)
  }
  if (q.flagsOnly) {
    parsers.push('| flags != "-"')
  }
  if (q.shortMs != null && q.shortMs > 0) {
    parsers.push(`| duration_ms < ${Math.floor(q.shortMs)}`)
  }
  if (parsers.length) {
    expr += ` ${parsers.join(" ")}`
  }
  return expr
}

export function buildLokiExploreHref(expr: string, range?: LokiExploreRange): string {
  const from = range?.from?.trim() || "now-1h"
  const to = range?.to?.trim() || "now"
  const panes = {
    rg: {
      datasource: "loki",
      queries: [
        {
          refId: "A",
          expr,
          queryType: "range",
          datasource: { type: "loki", uid: "loki" },
        },
      ],
      range: { from, to },
    },
  }
  const params = new URLSearchParams({
    schemaVersion: "1",
    panes: JSON.stringify(panes),
    orgId: "1",
  })
  return `/grafana/explore?${params.toString()}`
}

export function sessionLogsExploreHref(q: SessionLogQuery): string {
  return buildLokiExploreHref(buildSessionLogQL(q), q)
}

/** Preset: anomaly flags for optional gateway. */
export function anomalyFlagsExploreHref(gateway?: string, range?: LokiExploreRange): string {
  return sessionLogsExploreHref({ gateway, flagsOnly: true, ...range })
}

/** Preset: short sessions. */
export function shortSessionsExploreHref(
  gateway?: string,
  shortMs = 2000,
  range?: LokiExploreRange,
): string {
  return sessionLogsExploreHref({ gateway, shortMs, ...range })
}

export const TCP_SESSION_DASHBOARD_HREF =
  "/grafana/d/relaygate-tcp-session-logs/tcp-session-logs?orgId=1"
