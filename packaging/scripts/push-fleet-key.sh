#!/usr/bin/env bash
# 从本机向网关主机灌入 fleet deploy 公钥，便于后续 BatchMode 自动化（fleet / scp）。
#
# 用法:
#   # 用已有密钥登录目标机（推荐）
#   BOOTSTRAP_IDENTITY=~/.ssh/id_ed25519 ./packaging/scripts/push-fleet-key.sh root@203.0.113.10
#   ./packaging/scripts/push-fleet-key.sh -i ~/.ssh/old_key -p 22 root@203.0.113.10
#
#   # 用密码登录（密码勿写进仓库；交互输入或 SSHPASS）
#   SSHPASS='...' ./packaging/scripts/push-fleet-key.sh --password root@203.0.113.10
#   ./packaging/scripts/push-fleet-key.sh --password root@203.0.113.10   # 提示输入
#
#   # 按 inventory 批量
#   ./packaging/scripts/push-fleet-key.sh --inventory /opt/relaygate/data/inventory/gateways.env
#   GATEWAYS=gateway-01,gateway-02 ./packaging/scripts/push-fleet-key.sh --inventory ...
#
# 环境变量:
#   FLEET_KEY / FLEET_PUB     私钥/公钥路径（默认 ~/.ssh/relaygate_fleet[.pub]）
#   BOOTSTRAP_IDENTITY       首次登录用的已有私钥
#   SSHPASS                  密码（仅 --password）
#   INVENTORY                inventory 路径
#   GATEWAYS                 逗号分隔网关名（默认读 GATEWAY_MATRIX）
set -euo pipefail

FLEET_KEY="${FLEET_KEY:-$HOME/.ssh/relaygate_fleet}"
FLEET_PUB="${FLEET_PUB:-${FLEET_KEY}.pub}"
BOOTSTRAP_IDENTITY="${BOOTSTRAP_IDENTITY:-}"
USE_PASSWORD=0
INVENTORY="${INVENTORY:-}"
SSH_PORT_DEFAULT="${SSH_PORT_DEFAULT:-22}"
STRICT_OPTS=(-o StrictHostKeyChecking=accept-new -o ConnectTimeout=15)

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

ensure_fleet_key() {
  if [[ ! -f "$FLEET_PUB" ]]; then
    echo "==> 生成 fleet deploy 密钥: $FLEET_KEY"
    mkdir -p "$(dirname "$FLEET_KEY")"
    ssh-keygen -t ed25519 -f "$FLEET_KEY" -C "relaygate-fleet@$(hostname -s 2>/dev/null || hostname)" -N ""
    chmod 600 "$FLEET_KEY"
  fi
  if [[ ! -f "$FLEET_KEY" ]]; then
    echo "缺少私钥 $FLEET_KEY（有公钥无私钥无法做后续 BatchMode 验证）" >&2
    exit 1
  fi
  chmod 600 "$FLEET_KEY" 2>/dev/null || true
  echo "==> 将灌入公钥:"
  cat "$FLEET_PUB"
}

# parse inventory KEY=VAL lines
inv_get() {
  local file="$1" key="$2"
  # shellcheck disable=SC1090
  awk -F= -v k="$key" '$1==k {sub(/^[^=]*=/,""); print; exit}' "$file" | tr -d '\r'
}

