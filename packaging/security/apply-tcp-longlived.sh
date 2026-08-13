#!/usr/bin/env bash
# TCP 长连接（long-lived / persistent TCP）网关：打印推荐参数，并可选应用 sysctl SYN 加固。
# 不改正式 nftables 真相源（desired state）、不引入 global rate limiting via external RLS（Redis 等）、
# 不静默改 resources.yaml。
#
# 用法:
#   ./packaging/security/apply-tcp-longlived.sh
#   sudo ./packaging/security/apply-tcp-longlived.sh --apply
#   sudo RELAYGATE_CONFIRM=Confirm ./packaging/security/apply-tcp-longlived.sh --apply
#
# --apply 仅调用 apply-sysctl-harden.sh --apply；业务 defaults 请用:
#   relaygate profile apply tcp-longlived
# nftables 新建连接（new-conn）限速仍走 defaults.nftables + relaygate firewall apply。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
APPLY=0

usage() {
  sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

for arg in "$@"; do
  case "$arg" in
    -h|--help) usage 0 ;;
    --apply) APPLY=1 ;;
    *) echo "未知参数: $arg" >&2; usage 1 ;;
  esac
done

cat <<'EOF'
==> TCP 长连接场景安全策略 · defense in depth
  • nftables：established/related 先放行；只对 ct state new 做每 IP rate limiting（见 gateway.nft）
  • Envoy：local rate limit / max_connections 约束新建与并发，不是业务 PPS
  • 正常业务多为小包（0–199B）心跳/短载荷，走 established，不受 new 限速
  • 近 MTU 体量攻击（~1400–1499B 单峰）：云高防；本机只减缓握手 / 新建滥用
  • 流量对照：docs/packet-size-traffic-analysis.md
  • 术语与攻击×分层：packaging/security/threat-analysis.md

==> 推荐参数（profile: tcp-longlived，相对 default 放宽 idle/并发与新建余量）
  tcp_idle_timeout: 14400s
  max_connections: 4096
  max_pending_requests: 1024
  gateway_new_conn_limit: per_sec 150 · burst 300
  firewall_new_conn_limit: tcp_per_ip 40/second · burst 80

==> 建议操作顺序
  1. 可选 sysctl（SYN cookies / backlog）: packaging/security/apply-sysctl-harden.sh --apply
  2. 套用档位: relaygate profile apply tcp-longlived
  3. 热更新本机应用（Panel/CLI，须确认词）
  4. 若改了 nftables 默认值: relaygate firewall apply（须确认词；防 desired/actual 漂移）
  5. FD limits: 参阅 packaging/security/systemd-nofile.snippet
  6. 调高 max_connections 后核对 Prometheus EnvoyConnectionsNearLimit（约 80% ≈ 3277）
  7. 对照说明: packaging/security/nft-newconn-syn.snippet.nft（勿直接 flush 现网）

EOF

if [[ -f "${ROOT}/packaging/profiles/tcp-longlived.yaml" ]]; then
  echo "==> profile 文件: packaging/profiles/tcp-longlived.yaml"
fi

echo "==> ulimit / FD limits 速查（当前 shell，仅供参考）"
ulimit -n 2>/dev/null || true
if [[ -r /proc/sys/fs/file-max ]]; then
  echo "fs.file-max=$(cat /proc/sys/fs/file-max)"
fi

if [[ "$APPLY" -ne 1 ]]; then
  echo
  echo "预览结束。应用内核 SYN 加固请加: sudo $0 --apply"
  exit 0
fi

echo
echo "==> --apply：调用 sysctl 加固脚本"
exec "${SCRIPT_DIR}/apply-sysctl-harden.sh" --apply
