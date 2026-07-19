#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

RENDER="${RENDER_BIN:-$ROOT/bin/gateway-render}"
if [[ ! -x "$RENDER" ]]; then
  if command -v go >/dev/null 2>&1; then
    bash scripts/build.sh
  else
    echo "ERROR: 缺少 $RENDER，请先 bash scripts/build.sh"
    exit 1
  fi
fi

echo "==> gateway-render --check-only"
"$RENDER" --check-only
echo "==> gateway-render"
"$RENDER"

echo "==> 检查生成文件"
test -f gateway/generated/envoy.yaml
test -f deploy/firewall/generated/game-ports.nft
echo "generated files OK"

if command -v docker >/dev/null 2>&1; then
  echo "==> Envoy --mode validate"
  docker run --rm \
    -v "$ROOT/gateway/generated/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
    "${ENVOY_IMAGE:-envoyproxy/envoy:v1.39.0}" \
    /usr/local/bin/envoy -c /etc/envoy/envoy.yaml --mode validate
else
  echo "WARN: 未安装 docker，跳过 envoy --mode validate"
fi

if command -v nft >/dev/null 2>&1; then
  echo "==> nftables 语法检查"
  ( cd deploy/firewall && nft -c -f gateway.nft )
else
  echo "WARN: 未安装 nft，跳过防火墙语法检查"
fi

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  echo "==> compose 配置检查"
  if [[ ! -f .env ]]; then
    GRAFANA_ADMIN_PASSWORD=validate-only PANEL_ADMIN_PASSWORD=validate-only \
      docker compose -f deploy/compose.yaml config >/dev/null
  else
    docker compose -f deploy/compose.yaml --env-file .env config >/dev/null
  fi
fi

echo "全部校验通过"