push_one() {
  local user_host="$1" port="$2"
  local user host
  if [[ "$user_host" == *@* ]]; then
    user="${user_host%%@*}"
    host="${user_host#*@}"
  else
    user="root"
    host="$user_host"
  fi
  # host:port shorthand
  if [[ "$host" == *:* && "$host" != \[* ]]; then
    port="${host##*:}"
    host="${host%:*}"
  fi
  port="${port:-$SSH_PORT_DEFAULT}"

  echo ""
  echo "========== $user@$host:$port =========="

  local -a ssh_base=(ssh "${STRICT_OPTS[@]}" -p "$port")
  local -a scp_base=(scp "${STRICT_OPTS[@]}" -P "$port")

  if [[ "$USE_PASSWORD" -eq 1 ]]; then
    if ! command -v sshpass >/dev/null 2>&1; then
      echo "需要 sshpass：apt-get install -y sshpass" >&2
      exit 1
    fi
    if [[ -z "${SSHPASS:-}" ]]; then
      read -r -s -p "SSH 密码 ($user@$host): " SSHPASS
      echo
      export SSHPASS
    fi
    ssh_base=(sshpass -e "${ssh_base[@]}" -o PreferredAuthentications=password -o PubkeyAuthentication=no)
    scp_base=(sshpass -e "${scp_base[@]}" -o PreferredAuthentications=password -o PubkeyAuthentication=no)
  else
    if [[ -n "$BOOTSTRAP_IDENTITY" ]]; then
      if [[ ! -f "$BOOTSTRAP_IDENTITY" ]]; then
        echo "BOOTSTRAP_IDENTITY 不存在: $BOOTSTRAP_IDENTITY" >&2
        exit 1
      fi
      ssh_base+=(-i "$BOOTSTRAP_IDENTITY" -o IdentitiesOnly=yes)
      scp_base+=(-i "$BOOTSTRAP_IDENTITY" -o IdentitiesOnly=yes)
    fi
  fi

  # 幂等写入 authorized_keys（避免重复行）
  local pub
  pub="$(tr -d '\r\n' <"$FLEET_PUB")"
  "${ssh_base[@]}" "$user@$host" "mkdir -p ~/.ssh && chmod 700 ~/.ssh && touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && grep -qxF $(printf '%q' "$pub") ~/.ssh/authorized_keys || echo $(printf '%q' "$pub") >> ~/.ssh/authorized_keys && echo OK_AUTHORIZED"

  echo "==> 验证 fleet 钥 BatchMode..."
  if ssh -i "$FLEET_KEY" -o BatchMode=yes -o IdentitiesOnly=yes "${STRICT_OPTS[@]}" -p "$port" "$user@$host" 'echo FLEET_OK; hostname'; then
    echo "==> $user@$host:$port 就绪（可用 fleet 自动化）"
  else
    echo "==> 灌钥可能成功，但 BatchMode 验证失败（检查 PermitRootLogin / PubkeyAuthentication）" >&2
    return 1
  fi
}

push_inventory() {
  local inv="$1"
  if [[ ! -f "$inv" ]]; then
    echo "inventory 不存在: $inv" >&2
    exit 1
  fi
  local matrix gws
  matrix="$(inv_get "$inv" GATEWAY_MATRIX)"
  gws="${GATEWAYS:-$matrix}"
  if [[ -z "$gws" ]]; then
    echo "未设置 GATEWAYS 且 inventory 无 GATEWAY_MATRIX" >&2
    exit 1
  fi
  local gw key host port user
  local failed=0
  IFS=',' read -r -a arr <<<"$gws"
  for gw in "${arr[@]}"; do
    gw="$(echo "$gw" | tr -d '[:space:]')"
    [[ -z "$gw" ]] && continue
    key="${gw//-/_}"
    host="$(inv_get "$inv" "HOST_$key")"
    port="$(inv_get "$inv" "SSH_PORT_$key")"
    user="$(inv_get "$inv" "SSH_USER_$key")"
    user="${user:-root}"
    port="${port:-$SSH_PORT_DEFAULT}"
    if [[ -z "$host" ]]; then
      echo "跳过 $gw：无 HOST_$key" >&2
      failed=1
      continue
    fi
    if ! push_one "$user@$host" "$port"; then
      failed=1
    fi
  done
  return "$failed"
}

TARGETS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage 0 ;;
    -i|--identity)
      BOOTSTRAP_IDENTITY="$2"
      shift 2
      ;;
    --password|-P)
      USE_PASSWORD=1
      shift
      ;;
    --inventory)
      INVENTORY="$2"
      shift 2
      ;;
    -p|--port)
      SSH_PORT_DEFAULT="$2"
      shift 2
      ;;
    --)
      shift
      TARGETS+=("$@")
      break
      ;;
    -*)
      echo "未知参数: $1" >&2
      usage 1
      ;;
    *)
      TARGETS+=("$1")
      shift
      ;;
  esac
done

ensure_fleet_key

ec=0
if [[ -n "$INVENTORY" ]]; then
  push_inventory "$INVENTORY" || ec=$?
fi

if [[ ${#TARGETS[@]} -gt 0 ]]; then
  for t in "${TARGETS[@]}"; do
    push_one "$t" "$SSH_PORT_DEFAULT" || ec=1
  done
fi

if [[ -z "$INVENTORY" && ${#TARGETS[@]} -eq 0 ]]; then
  echo "请指定目标主机，或 --inventory <gateways.env>" >&2
  usage 1
fi

echo ""
echo "==> 后续自动化请使用:"
echo "    export SSH_OPTS=\"-o StrictHostKeyChecking=accept-new -o BatchMode=yes -i $FLEET_KEY\""
echo "    GATEWAYS=... relaygate fleet"
exit "$ec"
