#!/usr/bin/env bash
# RelayGate bootstrap installer — release-tar based (no default docker build / git clone).
#
# Default: download prebuilt release tar → extract → relaygate setup/apply/panel/smoke
# Escape hatch: FROM_SOURCE=1 / RELAYGATE_SOURCE_DIR；私有仓无 Release 时可用 RELAYGATE_GIT_FALLBACK=1
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
GIT_FALLBACK="${RELAYGATE_GIT_FALLBACK:-0}"
INSTALL_DIR="${RELAYGATE_INSTALL_DIR:-/opt/relaygate}"
VERSION="${RELAYGATE_VERSION:-}"
TAR_PATH="${RELAYGATE_TAR:-}"
REPO_SLUG="${RELAYGATE_REPO_SLUG:-relaygate/relaygate}"
GITHUB_TOKEN_EFFECTIVE="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
# 默认 HTTPS（公开仓无需密钥）；私有仓可带 token。SSH 请显式设 RELAYGATE_REPO_URL。
if [[ -n "${RELAYGATE_REPO_URL:-}" ]]; then
  REPO_URL="$RELAYGATE_REPO_URL"
elif [[ -n "$GITHUB_TOKEN_EFFECTIVE" ]]; then
  REPO_URL="https://x-access-token:${GITHUB_TOKEN_EFFECTIVE}@github.com/${REPO_SLUG}.git"
else
  REPO_URL="https://github.com/${REPO_SLUG}.git"
fi
RELEASES_BASE="${RELAYGATE_RELEASES_BASE:-https://github.com/${REPO_SLUG}/releases/download}"
API_BASE="${RELAYGATE_API_BASE:-https://api.github.com}"
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

一键示例（可复制）:
  # 1) 安装主控（Panel；首启默认密码 relaygate，生产务必改密）
  curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
    | sudo env ENABLE_PANEL=1 NONINTERACTIVE=1 bash -s -- -y

  # 2) 安装节点：优先用主控 fleet join / Panel「接入」生成的一行；
  #    或自行指定 PRIMARY_URL + GATEWAY_NAME + AGENT_TOKEN：
  curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
    | sudo env PRIMARY_URL='http://203.0.113.10:9000' GATEWAY_NAME=gateway-02 \
        AGENT_TOKEN='<token>' ENABLE_PANEL=0 NONINTERACTIVE=1 bash -s -- -y

  # 3) 升级主控 / 4) 升级节点（同一命令；读现有 .env 保留角色与 DataDir）
  curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
    | sudo bash -s -- --upgrade -y
  # 已安装本机也可:  sudo /opt/relaygate/bin/relaygate upgrade
  #                或 sudo bash /opt/relaygate/install.sh --upgrade -y

默认流程:
  检测 root/OS/arch/systemd/Docker
  → 下载并校验 release tar（或 RELAYGATE_TAR）
  → 解压到 /opt/relaygate（保留 .env / data/）
  → relaygate setup → apply → panel 或 agent install → smoke

环境变量:
  RELAYGATE_VERSION=<tag|sha|latest>  # 默认 latest（解析为最新 GitHub Release / git tag）
  RELAYGATE_TAR=/path/to.tar.gz       # 本地包，跳过下载
  RELAYGATE_INSTALL_DIR=/opt/relaygate
  GH_TOKEN / GITHUB_TOKEN             # 私有仓访问 Releases API / 下载资产
  FROM_SOURCE=1                       # 从 git 源码构建（含 UI）
  RELAYGATE_GIT_FALLBACK=1            # Release 不可用时自动回退到 FROM_SOURCE
  RELAYGATE_SOURCE_DIR=/path/src      # 配合 FROM_SOURCE（跳过 clone）
  RELAYGATE_REPO_URL                  # 默认 HTTPS；私有仓自动带 token
  GATEWAY_NAME / GATEWAY_PUBLIC_IP    # 公网 IP 非交互时可自动探测
  GATEWAY_SSH_PORT                    # 安装时指定，常见 22 或其他
  ENABLE_PANEL / ENABLE_GRAFANA / APPLY_FIREWALL
  PRIMARY_URL / AGENT_TOKEN           # 节点一句话接入：写令牌、装 agent、连主控
  NONINTERACTIVE=1
