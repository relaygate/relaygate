import assert from "node:assert/strict"
import { defaultSecurityState, policyById } from "./securityPolicies.ts"
import { buildComponentSummaries } from "./securityPreviewSummary.ts"
import type { SecurityPreview } from "./types.ts"

const disabled = "未启用"

const basePreview: SecurityPreview = {
  execution_order: [],
  surfaces: [],
  kernel: { enabled: true, content: "net.ipv4.tcp_syncookies=1" },
  nic: { enabled: false, egress_enabled: false, ingress_enabled: false },
  firewall: { forward_ports: "", gateway_excerpt: "" },
  gateway: {
    max_connections: 1024,
    local_ratelimit_per_sec: 200,
    local_ratelimit_burst: 400,
    listeners_with_rate_limit: 0,
    enabled_tcp_forwards: 0,
    rate_limit_enabled: true,
    gateway_conn_limit_enabled: true,
  },
  notes: [],
}

const state = defaultSecurityState()
const off = buildComponentSummaries(basePreview, state, disabled)
const nicOff = off.find((s) => s.id === "nic")
assert.ok(nicOff)
assert.equal(nicOff.enabled, false)
assert.equal(nicOff.applyPathKey, "security.preview_apply_nic")
assert.equal(nicOff.params[0]?.key, "nic_egress_shape")
assert.equal(nicOff.params[1]?.key, "nic_ingress_police")

const onState = structuredClone(state)
const nicPol = policyById(onState, "nic_egress_shape")
assert.ok(nicPol)
nicPol.enabled = true
nicPol.params.device = "eth0"
nicPol.params.rate = "3mbit"
const ingPol = policyById(onState, "nic_ingress_police")
assert.ok(ingPol)
ingPol.enabled = true
ingPol.params.device = "eth0"
ingPol.params.rate = "3mbit"

const onPreview: SecurityPreview = {
  ...basePreview,
  nic: {
    enabled: true,
    egress_enabled: true,
    ingress_enabled: true,
    device: "eth0",
    rate: "3mbit",
    ingress_device: "eth0",
    ingress_rate: "3mbit",
    apply_script: "relaygate security apply-nic --verify",
  },
}
const on = buildComponentSummaries(onPreview, onState, disabled)
const nicOn = on.find((s) => s.id === "nic")
assert.ok(nicOn)
assert.equal(nicOn.enabled, true)
assert.deepEqual(nicOn.params, [
  { key: "egress.device", value: "eth0" },
  { key: "egress.rate", value: "3mbit" },
  { key: "ingress.device", value: "eth0" },
  { key: "ingress.rate", value: "3mbit" },
])

console.log("securityPreviewSummary.selftest: ok")
