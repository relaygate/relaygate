#!/usr/bin/env bash
# 受控部署：备份 -> 校验 -> 启动/热更新
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"
STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP_PATH="$BACKUP_DIR/$STAMP"

if [[ ! -f .env ]]; then
  echo "ERROR: 缺少 .env，请先: cp .env.example .env && 编辑密码"
  exit 1
fi

# shellcheck disable=SC1091
set -a
source .env
set +a

mkdir -p "$BACKUP_PATH"

echo "==> 备份当前配置到 $BACKUP_PATH"
[[ -f envoy/generated/envoy.yaml ]] && cp -a envoy/generated/envoy.yaml "$BACKUP_PATH/"
[[ -f compose.yaml ]] && cp -a compose.yaml "$BACKUP_PATH/"
[[ -f config/resources.yaml ]] && cp -a config/resources.yaml "$BACKUP_PATH/"
echo "$STAMP" > "$BACKUP_DIR/latest"

echo "==> 校验"
bash scripts/validate.sh

echo "==> 应用内核参数（若存在）"
# 仓库内用语义名 gateway-01.conf；安装到 /etc/sysctl.d/ 时加 99- 前缀以保证晚于系统默认加载
SYSCTL_SRC="sysctl/gateway-01.conf"
SYSCTL_DST="/etc/sysctl.d/99-gateway-01.conf"
if [[ -f "$SYSCTL_SRC" ]]; then
  if [[ "$(id -u)" -eq 0 ]]; then
    cp "$SYSCTL_SRC" "$SYSCTL_DST"
    sysctl --system >/dev/null
    echo "sysctl applied: $SYSCTL_DST"
  else
    echo "WARN: 非 root，跳过 sysctl；请手动: sudo cp $SYSCTL_SRC $SYSCTL_DST && sudo sysctl --system"
  fi
fi

echo "==> 启动 / 更新 compose 服务"
docker compose pull
docker compose up -d

echo "==> 等待 Envoy ready"
for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:9901/ready" >/dev/null 2>&1; then
    echo "Envoy ready"
    break
  fi
  if [[ "$i" -eq 30 ]]; then
    echo "ERROR: Envoy 未在超时内 ready"
    docker compose ps
    exit 1
  fi
  sleep 2
done

echo "==> 服务状态"
docker compose ps
echo
echo "部署完成。"
echo "Grafana: SSH 隧道后访问 http://127.0.0.1:3000"
echo "  ssh -p 30455 -L 3000:127.0.0.1:3000 root@107.149.191.37"
echo "Prometheus: ssh -p 30455 -L 9090:127.0.0.1:9090 root@107.149.191.37"
echo "回滚: bash scripts/rollback.sh"