EOF
}

is_true() {
  case "${1:-}" in 1|y|Y|yes|YES|true|TRUE|True) return 0 ;; *) return 1 ;; esac
}

# master/main 禁止作为安装版本；空与 latest 由 resolve_latest_tag 解析。
is_floating_version() {
  case "${1,,}" in ""|master|main) return 0 ;; *) return 1 ;; esac
}

# 下载到文件并打印 HTTP code（不经命令替换丢失状态）。
github_http_get_file() {
  local url="$1" outfile="$2"
  local args=()
  if [[ -n "$GITHUB_TOKEN_EFFECTIVE" ]]; then
    args+=(-H "Authorization: Bearer ${GITHUB_TOKEN_EFFECTIVE}" -H "X-GitHub-Api-Version: 2022-11-28")
  fi
  args+=(-H "Accept: application/vnd.github+json" -H "User-Agent: relaygate-install")
  curl -sS -L --max-time 60 -o "$outfile" -w '%{http_code}' "${args[@]}" "$url" || echo "000"
}

github_download() {
  local url="$1" outfile="$2"
  local args=(-fL --retry 3 --connect-timeout 20 -o "$outfile")
  if [[ -n "$GITHUB_TOKEN_EFFECTIVE" ]]; then
    args+=(-H "Authorization: Bearer ${GITHUB_TOKEN_EFFECTIVE}")
  fi
  args+=(-H "User-Agent: relaygate-install" "$url")
  curl "${args[@]}"
}

# 通过 git ls-remote 取最新 v* tag（API/Release 不可用时的回退）。
resolve_latest_git_tag() {
  local remote="$REPO_URL" line tag="" best=""
  command -v git >/dev/null 2>&1 || return 1
  while IFS= read -r line; do
    tag="${line##*/}"
    [[ "$tag" == v* ]] || continue
    best="$tag"
  done < <(git ls-remote --tags --refs "$remote" 2>/dev/null | awk '{print $2}' | sed 's#refs/tags/##' | sort -V)
  [[ -n "$best" ]] || return 1
  printf '%s\n' "$best"
}

