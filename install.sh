#!/usr/bin/env bash
# RelayGate bootstrap installer — release-tar based (no default docker build / git clone).
#
# Default: download prebuilt release tar → extract → relaygate setup/apply/panel/smoke
# Escape hatch: FROM_SOURCE=1 或 RELAYGATE_SOURCE_DIR（开发用，非默认）
set -Eeuo pipefail
umask 077

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ACTION=install
DRY_RUN=0
PURGE=0
NONINTERACTIVE="${NONINTERACTIVE:-0}"
FROM_SOURCE="${FROM_SOURCE:-0}"
INSTALL_DIR="${RELAYGATE_INSTALL_DIR:-/opt/relaygate}"
VERSION="${RELAYGATE_VERSION:-}"
TAR_PATH="${RELAYGATE_TAR:-}"
REPO_SLUG="${RELAYGATE_REPO_SLUG:-relaygate/relaygate}"
REPO_URL="${RELAYGATE_REPO_URL:-https://github.com/${REPO_SLUG}.git}"
RELEASES_BASE="${RELAYGATE_RELEASES_BASE:-https://github.com/${REPO_SLUG}/releases/download}"
SECRETS_DIR="${RELAYGATE_SECRETS_DIR:-/etc/relaygate/secrets}"
APPLY_FIREWALL="${APPLY_FIREWALL:-0}"
SOURCE_DIR="${RELAYGATE_SOURCE_DIR:-}"
TMP_DIR=""
LOG_FILE="${RELAYGATE_INSTALL_LOG:-/var/log/relaygate-install.log}"
START_TS="$(date +%s)"
ARCH=""
OS_FAMILY=""

_ts() { date +"%Y-%m-%d %H:%M:%S"; }
_log_line() {
  local color="$1" level="$2" msg="$3"
  local line="[RelayGate $(_ts) ${level}]: ${msg}"
  if [[ -t 1 ]] || [[ "${FORCE_COLOR:-0}" == "1" ]]; then
    echo -e "${color}${line}${NC}"
  else
    echo "${line}"
  fi
  if [[ -n "${LOG_FILE:-}" ]] && mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null; then
    echo "${line}" >>"$LOG_FILE" 2>/dev/null || true
  fi
}
log()  { _log_line "$BLUE"   "INFO"    "$*"; }
ok()   { _log_line "$GREEN"  "SUCCESS" "$*"; }
warn() { _log_line "$YELLOW" "WARN"    "$*" >&2; }
die()  {
  _log_line "$RED" "ERROR" "$*" >&2
  _log_line "$YELLOW" "HINT" "日志: ${LOG_FILE:-"(未写入)"}" >&2
  exit 1
}
run() {
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] $(printf '%q ' "$@")"
  else
    "$@"
  fi
}
cleanup() { [[ -z "${TMP_DIR:-}" || ! -d "$TMP_DIR" ]] || rm -rf "$TMP_DIR"; }
trap cleanup EXIT
trap 'die "安装在第 ${LINENO} 行失败（action=${ACTION}）。"' ERR

usage() {
  cat <<'EOF'
RelayGate 安装器（bootstrap · 预编译 release tar）

用法:
  install.sh [--install|--upgrade|--uninstall] [--purge] [--dry-run] [-y] [-h]

默认流程:
  检测 root/OS/arch/systemd/Docker
  → 下载并校验 release tar（或 RELAYGATE_TAR）
  → 解压到 /opt/relaygate（保留 .env / data/）
  → relaygate setup → apply → panel install → smoke

环境变量:
  RELAYGATE_VERSION=<tag|sha>     # 推荐不可变版本（GitHub Release tag）
  RELAYGATE_TAR=/path/to.tar.gz   # 本地包，跳过下载
  RELAYGATE_INSTALL_DIR=/opt/relaygate
  FROM_SOURCE=1                   # 开发兜底：源码构建（非默认）
  RELAYGATE_SOURCE_DIR=/path/src  # 配合 FROM_SOURCE
  GATEWAY_NAME / GATEWAY_PUBLIC_IP / GATEWAY_SSH_PORT
  ENABLE_PANEL=1 ENABLE_GRAFANA=1 APPLY_FIREWALL=0
  NONINTERACTIVE=1
EOF
}

is_true() {
  case "${1:-}" in 1|y|Y|yes|YES|true|TRUE|True) return 0 ;; *) return 1 ;; esac
}

