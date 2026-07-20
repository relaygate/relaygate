# RelayGate

基于 **Envoy** 的游戏网关：单机起步，可演进为双活 + 云 L4 LB。一份运行态 `resources.yaml` 驱动转发与限流；日常运维统一用二进制 `relaygate`。

## 功能

- **数据面**：Envoy 固定目标 TCP/UDP 转发，连接/PPS 限流
- **运维入口**：`relaygate`（setup、doctor、渲染、部署、drain、冒烟、防火墙、ACL、profile、Panel）
- **管理面**：Panel 默认 `127.0.0.1:9000`（Grafana 同源反代）；监控仅 loopback
- **智能化**：增服自动分配端口；`reload` 自动 drain → 校验 → 重启 → ready
- **加深能力**：IP 黑白名单（nft）、defaults 变更摘要、游戏类型 profile、per-rule 限速观测、NLB/drain 协同检查

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
→ 解压到 /opt/relaygate（保留已有 .env 与 DataDir）
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
| `RELAYGATE_DATA_DIR` | 运行态目录；安装默认 `$INSTALL_DIR/data` |
| `FROM_SOURCE=1` | 开发兜底：源码构建（非默认） |
| `GATEWAY_NAME` / `GATEWAY_PUBLIC_IP` | 网关身份 |
| `GATEWAY_SSH_PORT` | 默认 `30455`（与 `.env.example` / firewall 模板一致） |
| `ENABLE_PANEL` / `ENABLE_GRAFANA` | 默认 1 |
| `APPLY_FIREWALL` | 默认 0（只校验） |

### 开发机（源码）

源码树**没有**顶层 `data/`。运行态默认写到 gitignore 的 `.runtime/`（或显式 `RELAYGATE_DATA_DIR`）。

```bash
cp .env.example .env && chmod 600 .env
make build
./bin/relaygate setup --noninteractive   # 写入 .env 中 RELAYGATE_DATA_DIR=<repo>/.runtime
./bin/relaygate validate && ./bin/relaygate apply && ./bin/relaygate smoke

# 本地打与 CI 相同结构的 release 包
make dist VERSION=dev
```

## 日常运维

```text
首启     relaygate setup [--noninteractive] [--sysctl]
         relaygate doctor [--strict-ports]   # 含 admin / drain 端点 / 双活 env

配置     relaygate render [--check-only] [--observability]
         relaygate validate                  # 端口冲突、rule→server、nft/ACL 同源校验
         relaygate server status             # 每服 canary / production 是否生效
         relaygate server enable|disable <server-01>
         relaygate acl list|add|remove …     # IP 黑白名单（写 resources → firewall apply）
         relaygate profile list|show|apply   # 游戏类型 defaults 模板
         relaygate changes [--limit N]       # 列出 backups/*/change-summary.txt

数据面   relaygate apply                 # 首次/全量（含变更摘要备份，含 defaults/acl）
         relaygate reload                # 改配置后：backup→drain→restart→ready（分阶段计时）
         relaygate rollback [STAMP]
         relaygate drain fail|ok|status  # NLB 摘流 / 恢复（DRAIN_WAIT + 控制台提示）

检查     relaygate smoke [HOST]
         relaygate canary [HOST]         # 读 resources 启用 canary 端口
         relaygate baseline
         relaygate doctor                # admin/drain/双活 + NLB/高防清单

防火墙   relaygate firewall [check|apply]  # 端口集 + defaults.nft + ACL set
Panel    sudo relaygate panel install | uninstall
多机     GATEWAYS=gateway-01,gateway-02 relaygate fleet
```

### IP 黑白名单（ACL）

`resources.yaml` 顶层 `acl`（nft 为真相源；SSH 不受约束）：

```yaml
acl:
  deny: ["1.2.3.4/32"]   # 立即丢弃
  allow: []              # 非空 = 严格模式，仅名单可进转发口
```

```bash
relaygate acl add deny 1.2.3.4/32
relaygate acl list
relaygate validate
sudo FIREWALL_CONFIRM=YES_FLUSH_NFTABLES ./bin/relaygate firewall apply
# Panel → ACL 页亦可 CRUD；改名单通常无需 reload Envoy
```

### 游戏类型 profile

