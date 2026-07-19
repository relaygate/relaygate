#!/usr/bin/env bash
# 根据 .env 渲染可观测性配置（Prometheus 标签等）
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${1:-.env}"
if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a
  # shellcheck disable=SC1091
  source "$ENV_FILE"
  set +a
fi

GATEWAY_NAME="${GATEWAY_NAME:-gateway-01}"
ENVOY_ADMIN_PORT="${ENVOY_ADMIN_PORT:-9901}"
PROMETHEUS_REMOTE_WRITE_URL="${PROMETHEUS_REMOTE_WRITE_URL:-}"

export GATEWAY_NAME ENVOY_ADMIN_PORT PROMETHEUS_REMOTE_WRITE_URL

TPL="deploy/prometheus/prometheus.yml.tpl"
OUT="deploy/prometheus/prometheus.yml"

if [[ ! -f "$TPL" ]]; then
  echo "ERROR: missing $TPL"
  exit 1
fi

# 优先 envsubst；否则用 sed 兜底（仅替换已知占位符）
if command -v envsubst >/dev/null 2>&1; then
  envsubst '${GATEWAY_NAME} ${ENVOY_ADMIN_PORT} ${PROMETHEUS_REMOTE_WRITE_URL}' < "$TPL" > "$OUT.tmp"
else
  sed \
    -e "s|\${GATEWAY_NAME}|${GATEWAY_NAME}|g" \
    -e "s|\${ENVOY_ADMIN_PORT}|${ENVOY_ADMIN_PORT}|g" \
    -e "s|\${PROMETHEUS_REMOTE_WRITE_URL}|${PROMETHEUS_REMOTE_WRITE_URL}|g" \
    "$TPL" > "$OUT.tmp"
fi

# 可选 remote_write 块
if [[ -n "$PROMETHEUS_REMOTE_WRITE_URL" ]]; then
  cat >> "$OUT.tmp" <<EOF

remote_write:
  - url: ${PROMETHEUS_REMOTE_WRITE_URL}
    queue_config:
      max_samples_per_send: 1000
      capacity: 10000
      max_shards: 4
EOF
fi

mv "$OUT.tmp" "$OUT"
echo "rendered $OUT (gateway=${GATEWAY_NAME})"
