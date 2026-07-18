#!/usr/bin/env bash
# 安全应用 nftables：先语法检查，再 apply，并提示确认 SSH
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "ERROR: 需要 root 执行（涉及 nftables）"
  exit 1
fi

SSH_PORT="${SSH_PORT:-30455}"

echo "==> 渲染端口定义"
python3 scripts/render_config.py

echo "==> 语法检查"
(
  cd firewall
  nft -c -f gateway.nft
)

echo
echo "即将应用 firewall/gateway.nft"
echo "请确认："
echo "  1. 当前 SSH 会话保持打开"
echo "  2. SSH 端口为 ${SSH_PORT}"
echo "  3. 有云厂商 VNC/控制台备用入口"
read -r -p "输入 YES 继续: " ans
if [[ "$ans" != "YES" ]]; then
  echo "已取消"
  exit 1
fi

# 备份旧规则
BACKUP="/root/nft-backup-$(date +%Y%m%d-%H%M%S).nft"
nft list ruleset > "$BACKUP" || true
echo "旧规则已备份到 $BACKUP"

(
  cd firewall
  nft -f gateway.nft
)

echo "==> 当前关键规则"
nft list ruleset | sed -n '1,120p'
echo
echo "防火墙已应用。请立即在新终端测试 SSH: ssh -p ${SSH_PORT} root@<公网IP>"
