#!/usr/bin/env bash
# RelayGate one-command installer. Safe defaults: no firewall apply, no purge.
set -Eeuo pipefail
umask 077

ACTION=install
DRY_RUN=0
PURGE=0
INSTALL_DIR="${RELAYGATE_INSTALL_DIR:-/opt/relaygate}"
VERSION="${RELAYGATE_VERSION:-}"
REPO_URL="${RELAYGATE_REPO_URL:-https://github.com/relaygate/relaygate.git}"
SECRETS_DIR="${RELAYGATE_SECRETS_DIR:-/etc/relaygate/secrets}"
NONINTERACTIVE="${NONINTERACTIVE:-0}"
APPLY_FIREWALL="${APPLY_FIREWALL:-}"
SOURCE_DIR="${RELAYGATE_SOURCE_DIR:-}"
MIN_MEMORY_KB=900000
MIN_DISK_KB=4000000
TMP_DIR=""

usage() {
  cat <<'EOF'
RelayGate 安装器
用法: install.sh [--install|--upgrade|--uninstall] [--purge] [--dry-run]

常用环境变量:
  RELAYGATE_INSTALL_DIR=/opt/relaygate
  RELAYGATE_VERSION=<tag|branch|commit>   # 默认读取仓库 RELEASE
  GATEWAY_NAME=gateway-01
  GATEWAY_PUBLIC_IP=203.0.113.10
  GATEWAY_SSH_PORT=30455
  ENABLE_PANEL=1          # 宿主二进制 + systemd（非 Compose）
  ENABLE_GRAFANA=1        # Compose profile with-grafana
  NONINTERACTIVE=1
  APPLY_FIREWALL=0

防火墙默认只生成并检查。应用必须显式设置 APPLY_FIREWALL=1；
非交互应用还必须设置 FIREWALL_CONFIRM=YES_FLUSH_NFTABLES。
Panel 默认以 systemd 服务 relaygate-panel 运行 /opt/relaygate/bin/relaygate panel。
数据面（Envoy/Prometheus/Grafana/node_exporter）仍用 Docker Compose。
EOF
}

while (($#)); do
  case "$1" in
    --install) ACTION=install ;;
    --upgrade) ACTION=upgrade ;;
    --uninstall) ACTION=uninstall ;;
    --purge) PURGE=1 ;;
    --dry-run) DRY_RUN=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR: 未知参数: $1" >&2; usage; exit 2 ;;
  esac
  shift
done

log() { printf '%s\n' "==> $*"; }
warn() { printf '%s\n' "WARN: $*" >&2; }
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
cleanup() {
  [[ -z "$TMP_DIR" || ! -d "$TMP_DIR" ]] || rm -rf "$TMP_DIR"
  return 0
}
trap cleanup EXIT
trap 'die "安装在第 ${LINENO} 行失败。修复后可重跑；已有配置不会被覆盖。"' ERR

prompt() {
  local variable="$1" message="$2" default="${3:-}" value
  value="${!variable:-}"
  [[ -n "$value" ]] && return 0
  if [[ "$NONINTERACTIVE" == "1" ]]; then
    [[ -n "$default" ]] || die "NONINTERACTIVE=1 时必须设置 $variable"
    printf -v "$variable" '%s' "$default"
    return 0
  fi
  [[ -r /dev/tty ]] || die "无交互终端；请设置 NONINTERACTIVE=1 及所需环境变量"
  read -r -p "${message}${default:+ [$default]}: " value </dev/tty
  printf -v "$variable" '%s' "${value:-$default}"
}

confirm() {
  local message="$1" answer
  [[ "$NONINTERACTIVE" != "1" ]] || return 1
  read -r -p "$message [y/N]: " answer </dev/tty
  [[ "$answer" == "y" || "$answer" == "Y" ]]
}

require_linux() {
  [[ "$(uname -s)" == "Linux" ]] || die "仅支持 Linux"
  [[ -r /etc/os-release ]] || die "无法识别发行版（缺少 /etc/os-release）"
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_ID="${ID,,}"
  OS_LIKE="${ID_LIKE:-}"
  case "$OS_ID" in
    ubuntu|debian) OS_FAMILY=deb ;;
    rhel|rocky|almalinux|centos) OS_FAMILY=rpm ;;
    *)
      if [[ "$OS_LIKE" == *debian* ]]; then OS_FAMILY=deb
      elif [[ "$OS_LIKE" == *rhel* || "$OS_LIKE" == *fedora* ]]; then OS_FAMILY=rpm
      else die "不支持的发行版: ${PRETTY_NAME:-$OS_ID}"; fi
      ;;
  esac
  command -v systemctl >/dev/null 2>&1 || die "需要 systemd"
  [[ "$(ps -p 1 -o comm= 2>/dev/null)" == "systemd" ]] || die "PID 1 不是 systemd"
  case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "不支持的 CPU 架构: $(uname -m)" ;;
  esac
}