预设在 `packaging/profiles/`（`default-safe`、`fps-udp-heavy`、`moba-tcp-stable`）：

```bash
relaygate profile list
relaygate profile show fps-udp-heavy
relaygate profile apply fps-udp-heavy   # 覆盖 resources defaults
relaygate validate && relaygate reload
sudo ./bin/relaygate firewall apply     # 若改了 nft 档位
```

### 变更摘要与限速观测

- `reload`/`apply` 备份含 `change-summary.txt`，对比 **servers/rules/defaults/acl**
- `relaygate changes --limit 10` 浏览历史摘要
- Panel Overview：聚合 RL + **per-rule Top**（Envoy `rl_<rule>`）；Grafana 按 `envoy_local_rate_limit` 分解

### 推荐流程（增服 → canary → production）

```bash
# 1) Panel 添加 Server（自动分配 production 端口，规则默认关闭）
#    或编辑 DataDir/resources.yaml

relaygate server status          # 查看 canary/production 是否生效
relaygate validate               # 端口冲突 / 引用检查
relaygate canary 127.0.0.1       # 探针走启用中的 canary 端口

# 2) canary 通过后放量到 production
relaygate server enable server-01
relaygate reload                 # 输出变更摘要 + 各阶段耗时；备份含 change-summary.txt
relaygate smoke

# 3) 宿主防火墙与 Envoy 端口集对齐
sudo FIREWALL_CONFIRM=YES_FLUSH_NFTABLES ./bin/relaygate firewall apply
```

### L4 维护窗口剧本（drain ↔ NLB 摘流）

云 L4（如 NLB）通常探测 Envoy `/ready`。维护时：

```bash
relaygate doctor                 # 确认 /healthcheck/* 与 admin 可达；核对 DRAIN_WAIT
relaygate drain fail             # POST /healthcheck/fail → 等 DRAIN_WAIT（未设 env 默认 15s）
# …此时 NLB 应摘流；再改配置 / reload / 升级…
relaygate reload                 # 内置：drain → restart → poll /ready → healthcheck/ok
# 或手动恢复：
relaygate drain ok               # POST /healthcheck/ok
relaygate smoke                  # 冒烟
```

`DRAIN_WAIT` 写在 `.env`（见 `.env.example`）。`reload` 使用该值；单独 `drain fail` 在未设置时默认多等一会儿（15s），给 LB 失败窗口留余量。

### 同源限流（Envoy + nft）

`DataDir/resources.yaml` 的 `defaults` 是单一意图源：

| 字段 | 生成物 |
|------|--------|
| `tcp_local_rate_limit_*` | Envoy TCP listener 本地限速（`stat_prefix: rl_<rule>`） |
| `defaults.nft.*` | `DataDir/firewall/forward-ports.nft` 中的 `FORWARD_*_RATE/BURST` |
| 启用中的 rules 端口 | Envoy listeners + `FORWARD_TCP/UDP_PORTS` |
| `acl.deny` / `acl.allow` | `ACL_DENY` / `ACL_ALLOW`（`gateway.nft` 在限速前 drop） |

`packaging/firewall/gateway.nft` 引用这些 define，不再硬编码限速数字。改档位：编辑 `resources.yaml` 或 `profile apply` → `validate` / `firewall check` → `firewall apply`。

### 双活（可选）

```text
玩家 → 云 L4 LB
         ├─ gateway-01（ENABLE_PANEL=1, PANEL_ROLE=primary）
         └─ gateway-02（ENABLE_PANEL=0, PANEL_ROLE=standby）
```

```bash
./bin/relaygate apply && ./bin/relaygate smoke
# 仅 primary：sudo ./bin/relaygate panel install
```

双活滚动：对即将变更的节点先 `drain fail`，确认 NLB 摘流后再 `reload`，另一台继续接流量；恢复后 `drain ok` / smoke。云 NLB 模板见 [`packaging/terraform/nlb/`](packaging/terraform/nlb/)。分批：`GATEWAYS=gateway-01,gateway-02 ./bin/relaygate fleet`。

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

## 仓库布局

单 Go module + 预编译 release。`core/` 是应用代码；`packaging/` 是版本化安装资产；**运行态不在源码树内**。