# 解析最新可用版本：优先 GitHub Release；失败则 git tag；并诊断私有仓/无 Release。
resolve_latest_tag() {
  local tag="" api_json="" code="" url msg bodyfile
  bodyfile="$(mktemp)"
  code="$(github_http_get_file "${API_BASE}/repos/${REPO_SLUG}/releases/latest" "$bodyfile")"
  api_json="$(cat "$bodyfile" 2>/dev/null || true)"
  rm -f "$bodyfile"
  if [[ "$code" == "200" && -n "$api_json" ]]; then
    tag="$(printf '%s' "$api_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  fi
  if [[ -z "$tag" ]]; then
    local curl_args=()
    [[ -n "$GITHUB_TOKEN_EFFECTIVE" ]] && curl_args+=(-H "Authorization: Bearer ${GITHUB_TOKEN_EFFECTIVE}")
    url="$(curl -sS --max-time 20 -o /dev/null -w '%{url_effective}' \
      "${curl_args[@]}" \
      "https://github.com/${REPO_SLUG}/releases/latest" 2>/dev/null || true)"
    if [[ "$url" == */releases/tag/* ]]; then
      tag="${url##*/releases/tag/}"
    fi
    [[ "${tag,,}" == "latest" ]] && tag=""
  fi
  if [[ -z "$tag" ]]; then
    if tag="$(resolve_latest_git_tag)"; then
      warn "GitHub Releases API 不可用（HTTP ${code:-?}），回退到 git tag: ${tag}"
      printf '%s\n' "$tag"
      return 0
    fi
  fi
  tag="$(printf '%s' "${tag:-}" | tr -d '[:space:]')"
  case "${tag,,}" in
    ""|latest|releases)
      msg="无法解析最新版本（repo=${REPO_SLUG}）。"
      case "${code}" in
        404)
          msg+=" GitHub API 返回 404：仓库不存在、未公开，或尚未创建 Release。"
          msg+=" 私有仓请设置 GH_TOKEN/GITHUB_TOKEN；或使用 FROM_SOURCE=1 / RELAYGATE_GIT_FALLBACK=1。"
          ;;
        401|403)
          msg+=" GitHub API 返回 ${code}：token 无效或权限不足（需要 contents:read）。"
          ;;
        ""|000)
          msg+=" 无法访问 ${API_BASE}；检查网络，或改用 FROM_SOURCE=1。"
          ;;
        *)
          msg+=" HTTP ${code}。见 https://github.com/${REPO_SLUG}/releases"
          ;;
      esac
      die "$msg"
      ;;
  esac
  printf '%s\n' "$tag"
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
  # /etc/os-release 会定义 VERSION=；必须先保存安装器的 Release 版本变量
  local _rg_version="$VERSION"
  # shellcheck disable=SC1091
  . /etc/os-release
  VERSION="$_rg_version"
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
    # .env 也可能含 VERSION=；恢复安装器版本意图
    VERSION="${RELAYGATE_VERSION:-$_rg_version}"
    SECRETS_DIR="${RELAYGATE_SECRETS_DIR:-$SECRETS_DIR}"
  fi
  # 角色：节点（AGENT_TOKEN / ENABLE_PANEL=0）默认不装 Panel / Grafana；主控默认都开
  if [[ -n "${AGENT_TOKEN:-}" ]]; then
    ENABLE_PANEL="${ENABLE_PANEL:-0}"
    ENABLE_GRAFANA="${ENABLE_GRAFANA:-0}"
    [[ -n "${PRIMARY_URL:-}" ]] || die "节点接入需要 PRIMARY_URL（主控地址）"
    [[ -n "${GATEWAY_NAME:-}" ]] || die "节点接入需要 GATEWAY_NAME"
  elif [[ "${ENABLE_PANEL:-}" == "0" ]]; then
    ENABLE_GRAFANA="${ENABLE_GRAFANA:-0}"
  else
    ENABLE_PANEL="${ENABLE_PANEL:-1}"
    ENABLE_GRAFANA="${ENABLE_GRAFANA:-1}"
  fi
  ok "OS=${PRETTY_NAME:-$OS_ID} arch=${ARCH} panel=${ENABLE_PANEL}"
}

Install_Packages() {
  log "阶段 2/5 · 基础包 + Docker"
  local pkgs=(ca-certificates curl openssl nftables)
  [[ "${ENABLE_PANEL}" == "1" ]] && pkgs+=(sudo)
  if { is_true "$FROM_SOURCE" || is_true "$GIT_FALLBACK"; } && [[ -z "$TAR_PATH" ]]; then
    pkgs+=(git)
  fi
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
  # 未指定、latest，或被 /etc/os-release 污染的非 tag 值 → 解析最新 Release
  local ver_ok=0
  if [[ -n "$VERSION" && "${VERSION,,}" != "latest" ]]; then
    if [[ "$VERSION" =~ ^v[0-9] ]] || [[ "$VERSION" =~ ^[0-9a-f]{7,40}$ ]]; then
      ver_ok=1
    fi
  fi
  if [[ "$ver_ok" != "1" ]]; then
    if [[ -n "$VERSION" && "${VERSION,,}" != "latest" ]]; then
      warn "忽略无效 VERSION='${VERSION}'（常见原因：source /etc/os-release 覆盖），改为解析最新 Release"
    fi
    log "解析最新 GitHub Release tag…"
    VERSION="$(resolve_latest_tag)"
    ok "使用 Release: ${VERSION}"
  fi
  if is_floating_version "$VERSION"; then
    die "RELAYGATE_VERSION 不能为 master/main（当前: ${VERSION}）；请省略以用最新 tag，或指定具体 Release tag"
  fi
}