validate_paths() {
  [[ "$INSTALL_DIR" == /* && "$INSTALL_DIR" != "/" ]] || die "RELAYGATE_INSTALL_DIR 必须是非根绝对路径"
  [[ "$SECRETS_DIR" == /* && "$SECRETS_DIR" != "/" && "$SECRETS_DIR" != "/etc" ]] ||
    die "RELAYGATE_SECRETS_DIR 必须是非根专用绝对路径"
  [[ "$INSTALL_DIR" != *[[:space:]]* && "$SECRETS_DIR" != *[[:space:]]* ]] ||
    die "安装目录和密钥目录不能包含空白字符"
}

check_capacity() {
  local memory_kb disk_kb
  memory_kb="$(awk '/MemTotal/ {print $2}' /proc/meminfo)"
  disk_kb="$(df -Pk "$(dirname "$INSTALL_DIR")" 2>/dev/null | awk 'NR==2 {print $4}')"
  if [[ -z "$disk_kb" ]]; then
    disk_kb="$(df -Pk / | awk 'NR==2 {print $4}')"
  fi
  log "环境: ${PRETTY_NAME:-$OS_ID}, ${ARCH}, 内存 $((memory_kb / 1024)) MiB, 可用磁盘 $((disk_kb / 1024)) MiB"
  (( memory_kb >= MIN_MEMORY_KB )) || die "内存不足：至少需要约 1 GiB"
  (( disk_kb >= MIN_DISK_KB )) || die "磁盘不足：至少需要约 4 GiB 可用空间"
}

install_base_packages() {
  log "安装基础依赖"
  if [[ "$OS_FAMILY" == "deb" ]]; then
    run apt-get update
    run env DEBIAN_FRONTEND=noninteractive apt-get install -y \
      ca-certificates curl git openssl nftables iproute2 sudo
  else
    local pm=dnf
    command -v dnf >/dev/null 2>&1 || pm=yum
    run "$pm" install -y ca-certificates curl git openssl nftables iproute sudo
  fi
}

version_ge() {
  [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | tail -n1)" == "$1" ]]
}

install_docker() {
  local install_needed=1 compose_version=""
  if command -v docker >/dev/null 2>&1; then
    if docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
      install_needed=0
      log "已检测 Docker $(docker version --format '{{.Server.Version}}')"
    fi
  fi
  if [[ "$install_needed" == "1" ]]; then
    log "从 Docker 官方仓库安装 Docker Engine"
    if [[ "$OS_FAMILY" == "deb" ]]; then
      run install -m 0755 -d /etc/apt/keyrings
      if [[ "$DRY_RUN" == "1" ]]; then
        log "[dry-run] 下载并校验 Docker apt GPG key，写入官方软件源"
      else
        curl -fsSL "https://download.docker.com/linux/${OS_ID}/gpg" -o /etc/apt/keyrings/docker.asc
        chmod a+r /etc/apt/keyrings/docker.asc
        # shellcheck disable=SC1091
        . /etc/os-release
        printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/%s %s stable\n' \
          "$(dpkg --print-architecture)" "$OS_ID" "$VERSION_CODENAME" > /etc/apt/sources.list.d/docker.list
      fi
      run apt-get update
      run env DEBIAN_FRONTEND=noninteractive apt-get install -y \
        docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    else
      local pm=dnf repo_os="$OS_ID"
      command -v dnf >/dev/null 2>&1 || pm=yum
      case "$OS_ID" in rocky|almalinux|centos) repo_os=centos ;; rhel) repo_os=rhel ;; esac
      run "$pm" install -y dnf-plugins-core
      run "$pm" config-manager --add-repo "https://download.docker.com/linux/${repo_os}/docker-ce.repo"
      run "$pm" install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    fi
  fi
  run systemctl enable --now docker
  if [[ "$DRY_RUN" != "1" ]]; then
    docker compose version >/dev/null 2>&1 || die "Docker Compose v2 不可用"
    compose_version="$(docker compose version --short | sed 's/^v//')"
    version_ge "$compose_version" "2.20.0" || die "Docker Compose 版本过旧: $compose_version（需要 >= 2.20）"
    log "Docker Compose v${compose_version}"
  fi
}

detect_ssh_port() {
  local detected=""
  if command -v sshd >/dev/null 2>&1; then
    detected="$(sshd -T 2>/dev/null | awk '$1=="port" {print $2; exit}')"
  fi
  GATEWAY_SSH_PORT="${GATEWAY_SSH_PORT:-${detected:-22}}"
  [[ "$GATEWAY_SSH_PORT" =~ ^[0-9]+$ ]] || die "GATEWAY_SSH_PORT 必须是数字"
  (( GATEWAY_SSH_PORT >= 1 && GATEWAY_SSH_PORT <= 65535 )) || die "GATEWAY_SSH_PORT 超出范围"
}

valid_ipv4() {
  local ip="$1" part
  local -a parts
  [[ "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || return 1
  IFS=. read -r -a parts <<<"$ip"
  for part in "${parts[@]}"; do
    ((10#$part >= 0 && 10#$part <= 255)) || return 1
  done
}

collect_settings() {
  local detected_ip="" panel_default=1 grafana_default=1
  detect_ssh_port
  if [[ -z "${GATEWAY_PUBLIC_IP:-}" && "$NONINTERACTIVE" != "1" ]]; then
    detected_ip="$(curl -4fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
  fi
  prompt GATEWAY_NAME "网关名称" "gateway-01"
  prompt GATEWAY_PUBLIC_IP "公网 IPv4" "$detected_ip"
  [[ "$GATEWAY_NAME" =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]*$ ]] || die "网关名称格式无效"
  valid_ipv4 "$GATEWAY_PUBLIC_IP" || die "必须提供有效的 GATEWAY_PUBLIC_IP"

  if [[ -z "${ENABLE_PANEL:-}" ]]; then
    if [[ "$NONINTERACTIVE" == "1" ]]; then ENABLE_PANEL="$panel_default"
    elif confirm "启用仅本机监听的 Panel？"; then ENABLE_PANEL=1; else ENABLE_PANEL=0; fi
  fi
  if [[ -z "${ENABLE_GRAFANA:-}" ]]; then
    if [[ "$NONINTERACTIVE" == "1" ]]; then ENABLE_GRAFANA="$grafana_default"
    elif confirm "启用仅本机监听的 Grafana？"; then ENABLE_GRAFANA=1; else ENABLE_GRAFANA=0; fi
  fi
  if [[ -z "$APPLY_FIREWALL" ]]; then
    if [[ "$NONINTERACTIVE" == "1" ]]; then
      APPLY_FIREWALL=0
    else
      warn "nftables 模板包含 flush ruleset；应用前必须准备云控制台并保持当前 SSH 会话"
      if confirm "现在应用 RelayGate nftables 规则？"; then APPLY_FIREWALL=1; else APPLY_FIREWALL=0; fi
    fi
  fi
  [[ "$ENABLE_PANEL" == "0" || "$ENABLE_PANEL" == "1" ]] || die "ENABLE_PANEL 只能是 0 或 1"
  [[ "$ENABLE_GRAFANA" == "0" || "$ENABLE_GRAFANA" == "1" ]] || die "ENABLE_GRAFANA 只能是 0 或 1"
  [[ "$APPLY_FIREWALL" == "0" || "$APPLY_FIREWALL" == "1" ]] || die "APPLY_FIREWALL 只能是 0 或 1"
}

check_ports() {
  local ports="9901 9090 9100" port
  [[ "${ENABLE_PANEL:-1}" == "1" ]] && ports="$ports 9000"
  [[ "${ENABLE_GRAFANA:-1}" == "1" ]] && ports="$ports 3000"
  command -v ss >/dev/null 2>&1 || return 0
  for port in $ports; do
    if ss -H -lnt "( sport = :$port )" 2>/dev/null | grep -q .; then
      if [[ "$ACTION" == "upgrade" && -f "$INSTALL_DIR/.env" ]]; then
        warn "端口 $port 已占用（升级时可能是现有 RelayGate 容器）"
      else
        die "端口 $port 已被占用；请先释放或禁用对应组件"
      fi
    fi
  done
}

acquire_source() {
  TMP_DIR="$(mktemp -d)"
  local release_url="https://raw.githubusercontent.com/relaygate/relaygate/master/RELEASE"
  if [[ -z "$VERSION" ]]; then
    VERSION="$(curl -fsSL --max-time 10 "$release_url" 2>/dev/null | tr -d '[:space:]' || true)"
    VERSION="${VERSION:-master}"
  fi
  log "获取 RelayGate 版本: $VERSION"
  if [[ -n "$SOURCE_DIR" ]]; then
    [[ -f "$SOURCE_DIR/go.mod" && -f "$SOURCE_DIR/deploy/compose.yaml" ]] || die "RELAYGATE_SOURCE_DIR 不是有效源码目录"
    SOURCE_PATH="$SOURCE_DIR"
    return
  fi
  if [[ "$DRY_RUN" == "1" ]]; then
    SOURCE_PATH="$TMP_DIR/source"
    log "[dry-run] 将从 $REPO_URL 获取 $VERSION"
    return
  fi
  git -c advice.detachedHead=false init -q "$TMP_DIR/source"
  git -C "$TMP_DIR/source" remote add origin "$REPO_URL"
  git -C "$TMP_DIR/source" fetch -q --depth 1 origin "$VERSION"
  git -C "$TMP_DIR/source" checkout -q --detach FETCH_HEAD
  SOURCE_PATH="$TMP_DIR/source"
}

backup_existing() {
  [[ -d "$INSTALL_DIR" ]] || return 0
  local stamp backup
  stamp="$(date +%Y%m%d-%H%M%S)"
  backup="$INSTALL_DIR/backups/installer-$stamp"
  run mkdir -p "$backup"
  [[ -f "$INSTALL_DIR/.env" ]] && run cp -a "$INSTALL_DIR/.env" "$backup/.env"
  [[ -f "$INSTALL_DIR/config/resources.yaml" ]] && run cp -a "$INSTALL_DIR/config/resources.yaml" "$backup/resources.yaml"
  [[ -f "$INSTALL_DIR/gateway/generated/envoy.yaml" ]] && run cp -a "$INSTALL_DIR/gateway/generated/envoy.yaml" "$backup/envoy.yaml"
  if [[ "$DRY_RUN" != "1" ]]; then
    printf '%s\n' "$backup" > "$INSTALL_DIR/backups/installer-latest"
    printf '%s\n' "${backup##*/}" > "$INSTALL_DIR/backups/latest"
  fi
  log "升级前配置备份: $backup"
}

copy_source() {
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] 将源码复制到 $INSTALL_DIR，并保留 .env、resources.yaml 和密钥"
    return
  fi
  mkdir -p "$INSTALL_DIR"
  local saved_resources="" saved_env=""
  if [[ -f "$INSTALL_DIR/config/resources.yaml" ]]; then
    saved_resources="$(mktemp)"
    cp -a "$INSTALL_DIR/config/resources.yaml" "$saved_resources"
  fi
  if [[ -f "$INSTALL_DIR/.env" ]]; then
    saved_env="$(mktemp)"
    cp -a "$INSTALL_DIR/.env" "$saved_env"
  fi
  cp -a "$SOURCE_PATH/." "$INSTALL_DIR/"
  [[ -n "$saved_resources" ]] && cp -a "$saved_resources" "$INSTALL_DIR/config/resources.yaml" && rm -f "$saved_resources"
  [[ -n "$saved_env" ]] && cp -a "$saved_env" "$INSTALL_DIR/.env" && rm -f "$saved_env"
  # 真实 config/resources.yaml 属敏感资产、不入库；首次安装从模板生成占位，供后续替换真实后端。
  if [[ ! -f "$INSTALL_DIR/config/resources.yaml" && -f "$INSTALL_DIR/config/resources.example.yaml" ]]; then
    cp -a "$INSTALL_DIR/config/resources.example.yaml" "$INSTALL_DIR/config/resources.yaml"
    warn "已从模板生成 $INSTALL_DIR/config/resources.yaml（占位 IP）；请填入真实后端后重跑 relaygate render / reload。"
  fi
}

