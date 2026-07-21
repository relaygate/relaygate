# RelayGate

基于 **Envoy** 的游戏 L4 网关：单机起步，可演进为双活 + 云 L4 LB。一份运行态 `resources.yaml` 驱动转发与限流；日常运维统一用二进制 `relaygate`。

## 功能特性

### 已具备

- [x] L4 TCP/UDP 固定目标转发（Envoy）与连接/PPS 限流（`rl_<forward>`）
- [x] CLI 闭环：`setup` / `doctor` / `render` / `validate` / `apply` / `reload` / `rollback` / `smoke` / `canary`
- [x] Panel（默认 `127.0.0.1:9000`）：Servers / 转发规则 / ACL / Apply / Overview（含 per-rule Top）/ Monitoring（中文，Grafana 同源反代）
- [x] IP 黑白名单 ACL（nftables 真相源；SSH 不受约束）
- [x] 游戏类型 profile（`packaging/profiles/`）与 `defaults` 变更摘要（`changes`）
- [x] `defaults.nftables.*` 同源限流 → Envoy + `forward-ports.nft`
- [x] 转发规则命名 `forward-{server}-{stage}-{proto}`；渲染包 `core/render`
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
| Overview | 聚合 RL + 转发规则限速 Top |
| Servers / 转发规则 | CRUD、启停、自动分配端口 |
| ACL | 防火墙黑白名单 CRUD（改后需 `firewall apply`） |
| Apply | 校验 → 备份摘要 → drain → 重启 Envoy |
| Monitoring | Grafana 嵌入（中文文案） |

drain / smoke / doctor / rollback / profile / fleet 目前以 **CLI** 为准（见规划中）。

## 配置（resources / DataDir / nftables）

`DataDir/resources.yaml` 的 `defaults` 是单一意图源：

| 字段 | 生成物 |
|------|--------|
| `tcp_local_rate_limit_*` 等 | Envoy TCP 本地限速（限速指标 `rl_<转发规则名>`） |
| `defaults.nftables.*` | `DataDir/firewall/forward-ports.nft` 的 `FORWARD_*_RATE/BURST` |
| 启用中的转发规则端口 | 入口 Listener + `FORWARD_TCP/UDP_PORTS` |
| `acl.deny` / `acl.allow` | 防火墙集合 `ACL_DENY` / `ACL_ALLOW`（`gateway.nft` 在限速前 drop） |

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

## 命名规范

产品里**只有** `resources.yaml` 的 `rules[]`（标识符前缀 `forward-`）叫「转发规则」。入口 / 上游 / 限速指标 / nft 集合都是其**派生物或并列资产**，不要统称「规则」，也**不用** `rule-` 前缀。

### 术语表（中英对照）

| 中文 | 英文标识前缀 / 键 | 一句话定义 | 命名格式与示例 |
|------|-------------------|------------|----------------|
| 网关实例 | `gateway` | 一台 RelayGate 主机（Envoy + nft） | `gateway-{nn}` → `gateway-01`；nft 表 `inet relaygate` |
| 后端节点 | `server` | 游戏进程所在机器，回源目标 | `server-{nn}` → `server-01`（`servers[].name`） |
| **转发规则** | `forward-` / YAML 键 `rules[]` | 某入口端口 → 某后端某协议（转发/代理） | `forward-{server}-{stage}-{proto}` → `forward-server-01-production-tcp` |
| 阶段 | `production` / `canary`（`kind`） | 转发规则生命周期：旁路验证 vs 正式 | `canary` / `production` |
| 入口 | `ingress-` | Envoy 用户入口 Listener（由转发规则 1:1 生成） | `ingress-{forwardName}` → `ingress-forward-server-01-canary-tcp` |
| 上游 | `upstream-` | Envoy 回源 Cluster（按 server+协议，多条转发可共用） | `upstream-{server}-{proto}` → `upstream-server-01-tcp` |
| 限速指标 | `rl_` + forward 名 | Envoy 本地限速 **stat_prefix**，不是转发规则名本身 | `rl_{forward名,-→_}` → `rl_forward_server_01_canary_tcp` |
| 防火墙端口集 | `FORWARD_*` | nft 放行的 TCP/UDP 入口端口集合 | `FORWARD_TCP_PORTS` / `FORWARD_UDP_PORTS`（`forward-ports.nft`） |
| 防火墙限速 | `FORWARD_*_RATE/BURST` | 主机侧每 IP 新建连接 / PPS 限速常量 | `FORWARD_TCP_NEW_CONN_RATE` 等 |
| ACL 集合 | `ACL_*` | 访问控制名单/集合（SSH 不受此约束） | `ACL_DENY` / `ACL_ALLOW`；严格模式 `ACL_ALLOW_STRICT` |

### 派生对照（同一条转发规则）

以 `forward-server-01-canary-tcp`（listen `11001`）为例：

| 层级 | 标识 |
|------|------|
| 转发规则（真相源） | `forward-server-01-canary-tcp` |
| 入口 | `ingress-forward-server-01-canary-tcp` |
| 上游 | `upstream-server-01-tcp`（与 production 等同 server+协议共用） |
| 限速指标 | `rl_forward_server_01_canary_tcp` |
| 防火墙端口集成员 | `11001` ∈ `FORWARD_TCP_PORTS` |

### 中文用词约定

| 场景 | 推荐说法 | 避免 |
|------|----------|------|
| Panel / 文档标题 | **转发规则**（对应 `forward-`） | `rule-` 前缀；单独「规则」（易与 nft/Envoy 混淆） |
| 口语简称（上下文已明） | 「这条转发」= 转发规则 | 把入口 / `rl_*` / `FORWARD_*` 也叫规则 |
| 入口 / 上游 | **入口**（`ingress-`）、**上游**（`upstream-`） | listener / cluster 口语替代中文产品词 |
| ACL 页 | **ACL 集合** / 访问控制名单 | 「防火墙规则」（易与整份 nft ruleset 混淆） |
| `firewall apply` | 应用主机防火墙配置 | 「应用规则」而不说明是 nft |
| Overview 限速 Top | 转发规则 + 限速指标 | 把 `rl_*` 当成转发规则名单独展示 |

基础设施命名避免 `game`/`player`（`meta.game_name` 等产品域字段除外）。代码标识（YAML 键 `rules`、Go 类型 `Rule`、路径 `/rules`）可保留以降低 diff；**对外标识符与文档一律用 `forward-`**，UI/文档优先「转发规则」。

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
