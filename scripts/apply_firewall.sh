#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

[[ "$(id -u)" -eq 0 ]] || { echo "ERROR: 需要 root"; exit 1; }
SSH_PORT="${SSH_PORT:-30455}"
RENDER="${RENDER_BIN:-$ROOT/bin/gateway-render}"

echo "==> 渲染端口定义"
"$RENDER"

echo "==> 语法检查"
( cd deploy/firewall && nft -c -f gateway.nft )

echo
echo "即将应用 deploy/firewall/gateway.nft"
echo "请确认 SSH 端口 ${SSH_PORT} 且保留现有会话"
read -r -p "输入 YES 继续: " ans
[[ "$ans" == "YES" ]] || { echo "已取消"; exit 1; }

BACKUP="/root/nft-backup-$(date +%Y%m%d-%H%M%S).nft"
nft list ruleset > "$BACKUP" || true
echo "旧规则备份: $BACKUP"
( cd deploy/firewall && nft -f gateway.nft )
nft list ruleset | sed -n '1,120p'
echo "请立即新开终端测试 SSH: ssh -p ${SSH_PORT} root@<公网IP>"