generate_configuration() {
  local profiles="" image_tag="source" grafana_url=""
  [[ "$DRY_RUN" == "1" ]] || image_tag="$(git -C "$INSTALL_DIR" rev-parse --short=12 HEAD 2>/dev/null || date +%Y%m%d%H%M)"
  # Panel 已迁出 Compose；profiles 仅控制 Grafana 等数据面可选组件
  [[ "$ENABLE_GRAFANA" == "1" ]] && profiles="with-grafana"
  # Panel 反代目标：仅在同时启用 Panel + Grafana 时写入
  [[ "$ENABLE_PANEL" == "1" && "$ENABLE_GRAFANA" == "1" ]] && grafana_url="http://127.0.0.1:3000"
  if [[ ! -f "$INSTALL_DIR/.env" ]]; then
    log "生成 $INSTALL_DIR/.env"
    if [[ "$DRY_RUN" != "1" ]]; then
      cat > "$INSTALL_DIR/.env" <<EOF
GATEWAY_NAME=$GATEWAY_NAME
GATEWAY_PUBLIC_IP=$GATEWAY_PUBLIC_IP
GATEWAY_SSH_PORT=$GATEWAY_SSH_PORT
PANEL_ROLE=primary
ENABLE_PANEL=$ENABLE_PANEL
COMPOSE_PROJECT_NAME=relaygate-$GATEWAY_NAME
COMPOSE_PROFILES=$profiles
ENVOY_IMAGE=envoyproxy/envoy:v1.39.0
ENVOY_ADMIN_PORT=9901
ENVOY_CONCURRENCY=0
IMAGE_TAG=$image_tag
PANEL_BIND=127.0.0.1:9000
GRAFANA_ADMIN_USER=admin
GRAFANA_URL=$grafana_url
GRAFANA_ROOT_URL=/grafana/
GRAFANA_ANONYMOUS=true
PROMETHEUS_RETENTION=15d
RELAYGATE_SECRETS_DIR=$SECRETS_DIR
EOF
      chmod 600 "$INSTALL_DIR/.env"
    fi
  else
    log "保留现有 .env"
    if [[ "$ACTION" == "upgrade" && "$DRY_RUN" != "1" ]]; then
      if grep -q '^IMAGE_TAG=' "$INSTALL_DIR/.env"; then
        sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=$image_tag/" "$INSTALL_DIR/.env"
      else
        printf 'IMAGE_TAG=%s\n' "$image_tag" >> "$INSTALL_DIR/.env"
      fi
      # 迁移：去掉已废弃的 with-panel profile
      if grep -q '^COMPOSE_PROFILES=' "$INSTALL_DIR/.env"; then
        local cleaned
        cleaned="$(grep '^COMPOSE_PROFILES=' "$INSTALL_DIR/.env" | head -n1 | cut -d= -f2- | tr ',' '\n' | grep -v '^with-panel$' | grep -v '^$' | paste -sd, -)"
        sed -i "s/^COMPOSE_PROFILES=.*/COMPOSE_PROFILES=$cleaned/" "$INSTALL_DIR/.env"
      fi
      if ! grep -q '^ENABLE_PANEL=' "$INSTALL_DIR/.env"; then
        printf 'ENABLE_PANEL=%s\n' "$ENABLE_PANEL" >> "$INSTALL_DIR/.env"
      fi
      # 移除无用的 PANEL_IMAGE（若存在）
      sed -i '/^PANEL_IMAGE=/d' "$INSTALL_DIR/.env" || true
      IMAGE_TAG="$image_tag"
      export IMAGE_TAG
      log "仅将受管镜像标签更新为源码提交 $image_tag；已迁移去掉 with-panel"
    fi
  fi
  run install -d -m 0750 -o root -g root "$SECRETS_DIR"
  if [[ ! -s "$SECRETS_DIR/panel_admin_password" ]]; then
    [[ "$DRY_RUN" == "1" ]] || openssl rand -base64 36 > "$SECRETS_DIR/panel_admin_password"
  fi
  if [[ ! -s "$SECRETS_DIR/grafana_admin_password" ]]; then
    [[ "$DRY_RUN" == "1" ]] || openssl rand -base64 36 > "$SECRETS_DIR/grafana_admin_password"
  fi
  if [[ "$DRY_RUN" != "1" ]]; then
    # Panel 密码：relaygate 组可读；Grafana 密码仅 root（容器以 root 读挂载）
    chmod 0750 "$SECRETS_DIR"
    chmod 0640 "$SECRETS_DIR/panel_admin_password" 2>/dev/null || true
    chmod 0600 "$SECRETS_DIR/grafana_admin_password" 2>/dev/null || true
    # 用户可能尚未创建；install_panel_service 会再校正属主
    chown root:root "$SECRETS_DIR" "$SECRETS_DIR/grafana_admin_password" 2>/dev/null || true
    chown root:root "$SECRETS_DIR/panel_admin_password" 2>/dev/null || true
  fi
  log "密钥保存在 $SECRETS_DIR（panel: root:relaygate 0640；grafana: root 0600；不会打印明文）"
}

