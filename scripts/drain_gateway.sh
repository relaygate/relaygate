#!/usr/bin/env bash
# 将当前网关从 L4 LB / 健康检查摘流（复用 Envoy /ready + /healthcheck/fail）
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -f .env ]]; then
  # shellcheck disable=SC1091
  set -a
  source .env
  set +a
fi
GATEWAY_NAME="${GATEWAY_NAME:-gateway-01}"
ENVOY_ADMIN_PORT="${ENVOY_ADMIN_PORT:-9901}"
ACTION="${1:-fail}"

case "$ACTION" in
  fail|drain)
    echo "==> ${GATEWAY_NAME}: healthcheck/fail (drain)"
    curl -fsS -X POST "http://127.0.0.1:${ENVOY_ADMIN_PORT}/healthcheck/fail"
    echo
    curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready" || true
    echo
    echo "等待 LB 健康检查失败窗口（建议 ${DRAIN_WAIT:-15}s）…"
    sleep "${DRAIN_WAIT:-15}"
    ;;
  ok|undrain)
    echo "==> ${GATEWAY_NAME}: healthcheck/ok (undrain)"
    curl -fsS -X POST "http://127.0.0.1:${ENVOY_ADMIN_PORT}/healthcheck/ok"
    echo
    curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready"
    echo
    ;;
  status)
    echo "==> ${GATEWAY_NAME} ready:"
    curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready" || true
    echo
    ;;
  *)
    echo "usage: $0 fail|ok|status"
    exit 2
    ;;
esac
