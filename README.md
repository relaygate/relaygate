# RelayGate

基于 **Envoy** 的游戏 L4 网关：单机起步，可演进为双活 + 云 L4 LB。一份运行态 `resources.yaml` 驱动转发与限流；日常运维统一用二进制 `relaygate`。

## 功能特性

### 已具备

- [x] L4 TCP/UDP 固定目标转发（Envoy）与连接/PPS 限流（`rl_<rule>`）
- [x] CLI 闭环：`setup` / `doctor` / `render` / `validate` / `apply` / `reload` / `rollback` / `smoke` / `canary`
- [x] Panel（默认 `127.0.0.1:9000`）：Servers / Rules / ACL / Apply / Overview（含 per-rule Top）/ Monitoring（中文，Grafana 同源反代）
- [x] IP 黑白名单 ACL（nftables 真相源；SSH 不受约束）
- [x] 游戏类型 profile（`packaging/profiles/`）与 `defaults` 变更摘要（`changes`）
- [x] `defaults.nftables.*` 同源限流 → Envoy + `forward-ports.nft`
- [x] 规则命名 `rule-{server}-{stage}-{proto}`；渲染包 `core/render`
- [x] DataDir / `.runtime` 运行态约定；release tar 打包（`make dist` / `install.sh`）
- [x] `DRAIN_WAIT` 默认 30s，与 NLB 模板 HC（3×10s）对齐；`doctor` 过短告警
- [x] `relaygate upgrade [--drain]`：二进制/packaging 升级分流（委托 `install.sh --upgrade`）
- [x] `fleet` 分批：drain → release tar / `install.sh --upgrade` → smoke（不用 git）

### 规划中

- [ ] Panel：drain / smoke / doctor 面板化
- [ ] Panel：rollback UX、profile 管理 UI、fleet 向导
- [ ] Envoy 热加载（远期）
- [ ] 完整 WAF / Agones 集成（不做或远期）

## 架构 / 工作原理

```text
玩家 → 云 L4 LB（可选）→ gateway:ingress 端口
                         → Envoy（TCP/UDP + 本地限速）
                         → server:7777/7778
```

- **意图源**：`DataDir/resources.yaml`（servers / rules / defaults / acl）
- **生成物**：Envoy 配置、`firewall/forward-ports.nft`（由 `render` / `apply` / `reload`）
- **摘流**：Envoy admin `/healthcheck/fail|ok` + `/ready`，供 NLB 健康检查

## 快速开始 / 安装

`install.sh` **仅 bootstrap**：检测 → 下载 release tar → 解压到 `/opt/relaygate` → 调用 CLI。  
数据面 Compose、Panel、防火墙由 `relaygate` 完成。默认不在安装机 `docker build` 或整棵 git clone。

支持 Ubuntu / Debian / RHEL / Rocky / Alma / CentOS Stream / Fedora / Amazon Linux（systemd，`amd64`/`arm64`）。

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh -o /tmp/relaygate-install.sh
sudo RELAYGATE_VERSION=v0.1.0 bash /tmp/relaygate-install.sh
```

生产务必固定 **Release tag**（禁止 `master`/`main`/`latest`）。本地包：

```bash
sudo RELAYGATE_TAR=/path/relaygate-v0.1.0-linux-amd64.tar.gz bash /tmp/relaygate-install.sh
```

| 变量 | 说明 |
|------|------|
| `RELAYGATE_VERSION` | GitHub Release tag（推荐） |
| `RELAYGATE_TAR` | 本地 tar.gz，跳过下载 |
| `RELAYGATE_INSTALL_DIR` | 默认 `/opt/relaygate` |
| `RELAYGATE_DATA_DIR` | 运行态；安装默认 `$INSTALL_DIR/data` |
| `FROM_SOURCE=1` | 开发兜底：源码构建（非默认） |
| `GATEWAY_NAME` / `GATEWAY_PUBLIC_IP` | 网关身份 |
| `GATEWAY_SSH_PORT` | 默认 `30455` |
| `ENABLE_PANEL` / `ENABLE_GRAFANA` | 默认 1 |
| `APPLY_FIREWALL` | 默认 0（只校验） |

### 开发机（源码）

源码树**没有**顶层 `data/`。运行态默认写到 gitignore 的 `.runtime/`。

```bash
cp .env.example .env && chmod 600 .env
make build
./bin/relaygate setup --noninteractive
./bin/relaygate validate && ./bin/relaygate apply && ./bin/relaygate smoke
make dist VERSION=dev   # 与 CI 同结构的 release 包
```

## 日常运维（CLI）

```text
首启     relaygate setup [--noninteractive] [--sysctl]
         relaygate doctor [--strict-ports]

配置     relaygate render [--check-only] [--observability]
         relaygate validate
         relaygate server status|enable|disable <server>
         relaygate acl list|add|remove …
         relaygate profile list|show|apply
         relaygate changes [--limit N]

数据面   relaygate apply                 # 首次/全量
         relaygate reload                # resources/Envoy：backup→drain→restart→ready
         relaygate rollback [STAMP]
         relaygate drain fail|ok|status
         relaygate upgrade [--drain]     # 二进制/packaging

检查     relaygate smoke | canary | baseline | doctor

防火墙   relaygate firewall [check|apply]   # ACL/nftables-only
Panel    sudo relaygate panel install|uninstall
多机     GATEWAYS=… RELAYGATE_VERSION=… relaygate fleet
```

**变更分流**

| 变更类型 | 命令 |
|----------|------|
| ACL / nftables-only | `firewall apply`（通常无需 reload Envoy） |
| resources / Envoy | `reload` |
| 二进制 / packaging | `upgrade [--drain]` 或 `install.sh --upgrade` |

推荐增服流程：Panel 添加 Server → `canary` → `server enable` → `reload` → `smoke` → `firewall apply`。

## Panel

默认绑定 loopback：`http://127.0.0.1:9000`（Grafana 同源反代）。

