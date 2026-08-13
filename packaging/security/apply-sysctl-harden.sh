#!/usr/bin/env bash
# 安装可选 TCP / SYN Flood 相关 sysctl 加固（叠加 overlay，不覆盖 packaging/sysctl/gateway.conf）。
# 针对 half-open / SYN cookies / backlog；不影响已建立（established）长连接。
#
# 用法:
#   ./packaging/security/apply-sysctl-harden.sh           # 预览（优先从 resources.yaml 渲染）
#   sudo ./packaging/security/apply-sysctl-harden.sh --apply
#   sudo ./packaging/security/apply-sysctl-harden.sh --apply --verify
#   sudo RELAYGATE_CONFIRM=Confirm ./packaging/security/apply-sysctl-harden.sh --apply
#
# 推荐：relaygate security apply-kernel --verify（与 agent 拉取后落地同源）
#
# 环境变量:
#   RELAYGATE_CONFIRM  设为 Confirm（或交互输入「确认」/Confirm）后才真正写入
#   RELAYGATE_BIN      relaygate 可执行文件路径（默认 relaygate）
#   DEST               目标路径（默认 /etc/sysctl.d/99-relaygate-tcp-harden.conf）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATIC_SRC="${SCRIPT_DIR}/sysctl-tcp-harden.conf"
DEST="${DEST:-/etc/sysctl.d/99-relaygate-tcp-harden.conf}"
RELAYGATE_BIN="${RELAYGATE_BIN:-relaygate}"
APPLY=0
VERIFY=0
TMP_SRC=""

usage() {
  sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

for arg in "$@"; do
  case "$arg" in
    -h|--help) usage 0 ;;
    --apply) APPLY=1 ;;
    --verify) VERIFY=1 ;;
    *) echo "未知参数: $arg" >&2; usage 1 ;;
  esac
done

resolve_src() {
  if command -v "$RELAYGATE_BIN" >/dev/null 2>&1; then
    TMP_SRC="$(mktemp)"
    if "$RELAYGATE_BIN" security kernel-conf >"$TMP_SRC" 2>/dev/null && [[ -s "$TMP_SRC" ]]; then
      echo "$TMP_SRC"
      return 0
    fi
    rm -f "$TMP_SRC"
    TMP_SRC=""
  fi
  if [[ -f "$STATIC_SRC" ]]; then
    echo "$STATIC_SRC"
    return 0
  fi
  return 1
}

SRC="$(resolve_src)" || {
  echo "缺少 sysctl 源：relaygate security kernel-conf 或 $STATIC_SRC" >&2
  exit 1
}

cleanup() {
  if [[ -n "$TMP_SRC" && -f "$TMP_SRC" ]]; then
    rm -f "$TMP_SRC"
  fi
}
trap cleanup EXIT

echo "==> 将安装 sysctl 叠加（SYN cookies / backlog；不影响 gateway.conf 已有键）"
if [[ "$SRC" == "$STATIC_SRC" ]]; then
  echo "    源: $SRC（静态模板）"
else
  echo "    源: resources.yaml（relaygate security kernel-conf）"
fi
echo "    目标: $DEST"
echo "----"
grep -v '^#' "$SRC" | grep -v '^$' || true
echo "----"

if [[ "$APPLY" -ne 1 ]]; then
  echo "预览模式。写入请加 --apply（需 root，并确认）。"
  exit 0
fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "需要 root：sudo $0 --apply" >&2
  exit 1
fi

confirm="${RELAYGATE_CONFIRM:-}"
if [[ "$confirm" != "Confirm" && "$confirm" != "确认" ]]; then
  read -r -p "将写入内核参数文件并执行 sysctl --system。请输入「确认」或 Confirm: " confirm
fi
if [[ "$confirm" != "Confirm" && "$confirm" != "确认" ]]; then
  echo "已取消（确认词不匹配）。" >&2
  exit 1
fi

install -m 0644 "$SRC" "$DEST"
sysctl --system >/dev/null
echo "已应用: $DEST"
sysctl net.ipv4.tcp_syncookies net.ipv4.tcp_max_syn_backlog \
  net.ipv4.tcp_synack_retries net.ipv4.tcp_syn_retries \
  net.ipv4.tcp_abort_on_overflow 2>/dev/null || true

if [[ "$VERIFY" -eq 1 ]]; then
  if command -v "$RELAYGATE_BIN" >/dev/null 2>&1; then
    "$RELAYGATE_BIN" security verify || {
      echo "sysctl/nftables/Envoy 校验未全部通过（见上方输出）" >&2
      exit 1
    }
  else
    echo "WARN: 未找到 $RELAYGATE_BIN，跳过 --verify" >&2
  fi
fi