build_and_validate() {
  [[ "$DRY_RUN" == "1" ]] && { log "[dry-run] 将构建 Panel/relaygate、渲染并校验 Compose/Envoy/nftables"; return; }
  cd "$INSTALL_DIR"
  # shellcheck disable=SC1091
  set -a; source .env; set +a
  log "构建固定标签镜像 relaygate-panel:${IMAGE_TAG}（仅用于提取 Linux 二进制）"
  docker build --pull -f Dockerfile.panel -t "relaygate-panel:${IMAGE_TAG}" .
  local container
  container="$(docker create "relaygate-panel:${IMAGE_TAG}")"
  mkdir -p bin
  docker cp "$container:/usr/local/bin/relaygate" bin/relaygate
  docker rm "$container" >/dev/null
  chmod 755 bin/relaygate
  chown root:root bin/relaygate
  log "渲染并校验配置"
  bash scripts/validate.sh
  docker compose -f deploy/compose.yaml --env-file .env config -q
}

install_or_update_panel_service() {
  local grafana_url=""
  if [[ "$ENABLE_PANEL" != "1" ]]; then
    log "未启用 Panel：停止/禁用 systemd 服务（若存在）"
    if [[ "$DRY_RUN" == "1" ]]; then
      log "[dry-run] bash scripts/uninstall_panel_service.sh（保留用户）"
    else
      cd "$INSTALL_DIR"
      DRY_RUN=0 PURGE=0 bash scripts/uninstall_panel_service.sh || true
      # 停掉可能残留的旧 Compose panel 容器
      docker rm -f "${GATEWAY_NAME:-gateway-01}-panel" 2>/dev/null || true
    fi
    return
  fi
  [[ "$ENABLE_GRAFANA" == "1" ]] && grafana_url="http://127.0.0.1:3000"
  log "安装/更新 Panel systemd 服务（二进制模式）"
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] RELAYGATE_INSTALL_DIR=$INSTALL_DIR GRAFANA_URL=$grafana_url bash scripts/install_panel_service.sh"
    return
  fi
  cd "$INSTALL_DIR"
  # 升级前健康快照（失败时提示 rollback）
  local pre_ok=0
  if [[ "$ACTION" == "upgrade" ]] && curl -fsS "http://127.0.0.1:9000/login" >/dev/null 2>&1; then
    pre_ok=1
  fi
  RELAYGATE_INSTALL_DIR="$INSTALL_DIR" \
    RELAYGATE_SECRETS_DIR="$SECRETS_DIR" \
    GRAFANA_URL="$grafana_url" \
    ENABLE_NOW=1 \
    DRY_RUN=0 \
    bash scripts/install_panel_service.sh
  # 停掉可能残留的旧 Compose panel 容器（迁移）
  docker rm -f "${GATEWAY_NAME:-gateway-01}-panel" 2>/dev/null || true
  if [[ "$ACTION" == "upgrade" && "$pre_ok" == "1" ]]; then
    if ! wait_http "http://127.0.0.1:9000/login" 30; then
      warn "升级后 Panel 健康检查失败；可尝试: bash scripts/rollback.sh 并检查 journalctl -u relaygate-panel"
      die "Panel 升级后未 ready；日志: journalctl -u relaygate-panel -n 100 --no-pager"
    fi
  fi
}

