#!/usr/bin/env bash
# 部署后冒烟：Envoy ready、Prometheus 标签、可选 canary TCP
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
TARGET="${1:-127.0.0.1}"

echo "==> smoke ${GATEWAY_NAME} @ ${TARGET}"

echo "-- Envoy /ready --"
curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready"
echo

echo "-- Envoy /stats prometheus sample --"
curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/stats/prometheus" | head -n 5 >/dev/null
echo "stats OK"

if curl -fsS "http://127.0.0.1:9090/-/ready" >/dev/null 2>&1; then
  echo "-- Prometheus ready --"
  curl -fsS "http://127.0.0.1:9090/-/ready"
  echo
  echo "-- Prometheus external label gateway --"
  curl -fsS "http://127.0.0.1:9090/api/v1/status/config" \
    | grep -o "gateway: ${GATEWAY_NAME}" \
    && echo "label OK" \
    || echo "WARN: 未在 Prometheus config 中看到 gateway: ${GATEWAY_NAME}（请先 render_observability）"
else
  echo "WARN: Prometheus 未 ready，跳过标签检查"
fi

if docker inspect "${GATEWAY_NAME}-envoy" >/dev/null 2>&1; then
  echo "-- container ${GATEWAY_NAME}-envoy running --"
  docker inspect -f '{{.State.Status}}' "${GATEWAY_NAME}-envoy"
else
  echo "WARN: 容器 ${GATEWAY_NAME}-envoy 不存在"
fi

if [[ -x "$ROOT/scripts/canary_test.sh" ]]; then
  bash "$ROOT/scripts/canary_test.sh" "$TARGET" || true
fi

echo "smoke OK: ${GATEWAY_NAME}"