| 页 | 能力 |
|----|------|
| Overview | 聚合 RL + per-rule Top |
| Servers / Rules | CRUD、启停、自动分配端口 |
| ACL | 黑白名单 CRUD（改后需 `firewall apply`） |
| Apply | 校验 → 备份摘要 → drain → 重启 Envoy |
| Monitoring | Grafana 嵌入（中文文案） |

drain / smoke / doctor / rollback / profile / fleet 目前以 **CLI** 为准（见规划中）。

## 配置（resources / DataDir / nftables）

`DataDir/resources.yaml` 的 `defaults` 是单一意图源：

| 字段 | 生成物 |
|------|--------|
| `tcp_local_rate_limit_*` 等 | Envoy TCP 本地限速（`stat_prefix: rl_<rule>`） |
| `defaults.nftables.*` | `DataDir/firewall/forward-ports.nft` 的 `FORWARD_*_RATE/BURST` |
| 启用中的 rules 端口 | Envoy listeners + `FORWARD_TCP/UDP_PORTS` |
| `acl.deny` / `acl.allow` | `ACL_DENY` / `ACL_ALLOW`（`gateway.nft` 在限速前 drop） |

```yaml
acl:
  deny: ["1.2.3.4/32"]
  allow: []              # 非空 = 严格模式
```

| 场景 | 默认 DataDir |
|------|----------------|
| 源码开发 | `<repo>/.runtime/` |
| 安装前缀 | `$INSTALL_DIR/data`（默认 `/opt/relaygate/data`） |
| 覆盖 | `RELAYGATE_DATA_DIR` |

`setup` / 首次 `apply` 在目标缺失时从 `*.example` seed；`--reset-defaults` 才覆盖已有 `resources.yaml`。

## 双活与维护窗口

```text
玩家 → 云 L4 LB
         ├─ gateway-01（ENABLE_PANEL=1, PANEL_ROLE=primary）
         └─ gateway-02（ENABLE_PANEL=0, PANEL_ROLE=standby）
```

```bash
relaygate doctor                 # 核对 /ready、DRAIN_WAIT、双活角色
relaygate drain fail             # POST /healthcheck/fail → 等 DRAIN_WAIT（默认 30s）
# 控制台确认 NLB target unhealthy 后：
relaygate reload                 # 或 upgrade --drain
relaygate drain ok               # 若未走内置 undrain
relaygate smoke
```

`DRAIN_WAIT` 写在 `.env`（见 `.env.example`）。默认 **30s**，与 NLB 模板 HC `unhealthy_threshold(3) × interval(10s)` 对齐；过短时 CLI WARN，双活/NLB 迹象下 `doctor` 硬失败。模板见 [`packaging/terraform/nlb/`](packaging/terraform/nlb/)。

### fleet 分批升级（release tar，不用 git）

```bash
export GATEWAYS=gateway-01,gateway-02
export BATCH_PAUSE_SEC=10
export RELAYGATE_VERSION=v0.1.0          # 或 RELAYGATE_TAR=/path/to.tar.gz / DEPLOY_REF
./bin/relaygate fleet
```

每台：`drain fail` → `install.sh --upgrade` → `smoke` → `drain ok`。SSH/升级失败清晰报错并停止，**无** git checkout 回退。

### 本机升级 / 回滚 / 卸载

```bash
sudo RELAYGATE_VERSION=v0.1.0 ./bin/relaygate upgrade --drain
# 或：sudo RELAYGATE_VERSION=v0.1.0 bash install.sh --upgrade
relaygate rollback
sudo PURGE=1 bash install.sh --uninstall
```

游戏后端默认：TCP `7777` / UDP `7778`；玩家入口示例 `server-01` → `:10001`（canary `:11001`）。后端防火墙只放行网关回源 IP（双活放行全部网关）。

## 命名规范（简表）

| 角色 | 示例 |
|------|------|
| 网关 | `gateway-01`；nftables 表 `inet relaygate` |
| 后端 | `servers[].name` → `server-01` |
| 规则 | `rule-server-01-production-tcp` |
| Listener / Cluster | `ingress-rule-…` / `upstream-server-01-tcp` |
| 限速 stat | `rl_rule_server_01_canary_tcp` |
| 防火墙端口集 | `DataDir/firewall/forward-ports.nft` |

基础设施命名避免 `game`/`player`（`meta.game_name` 等产品域字段除外）。

## 仓库布局

```text
*.example / .env.example   # seed 源
packaging/                 # compose、systemd、grafana、firewall、profiles、terraform…
core/                      # cmd、cli、config、ops、panel、render、resources、doctor…
frontend/                  # Panel UI
install.sh                 # bootstrap：下载 release tar → setup/apply
Makefile                   # build / test / dist
```

发布：`make dist` 或 tag `v*` → `relaygate-$VERSION-linux-{amd64,arm64}.tar.gz` + `.sha256`。  
模块：`github.com/relaygate/relaygate`。

## 已知边界

- 固定目标转发，无跨服负载均衡
- UDP 无可靠主动健康检查
- 后端看到的是网关源 IP
- Envoy 非抗 DDoS；大流量需云高防
- Panel 尚未覆盖全部 CLI 运维动作（见规划中）

## 贡献 / License

欢迎 Issue / PR。当前仓库**未附 LICENSE** 文件；使用前请与维护者确认授权条款。