apply_sysctl() {
  local destination="/etc/sysctl.d/99-${GATEWAY_NAME}.conf"
  if [[ -f "$destination" ]]; then
    run cp -a "$destination" "${destination}.bak.$(date +%Y%m%d-%H%M%S)"
  fi
  run cp "$INSTALL_DIR/deploy/sysctl/gateway.conf" "$destination"
  run sysctl --system
}

wait_http() {
  local url="$1" attempts="${2:-30}" i
  for i in $(seq 1 "$attempts"); do
    curl -fsS "$url" >/dev/null 2>&1 && return 0
    sleep 2
  done
  return 1
}

save_failure_logs() {
  mkdir -p /var/log/relaygate
  docker compose -f deploy/compose.yaml --env-file .env logs --no-color \
    > /var/log/relaygate/install-failure.log 2>&1 || true
  docker compose -f deploy/compose.yaml --env-file .env ps || true
  if command -v journalctl >/dev/null 2>&1; then
    journalctl -u relaygate-panel -n 200 --no-pager \
      >> /var/log/relaygate/install-failure.log 2>&1 || true
  fi
}

start_and_check() {
  [[ "$DRY_RUN" == "1" ]] && { log "[dry-run] 将启动 Compose 数据面与（可选）Panel systemd，并执行 readiness 检查"; return; }
  cd "$INSTALL_DIR"
  log "启动 RelayGate 数据面（Compose）"
  docker compose -f deploy/compose.yaml --env-file .env up -d --build
  if ! wait_http "http://127.0.0.1:9901/ready" 45; then
    local recovery="停止并保留配置: bash install.sh --uninstall"
    [[ "$ACTION" != "upgrade" ]] || recovery="回滚: bash scripts/rollback.sh"
    save_failure_logs
    die "Envoy 未 ready；日志: /var/log/relaygate/install-failure.log；$recovery"
  fi
  install_or_update_panel_service
  if [[ "$ENABLE_PANEL" == "1" ]] && ! wait_http "http://127.0.0.1:9000/login"; then
    save_failure_logs
    die "Panel 健康检查失败；journalctl -u relaygate-panel；日志: /var/log/relaygate/install-failure.log"
  fi
  if [[ "$ENABLE_GRAFANA" == "1" ]] && ! wait_http "http://127.0.0.1:3000/api/health"; then
    save_failure_logs
    die "Grafana 健康检查失败；日志: /var/log/relaygate/install-failure.log"
  fi
  docker compose -f deploy/compose.yaml --env-file .env ps
  if [[ "$ENABLE_PANEL" == "1" ]]; then
    systemctl --no-pager --full status relaygate-panel.service || true
  fi
  bash scripts/smoke_test.sh
}