ensure_ui_dist() {
  local root="$1"
  [[ -d "${root}/ui/dist" && -f "${root}/ui/dist/index.html" ]] && return 0
  log "构建 Panel UI（ui/dist 缺失）"
  [[ "$DRY_RUN" == "1" ]] && return 0
  if [[ -f "${root}/ui/package.json" ]]; then
    if command -v npm >/dev/null 2>&1; then
      (cd "${root}/ui" && npm ci && npm run build) || die "npm 构建 UI 失败"
    else
      docker run --rm -v "${root}:/src" -w /src/ui node:20-bookworm \
        bash -lc "npm ci && npm run build" || die "Docker 构建 UI 失败"
    fi
  fi
  [[ -d "${root}/ui/dist" && -f "${root}/ui/dist/index.html" ]] ||
    die "缺少 ui/dist；Release 包应自带，或 FROM_SOURCE 需能构建 UI"
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
    if ! github_download "$url" "$tarfile"; then
      if is_true "$GIT_FALLBACK"; then
        warn "Release 下载失败，RELAYGATE_GIT_FALLBACK=1 → 改为 FROM_SOURCE"
        FROM_SOURCE=1
        Acquire_From_Source
        return 0
      fi
      die "下载失败: $url（检查 RELAYGATE_VERSION / Release 资产 / 仓库可见性；私有仓设 GH_TOKEN；或 RELAYGATE_GIT_FALLBACK=1）"
    fi
    if github_download "${url}.sha256" "$checksum_file" 2>/dev/null; then
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
  [[ -d "${PACKAGE_ROOT}/ui/dist" && -d "${PACKAGE_ROOT}/packaging" ]] || die "包结构不完整"
  ok "release 就绪: ${VERSION} (${ARCH})"
}

Acquire_From_Source() {
  warn "FROM_SOURCE=1：从 git 构建（默认应使用 release tar）"
  if [[ -n "$SOURCE_DIR" ]]; then
    [[ -f "$SOURCE_DIR/go.mod" && -f "$SOURCE_DIR/packaging/compose.yaml" ]] ||
      die "RELAYGATE_SOURCE_DIR 无效: $SOURCE_DIR"
    PACKAGE_ROOT="$SOURCE_DIR"
  else
    resolve_version
    command -v git >/dev/null 2>&1 || die "FROM_SOURCE 需要 git"
    git -c advice.detachedHead=false init -q "${TMP_DIR}/source"
    git -C "${TMP_DIR}/source" remote add origin "$REPO_URL"
    git -C "${TMP_DIR}/source" fetch -q --depth 1 origin "refs/tags/${VERSION}:refs/tags/${VERSION}" \
      || git -C "${TMP_DIR}/source" fetch -q --depth 1 origin "$VERSION" \
      || die "git fetch 失败: ${VERSION}（remote=${REPO_URL}）"
    git -C "${TMP_DIR}/source" checkout -q --detach "refs/tags/${VERSION}" 2>/dev/null \
      || git -C "${TMP_DIR}/source" checkout -q --detach FETCH_HEAD
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
  ensure_ui_dist "$PACKAGE_ROOT"
  [[ -d "${PACKAGE_ROOT}/packaging" ]] || die "源码树缺少 packaging/"
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
      --exclude '/dist/' \
      --exclude 'bin/' \
      "${PACKAGE_ROOT}/" "${INSTALL_DIR}/"
    mkdir -p "$INSTALL_DIR/bin"
    cp -a "${PACKAGE_ROOT}/bin/relaygate" "$INSTALL_DIR/bin/relaygate"
  else
    # 无 rsync：逐项覆盖产品目录（安装前缀含 packaging/ + data/ 运行态骨架）
    mkdir -p "$INSTALL_DIR/bin" "$INSTALL_DIR/ui"
    cp -a "${PACKAGE_ROOT}/bin/relaygate" "$INSTALL_DIR/bin/relaygate"
    rm -rf "$INSTALL_DIR/ui" "$INSTALL_DIR/packaging"
    mkdir -p "$INSTALL_DIR/ui"
    cp -a "${PACKAGE_ROOT}/ui/dist" "$INSTALL_DIR/ui/"
    cp -a "${PACKAGE_ROOT}/packaging" "$INSTALL_DIR/"
    for f in install.sh RELEASE go.mod; do
      [[ -e "${PACKAGE_ROOT}/$f" ]] && cp -a "${PACKAGE_ROOT}/$f" "$INSTALL_DIR/$f"
    done
  fi

  mkdir -p "$INSTALL_DIR/data"/{envoy,firewall,prometheus,backups,inventory}
  mkdir -p "$INSTALL_DIR/data/envoy/logs"
  # umask 077 下 mkdir 会变成 0700；Envoy 容器需读 yaml、写 logs
  chmod 0755 "$INSTALL_DIR/data" "$INSTALL_DIR/data/envoy" \
    "$INSTALL_DIR/data/firewall" "$INSTALL_DIR/data/prometheus" \
    "$INSTALL_DIR/data/backups" "$INSTALL_DIR/data/inventory" 2>/dev/null || true
  chmod 0777 "$INSTALL_DIR/data/envoy/logs" 2>/dev/null || true
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
  [[ -d "$INSTALL_DIR/ui/dist" && -d "$INSTALL_DIR/packaging" ]] || die "安装后产品树不完整"
  ok "产品已就位"
}

