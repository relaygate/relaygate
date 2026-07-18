#!/usr/bin/env bash
# 回滚到最近一次部署备份的 Envoy / resources 配置
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"

if [[ $# -ge 1 ]]; then
  TARGET="$BACKUP_DIR/$1"
else
  if [[ ! -f "$BACKUP_DIR/latest" ]]; then
    echo "ERROR: 没有可用备份（$BACKUP_DIR/latest 不存在）"
    exit 1
  fi
  STAMP="$(cat "$BACKUP_DIR/latest")"
  TARGET="$BACKUP_DIR/$STAMP"
fi

if [[ ! -d "$TARGET" ]]; then
  echo "ERROR: 备份目录不存在: $TARGET"
  echo "可用备份:"
  ls -1 "$BACKUP_DIR" 2>/dev/null || true
  exit 1
fi

echo "==> 回滚自 $TARGET"
if [[ -f "$TARGET/resources.yaml" ]]; then
  cp -a "$TARGET/resources.yaml" config/resources.yaml
fi
if [[ -f "$TARGET/envoy.yaml" ]]; then
  mkdir -p envoy/generated
  cp -a "$TARGET/envoy.yaml" envoy/generated/envoy.yaml
else
  echo "WARN: 备份中无 envoy.yaml，尝试重新渲染"
  python3 scripts/render_config.py
fi

if [[ ! -f .env ]]; then
  echo "ERROR: 缺少 .env"
  exit 1
fi

echo "==> 校验并重启 Envoy"
bash scripts/validate.sh
docker compose up -d envoy

for i in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:9901/ready" >/dev/null 2>&1; then
    echo "Envoy ready（回滚完成）"
    exit 0
  fi
  sleep 2
done

echo "ERROR: 回滚后 Envoy 未 ready"
exit 1