handle_firewall() {
  [[ "$DRY_RUN" == "1" ]] && { log "[dry-run] 防火墙仅生成/检查；不会应用"; return; }
  cd "$INSTALL_DIR"
  SSH_PORT="$GATEWAY_SSH_PORT" APPLY_FIREWALL="$APPLY_FIREWALL" \
    NONINTERACTIVE="$NONINTERACTIVE" FIREWALL_CONFIRM="${FIREWALL_CONFIRM:-}" \
    bash scripts/apply_firewall.sh
}

uninstall_relaygate() {
  [[ -d "$INSTALL_DIR" ]] || die "未发现安装目录: $INSTALL_DIR"
  if [[ "$DRY_RUN" == "1" ]]; then
    log "[dry-run] 将停止 Panel systemd 与 Compose 容器，默认保留配置、密钥和数据卷"
    [[ "$PURGE" == "1" ]] && log "[dry-run] --purge 将删除安装目录、密钥、数据卷及 relaygate 用户"
    return
  fi
  cd "$INSTALL_DIR"
  if [[ -f scripts/uninstall_panel_service.sh ]]; then
    PURGE="$PURGE" DRY_RUN=0 bash scripts/uninstall_panel_service.sh || true
  else
    systemctl disable --now relaygate-panel.service 2>/dev/null || true
    rm -f /etc/systemd/system/relaygate-panel.service \
      /etc/sudoers.d/relaygate-panel \
      /usr/local/libexec/relaygate/apply \
      /etc/relaygate/panel.env
    systemctl daemon-reload 2>/dev/null || true
  fi
  if [[ -f .env ]]; then
    # shellcheck disable=SC1091
    set -a; source .env; set +a
    SECRETS_DIR="${RELAYGATE_SECRETS_DIR:-$SECRETS_DIR}"
    docker compose -f deploy/compose.yaml --env-file .env down
  fi
  rm -f "/etc/sysctl.d/99-${GATEWAY_NAME:-gateway-01}.conf"
  sysctl --system >/dev/null || true
  if [[ -f "$INSTALL_DIR/backups/firewall-latest" ]]; then
    warn "nftables 规则未自动恢复，避免覆盖安装后的规则变更。"
    warn "如需恢复安装前规则，请审阅后执行: $(<"$INSTALL_DIR/backups/firewall-latest")"
  fi
  if [[ "$PURGE" != "1" ]]; then
    log "Panel systemd 与容器已停止；配置、密钥、镜像和数据卷均已保留"
    log "重新安装: bash $INSTALL_DIR/install.sh"
    return
  fi
  if [[ "$NONINTERACTIVE" == "1" ]]; then
    [[ "${PURGE_CONFIRM:-}" == "DELETE_RELAYGATE_DATA" ]] || die "非交互清除需 PURGE_CONFIRM=DELETE_RELAYGATE_DATA"
  else
    local answer
    read -r -p "将永久删除配置、密钥和数据卷。输入 DELETE_RELAYGATE_DATA: " answer </dev/tty
    [[ "$answer" == "DELETE_RELAYGATE_DATA" ]] || die "已取消清除"
  fi
  docker compose -f deploy/compose.yaml --env-file .env down -v --remove-orphans 2>/dev/null || true
  # purge 时再确保用户已删（uninstall_panel_service 在 PURGE=1 时已删）
  if getent passwd relaygate >/dev/null 2>&1; then
    userdel relaygate 2>/dev/null || true
  fi
  if getent group relaygate >/dev/null 2>&1; then
    groupdel relaygate 2>/dev/null || true
  fi
  rm -rf "$SECRETS_DIR" "$INSTALL_DIR" /etc/relaygate
  log "RelayGate 已彻底清除"
}

