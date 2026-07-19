#!/usr/bin/env bash
# Idempotent install of RelayGate Panel as a host systemd service (binary mode).
# Safe to re-run. Does not start Compose data-plane services.
set -euo pipefail

INSTALL_DIR="${RELAYGATE_INSTALL_DIR:-/opt/relaygate}"
SECRETS_DIR="${RELAYGATE_SECRETS_DIR:-/etc/relaygate/secrets}"
HELPER_DIR="/usr/local/libexec/relaygate"
HELPER_PATH="${HELPER_DIR}/apply"
UNIT_DST="/etc/systemd/system/relaygate-panel.service"
SUDOERS_DST="/etc/sudoers.d/relaygate-panel"
PANEL_ENV="/etc/relaygate/panel.env"
DRY_RUN="${DRY_RUN:-0}"
ENABLE_NOW="${ENABLE_NOW:-1}"
GRAFANA_URL="${GRAFANA_URL:-http://127.0.0.1:3000}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
UNIT_SRC="${UNIT_SRC:-$REPO_ROOT/deploy/systemd/relaygate-panel.service}"
HELPER_SRC="${HELPER_SRC:-$REPO_ROOT/deploy/systemd/relaygate-apply}"
SUDOERS_SRC="${SUDOERS_SRC:-$REPO_ROOT/deploy/systemd/relaygate-panel.sudoers}"

log() { printf '%s\n' "==> $*"; }
die() { printf '%s\n' "ERROR: $*" >&2; exit 1; }
run() {
  if [[ "$DRY_RUN" == "1" ]]; then
    printf '[dry-run]'
    printf ' %q' "$@"
    printf '\n'
  else
    "$@"
  fi
}

[[ "$(id -u)" == "0" || "$DRY_RUN" == "1" ]] || die "请以 root 运行（或 DRY_RUN=1）"
[[ -d "$INSTALL_DIR" ]] || die "安装目录不存在: $INSTALL_DIR"
[[ -x "$INSTALL_DIR/bin/relaygate" ]] || die "缺少可执行文件: $INSTALL_DIR/bin/relaygate"
[[ -f "$UNIT_SRC" && -f "$HELPER_SRC" && -f "$SUDOERS_SRC" ]] || die "缺少 deploy/systemd 模板"

# Ensure source templates used when installing from INSTALL_DIR tree.
if [[ -f "$INSTALL_DIR/deploy/systemd/relaygate-panel.service" ]]; then
  UNIT_SRC="$INSTALL_DIR/deploy/systemd/relaygate-panel.service"
  HELPER_SRC="$INSTALL_DIR/deploy/systemd/relaygate-apply"
  SUDOERS_SRC="$INSTALL_DIR/deploy/systemd/relaygate-panel.sudoers"
fi

render_unit() {
  local src="$1" dest="$2"
  if [[ "$INSTALL_DIR" == "/opt/relaygate" && "$SECRETS_DIR" == "/etc/relaygate/secrets" ]]; then
    run install -m 0644 -o root -g root "$src" "$dest"
    return
  fi
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] 将按 INSTALL_DIR=$INSTALL_DIR SECRETS_DIR=$SECRETS_DIR 渲染 unit → $dest"
    return
  fi
  sed \
    -e "s|/opt/relaygate|${INSTALL_DIR}|g" \
    -e "s|/etc/relaygate/secrets|${SECRETS_DIR}|g" \
    "$src" > "$dest"
  chmod 0644 "$dest"
  chown root:root "$dest"
}

ensure_user() {
  if ! getent group relaygate >/dev/null 2>&1; then
    run groupadd --system relaygate
  fi
  if ! getent passwd relaygate >/dev/null 2>&1; then
    run useradd --system --gid relaygate --home-dir "$INSTALL_DIR" \
      --shell /usr/sbin/nologin --comment "RelayGate Panel" relaygate
  fi
  # Must never be in docker group.
  if id -nG relaygate 2>/dev/null | tr ' ' '\n' | grep -qx docker; then
    die "用户 relaygate 在 docker 组中；请先: gpasswd -d relaygate docker"
  fi
}

