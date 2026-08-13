import assert from "node:assert/strict"
import {
  addProtection,
  availableCatalogEntries,
  cloneSecurityState,
  defaultSecurityState,
  findInvalidAllowlistEntry,
  isValidCIDR,
  normalizeAllowlistEntries,
  paramsToRows,
  parseAllowlistLines,
  parseParamValue,
  parseSecurityPolicies,
  patchSecurityPolicies,
  policiesEqual,
  policyById,
  rowsToParams,
} from "./securityPolicies.ts"

const sample = `
security:
  access:
    enabled: true
    deny: []
    allow: []
  protections:
    - id: firewall_new_conn_limit
      type: firewall_new_conn_limit
      enabled: true
      attack_tags: [T1, T4]
      params:
        tcp_per_ip: 40/second
        burst: 80
    - id: gateway_new_conn_limit
      type: gateway_new_conn_limit
      enabled: true
      params:
        per_sec: 100
        burst: 200
    - id: gateway_conn_limit
      type: gateway_conn_limit
      enabled: false
      attack_tags: [T2]
`

const parsed = parseSecurityPolicies(sample)
assert.equal(policyById(parsed, "firewall_new_conn_limit")?.enabled, true)
assert.equal(policyById(parsed, "firewall_new_conn_limit")?.params.tcp_per_ip, "40/second")
assert.equal(policyById(parsed, "gateway_new_conn_limit")?.params.per_sec, 100)
assert.equal(policyById(parsed, "gateway_conn_limit")?.enabled, false)
assert.equal(policyById(parsed, "kernel_syn"), undefined)
assert.equal(parsed.access.enabled, true)
assert.equal(parsed.protections.length, 3)

const missing = availableCatalogEntries(parsed).map((e) => e.id)
assert.ok(missing.includes("kernel_syn"))
assert.ok(missing.includes("nic_egress_shape"))
assert.ok(missing.includes("nic_ingress_police"))
assert.ok(missing.includes("firewall_udp_limit"))
assert.ok(!missing.includes("firewall_new_conn_limit"))

const withKernel = addProtection(parsed, "kernel_syn")
assert.equal(policyById(withKernel, "kernel_syn")?.enabled, true)
assert.equal(policyById(withKernel, "kernel_syn")?.params.tcp_syncookies, 1)
assert.equal(availableCatalogEntries(withKernel).some((e) => e.id === "kernel_syn"), false)

const withNic = addProtection(parsed, "nic_egress_shape")
assert.equal(policyById(withNic, "nic_egress_shape")?.enabled, false)
assert.equal(policyById(withNic, "nic_egress_shape")?.params.rate, "3mbit")

const withIngress = addProtection(parsed, "nic_ingress_police")
assert.equal(policyById(withIngress, "nic_ingress_police")?.enabled, false)
assert.equal(policyById(withIngress, "nic_ingress_police")?.params.rate, "3mbit")

const toggled = {
  ...parsed,
  access: { ...parsed.access, enabled: false },
  protections: parsed.protections.map((p) =>
    p.id === "gateway_conn_limit" ? { ...p, enabled: false } : p,
  ),
}
const patched = patchSecurityPolicies(sample, toggled)
assert.match(patched, /access:/)
assert.match(patched, /enabled: false/)
assert.doesNotMatch(patched, /id: kernel_syn/)
assert.match(patched, /protections:/)

const patchedAdd = patchSecurityPolicies(sample, withKernel)
assert.match(patchedAdd, /id: kernel_syn/)
assert.match(patchedAdd, /tcp_syncookies:/)

const empty = parseSecurityPolicies("")
assert.equal(policyById(empty, "gateway_new_conn_limit")?.params.per_sec, 200)
assert.equal(empty.protections.length, defaultSecurityState().protections.length)

const a = cloneSecurityState(parsed)
const b = cloneSecurityState(parsed)
assert.equal(policiesEqual(a, b), true)
b.protections = b.protections.map((p) =>
  p.id === "gateway_conn_limit" ? { ...p, enabled: true } : p,
)
assert.equal(policiesEqual(a, b), false)
assert.equal(policiesEqual(parsed, withKernel), false)

const rows = paramsToRows({ per_sec: 10, burst: 20, enabled: true, rate: "3mbit" })
assert.deepEqual(
  rows.map((r) => r.key).sort(),
  ["burst", "enabled", "per_sec", "rate"],
)
assert.equal(parseParamValue("true"), true)
assert.equal(parseParamValue("false"), false)
assert.equal(parseParamValue("42"), 42)
assert.equal(parseParamValue("3mbit"), "3mbit")
const roundTrip = rowsToParams(rows)
assert.equal(roundTrip.duplicateKey, null)
assert.equal(roundTrip.params.per_sec, 10)
assert.equal(roundTrip.params.burst, 20)
assert.equal(roundTrip.params.enabled, true)
assert.equal(roundTrip.params.rate, "3mbit")
// Extra / unknown keys survive object round-trip
const withExtra = rowsToParams([
  ...paramsToRows({ per_sec: 5 }),
  { key: "custom_flag", value: "on" },
])
assert.equal(withExtra.params.per_sec, 5)
assert.equal(withExtra.params.custom_flag, "on")
assert.equal(rowsToParams([{ key: "", value: "x" }, { key: "a", value: "1" }]).params.a, 1)
assert.equal(
  rowsToParams([
    { key: "a", value: "1" },
    { key: "a", value: "2" },
  ]).duplicateKey,
  "a",
)
assert.equal(
  rowsToParams([
    { key: "a", value: "1" },
    { key: "a", value: "2" },
  ]).params.a,
  1,
)

assert.deepEqual(parseAllowlistLines(" 1.2.3.4 \n\n 5.6.7.8/32 "), ["1.2.3.4", "5.6.7.8/32"])
assert.deepEqual(normalizeAllowlistEntries([" 1.2.3.4 ", "1.2.3.4", ""]), ["1.2.3.4"])
assert.equal(isValidCIDR("203.0.113.0/24"), true)
assert.equal(isValidCIDR("203.0.113.1"), true)
assert.equal(isValidCIDR("not-a-cidr"), false)
assert.equal(findInvalidAllowlistEntry(["203.0.113.1", "bad"]), "bad")

console.log("securityPolicies.selftest: ok")