```text
*.example / .env.example   # seed 源（setup 复制到 DataDir）
packaging/                 # 版本化安装资产：compose、systemd、grafana、
                           # prometheus tpl、firewall、profiles、sysctl、terraform、observability
core/
  cmd/relaygate/           # 薄 main
  config/                  # 路径 / 默认值 / LoadEnv（唯一入口）
  cli/ panel/ setup/ doctor/ render/ status/ resources/ profile/
  ops/                     # 数据面运维（apply/reload/seed/firewall/changes…）
  host/                    # 宿主安装（Panel systemd），与 panel HTTP 分离
frontend/                  # Panel UI
install.sh                 # bootstrap：下载 release tar → setup/apply
Makefile                   # build / test / dist
```

### DataDir（运行态）约定

| 场景 | 默认 DataDir | 说明 |
|------|----------------|------|
| 源码开发 | `<repo>/.runtime/` | gitignore；**不是**源码目录 |
| 安装前缀 | `$INSTALL_DIR/data`（默认 `/opt/relaygate/data`） | 仅出现在安装树，不在 git 布局里展示为源码 |
| 任意覆盖 | `RELAYGATE_DATA_DIR` | 绝对路径，或相对产品根；`setup` 写入 `.env` |

`clone` 后仓库根下**没有** `data/`。`relaygate setup` 后才出现 `.runtime/`（开发）或安装目录下的 `data/`（生产）。Compose 通过 `.env` 的 `RELAYGATE_DATA_DIR` 挂载运行态文件。

### 预设配置 vs 运行配置

| 环节 | 预设（入库 / release） | 运行（gitignore / 宿主机） |
|------|------------------------|----------------------------|
| 开发 | `*.example`、`packaging/`、`frontend/`、`core/` | 本地 `.env`、`.runtime/`（setup seed） |
| 安装 | release tar 内模板 + 空 `data/` 骨架 | `/opt/relaygate/data/`、`.env`、`/etc/relaygate/secrets/` |
| 容器 | compose 挂载 `packaging/` 模板 + `${RELAYGATE_DATA_DIR}` 生成物 | named volume（Grafana/Prometheus TSDB） |
| 升级 | 新 tar 覆盖 `bin/`、`frontend/`、`packaging/` | **保留** `.env` 与 DataDir；seed 只补缺失文件 |
| 备份 | — | `DataDir/backups/`（apply/installer 自动） |

`relaygate setup` / 首次 `apply` 在目标缺失时从 `*.example` seed 到 DataDir；`--reset-defaults` 才覆盖已有 `resources.yaml` / inventory。Grafana provisioning 始终挂载 `packaging/grafana/`，不进 DataDir。Panel 强制 `PANEL_BIND` 为 loopback。

可选可观测性栈：

```bash
cd packaging/observability
cp .env.example .env && chmod 600 .env
docker compose --env-file .env up -d
```

发布：`make dist` 或 tag `v*` 触发 `.github/workflows/release.yml`，产出  
`relaygate-$VERSION-linux-{amd64,arm64}.tar.gz` + `.sha256`。

单 module：`github.com/relaygate/relaygate`。

## 命名规范

| 角色/阶段 | 英文标识 | 示例 |
|-----------|----------|------|
| 网关产品/进程 | gateway / relaygate | `gateway-01`、nft 表 `inet relaygate` |
| 后端节点 | server / upstream | `servers[].name` → `server-01` |
| 用户入口（转发规则） | rule / listener / ingress | `rule-server-01-production-tcp`、`rule-server-01-canary-udp` |
| 上游集群 | cluster | `cluster-server-01-tcp` |
| Envoy listener | listener | `listener-rule-server-01-production-tcp` |
| 指标 stat_prefix | 与 rule 对齐 | `rl_rule_server_01_production_tcp`、`tcp_rule_server_01_canary_tcp` |
| 防火墙端口集 | forward-ports / FORWARD_* | `DataDir/firewall/forward-ports.nft` |

规则名模式：`rule-<server>-<kind>-<proto>`（kind = `production` \| `canary`）。禁止在基础设施命名中使用 `game`/`player`（`meta.game_name` 等产品域字段除外）。

## 已知边界

- 固定目标转发，无跨服负载均衡
- UDP 无可靠主动健康检查
- 后端看到的是网关源 IP
- Envoy 非抗 DDoS；大流量需云高防
