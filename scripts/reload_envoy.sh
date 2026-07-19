#!/usr/bin/env bash
# 渲染后以 drain 方式重启 Envoy
# - 宿主机 root / 运维：直接 docker / compose
# - Panel（systemd 用户 relaygate）：经 RELAYGATE_PRIVILEGED_HELPER + sudoers 提权，
#   不把用户加入 docker 组，也不挂载 docker.sock 到 Panel 进程
set -euo pipefail

# Panel Apply: non-root + helper path → re-exec via sudo -n (exact whitelist).
if [[ -n "${RELAYGATE_PRIVILEGED_HELPER:-}" && "$(id -u)" -ne 0 ]]; then
  if [[ ! -x "$RELAYGATE_PRIVILEGED_HELPER" ]]; then
    echo "ERROR: privileged helper 不可执行: $RELAYGATE_PRIVILEGED_HELPER" >&2
    exit 1
  fi
  exec sudo -n "$RELAYGATE_PRIVILEGED_HELPER" "$@"
fi

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
ENVOY_CONTAINER="${GATEWAY_NAME}-envoy"

RELAYGATE="${RELAYGATE_BIN:-$ROOT/bin/relaygate}"
if [[ ! -x "$RELAYGATE" ]] && command -v relaygate >/dev/null 2>&1; then
  RELAYGATE="$(command -v relaygate)"
fi
if [[ ! -x "$RELAYGATE" ]]; then
  echo "ERROR: relaygate 不可用"
  exit 1
fi

"$RELAYGATE" render

if command -v docker >/dev/null 2>&1; then
  docker run --rm \
    -v "$ROOT/gateway/generated/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
    "${ENVOY_IMAGE:-envoyproxy/envoy:v1.39.0}" \
    /usr/local/bin/envoy -c /etc/envoy/envoy.yaml --mode validate \
    || echo "WARN: envoy validate 容器未能运行，继续重启"
fi

echo "==> draining ${ENVOY_CONTAINER} (/healthcheck/fail → LB 摘流)"
curl -fsS -X POST "http://127.0.0.1:${ENVOY_ADMIN_PORT}/healthcheck/fail" >/dev/null 2>&1 || true
DRAIN_WAIT="${DRAIN_WAIT:-5}"
sleep "$DRAIN_WAIT"

if docker inspect "$ENVOY_CONTAINER" >/dev/null 2>&1; then
  docker restart "$ENVOY_CONTAINER"
else
  COMPOSE=(docker compose -f deploy/compose.yaml)
  if [[ -f .env ]]; then
    COMPOSE+=( --env-file .env )
  fi
  "${COMPOSE[@]}" up -d --force-recreate --no-deps envoy
fi

for i in $(seq 1 45); do
  if curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready" >/dev/null 2>&1; then
    curl -fsS -X POST "http://127.0.0.1:${ENVOY_ADMIN_PORT}/healthcheck/ok" >/dev/null 2>&1 || true
    echo "Envoy reloaded and ready (${GATEWAY_NAME})"
    exit 0
  fi
  sleep 2
done
echo "ERROR: Envoy 未 ready (${GATEWAY_NAME})"
exit 1