is_floating_version() {
  case "${1,,}" in ""|master|main|latest) return 0 ;; *) return 1 ;; esac
}

Check_Root() {
  if [[ "$DRY_RUN" == "1" ]]; then
    [[ "$(id -u)" == "0" ]] || warn "dry-run 以非 root 运行；真实安装需要 root"
    return 0
  fi
  [[ "$(id -u)" == "0" ]] || die "请以 root 运行（或 --dry-run）"
}

Prepare_System() {
  log "阶段 1/5 · 核心检测（root / OS / arch / systemd）"
  [[ "$(uname -s)" == "Linux" ]] || die "仅支持 Linux"
  [[ -r /etc/os-release ]] || die "缺少 /etc/os-release"
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_ID="${ID,,}"
  OS_LIKE="${ID_LIKE:-}"
  case "$OS_ID" in
    ubuntu|debian) OS_FAMILY=deb ;;
    rhel|rocky|almalinux|centos|fedora|amzn) OS_FAMILY=rpm ;;
    *)
      if [[ "$OS_LIKE" == *debian* ]]; then OS_FAMILY=deb
      elif [[ "$OS_LIKE" == *rhel* || "$OS_LIKE" == *fedora* || "$OS_LIKE" == *centos* ]]; then OS_FAMILY=rpm
      else die "不支持的发行版: ${PRETTY_NAME:-$OS_ID}"
      fi
      ;;
  esac
  command -v systemctl >/dev/null 2>&1 || die "需要 systemd"
  case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "不支持的架构: $(uname -m)" ;;
  esac
  [[ "$INSTALL_DIR" == /* && "$INSTALL_DIR" != "/" ]] || die "RELAYGATE_INSTALL_DIR 无效"
  if [[ "$ACTION" == "install" && -f "$INSTALL_DIR/.env" ]]; then
    die "已存在安装（${INSTALL_DIR}/.env）；请用 --upgrade"
  fi
  if [[ "$ACTION" == "upgrade" && ! -f "$INSTALL_DIR/.env" ]]; then
    die "未检测到安装；请用 --install"
  fi
  if [[ "$ACTION" == "upgrade" && -f "$INSTALL_DIR/.env" ]]; then
    # shellcheck disable=SC1091
    set -a; source "$INSTALL_DIR/.env"; set +a
    SECRETS_DIR="${RELAYGATE_SECRETS_DIR:-$SECRETS_DIR}"
  fi
  ENABLE_PANEL="${ENABLE_PANEL:-1}"
  ENABLE_GRAFANA="${ENABLE_GRAFANA:-1}"
  ok "OS=${PRETTY_NAME:-$OS_ID} arch=${ARCH}"
}

Install_Packages() {
  log "阶段 2/5 · 基础包 + Docker"
  local pkgs=(ca-certificates curl openssl nftables)
  [[ "${ENABLE_PANEL}" == "1" ]] && pkgs+=(sudo)
  is_true "$FROM_SOURCE" && [[ -z "$TAR_PATH" ]] && pkgs+=(git)
  if [[ "$OS_FAMILY" == "deb" ]]; then
    pkgs+=(iproute2)
    run apt-get update
    run env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${pkgs[@]}"
  else
    local pm=dnf
    command -v dnf >/dev/null 2>&1 || pm=yum
    pkgs+=(iproute)
    run "$pm" install -y "${pkgs[@]}"
  fi

  if ! command -v docker >/dev/null 2>&1 || ! docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
    log "安装 Docker CE"
    if [[ "$OS_FAMILY" == "deb" ]]; then
      run install -m 0755 -d /etc/apt/keyrings
      if [[ "$DRY_RUN" != "1" ]]; then
        case "$OS_ID" in ubuntu|debian) ;; *) die "deb 系仅支持 Ubuntu/Debian Docker 源" ;; esac
        curl -fsSL "https://download.docker.com/linux/${OS_ID}/gpg" -o /etc/apt/keyrings/docker.asc
        chmod a+r /etc/apt/keyrings/docker.asc
        # shellcheck disable=SC1091
        . /etc/os-release
        printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/%s %s stable\n' \
          "$(dpkg --print-architecture)" "$OS_ID" "$VERSION_CODENAME" >/etc/apt/sources.list.d/docker.list
      fi
      run apt-get update
      run env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    else
      local pm=dnf repo_os="$OS_ID"
      command -v dnf >/dev/null 2>&1 || pm=yum
      case "$OS_ID" in rocky|almalinux|centos|amzn) repo_os=centos ;; rhel) repo_os=rhel ;; fedora) repo_os=fedora ;; esac
      if [[ "$pm" == "dnf" ]]; then
        run dnf install -y dnf-plugins-core
        run dnf config-manager --add-repo "https://download.docker.com/linux/${repo_os}/docker-ce.repo"
        run dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
      else
        run yum install -y yum-utils
        run yum-config-manager --add-repo "https://download.docker.com/linux/${repo_os}/docker-ce.repo"
        run yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
      fi
    fi
  fi
  run systemctl enable --now docker
  if [[ "$DRY_RUN" != "1" ]]; then
    docker compose version >/dev/null 2>&1 || die "需要 Docker Compose v2"
  fi
  ok "Docker 可用"
}

resolve_version() {
  if [[ -n "$TAR_PATH" ]]; then
    [[ -f "$TAR_PATH" ]] || die "RELAYGATE_TAR 不存在: $TAR_PATH"
    VERSION="${VERSION:-local}"
    return 0
  fi
  if [[ -z "$VERSION" ]]; then
    VERSION="$(curl -fsSL --max-time 15 "https://raw.githubusercontent.com/${REPO_SLUG}/master/RELEASE" 2>/dev/null | tr -d '[:space:]' || true)"
  fi
  if is_floating_version "$VERSION"; then
    if is_true "$NONINTERACTIVE"; then
      die "请设置 RELAYGATE_VERSION=<tag>（当前: ${VERSION:-empty}）"
    fi
    [[ -r /dev/tty ]] || die "请设置 RELAYGATE_VERSION"
    read -r -p "RELAYGATE_VERSION（GitHub Release tag）: " VERSION </dev/tty
    is_floating_version "$VERSION" && die "仍是浮动版本"
  fi
}

Acquire_Release() {
  log "阶段 3/5 · 获取 release 包"
  TMP_DIR="$(mktemp -d)"
  resolve_version

  if is_true "$FROM_SOURCE" || [[ -n "$SOURCE_DIR" ]]; then
    Acquire_From_Source
    return 0
  fi

  local tarfile checksum_file
  if [[ -n "$TAR_PATH" ]]; then
    tarfile="$TAR_PATH"
    ok "使用本地包: $tarfile"
  else
    local name="relaygate-${VERSION}-linux-${ARCH}.tar.gz"
    local url="${RELEASES_BASE}/${VERSION}/${name}"
    tarfile="${TMP_DIR}/${name}"
    checksum_file="${tarfile}.sha256"
    log "下载 ${url}"
    if [[ "$DRY_RUN" == "1" ]]; then
      log "[dry-run] curl -fL $url"
      PACKAGE_ROOT="${TMP_DIR}/pkg"
      return 0
    fi
    curl -fL --retry 3 --connect-timeout 20 -o "$tarfile" "$url" ||
      die "下载失败: $url（检查 RELAYGATE_VERSION / Release 是否含 ${ARCH} 包）"
    if curl -fsSL -o "$checksum_file" "${url}.sha256" 2>/dev/null; then
      log "校验 sha256"
      (cd "$(dirname "$tarfile")" && sha256sum -c "$(basename "$checksum_file")") || die "checksum 校验失败"
    else
      warn "未找到 ${name}.sha256，跳过校验"
    fi
  fi

  if [[ "$DRY_RUN" == "1" ]]; then
    PACKAGE_ROOT="${TMP_DIR}/pkg"
    return 0
  fi
  mkdir -p "${TMP_DIR}/extract"
  tar -xzf "$tarfile" -C "${TMP_DIR}/extract"
  # tarball 内一层目录 relaygate-VERSION-linux-ARCH/
  PACKAGE_ROOT="$(find "${TMP_DIR}/extract" -mindepth 1 -maxdepth 1 -type d | head -1)"
  [[ -x "${PACKAGE_ROOT}/bin/relaygate" ]] || die "包内缺少 bin/relaygate"
  [[ -d "${PACKAGE_ROOT}/frontend" && -d "${PACKAGE_ROOT}/packaging" ]] || die "包结构不完整"
  ok "release 就绪: ${VERSION} (${ARCH})"
}

Acquire_From_Source() {
  warn "FROM_SOURCE=1：开发兜底路径（默认应使用 release tar）"
  if [[ -n "$SOURCE_DIR" ]]; then
    [[ -f "$SOURCE_DIR/go.mod" && -f "$SOURCE_DIR/packaging/compose.yaml" ]] ||
      die "RELAYGATE_SOURCE_DIR 无效: $SOURCE_DIR"
    PACKAGE_ROOT="$SOURCE_DIR"
  else
    resolve_version
    git -c advice.detachedHead=false init -q "${TMP_DIR}/source"
    git -C "${TMP_DIR}/source" remote add origin "$REPO_URL"
    git -C "${TMP_DIR}/source" fetch -q --depth 1 origin "$VERSION" || die "git fetch 失败: $VERSION"
    git -C "${TMP_DIR}/source" checkout -q --detach FETCH_HEAD
    PACKAGE_ROOT="${TMP_DIR}/source"
  fi
  if [[ ! -x "${PACKAGE_ROOT}/bin/relaygate" ]]; then
    log "源码构建 bin/relaygate（Docker）"
    [[ "$DRY_RUN" == "1" ]] && return 0
    docker build -f "${PACKAGE_ROOT}/Dockerfile.build" -t "relaygate-build:local" "$PACKAGE_ROOT"
    local c
    c="$(docker create relaygate-build:local)"
    mkdir -p "${PACKAGE_ROOT}/bin"
    docker cp "$c:/usr/local/bin/relaygate" "${PACKAGE_ROOT}/bin/relaygate"
    docker rm "$c" >/dev/null
    chmod 755 "${PACKAGE_ROOT}/bin/relaygate"
  fi
  ok "源码产品就绪"
}

Place_Product() {
  log "阶段 4/5 · 安装到 ${INSTALL_DIR}"
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] 解压/同步到 ${INSTALL_DIR}（保留 .env 与 data/）"
    return 0
  fi

  mkdir -p "$INSTALL_DIR"
  local saved_env="" saved_data=""
  if [[ -f "$INSTALL_DIR/.env" ]]; then
    saved_env="$(mktemp)"
    cp -a "$INSTALL_DIR/.env" "$saved_env"
  fi
  if [[ -d "$INSTALL_DIR/data" ]]; then
    saved_data="$(mktemp -d)"
    cp -a "$INSTALL_DIR/data/." "$saved_data/"
  fi

  if [[ -d "$INSTALL_DIR" && -f "$INSTALL_DIR/bin/relaygate" ]]; then
    local stamp backup
    stamp="$(date +%Y%m%d-%H%M%S)"
    backup="$INSTALL_DIR/data/backups/installer-$stamp"
    mkdir -p "$backup"
    [[ -f "$INSTALL_DIR/.env" ]] && cp -a "$INSTALL_DIR/.env" "$backup/.env" || true
    [[ -f "$INSTALL_DIR/data/resources.yaml" ]] && cp -a "$INSTALL_DIR/data/resources.yaml" "$backup/resources.yaml" || true
    [[ -f "$INSTALL_DIR/data/envoy/envoy.yaml" ]] && cp -a "$INSTALL_DIR/data/envoy/envoy.yaml" "$backup/envoy.yaml" || true
    printf '%s\n' "$backup" >"$INSTALL_DIR/data/backups/installer-latest"
  fi

  # 同步产品文件；不覆盖运行态由下方 restore 处理
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete \
      --exclude '.env' \
      --exclude 'data/' \
      --exclude '.runtime/' \
      --exclude 'core/' \
      --exclude '.git/' \
      --exclude 'dist/' \
      --exclude 'bin/' \
      "${PACKAGE_ROOT}/" "${INSTALL_DIR}/"
    mkdir -p "$INSTALL_DIR/bin"
    cp -a "${PACKAGE_ROOT}/bin/relaygate" "$INSTALL_DIR/bin/relaygate"
  else
    # 无 rsync：逐项覆盖产品目录（安装前缀含 packaging/ + data/ 运行态骨架）
    mkdir -p "$INSTALL_DIR/bin" "$INSTALL_DIR/frontend"
    cp -a "${PACKAGE_ROOT}/bin/relaygate" "$INSTALL_DIR/bin/relaygate"
    rm -rf "$INSTALL_DIR/frontend" "$INSTALL_DIR/packaging"
    cp -a "${PACKAGE_ROOT}/frontend" "$INSTALL_DIR/"
    cp -a "${PACKAGE_ROOT}/packaging" "$INSTALL_DIR/"
    for f in .env.example resources.example.yaml gateway-01.env.example gateway-02.env.example \
      gateways.env.example install.sh RELEASE go.mod; do
      [[ -e "${PACKAGE_ROOT}/$f" ]] && cp -a "${PACKAGE_ROOT}/$f" "$INSTALL_DIR/$f"
    done
  fi

  mkdir -p "$INSTALL_DIR/data"/{envoy,firewall,prometheus,backups,inventory}
  # Grafana 持久化用 compose 命名卷；密钥在 SECRETS_DIR（setup 生成）
  mkdir -p "$SECRETS_DIR"
  chmod 750 "$SECRETS_DIR" 2>/dev/null || true
  [[ -n "$saved_env" ]] && cp -a "$saved_env" "$INSTALL_DIR/.env" && rm -f "$saved_env"
  if [[ -n "$saved_data" ]]; then
    cp -a "$saved_data/." "$INSTALL_DIR/data/"
    rm -rf "$saved_data"
  fi
  chmod 755 "$INSTALL_DIR/bin/relaygate"
  [[ -f "$INSTALL_DIR/install.sh" ]] || cp -a "$0" "$INSTALL_DIR/install.sh"
  [[ -x "$INSTALL_DIR/bin/relaygate" ]] || die "安装后缺少可执行 bin/relaygate"
  [[ -d "$INSTALL_DIR/frontend" && -d "$INSTALL_DIR/packaging" ]] || die "安装后产品树不完整"
  ok "产品已就位"
}

Invoke_Product() {
  log "阶段 5/5 · 运行产品 CLI"
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] setup → apply → panel install → smoke"
    return 0
  fi
  cd "$INSTALL_DIR"
  export RELAYGATE_INSTALL_DIR="$INSTALL_DIR" RELAYGATE_SECRETS_DIR="$SECRETS_DIR"
  export RELAYGATE_DATA_DIR="${RELAYGATE_DATA_DIR:-$INSTALL_DIR/data}"
  export NONINTERACTIVE ENABLE_PANEL ENABLE_GRAFANA
  export GATEWAY_NAME="${GATEWAY_NAME:-}" GATEWAY_PUBLIC_IP="${GATEWAY_PUBLIC_IP:-}" GATEWAY_SSH_PORT="${GATEWAY_SSH_PORT:-}"
  export APPLY_FIREWALL FIREWALL_CONFIRM="${FIREWALL_CONFIRM:-}"

  local setup_flags=(--noninteractive --sysctl)
  [[ "$ACTION" == "upgrade" ]] && setup_flags+=(--upgrade)

  log "→ relaygate setup ${setup_flags[*]}"
  ./bin/relaygate setup "${setup_flags[@]}" || die "setup 失败"

  log "→ relaygate apply"
  ./bin/relaygate apply || die "apply 失败；可试: ./bin/relaygate rollback"

  # shellcheck disable=SC1091
  set -a; source .env; set +a
  ENABLE_PANEL="${ENABLE_PANEL:-1}"
  if [[ "$ENABLE_PANEL" == "1" ]]; then
    local grafana_url=""
    if [[ "${ENABLE_GRAFANA:-1}" == "1" || ",${COMPOSE_PROFILES:-}," == *",with-grafana,"* ]]; then
      grafana_url="http://127.0.0.1:3000"
    fi
    log "→ relaygate panel install"
    GRAFANA_URL="$grafana_url" ENABLE_NOW=1 ./bin/relaygate panel install || die "panel install 失败"
  else
    PURGE=0 ./bin/relaygate panel uninstall || true
  fi

  log "→ relaygate smoke"
  ./bin/relaygate smoke || die "smoke 失败"

  if [[ "$APPLY_FIREWALL" == "1" ]]; then
    log "→ relaygate firewall apply"
    SSH_PORT="${GATEWAY_SSH_PORT:-30455}" APPLY_FIREWALL=1 \
      FIREWALL_CONFIRM="${FIREWALL_CONFIRM:-}" ./bin/relaygate firewall apply ||
      die "firewall apply 失败"
  else
    log "→ relaygate firewall check"
    SSH_PORT="${GATEWAY_SSH_PORT:-30455}" ./bin/relaygate firewall check || true
  fi
  ok "编排完成"
}

Show_Result() {
  local elapsed=$(( $(date +%s) - START_TS ))
  echo
  ok "RelayGate ${ACTION} 成功（${elapsed}s） version=${VERSION:-?} arch=${ARCH}"
  log "目录: ${INSTALL_DIR}  密钥: ${SECRETS_DIR}"
  log "运维: cd ${INSTALL_DIR} && ./bin/relaygate reload|smoke|doctor"
  if [[ "${ENABLE_PANEL:-1}" == "1" ]]; then
    log "Panel: ssh -p ${GATEWAY_SSH_PORT:-30455} -L 9000:127.0.0.1:9000 root@${GATEWAY_PUBLIC_IP:-<IP>}"
  fi
}

Uninstall_RelayGate() {
  log "卸载 RelayGate（purge=${PURGE}）"
  [[ -d "$INSTALL_DIR" ]] || die "未发现: $INSTALL_DIR"
  [[ "$DRY_RUN" == "1" ]] && { log "[dry-run] uninstall"; return 0; }
  cd "$INSTALL_DIR"
  if [[ -x ./bin/relaygate ]]; then
    PURGE="$PURGE" ./bin/relaygate panel uninstall || true
  else
    systemctl disable --now relaygate-panel.service 2>/dev/null || true
    rm -f /etc/systemd/system/relaygate-panel.service /etc/sudoers.d/relaygate-panel \
      /usr/local/libexec/relaygate/apply /etc/relaygate/panel.env
    systemctl daemon-reload 2>/dev/null || true
  fi
  if [[ -f .env ]]; then
    # shellcheck disable=SC1091
    set -a; source .env; set +a
    docker compose -f packaging/compose.yaml --env-file .env down || true
  fi
  rm -f "/etc/sysctl.d/99-${GATEWAY_NAME:-gateway-01}.conf"
  [[ "$PURGE" == "1" ]] || { ok "已停止；保留配置。彻底清除加 PURGE=1 PURGE_CONFIRM=DELETE_RELAYGATE_DATA"; return 0; }
  if is_true "$NONINTERACTIVE"; then
    [[ "${PURGE_CONFIRM:-}" == "DELETE_RELAYGATE_DATA" ]] || die "需要 PURGE_CONFIRM=DELETE_RELAYGATE_DATA"
  else
    local answer
    read -r -p "输入 DELETE_RELAYGATE_DATA 确认清除: " answer </dev/tty
    [[ "$answer" == "DELETE_RELAYGATE_DATA" ]] || die "已取消"
  fi
  docker compose -f packaging/compose.yaml --env-file .env down -v --remove-orphans 2>/dev/null || true
  getent passwd relaygate >/dev/null 2>&1 && userdel relaygate 2>/dev/null || true
  getent group relaygate >/dev/null 2>&1 && groupdel relaygate 2>/dev/null || true
  rm -rf "$SECRETS_DIR" "$INSTALL_DIR" /etc/relaygate
  ok "已彻底清除"
}

main() {
  while (($#)); do
    case "$1" in
      --install) ACTION=install ;;
      --upgrade) ACTION=upgrade ;;
      --uninstall) ACTION=uninstall ;;
      --purge) PURGE=1 ;;
      --dry-run) DRY_RUN=1 ;;
      -y|--non-interactive) NONINTERACTIVE=1 ;;
      -h|--help) usage; exit 0 ;;
      *) die "未知参数: $1" ;;
    esac
    shift
  done

  if [[ -z "${RELAYGATE_INSTALL_LOG:-}" ]]; then
    if [[ "$DRY_RUN" != "1" && "$(id -u)" == "0" ]]; then
      LOG_FILE="/var/log/relaygate-install.log"
    else
      LOG_FILE="${TMPDIR:-/tmp}/relaygate-install-$$.log"
    fi
  fi
  mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null || true

  log "bootstrap 开始 action=${ACTION} from_source=${FROM_SOURCE}"
  Check_Root
  if [[ "$ACTION" == "uninstall" ]]; then
    Uninstall_RelayGate
    return 0
  fi
  Prepare_System
  Install_Packages
  Acquire_Release
  Place_Product
  Invoke_Product
  Show_Result
}

main "$@"
