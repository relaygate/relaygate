#!/usr/bin/env bash
# 渲染后以 drain 方式重启 Envoy
# - 宿主机：可用 compose
# - Panel 容器内：通过 docker.sock 对 gateway-01-envoy 执行 restart（避免 compose 相对路径在套接字场景失效）
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

RENDER="${RENDER_BIN:-$ROOT/bin/gateway-render}"
if [[ ! -x "$RENDER" ]] && command -v gateway-render >/dev/null 2>&1; then
  RENDER="$(command -v gateway-render)"
fi
if [[ ! -x "$RENDER" ]]; then
  echo "ERROR: gateway-render 不可用"
  exit 1
fi

"$RENDER"

if command -v docker >/dev/null 2>&1; then
  docker run --rm \
    -v "$ROOT/gateway/generated/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
    "${ENVOY_IMAGE:-envoyproxy/envoy:v1.39.0}" \
    /usr/local/bin/envoy -c /etc/envoy/envoy.yaml --mode validate \
    || echo "WARN: envoy validate 容器未能运行，继续重启"
fi

echo "==> draining / restarting envoy"
curl -fsS -X POST "http://127.0.0.1:9901/healthcheck/fail" >/dev/null 2>&1 || true
sleep 2

if docker inspect gateway-01-envoy >/dev/null 2>&1; then
  docker restart gateway-01-envoy
else
  COMPOSE=(docker compose -f deploy/compose.yaml)
  if [[ -f .env ]]; then
    COMPOSE+=( --env-file .env )
  fi
  "${COMPOSE[@]}" up -d --force-recreate --no-deps envoy
fi

for i in $(seq 1 45); do
  if curl -fsS "http://127.0.0.1:9901/ready" >/dev/null 2>&1; then
    curl -fsS -X POST "http://127.0.0.1:9901/healthcheck/ok" >/dev/null 2>&1 || true
    echo "Envoy reloaded and ready"
    exit 0
  fi
  sleep 2
done
echo "ERROR: Envoy 未 ready"
exit 1
