#!/usr/bin/env bash
# 在当前网关主机上只读采集基线，输出到 docs/BASELINE.runtime.txt
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/docs/BASELINE.runtime.txt}"

if [[ -f "$ROOT/.env" ]]; then
  # shellcheck disable=SC1091
  set -a
  # shellcheck disable=SC1090
  source "$ROOT/.env"
  set +a
fi
GATEWAY_NAME="${GATEWAY_NAME:-gateway-01}"

{
  echo "# ${GATEWAY_NAME} runtime baseline collected at $(date -Is)"
  echo
  echo "## uname"
  uname -a || true
  echo
  echo "## os-release"
  cat /etc/os-release 2>/dev/null || true
  echo
  echo "## cpu"
  nproc || true
  lscpu 2>/dev/null | sed -n '1,20p' || true
  echo
  echo "## memory"
  free -h || true
  echo
  echo "## disk"
  df -h / || true
  echo
  echo "## addresses"
  ip -br addr || true
  echo
  echo "## routes"
  ip route || true
  echo
  echo "## listening sockets"
  ss -lntup || true
  echo
  echo "## docker"
  docker --version 2>/dev/null || echo "docker not installed"
  systemctl is-active docker 2>/dev/null || true
  echo
  echo "## containers"
  docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}' 2>/dev/null || true
  echo
  echo "## sysctl"
  sysctl net.core.somaxconn net.ipv4.ip_local_port_range net.core.rmem_max net.core.wmem_max fs.file-max 2>/dev/null || true
  echo
  echo "## firewall"
  if command -v nft >/dev/null 2>&1; then
    nft list ruleset 2>/dev/null || true
  elif command -v iptables >/dev/null 2>&1; then
    iptables -S 2>/dev/null || true
  fi
} | tee "$OUT"

echo
echo "已写入 $OUT"
echo "请将关键字段回填到 docs/BASELINE.md"