fix_permissions() {
  run install -d -m 0755 -o root -g root "$INSTALL_DIR"
  run install -d -m 0755 -o root -g root "$INSTALL_DIR/bin"
  run install -d -m 0755 -o root -g root "$INSTALL_DIR/scripts"
  run install -d -m 0755 -o root -g root "$INSTALL_DIR/web"
  run install -d -m 0750 -o root -g relaygate "$INSTALL_DIR/config"
  run install -d -m 0770 -o root -g relaygate "$INSTALL_DIR/gateway/generated"
  run install -d -m 0770 -o root -g relaygate "$INSTALL_DIR/deploy/firewall/generated"
  run install -d -m 0770 -o root -g relaygate "$INSTALL_DIR/backups"

  if [[ -f "$INSTALL_DIR/bin/relaygate" ]]; then
    run chown root:root "$INSTALL_DIR/bin/relaygate"
    run chmod 0755 "$INSTALL_DIR/bin/relaygate"
  fi
  if [[ -f "$INSTALL_DIR/config/resources.yaml" ]]; then
    run chown root:relaygate "$INSTALL_DIR/config/resources.yaml"
    run chmod 0660 "$INSTALL_DIR/config/resources.yaml"
  fi

  # Secrets: directory traversable by relaygate; panel password group-readable;
  # grafana password remains root-only.
  run install -d -m 0750 -o root -g relaygate "$SECRETS_DIR"
  if [[ -f "$SECRETS_DIR/panel_admin_password" ]]; then
    run chown root:relaygate "$SECRETS_DIR/panel_admin_password"
    run chmod 0640 "$SECRETS_DIR/panel_admin_password"
  fi
  if [[ -f "$SECRETS_DIR/grafana_admin_password" ]]; then
    run chown root:root "$SECRETS_DIR/grafana_admin_password"
    run chmod 0600 "$SECRETS_DIR/grafana_admin_password"
  fi
}

install_helper_and_sudoers() {
  run install -d -m 0755 -o root -g root "$HELPER_DIR"
  run install -m 0755 -o root -g root "$HELPER_SRC" "$HELPER_PATH"
  # Embed install dir for non-default prefixes.
  if [[ "$INSTALL_DIR" != "/opt/relaygate" && "$DRY_RUN" != "1" ]]; then
    sed -i "s|^INSTALL_DIR=.*|INSTALL_DIR=\"${INSTALL_DIR}\"|" "$HELPER_PATH" 2>/dev/null || \
      sed -i "s|RELAYGATE_INSTALL_DIR:-/opt/relaygate|RELAYGATE_INSTALL_DIR:-${INSTALL_DIR}|" "$HELPER_PATH"
  fi

  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] 将安装 sudoers → $SUDOERS_DST"
  else
    install -m 0440 -o root -g root "$SUDOERS_SRC" "$SUDOERS_DST"
    if command -v visudo >/dev/null 2>&1; then
      visudo -cf "$SUDOERS_DST" || die "sudoers 校验失败: $SUDOERS_DST"
    fi
  fi
}

write_panel_env() {
  local grafana_line
  if [[ -z "${GRAFANA_URL}" ]]; then
    grafana_line='GRAFANA_URL='
  else
    grafana_line="GRAFANA_URL=${GRAFANA_URL}"
  fi
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] 将写入 $PANEL_ENV ($grafana_line)"
    return
  fi
  install -d -m 0750 -o root -g relaygate /etc/relaygate
  umask 077
  cat > "$PANEL_ENV" <<EOF
# Managed by scripts/install_panel_service.sh — Panel systemd EnvironmentFile
PANEL_ROOT=${INSTALL_DIR}
PANEL_BIND=127.0.0.1:9000
PANEL_ADMIN_PASSWORD_FILE=${SECRETS_DIR}/panel_admin_password
RELAYGATE_BIN=${INSTALL_DIR}/bin/relaygate
RELAYGATE_PRIVILEGED_HELPER=${HELPER_PATH}
ENVOY_ADMIN_URL=http://127.0.0.1:9901
PROMETHEUS_URL=http://127.0.0.1:9090
${grafana_line}
EOF
  chown root:relaygate "$PANEL_ENV"
  chmod 0640 "$PANEL_ENV"
}

log "安装 Panel systemd 服务（INSTALL_DIR=$INSTALL_DIR）"
ensure_user
fix_permissions
install_helper_and_sudoers
render_unit "$UNIT_SRC" "$UNIT_DST"
write_panel_env

run systemctl daemon-reload
if [[ "$ENABLE_NOW" == "1" ]]; then
  run systemctl enable --now relaygate-panel.service
  log "已 enable --now relaygate-panel"
else
  run systemctl enable relaygate-panel.service
  run systemctl stop relaygate-panel.service 2>/dev/null || true
  log "已 enable；未启动（ENABLE_NOW=0）"
fi

log "状态: systemctl status relaygate-panel"
log "日志: journalctl -u relaygate-panel -f"
