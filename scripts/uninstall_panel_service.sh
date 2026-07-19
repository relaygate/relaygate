#!/usr/bin/env bash
# Idempotent uninstall of RelayGate Panel systemd service / helper / sudoers.
# Default: keep user relaygate, config, and secrets. PURGE=1 also removes user
# (only if confirmed) — does not delete INSTALL_DIR (use install.sh --purge).
set -euo pipefail

PURGE="${PURGE:-0}"
DRY_RUN="${DRY_RUN:-0}"
UNIT_DST="/etc/systemd/system/relaygate-panel.service"
SUDOERS_DST="/etc/sudoers.d/relaygate-panel"
HELPER_PATH="/usr/local/libexec/relaygate/apply"
HELPER_DIR="/usr/local/libexec/relaygate"
PANEL_ENV="/etc/relaygate/panel.env"

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

log "停止并禁用 relaygate-panel"
if [[ "$DRY_RUN" == "1" ]]; then
  log "[dry-run] systemctl disable --now relaygate-panel.service"
else
  systemctl disable --now relaygate-panel.service 2>/dev/null || true
  systemctl stop relaygate-panel.service 2>/dev/null || true
fi

run rm -f "$UNIT_DST"
run rm -f "$SUDOERS_DST"
run rm -f "$HELPER_PATH"
run rm -f "$PANEL_ENV"
# Remove helper dir only if empty.
if [[ "$DRY_RUN" != "1" && -d "$HELPER_DIR" ]]; then
  rmdir "$HELPER_DIR" 2>/dev/null || true
fi

run systemctl daemon-reload
run systemctl reset-failed relaygate-panel.service 2>/dev/null || true

if [[ "$PURGE" == "1" ]]; then
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] 将删除系统用户/组 relaygate"
  else
    if getent passwd relaygate >/dev/null 2>&1; then
      userdel relaygate 2>/dev/null || die "无法删除用户 relaygate"
    fi
    if getent group relaygate >/dev/null 2>&1; then
      groupdel relaygate 2>/dev/null || true
    fi
  fi
  log "已 purge 用户/组 relaygate（配置与密钥未删；用 install.sh --uninstall --purge）"
else
  log "已移除 unit/helper/sudoers；保留用户 relaygate 与配置/密钥"
fi
