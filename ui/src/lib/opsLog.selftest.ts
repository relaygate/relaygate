/**
 * Self-contained unit checks for ops log parsing / tone.
 * Run: npx --yes tsx src/lib/opsLog.selftest.ts
 */
import assert from "node:assert/strict"

import {
  isLifecycleInfoLine,
  opsLineTone,
  opsLogOutcome,
  parseOpsLog,
} from "./opsLog.ts"

function testTone() {
  assert.equal(opsLineTone("## 入口状态: 2 台上游"), "meta")
  assert.equal(opsLineTone("入口状态: 2 台上游"), "meta")
  assert.equal(
    opsLineTone("  · server-01 server=on validation=on TCP/11001 production=—"),
    "meta",
  )
  assert.equal(
    opsLineTone("  - server-01 server=on validation=off production=—"),
    "meta",
  )
  assert.equal(opsLineTone("==> [tcp] FAIL: connection refused"), "error")
  assert.equal(opsLineTone("TCP FAIL"), "error")
  assert.equal(opsLineTone("FAIL: boom"), "error")
  assert.equal(opsLineTone("WARN: drain"), "warn")
  assert.equal(opsLineTone("==> [envoy/ready] ok"), "ok")
  // lowercase "fail" in drain/healthcheck must not look like FAIL
  assert.equal(opsLineTone("drain fail → wait"), "ctx")
  assert.equal(opsLineTone("healthcheck/fail (drain)"), "ctx")
  assert.equal(opsLineTone("==> [smoke] start"), "step")
}

function testParseLifecycleAndSteps() {
  const text = [
    "## 入口状态: 2 台上游",
    "  · server-01 server=on validation=on TCP/11001 production=off（可启用正式入口后 reload）",
    "  · server-02 server=off validation=— production=—",
    "",
    "==> [canary] start host=127.0.0.1 source=resources validation tcp=11001 udp=11001",
    "==> [tcp] start",
    "==> [tcp] ok",
    "==> [udp] start",
    "==> [udp] ok",
    "==> [canary] ok",
  ].join("\n")

  const parsed = parseOpsLog(text)
  assert.ok(parsed.lifecycle)
  assert.equal(parsed.lifecycle!.count, 2)
  assert.equal(parsed.lifecycle!.rows.length, 2)
  assert.equal(parsed.lifecycle!.rows[0]!.name, "server-01")
  assert.equal(parsed.lifecycle!.rows[0]!.server, "on")

  const names = parsed.steps.map((s) => `${s.name}:${s.status}`)
  assert.ok(names.includes("canary:ok"))
  assert.ok(names.includes("tcp:ok"))
  assert.ok(names.includes("udp:ok"))
  assert.equal(opsLogOutcome(parsed.steps, parsed.detailLines), "ok")

  // Lifecycle must not appear as error-toned detail
  for (const line of parsed.detailLines) {
    assert.notEqual(opsLineTone(line), "error")
  }
}

function testDoctorStyle() {
  const text = [
    "==> [.env] start",
    "==> [.env] ok",
    "==> [ports] start",
    "WARN: 端口已占用: 9000（若已是本机 RelayGate 可忽略）",
    "==> [ports] ok",
    "==> [envoy ready] start",
    "==> [envoy ready] FAIL: Envoy 未 ready",
  ].join("\n")
  const parsed = parseOpsLog(text)
  assert.equal(parsed.steps.find((s) => s.name === ".env")?.status, "ok")
  assert.equal(parsed.steps.find((s) => s.name === "ports")?.status, "ok")
  assert.equal(parsed.steps.find((s) => s.name === "envoy ready")?.status, "fail")
  assert.equal(opsLogOutcome(parsed.steps, parsed.detailLines), "fail")
}

function testLegacyDoctorSections() {
  const text = ["-- docker --", "WARN: sock", "OK", "-- binary --", "FAIL: missing"].join("\n")
  const parsed = parseOpsLog(text)
  assert.equal(parsed.steps.find((s) => s.name === "docker")?.status, "warn")
  assert.equal(parsed.steps.find((s) => s.name === "binary")?.status, "fail")
}

function testIsLifecycle() {
  assert.equal(isLifecycleInfoLine("入口状态: 1 台上游"), true)
  assert.equal(isLifecycleInfoLine("## 入口状态: 1 台上游"), true)
  assert.equal(isLifecycleInfoLine("  - server foo"), false)
}

testTone()
testParseLifecycleAndSteps()
testDoctorStyle()
testLegacyDoctorSections()
testIsLifecycle()
console.log("opsLog.selftest: ok")
