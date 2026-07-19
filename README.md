# RelayGate

基于 **Envoy** 的游戏网关：单机起步，可演进为双活 + 云 L4 LB。一份 `data/resources.yaml` 驱动转发与限流；日常运维统一用二进制 `relaygate`。

## 功能

- **数据面**：Envoy 固定目标 TCP/UDP 转发，连接/PPS 限流
- **运维入口**：`relaygate`（setup、doctor、渲染、部署、drain、冒烟、防火墙、Panel）
- **管理面**：Panel 默认 `127.0.0.1:9000`（Grafana 同源反代）；监控仅 loopback
- **智能化**：增服自动分配端口；`reload` 自动 drain → 校验 → 重启 → ready

## 安装

`install.sh` **仅 bootstrap**：核心检测 → 下载预编译 release tar → 解压到 `/opt/relaygate` → 调用产品 CLI。  
数据面 Compose、Panel、防火墙由 `relaygate` 完成。**默认不再**在安装机 `docker build` 或整棵 git clone。

支持 Ubuntu / Debian / RHEL / Rocky / Alma / CentOS Stream / Fedora / Amazon Linux（systemd，`amd64`/`arm64`）。

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh -o /tmp/relaygate-install.sh
less /tmp/relaygate-install.sh
sudo RELAYGATE_VERSION=v0.1.0 bash /tmp/relaygate-install.sh
```

安装器内部等价于：

```text
root / OS / arch / systemd / Docker
→ 下载 relaygate-$VERSION-linux-$ARCH.tar.gz（+ sha256）
→ 解压到 /opt/relaygate（保留已有 .env 与 data/）
→ relaygate setup → apply → panel install → smoke
→ firewall check（默认不改主机）
```

生产务必固定 **Release tag**（禁止 `master`/`main`/`latest`）：

```bash
sudo RELAYGATE_VERSION=v0.1.0 bash /tmp/relaygate-install.sh
```

本地已有包：

```bash
sudo RELAYGATE_TAR=/path/relaygate-v0.1.0-linux-amd64.tar.gz bash /tmp/relaygate-install.sh
```

非交互示例：

```bash
sudo env NONINTERACTIVE=1 \
  RELAYGATE_VERSION=v0.1.0 \
  GATEWAY_NAME=gateway-01 \
  GATEWAY_PUBLIC_IP=<公网IP> \
  APPLY_FIREWALL=0 \
  bash /tmp/relaygate-install.sh
```

| 变量 | 说明 |
|------|------|
| `RELAYGATE_VERSION` | GitHub Release tag（推荐） |
| `RELAYGATE_TAR` | 本地 tar.gz，跳过下载 |
| `RELAYGATE_INSTALL_DIR` | 默认 `/opt/relaygate` |
| `FROM_SOURCE=1` | 开发兜底：源码构建（非默认） |
| `GATEWAY_NAME` / `GATEWAY_PUBLIC_IP` | 网关身份 |
| `ENABLE_PANEL` / `ENABLE_GRAFANA` | 默认 1 |
| `APPLY_FIREWALL` | 默认 0（只校验） |

### 开发机（源码）

```bash
cp resources.example.yaml data/resources.yaml
cp .env.example .env && chmod 600 .env
make build
./bin/relaygate setup --noninteractive
./bin/relaygate validate && ./bin/relaygate apply && ./bin/relaygate smoke

# 本地打与 CI 相同结构的 release 包
make dist VERSION=dev
```

## 日常运维

```text
首启     relaygate setup [--noninteractive] [--sysctl]
         relaygate doctor [--strict-ports]

配置     relaygate render [--check-only] [--observability]
         relaygate validate
         relaygate server enable|disable <server-01>

数据面   relaygate apply                 # 首次/全量
         relaygate reload                # 改配置后
         relaygate rollback [STAMP]
         relaygate drain fail|ok|status

检查     relaygate smoke [HOST]
         relaygate canary [HOST]
         relaygate baseline

防火墙   relaygate firewall [check|apply]
Panel    sudo relaygate panel install | uninstall
多机     GATEWAYS=gateway-01,gateway-02 relaygate fleet
```

### 推荐流程

```bash
relaygate server enable server-01
relaygate reload
relaygate smoke

relaygate canary 127.0.0.1
sudo FIREWALL_CONFIRM=YES_FLUSH_NFTABLES ./bin/relaygate firewall apply

