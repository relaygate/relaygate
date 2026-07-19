#!/usr/bin/env bash
# 按网关矩阵分批部署：逐台 drain → sync → reload → smoke，避免双机同时重启
#
# 用法（在具备 SSH 访问的运维机 / CI runner 上）:
#   GATEWAYS="gateway-01,gateway-02" bash scripts/deploy_multi.sh
#
# 需要 inventory 文件 deploy/inventory/gateways.env（见同目录 example）
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

INVENTORY="${INVENTORY:-$ROOT/deploy/inventory/gateways.env}"
GATEWAYS_CSV="${GATEWAYS:-}"
IMAGE_TAG="${IMAGE_TAG:-}"
SSH_OPTS="${SSH_OPTS:--o StrictHostKeyChecking=accept-new -o BatchMode=yes}"

if [[ ! -f "$INVENTORY" ]]; then
  echo "ERROR: 缺少 inventory: $INVENTORY"
  echo "请复制 deploy/inventory/gateways.env.example"
  exit 1
fi

# shellcheck disable=SC1090
source "$INVENTORY"

if [[ -z "$GATEWAYS_CSV" ]]; then
  GATEWAYS_CSV="${GATEWAY_MATRIX:-gateway-01,gateway-02}"
fi

IFS=',' read -r -a GATEWAY_LIST <<< "$GATEWAYS_CSV"

lookup_host() {
  local name="$1"
  local var="HOST_${name//-/_}"
  echo "${!var:-}"
}

lookup_ssh_port() {
  local name="$1"
  local var="SSH_PORT_${name//-/_}"
  echo "${!var:-30455}"
}

lookup_user() {
  local name="$1"
  local var="SSH_USER_${name//-/_}"
  echo "${!var:-root}"
}

lookup_remote_dir() {
  local name="$1"
  local var="REMOTE_DIR_${name//-/_}"
  echo "${!var:-/opt/relaygate}"
}

for gw in "${GATEWAY_LIST[@]}"; do
  gw="$(echo "$gw" | xargs)"
  [[ -n "$gw" ]] || continue
  host="$(lookup_host "$gw")"
  port="$(lookup_ssh_port "$gw")"
  user="$(lookup_user "$gw")"
  rdir="$(lookup_remote_dir "$gw")"
  if [[ -z "$host" ]]; then
    echo "ERROR: inventory 未定义 HOST_${gw//-/_}"
    exit 1
  fi

  echo
  echo "========== 分批部署: ${gw} (${user}@${host}:${port}) =========="

  echo "==> 1/5 drain"
  # shellcheck disable=SC2086
  ssh $SSH_OPTS -p "$port" "${user}@${host}" "cd '${rdir}' && bash scripts/drain_gateway.sh fail"

  echo "==> 2/5 sync git / artifact"
  # shellcheck disable=SC2086
  ssh $SSH_OPTS -p "$port" "${user}@${host}" "cd '${rdir}' && git fetch --all && git checkout \"${DEPLOY_REF:-main}\" && git pull --ff-only"
  if [[ -n "$IMAGE_TAG" ]]; then
    # shellcheck disable=SC2086
    ssh $SSH_OPTS -p "$port" "${user}@${host}" "cd '${rdir}' && sed -i 's/^IMAGE_TAG=.*/IMAGE_TAG=${IMAGE_TAG}/' .env || echo IMAGE_TAG=${IMAGE_TAG} >> .env"
  fi

  echo "==> 3/5 render + reload envoy"
  # shellcheck disable=SC2086
  ssh $SSH_OPTS -p "$port" "${user}@${host}" "cd '${rdir}' && bash scripts/render_observability.sh .env && bash scripts/reload_envoy.sh"

  echo "==> 4/5 undrain + smoke"
  # shellcheck disable=SC2086
  ssh $SSH_OPTS -p "$port" "${user}@${host}" "cd '${rdir}' && bash scripts/drain_gateway.sh ok && bash scripts/smoke_test.sh 127.0.0.1"

  echo "==> 5/5 ${gw} 完成，继续下一台前短暂等待"
  sleep "${BATCH_PAUSE_SEC:-10}"
done

echo
echo "全部分批部署完成: ${GATEWAYS_CSV}"
echo "回滚单台: ssh … 'cd /opt/relaygate && bash scripts/rollback.sh'"