main() {
  require_linux
  validate_paths
  if [[ "$DRY_RUN" != "1" ]]; then
    [[ "$(id -u)" == "0" ]] || die "请以 root 运行"
  elif [[ "$(id -u)" != "0" ]]; then
    warn "dry-run 以非 root 运行；真实安装需要 root"
  fi

  if [[ "$ACTION" == "uninstall" ]]; then
    uninstall_relaygate
    return
  fi
  check_capacity
  if [[ "$ACTION" == "install" && -f "$INSTALL_DIR/.env" ]]; then
    die "检测到现有安装；请使用 --upgrade"
  fi
  if [[ "$ACTION" == "upgrade" && ! -f "$INSTALL_DIR/.env" ]]; then
    die "未检测到现有安装；请使用 --install"
  fi
  if [[ "$ACTION" == "upgrade" ]]; then
    # shellcheck disable=SC1091
    set -a; source "$INSTALL_DIR/.env"; set +a
    SECRETS_DIR="${RELAYGATE_SECRETS_DIR:-$SECRETS_DIR}"
    if [[ -z "${ENABLE_PANEL:-}" ]]; then
      # 新约定：ENABLE_PANEL；兼容旧 with-panel profile 与已安装的 systemd unit
      if grep -q '^ENABLE_PANEL=' "$INSTALL_DIR/.env" 2>/dev/null; then
        ENABLE_PANEL="$(grep '^ENABLE_PANEL=' "$INSTALL_DIR/.env" | head -n1 | cut -d= -f2-)"
      elif systemctl is-enabled relaygate-panel.service >/dev/null 2>&1; then
        ENABLE_PANEL=1
      else
        case ",${COMPOSE_PROFILES:-}," in *,with-panel,*) ENABLE_PANEL=1 ;; *) ENABLE_PANEL=0 ;; esac
      fi
    fi
    if [[ -z "${ENABLE_GRAFANA:-}" ]]; then
      case ",${COMPOSE_PROFILES:-}," in *,with-grafana,*) ENABLE_GRAFANA=1 ;; *) ENABLE_GRAFANA=0 ;; esac
    fi
  fi

  collect_settings
  install_base_packages
  check_ports
  install_docker
  acquire_source
  backup_existing
  copy_source
  generate_configuration
  build_and_validate
  apply_sysctl
  start_and_check
  handle_firewall

  log "RelayGate ${ACTION} 完成"
  log "配置: $INSTALL_DIR/.env；密钥: $SECRETS_DIR"
  if [[ "$ENABLE_PANEL" == "1" ]]; then
    log "Panel: systemctl status relaygate-panel ；日志: journalctl -u relaygate-panel -f"
  fi
  log "Panel（含 Grafana 反代）SSH 隧道: ssh -p $GATEWAY_SSH_PORT -L 9000:127.0.0.1:9000 root@$GATEWAY_PUBLIC_IP"
  log "浏览器: http://127.0.0.1:9000  →  Monitoring /grafana/（无需单独隧道 3000）"
  log "升级: bash $INSTALL_DIR/install.sh --upgrade"
  log "卸载（保留数据）: bash $INSTALL_DIR/install.sh --uninstall"
}

main "$@"
