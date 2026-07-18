#!/usr/bin/env bash
# 渲染后以 drain 方式重启 Envoy（SIGTERM + stop_grace_period）
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

python3 scripts/render_config.py

if command -v docker >/dev/null 2>&1; then
  docker run --rm \
    -v "$ROOT/envoy/generated/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
    "${ENVOY_IMAGE:-envoyproxy/envoy:v1.39.0}" \
    /usr/local/bin/envoy -c /etc/envoy/envoy.yaml --mode validate
fi

echo "==> draining / restarting envoy"
# 先标记失败，减少新连接进入（若 admin 可达）
curl -fsS -X POST "http://127.0.0.1:9901/healthcheck/fail" >/dev/null 2>&1 || true
sleep 2
docker compose up -d --force-recreate --no-deps envoy

for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:9901/ready" >/dev/null 2>&1; then
    curl -fsS -X POST "http://127.0.0.1:9901/healthcheck/ok" >/dev/null 2>&1 || true
    echo "Envoy reloaded and ready"
    exit 0
  fi
  sleep 2
done

echo "ERROR: Envoy 未 ready"
exit 1
