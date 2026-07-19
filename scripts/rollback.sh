#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"

if [[ -f .env ]]; then
  # shellcheck disable=SC1091
  set -a
  source .env
  set +a
fi
GATEWAY_NAME="${GATEWAY_NAME:-gateway-01}"
ENVOY_ADMIN_PORT="${ENVOY_ADMIN_PORT:-9901}"

if [[ $# -ge 1 ]]; then
  TARGET="$BACKUP_DIR/$1"
else
  [[ -f "$BACKUP_DIR/latest" ]] || { echo "ERROR: 无备份"; exit 1; }
  TARGET="$BACKUP_DIR/$(cat "$BACKUP_DIR/latest")"
fi
[[ -d "$TARGET" ]] || { echo "ERROR: 备份不存在: $TARGET"; exit 1; }

echo "==> 回滚自 $TARGET (gateway=${GATEWAY_NAME})"
[[ -f "$TARGET/resources.yaml" ]] && cp -a "$TARGET/resources.yaml" config/resources.yaml
if [[ -f "$TARGET/envoy.yaml" ]]; then
  mkdir -p gateway/generated
  cp -a "$TARGET/envoy.yaml" gateway/generated/envoy.yaml
else
  RELAYGATE="${RELAYGATE_BIN:-$ROOT/bin/relaygate}"
  "$RELAYGATE" render
fi
[[ -f "$TARGET/prometheus.yml" ]] && cp -a "$TARGET/prometheus.yml" deploy/prometheus/prometheus.yml

[[ -f .env ]] || { echo "ERROR: 缺少 .env"; exit 1; }

echo "==> drain before recreate"
bash scripts/drain_gateway.sh fail || true

bash scripts/validate.sh
docker compose -f deploy/compose.yaml --env-file .env up -d --force-recreate --no-deps envoy

for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready" >/dev/null 2>&1; then
    curl -fsS -X POST "http://127.0.0.1:${ENVOY_ADMIN_PORT}/healthcheck/ok" >/dev/null 2>&1 || true
    echo "Envoy ready（回滚完成: ${GATEWAY_NAME}）"
    echo "验证: bash scripts/smoke_test.sh"
    exit 0
  fi
  sleep 2
done
exit 1