# 将 KEY=VAL 写入 env 文件（存在则替换）。
upsert_env_file() {
  local file="$1" key="$2" val="$3"
  [[ -f "$file" ]] || return 0
  # 无尾换行时 >> 会粘到上一行（例如 RELAYGATE_DATA_DIR=...ENVOY_CPU_LIMIT=2）
  if [[ -s "$file" ]] && [[ "$(tail -c1 "$file" | wc -l)" -eq 0 ]]; then
    printf '\n' >>"$file"
  fi
  if grep -qE "^${key}=" "$file" 2>/dev/null; then
    sed -i "s|^${key}=.*|${key}=${val}|" "$file"
  else
    printf '%s=%s\n' "$key" "$val" >>"$file"
  fi
}

# Docker 拒绝 cpus limit > 宿主机核数；按 nproc 自动封顶并写入 .env。
tune_compose_cpu_limits() {
  local host_cpus envoy_lim prom_lim
  host_cpus="$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
  [[ "$host_cpus" =~ ^[0-9]+$ ]] && (( host_cpus >= 1 )) || host_cpus=2

  envoy_lim="${ENVOY_CPU_LIMIT:-4.0}"
  prom_lim="${PROMETHEUS_CPU_LIMIT:-2.0}"
  # 浮点比较：若默认/环境值超过宿主机核数则封顶
  if awk -v lim="$envoy_lim" -v max="$host_cpus" 'BEGIN { exit !(lim+0 > max+0) }'; then
    envoy_lim="$host_cpus"
  fi
  if awk -v lim="$prom_lim" -v max="$host_cpus" 'BEGIN { exit !(lim+0 > max+0) }'; then
    prom_lim="$host_cpus"
  fi
  export ENVOY_CPU_LIMIT="$envoy_lim" PROMETHEUS_CPU_LIMIT="$prom_lim"
  if [[ -f "${INSTALL_DIR}/.env" ]]; then
    upsert_env_file "${INSTALL_DIR}/.env" ENVOY_CPU_LIMIT "$ENVOY_CPU_LIMIT"
    upsert_env_file "${INSTALL_DIR}/.env" PROMETHEUS_CPU_LIMIT "$PROMETHEUS_CPU_LIMIT"
  fi
  log "Compose CPU 限额已按宿主机 ${host_cpus} 核调整: ENVOY=${ENVOY_CPU_LIMIT} PROMETHEUS=${PROMETHEUS_CPU_LIMIT}"
}

