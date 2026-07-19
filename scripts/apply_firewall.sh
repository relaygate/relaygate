#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

[[ "$(id -u)" -eq 0 ]] || { echo "ERROR: 需要 root"; exit 1; }
if [[ -f .env ]]; then
  # shellcheck disable=SC1091
  set -a
  source .env
  set +a
fi
SSH_PORT="${SSH_PORT:-${GATEWAY_SSH_PORT:-30455}}"
RELAYGATE="${RELAYGATE_BIN:-$ROOT/bin/relaygate}"
APPLY_FIREWALL="${APPLY_FIREWALL:-0}"
NONINTERACTIVE="${NONINTERACTIVE:-0}"
RUNTIME_RULESET="deploy/firewall/.gateway-${SSH_PORT}.nft"

case "$SSH_PORT" in
  ''|*[!0-9]*) echo "ERROR: SSH 端口无效: $SSH_PORT"; exit 1 ;;
esac
if (( SSH_PORT < 1 || SSH_PORT > 65535 )); then
  echo "ERROR: SSH 端口超出范围: $SSH_PORT"
  exit 1
fi

echo "==> 渲染端口定义"
"$RELAYGATE" render
sed "s/^define SSH_PORT = .*/define SSH_PORT = ${SSH_PORT}/" \
  deploy/firewall/gateway.nft > "$RUNTIME_RULESET"

echo "==> 语法检查"
( cd deploy/firewall && nft -c -f "$(basename "$RUNTIME_RULESET")" )

echo
echo "规则已生成并校验: $RUNTIME_RULESET"
echo "警告：该规则包含 flush ruleset，应用错误可能中断 SSH。"
echo "将保留 SSH/TCP ${SSH_PORT}；应用前请保持当前会话并准备云控制台。"
if [[ "$APPLY_FIREWALL" != "1" ]]; then
  echo "默认未应用。确认无误后执行: sudo APPLY_FIREWALL=1 SSH_PORT=${SSH_PORT} bash scripts/apply_firewall.sh"
  exit 0
fi

if [[ "$NONINTERACTIVE" == "1" ]]; then
  [[ "${FIREWALL_CONFIRM:-}" == "YES_FLUSH_NFTABLES" ]] || {
    echo "ERROR: 非交互应用还需 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES"
    exit 1
  }
else
  if [[ -r /dev/tty ]]; then
    read -r -p "输入 YES_FLUSH_NFTABLES 继续: " ans </dev/tty
  else
    echo "ERROR: 无交互终端；请设置 NONINTERACTIVE=1 和 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES"
    exit 1
  fi
  [[ "$ans" == "YES_FLUSH_NFTABLES" ]] || { echo "已取消"; exit 1; }
fi

BACKUP="/root/nft-backup-$(date +%Y%m%d-%H%M%S).nft"
nft list ruleset > "$BACKUP"
RESTORE="${BACKUP%.nft}-restore.sh"
cat > "$RESTORE" <<EOF
#!/usr/bin/env bash
set -e
nft -f "$BACKUP"
EOF
chmod 700 "$RESTORE"
mkdir -p "$ROOT/backups"
printf '%s\n' "$RESTORE" > "$ROOT/backups/firewall-latest"
chmod 600 "$ROOT/backups/firewall-latest"
echo "旧规则备份: $BACKUP"
echo "恢复命令: $RESTORE"
( cd deploy/firewall && nft -f "$(basename "$RUNTIME_RULESET")" )
nft list ruleset
echo "请立即新开终端测试 SSH: ssh -p ${SSH_PORT} root@<公网IP>"