relaygate drain fail
# …变更…
relaygate drain ok
```

### 升级 / 回滚 / 卸载

```bash
sudo bash /tmp/relaygate-install.sh --upgrade
relaygate rollback
sudo PURGE=1 bash /tmp/relaygate-install.sh --uninstall
```

## 游戏后端放行

| 项 | 默认 |
|----|------|
| Server 游戏 TCP / UDP | `7777` / `7778` |
| 健康检查 | TCP `7777` |
| 玩家入口（server-01） | `<GATEWAY_PUBLIC_IP>:10001`；canary `11001` |

```text
玩家 → gateway:10001 → Envoy → server:7777(TCP) / :7778(UDP)
```

要点：游戏监听 `0.0.0.0` 或内网 IP；Server 防火墙只放行网关回源 IP；双活时放行全部网关 IP。

```bash
relaygate reload
relaygate smoke
```

## 双活（可选）

```text
玩家 → 云 L4 LB
         ├─ gateway-01（ENABLE_PANEL=1, PANEL_ROLE=primary）
         └─ gateway-02（ENABLE_PANEL=0, PANEL_ROLE=standby）
```

```bash
./bin/relaygate apply && ./bin/relaygate smoke
# 仅 primary：sudo ./bin/relaygate panel install
```

云 NLB 模板见 [`core/deploy/terraform/nlb/`](core/deploy/terraform/nlb/)。分批：`GATEWAYS=gateway-01,gateway-02 ./bin/relaygate fleet`。

## 仓库布局

单 Go module + 预编译 release：`core` 随版本走，`data` 是安装后运行态（gitignore）。

```text
*.example / .env.example   # 仓库根：seed 源（resources → data/；inventory → data/inventory/）
data/                      # 运行态（gitignore）：勿放 Go 代码 / Grafana provisioning
  resources.yaml           # seed 自 resources.example.yaml
  envoy/ firewall/ prometheus/ backups/ inventory/
core/
  cmd/relaygate/           # 薄 main
  cli/ panel/ setup/ doctor/ envoygen/ status/
  ops/                     # Go 运维包（apply/reload/seed…）——代码，不是数据目录
  resources/               # resources.yaml 领域模型
  deploy/                  # 版本化部署模板（compose、systemd、grafana provisioning、
                           # prometheus tpl、firewall tpl）；禁止运行生成物与用户可变配置
frontend/                  # Panel UI
install.sh                 # bootstrap：下载 release tar → setup/apply
Makefile                   # build / test / dist
```

职责边界：`deploy` = 模板（版本化）；`data` = 实例状态；`ops` = Go package（仅在 `core/ops`）。

### 预设配置 vs 运行配置

| 环节 | 预设（入库 / release） | 运行（gitignore / 宿主机） |
|------|------------------------|----------------------------|
| 开发 | `*.example`、`core/deploy/`、`frontend/` | 本地 `.env`、`data/`（setup seed） |
| 安装 | release tar 内模板 + 空 `data/` 骨架 | `/opt/relaygate/data/`、`.env`、`/etc/relaygate/secrets/` |
| 容器 | compose 挂载 `core/deploy` 模板 + `data/` 生成物 | named volume（Grafana/Prometheus TSDB） |
| 升级 | 新 tar 覆盖 `bin/`、`frontend/`、`core/deploy/` | **保留** `.env` 与 `data/`；seed 只补缺失文件 |
| 备份 | — | `data/backups/`（apply/installer 自动） |
| 运维 | CLI 子命令 | `relaygate reload/drain/smoke/firewall` |

`relaygate setup` / 首次 `apply` 在目标缺失时从 `*.example` seed 到 `data/`；`--reset-defaults` 才覆盖已有 `resources.yaml` / inventory。Grafana provisioning 始终挂载 `core/deploy/grafana/`，不进 `data/`。Panel 强制 `PANEL_BIND` 为 loopback。

分批：`DEPLOY_REF=<tag|sha> GATEWAYS=… ./bin/relaygate fleet`（禁止默认跟踪 main）。

可选可观测性栈（标准 Compose；与网关默认栈分离，复用 `prometheus/`、`grafana/` 模板）：

```bash
cd core/deploy/observability
cp .env.example .env && chmod 600 .env
docker compose --env-file .env up -d
```

发布：`make dist` 或 tag `v*` 触发 `.github/workflows/release.yml`，产出  
`relaygate-$VERSION-linux-{amd64,arm64}.tar.gz` + `.sha256`。

单 module：`github.com/relaygate/relaygate`。

## 已知边界

- 固定目标转发，无跨服负载均衡
- UDP 无可靠主动健康检查
- 后端看到的是网关源 IP
- Envoy 非抗 DDoS；大流量需云高防
