#!/usr/bin/env bash
# 旁路验证 canary 端口 11001（TCP/UDP）
# 用法:
#   bash scripts/canary_test.sh                 # 测本机 127.0.0.1
#   bash scripts/canary_test.sh 107.149.191.37  # 测公网入口
set -euo pipefail

TARGET="${1:-127.0.0.1}"
TCP_PORT="${TCP_PORT:-11001}"
UDP_PORT="${UDP_PORT:-11001}"
TIMEOUT="${TIMEOUT:-3}"

echo "==> Canary 目标: ${TARGET}"
echo "==> TCP ${TCP_PORT} / UDP ${UDP_PORT}"

echo
echo "-- TCP connect --"
if command -v nc >/dev/null 2>&1; then
  if nc -z -w "$TIMEOUT" "$TARGET" "$TCP_PORT"; then
    echo "TCP OK: ${TARGET}:${TCP_PORT} 可连接"
  else
    echo "TCP FAIL: ${TARGET}:${TCP_PORT} 不可连接"
    exit 1
  fi
elif command -v timeout >/dev/null 2>&1; then
  if timeout "$TIMEOUT" bash -c "echo >/dev/tcp/${TARGET}/${TCP_PORT}" 2>/dev/null; then
    echo "TCP OK: ${TARGET}:${TCP_PORT} 可连接"
  else
    echo "TCP FAIL: ${TARGET}:${TCP_PORT} 不可连接"
    exit 1
  fi
else
  echo "WARN: 无 nc/timeout，跳过 TCP 探测"
fi

echo
echo "-- UDP send (单向探测，不保证有回包) --"
if command -v nc >/dev/null 2>&1; then
  # 部分 nc 不支持 -u 回包验证；这里只验证能发出
  printf 'canary-ping\n' | nc -u -w1 "$TARGET" "$UDP_PORT" >/dev/null 2>&1 || true
  echo "UDP datagram 已发送到 ${TARGET}:${UDP_PORT}"
  echo "请在 Grafana/Envoy 指标中确认 udp_*_downstream_sess_* 有增长"
else
  echo "WARN: 无 nc，跳过 UDP 探测"
fi

echo
echo "-- Envoy admin (仅本机) --"
if [[ "$TARGET" == "127.0.0.1" || "$TARGET" == "localhost" ]]; then
  if curl -fsS "http://127.0.0.1:9901/ready" >/dev/null; then
    echo "Envoy /ready OK"
  else
    echo "Envoy /ready FAIL"
    exit 1
  fi
  echo "clusters:"
  curl -fsS "http://127.0.0.1:9901/clusters" | grep -E 'cluster-server-01' | head -n 20 || true
else
  echo "跳过 admin 检查（非本机）"
fi

echo
echo "Canary 基础探测完成。"
echo "下一步建议："
echo "  1. 用真实游戏客户端连 ${TARGET}:${TCP_PORT}/${UDP_PORT}"
echo "  2. 人为阻断 server-01，观察健康检查与告警"
echo "  3. 通过后执行: python3 scripts/enable_server.py server-01 && bash scripts/deploy.sh"
