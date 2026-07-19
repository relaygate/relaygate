#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"
STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP_PATH="$BACKUP_DIR/$STAMP"
COMPOSE=(docker compose -f deploy/compose.yaml --env-file .env)

if [[ ! -f .env ]]; then
  echo "ERROR: 缺少 .env，请先: cp .env.example .env && 编辑密码"
  echo "双活示例: cp deploy/env/gateway-01.env.example .env"
  exit 1
fi

# shellcheck disable=SC1091
set -a
source .env
set +a
GATEWAY_NAME="${GATEWAY_NAME:?set GATEWAY_NAME in .env}"
GATEWAY_PUBLIC_IP="${GATEWAY_PUBLIC_IP:-127.0.0.1}"
GATEWAY_SSH_PORT="${GATEWAY_SSH_PORT:-30455}"
ENVOY_ADMIN_PORT="${ENVOY_ADMIN_PORT:-9901}"

mkdir -p "$BACKUP_PATH"
echo "==> 备份到 $BACKUP_PATH (gateway=${GATEWAY_NAME})"
[[ -f gateway/generated/envoy.yaml ]] && cp -a gateway/generated/envoy.yaml "$BACKUP_PATH/"
[[ -f deploy/compose.yaml ]] && cp -a deploy/compose.yaml "$BACKUP_PATH/"
[[ -f config/resources.yaml ]] && cp -a config/resources.yaml "$BACKUP_PATH/"
[[ -f deploy/prometheus/prometheus.yml ]] && cp -a deploy/prometheus/prometheus.yml "$BACKUP_PATH/"
echo "$STAMP" > "$BACKUP_DIR/latest"

echo "==> 渲染可观测性配置"
bash scripts/render_observability.sh .env

echo "==> 校验"
bash scripts/validate.sh

echo "==> 应用内核参数（若存在）"
SYSCTL_SRC="deploy/sysctl/gateway.conf"
SYSCTL_DST="/etc/sysctl.d/99-${GATEWAY_NAME}.conf"
if [[ -f "$SYSCTL_SRC" ]]; then
  if [[ "$(id -u)" -eq 0 ]]; then
    cp "$SYSCTL_SRC" "$SYSCTL_DST"
    sysctl --system >/dev/null
    echo "sysctl applied: $SYSCTL_DST"
  else
    echo "WARN: 非 root，跳过 sysctl；sudo cp $SYSCTL_SRC $SYSCTL_DST && sudo sysctl --system"
  fi
fi

echo "==> compose up (${GATEWAY_NAME})"
"${COMPOSE[@]}" pull || true
"${COMPOSE[@]}" up -d --build

echo "==> 等待 Envoy ready"
for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${ENVOY_ADMIN_PORT}/ready" >/dev/null 2>&1; then
    echo "Envoy ready"
    break
  fi
  if [[ "$i" -eq 30 ]]; then
    echo "ERROR: Envoy 未 ready"
    "${COMPOSE[@]}" ps
    exit 1
  fi
  sleep 2
done

"${COMPOSE[@]}" ps
echo
echo "部署完成: ${GATEWAY_NAME}"
echo "Panel:   ssh -p ${GATEWAY_SSH_PORT} -L 8080:127.0.0.1:8080 root@${GATEWAY_PUBLIC_IP}"
echo "Grafana: ssh -p ${GATEWAY_SSH_PORT} -L 3000:127.0.0.1:3000 root@${GATEWAY_PUBLIC_IP}"
echo "回滚: bash scripts/rollback.sh"
echo "冒烟: bash scripts/smoke_test.sh"
