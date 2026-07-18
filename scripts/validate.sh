#!/usr/bin/env bash
# 校验资产文件、渲染结果与（可选）Envoy 配置语法
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> 校验 Python 依赖"
python3 - <<'PY'
import importlib.util, sys
if importlib.util.find_spec("yaml") is None:
    sys.exit("缺少 PyYAML，请: pip3 install -r requirements.txt")
print("PyYAML OK")
PY

echo "==> 渲染并校验 resources.yaml"
python3 scripts/render_config.py --check-only
python3 scripts/render_config.py

echo "==> 检查生成文件"
test -f envoy/generated/envoy.yaml
test -f firewall/generated/game-ports.nft
echo "generated files OK"

if command -v docker >/dev/null 2>&1; then
  echo "==> Envoy --mode validate（容器）"
  docker run --rm \
    -v "$ROOT/envoy/generated/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
    "${ENVOY_IMAGE:-envoyproxy/envoy:v1.39.0}" \
    /usr/local/bin/envoy -c /etc/envoy/envoy.yaml --mode validate
else
  echo "WARN: 未安装 docker，跳过 envoy --mode validate"
fi

if command -v nft >/dev/null 2>&1; then
  echo "==> nftables 语法检查"
  (
    cd firewall
    nft -c -f gateway.nft
  )
else
  echo "WARN: 未安装 nft，跳过防火墙语法检查"
fi

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  echo "==> compose 配置检查"
  if [[ ! -f .env ]]; then
    echo "WARN: 缺少 .env，使用临时变量做 compose config 检查"
    GRAFANA_ADMIN_PASSWORD=validate-only docker compose config >/dev/null
  else
    docker compose config >/dev/null
  fi
fi

echo "全部校验通过"
