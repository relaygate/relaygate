/**
 * Self-contained checks for resourcesDefaults rate-limit patch helpers.
 * Run: PATH=/tmp/node-v22.14.0-linux-x64/bin:$PATH npx --yes tsx src/lib/resourcesDefaults.selftest.ts
 */
import assert from "node:assert/strict"

import {
  parseRateLimitDefaults,
  patchRateLimitDefaults,
  validateRateLimitDefaults,
} from "./resourcesDefaults.ts"

const sample = `# header
meta:
  gateway_name: gateway-01
defaults:
  max_connections: 1024
  # TCP local token bucket
  tcp_local_rate_limit_per_sec: 200
  tcp_local_rate_limit_burst: 400
  nftables:
    tcp_new_conn_per_ip: 30/second
    udp_pps_per_ip: 500/second
    tcp_burst: 60
    udp_burst: 1000
servers:
  - name: server-01
`

function testParse() {
  const d = parseRateLimitDefaults(sample)
  assert.equal(d.tcpLocalRateLimitPerSec, 200)
  assert.equal(d.tcpLocalRateLimitBurst, 400)
  assert.equal(d.nftTcpNewConnPerIp, "30/second")
  assert.equal(d.nftUdpPpsPerIp, "500/second")
  assert.equal(d.nftTcpBurst, 60)
  assert.equal(d.nftUdpBurst, 1000)
}

function testPatchRoundTrip() {
  const next = {
    tcpLocalRateLimitPerSec: 350,
    tcpLocalRateLimitBurst: 700,
    nftTcpNewConnPerIp: "40/second",
    nftUdpPpsPerIp: "800/second",
    nftTcpBurst: 80,
    nftUdpBurst: 1600,
  }
  const patched = patchRateLimitDefaults(sample, next)
  assert.match(patched, /tcp_local_rate_limit_per_sec: 350/)
  assert.match(patched, /tcp_local_rate_limit_burst: 700/)
  assert.match(patched, /tcp_new_conn_per_ip: 40\/second/)
  assert.match(patched, /udp_pps_per_ip: 800\/second/)
  assert.match(patched, /tcp_burst: 80/)
  assert.match(patched, /udp_burst: 1600/)
  assert.match(patched, /# TCP local token bucket/)
  assert.match(patched, /max_connections: 1024/)
  const again = parseRateLimitDefaults(patched)
  assert.deepEqual(again, next)
}

function testInsertMissing() {
  const thin = `defaults:\n  max_connections: 10\nservers: []\n`
  const patched = patchRateLimitDefaults(thin, {
    tcpLocalRateLimitPerSec: 10,
    tcpLocalRateLimitBurst: 20,
    nftTcpNewConnPerIp: "10/second",
    nftUdpPpsPerIp: "100/second",
    nftTcpBurst: 5,
    nftUdpBurst: 50,
  })
  assert.match(patched, /nftables:/)
  assert.match(patched, /tcp_local_rate_limit_per_sec: 10/)
  assert.equal(parseRateLimitDefaults(patched).nftTcpBurst, 5)
}

function testValidate() {
  assert.equal(
    validateRateLimitDefaults({
      tcpLocalRateLimitPerSec: 1,
      tcpLocalRateLimitBurst: 2,
      nftTcpNewConnPerIp: "1/second",
      nftUdpPpsPerIp: "2/second",
      nftTcpBurst: 3,
      nftUdpBurst: 4,
    }),
    null,
  )
  assert.equal(
    validateRateLimitDefaults({
      tcpLocalRateLimitPerSec: -1,
      tcpLocalRateLimitBurst: 2,
      nftTcpNewConnPerIp: "1/second",
      nftUdpPpsPerIp: "2/second",
      nftTcpBurst: 3,
      nftUdpBurst: 4,
    }),
    "tcp_local_rate_limit_per_sec",
  )
}

testParse()
testPatchRoundTrip()
testInsertMissing()
testValidate()
console.log("resourcesDefaults.selftest: ok")