# Envoy 容器（非 root）需读 envoy.yaml、写 logs；纠正 umask 077 导致的过严权限。
ensure_envoy_runtime_perms() {
  local data="${RELAYGATE_DATA_DIR:-$INSTALL_DIR/data}"
  mkdir -p "${data}/envoy/logs"
  chmod 0755 "${data}/envoy" 2>/dev/null || true
  chmod 0777 "${data}/envoy/logs" 2>/dev/null || true
  if [[ -f "${data}/envoy/envoy.yaml" ]]; then
    chmod 0644 "${data}/envoy/envoy.yaml" 2>/dev/null || true
  fi
}

# 节点接入：落盘一次性代理令牌并写入 .env 字段（不含 Panel 密码）。
write_agent_join_creds() {
  [[ -n "${AGENT_TOKEN:-}" ]] || return 0
  mkdir -p "$SECRETS_DIR"
  local tok="${SECRETS_DIR}/agent.token"
  printf '%s\n' "$AGENT_TOKEN" >"$tok"
  chmod 600 "$tok"
  export AGENT_TOKEN_FILE="$tok"
  if [[ -f "${INSTALL_DIR}/.env" ]]; then
    upsert_env_file "${INSTALL_DIR}/.env" PRIMARY_URL "${PRIMARY_URL}"
    upsert_env_file "${INSTALL_DIR}/.env" AGENT_TOKEN_FILE "$tok"
    upsert_env_file "${INSTALL_DIR}/.env" ENABLE_PANEL "0"
    upsert_env_file "${INSTALL_DIR}/.env" PANEL_ROLE "standby"
    upsert_env_file "${INSTALL_DIR}/.env" ENABLE_GRAFANA "${ENABLE_GRAFANA:-0}"
    [[ -n "${GATEWAY_NAME:-}" ]] && upsert_env_file "${INSTALL_DIR}/.env" GATEWAY_NAME "$GATEWAY_NAME"
  fi
  log "已写入节点代理令牌: ${tok}"
}

