#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:-127.0.0.1}"
TCP_PORT="${TCP_PORT:-11001}"
UDP_PORT="${UDP_PORT:-11001}"
TIMEOUT="${TIMEOUT:-3}"

echo "==> Canary 目标: ${TARGET} TCP/UDP ${TCP_PORT}"

echo "-- TCP connect --"
if command -v nc >/dev/null 2>&1; then
  nc -z -w "$TIMEOUT" "$TARGET" "$TCP_PORT" && echo "TCP OK" || { echo "TCP FAIL"; exit 1; }
else
  echo "WARN: 无 nc，跳过 TCP"
fi

echo "-- UDP send --"
if command -v nc >/dev/null 2>&1; then
  printf 'canary-ping\n' | nc -u -w1 "$TARGET" "$UDP_PORT" >/dev/null 2>&1 || true
  echo "UDP datagram 已发送"
fi

if [[ "$TARGET" == "127.0.0.1" || "$TARGET" == "localhost" ]]; then
  curl -fsS "http://127.0.0.1:9901/ready" >/dev/null && echo "Envoy /ready OK"
  curl -fsS "http://127.0.0.1:9901/clusters" | grep -E 'cluster-server-01' | head -n 20 || true
fi

echo "下一步: ./bin/relaygate server enable server-01 && bash scripts/deploy.sh"
