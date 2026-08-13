import assert from "node:assert/strict"
import {
  cloneSecurityState,
  defaultSecurityState,
  findInvalidAllowlistEntry,
  isValidCIDR,
  normalizeAllowlistEntries,
  parseAllowlistLines,
  parseSecurityPolicies,
  patchSecurityPolicies,
  policiesEqual,
  policyById,
} from "./securityPolicies.ts"

const sample = `
security:
  policies:
    - id: firewall_new_conn_limit
      type: new_conn_limit_firewall
      enabled: true
      attack_tags: [T1, T4]
      params:
        tcp_per_ip: 40/second
        burst: 80
    - id: gateway_new_conn_limit
      type: new_conn_limit_gateway
      enabled: true
      params:
        per_sec: 100
        burst: 200
    - id: conn_limit
      type: conn_limit
      enabled: false
      attack_tags: [T2]
`

const parsed = parseSecurityPolicies(sample)
assert.equal(policyById(parsed, "firewall_new_conn_limit")?.enabled, true)
assert.equal(policyById(parsed, "firewall_new_conn_limit")?.params.tcp_per_ip, "40/second")
assert.equal(policyById(parsed, "gateway_new_conn_limit")?.params.per_sec, 100)
assert.equal(policyById(parsed, "conn_limit")?.enabled, false)
assert.equal(policyById(parsed, "allowlist")?.enabled, true)

const toggled = {
  policies: parsed.policies.map((p) => {
    if (p.id === "conn_limit") return { ...p, enabled: false }
    if (p.id === "allowlist") return { ...p, enabled: false }
    if (p.id === "kernel_syn") return { ...p, enabled: false }
    return p
  }),
}
const patched = patchSecurityPolicies(sample, toggled)
assert.match(patched, /id: allowlist/)
assert.match(patched, /enabled: false/)
assert.match(patched, /id: kernel_syn/)

const empty = parseSecurityPolicies("")
assert.equal(policyById(empty, "gateway_new_conn_limit")?.params.per_sec, 200)

const a = cloneSecurityState(parsed)
const b = cloneSecurityState(parsed)
assert.equal(policiesEqual(a, b), true)
b.policies = b.policies.map((p) =>
  p.id === "conn_limit" ? { ...p, enabled: true } : p,
)
assert.equal(policiesEqual(a, b), false)

assert.deepEqual(parseAllowlistLines(" 1.2.3.4 \n\n 5.6.7.8/32 "), ["1.2.3.4", "5.6.7.8/32"])
assert.deepEqual(normalizeAllowlistEntries([" 1.2.3.4 ", "1.2.3.4", ""]), ["1.2.3.4"])
assert.equal(isValidCIDR("203.0.113.0/24"), true)
assert.equal(isValidCIDR("203.0.113.1"), true)
assert.equal(isValidCIDR("not-a-cidr"), false)

console.log("securityPolicies.selftest: ok")