Invoke_Product() {
  log "阶段 5/5 · 运行产品 CLI"
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] setup → apply → panel/agent install → smoke"
    return 0
  fi
  cd "$INSTALL_DIR"
  # 产品写入的 yaml/日志目录需容器可读；临时放宽 umask（密钥已在 SECRETS_DIR）
  umask 022
  export RELAYGATE_INSTALL_DIR="$INSTALL_DIR" RELAYGATE_SECRETS_DIR="$SECRETS_DIR"
  export RELAYGATE_DATA_DIR="${RELAYGATE_DATA_DIR:-$INSTALL_DIR/data}"
  export NONINTERACTIVE ENABLE_PANEL ENABLE_GRAFANA
  export GATEWAY_NAME="${GATEWAY_NAME:-}" GATEWAY_PUBLIC_IP="${GATEWAY_PUBLIC_IP:-}" GATEWAY_SSH_PORT="${GATEWAY_SSH_PORT:-}"
  export APPLY_FIREWALL FIREWALL_CONFIRM="${FIREWALL_CONFIRM:-}"
  export PRIMARY_URL="${PRIMARY_URL:-}" AGENT_TOKEN="${AGENT_TOKEN:-}" AGENT_TOKEN_FILE="${AGENT_TOKEN_FILE:-}"

  local setup_flags=(--noninteractive --sysctl)
  [[ "$ACTION" == "upgrade" ]] && setup_flags+=(--upgrade)

  log "→ relaygate setup ${setup_flags[*]}"
  ./bin/relaygate setup "${setup_flags[@]}" || die "setup 失败"

  write_agent_join_creds

  tune_compose_cpu_limits
  ensure_envoy_runtime_perms

  log "→ relaygate apply"
  ./bin/relaygate apply || die "apply 失败；可试: ./bin/relaygate rollback"
  ensure_envoy_runtime_perms

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
    # Panel 以 User=relaygate 运行；确保能遍历密钥目录（防 umask 077 → 0700）
    chmod 0750 /etc/relaygate "${SECRETS_DIR}" 2>/dev/null || true
    chown root:relaygate /etc/relaygate "${SECRETS_DIR}" 2>/dev/null || true
    if [[ -f "${SECRETS_DIR}/panel_admin_password" ]]; then
      chown root:relaygate "${SECRETS_DIR}/panel_admin_password"
      chmod 0640 "${SECRETS_DIR}/panel_admin_password"
    fi
    # doctor/smoke 等以 relaygate 读 .env；勿留 0600 root:root
    if [[ -f "${INSTALL_DIR}/.env" ]]; then
      chown root:relaygate "${INSTALL_DIR}/.env"
      chmod 0640 "${INSTALL_DIR}/.env"
    fi
    if [[ -f "${INSTALL_DIR}/data/resources.yaml" ]]; then
      chown root:relaygate "${INSTALL_DIR}/data/resources.yaml"
      chmod 0660 "${INSTALL_DIR}/data/resources.yaml"
    fi
    if [[ -d "${INSTALL_DIR}/data/inventory" ]]; then
      chown root:relaygate "${INSTALL_DIR}/data/inventory"
      chmod 0750 "${INSTALL_DIR}/data/inventory"
    fi
    if [[ -f "${INSTALL_DIR}/data/inventory/gateways.env" ]]; then
      chown root:relaygate "${INSTALL_DIR}/data/inventory/gateways.env"
      chmod 0640 "${INSTALL_DIR}/data/inventory/gateways.env"
    fi
    # 升级后确保新二进制生效
    systemctl restart relaygate-panel 2>/dev/null || true
  else
    PURGE=0 ./bin/relaygate panel uninstall || true
    # 节点：有令牌 / PRIMARY_URL / 已装 agent 单元 → 安装或升级后重启 agent
    if [[ -n "${AGENT_TOKEN:-}" || -n "${AGENT_TOKEN_FILE:-}" || -n "${PRIMARY_URL:-}" ]] \
      || systemctl cat relaygate-agent.service >/dev/null 2>&1; then
      log "→ relaygate agent install"
      ENABLE_NOW=1 ./bin/relaygate agent install || die "agent install 失败"
      systemctl restart relaygate-agent 2>/dev/null || true
    else
      warn "ENABLE_PANEL=0 但未检测到 PRIMARY_URL/AGENT_TOKEN；跳过 agent install"
    fi
  fi

  log "→ relaygate smoke"
  ./bin/relaygate smoke || die "smoke 失败"

  if [[ "$APPLY_FIREWALL" == "1" ]]; then
    log "→ relaygate firewall apply"
    SSH_PORT="${GATEWAY_SSH_PORT:-22}" APPLY_FIREWALL=1 \
      FIREWALL_CONFIRM="${FIREWALL_CONFIRM:-}" ./bin/relaygate firewall apply ||
      die "firewall apply 失败"
  else
    log "→ relaygate firewall check"
    SSH_PORT="${GATEWAY_SSH_PORT:-22}" ./bin/relaygate firewall check || true
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
    log "Panel: http://${GATEWAY_PUBLIC_IP:-<IP>}:9000 （默认 PANEL_BIND=0.0.0.0:9000）"
    log "默认管理员密码: relaygate（密钥文件 panel_admin_password；生产务必改密）"
    log "升级: curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh | sudo bash -s -- --upgrade -y"
  elif [[ -n "${PRIMARY_URL:-}" ]]; then
    log "节点 Agent: systemctl status relaygate-agent ；主控 ${PRIMARY_URL}"
    log "升级: curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh | sudo bash -s -- --upgrade -y"
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
  systemctl disable --now relaygate-agent.service 2>/dev/null || true
  rm -f /etc/systemd/system/relaygate-agent.service
  systemctl daemon-reload 2>/dev/null || true
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
