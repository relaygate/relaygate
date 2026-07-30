# RelayGate

基于 **Envoy** 的游戏 **L4 TCP/UDP 网关**。用一份 Rules-first 的 `resources.yaml` 描述上游与转发；日常运维靠 `relaygate` CLI 与 Panel（`:9000`）。

```text
玩家（下游）→ 云 L4 LB（可选）→ 网关入口 / Listener → Envoy → 上游（游戏服）
```

| 概念 | 说明 |
|------|------|
| **上游** `servers` | 需要代理到的游戏服（地址 + TCP/UDP 端口）；固定目标，非多成员 LB |
| **转发** `rules` | 入口（类型 + 监听端口 + 协议）→ 某一上游；命名 `forward-{server}-{entry}-{proto}` |
| **入口类型** `entry` | `validation`（验证）/ `production`（正式），可并行；回退 = 关正式转发 |
| **运行态** | 默认 `/opt/relaygate/data`（可用 `RELAYGATE_DATA_DIR` 覆盖） |

**已知边界：** 固定目标转发（不做跨服 LB）· UDP 无可靠主动健康检查 · 上游看到的是网关源 IP · Envoy 非抗 DDoS（大流量需云高防）· 默认 `PROXY_PROTOCOL=off`（公网直连；有云 LB 发 PROXY 时再开，见 [logging-playbook](docs/logging-playbook.md)）

产品用语与 YAML 字段对照见文末 [术语](#术语运维对照)。

---

## 目录

1. [安装](#安装)
2. [配置正式业务](#配置正式业务)
3. [日常命令](#日常命令)
4. [Panel（:9000）](#panel9000)
5. [监控与日志](#监控与日志)
6. [双活维护与升级](#双活维护与升级)
7. [排障速查](#排障速查)
8. [术语](#术语运维对照)

---

## 安装

支持 Ubuntu / Debian / RHEL / Rocky / Alma / CentOS Stream / Fedora / Amazon Linux（`amd64` / `arm64`，systemd）。

`install.sh` 只做 bootstrap：下载 release tar → 解压到 `/opt/relaygate` → 调用 CLI。Compose、Panel、防火墙由 `relaygate` 完成。

### 一键安装

默认安装 **最新 GitHub Release**：

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh | sudo bash
```

需要固定版本或使用本地包时：

```bash
# 固定 Release tag
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh | sudo RELAYGATE_VERSION=v0.1.0 bash

# 本地 tar.gz（跳过下载）
sudo RELAYGATE_TAR=/path/relaygate-v0.1.0-linux-amd64.tar.gz bash install.sh
```

### 安装变量

| 变量 | 默认 / 说明 |
|------|-------------|
| `RELAYGATE_VERSION` | 默认最新 Release；可设具体 tag（勿用 `master` / `main`） |
| `RELAYGATE_TAR` | 本地 tar.gz，跳过下载 |
| `RELAYGATE_INSTALL_DIR` | `/opt/relaygate` |
| `RELAYGATE_DATA_DIR` | `$INSTALL_DIR/data` |
| `GATEWAY_NAME` / `GATEWAY_PUBLIC_IP` | 网关身份 |
| `GATEWAY_SSH_PORT` | 安装/配置时指定（常见 `22` 或其他；示例值见 [`.env.example`](.env.example)） |
| `ENABLE_PANEL` / `ENABLE_GRAFANA` | `1` |
| `PANEL_BIND` | `0.0.0.0:9000`（对外；可改 `127.0.0.1:9000`） |
| `APPLY_FIREWALL` | `0`（安装时只校验防火墙） |

### 首启检查

```bash
relaygate setup --noninteractive   # 若安装器未跑完
relaygate doctor
relaygate smoke
```

### Panel UI 从哪来

`ui/dist` 是构建产物（已 gitignore），**不必也不应提交进仓库**。Panel 读安装前缀下的 `ui/dist`（默认 `/opt/relaygate/ui/dist`）。

| 场景 | 怎么得到 UI |
|------|-------------|
| **生产 / 一键安装** | GitHub Release 的 tar 在 `make dist` 时已先 `make ui` 再打包；`install.sh` 解压后即可用 |
| **源码开发** | 仓库内执行 `make ui`（或 `cd ui && npm ci && npm run build`），再 `make panel` / `relaygate panel` |
| **`FROM_SOURCE=1`** | 只构建 Go 二进制，**不会**自动 build UI；需事先在源码树生成 `ui/dist`，否则安装校验失败 |

---

## 配置正式业务

真相源：`$RELAYGATE_DATA_DIR/resources.yaml`（模板见仓库 [`resources.example.yaml`](resources.example.yaml)）。可用 Panel 或直接改 YAML。

### 推荐流程

1. 添加**上游**（地址、TCP/UDP 端口）
2. 添加**转发**：先开 `validation` 验证入口，测通后再开 `production` 正式入口
3. **应用配置**（Envoy）→ `smoke` / 验证
4. 需要 ACL / 主机侧放行时再 **应用防火墙**（nft）

### Apply 分流（重要）

改完意图后按变更类型选命令，**不要混用**：

| 改了什么 | 怎么应用 |
|----------|----------|
| 上游 / 转发 / Envoy 限速等 | Panel「应用配置」或 `relaygate reload`（首次全量用 `apply`） |
| ACL / 仅 nftables | Panel「应用防火墙」或 `relaygate firewall apply` |
| 二进制 / packaging | `relaygate upgrade [--drain]` 或 `install.sh --upgrade` |

### 入口类型

| `entry` | 中文 | 典型用途 |
|---------|------|----------|
| `validation` | 验证 | 旁路验收；默认可先开 |
| `production` | 正式 | 对玩家正式放量；与验证可同时开 |

命名示例：`forward-server-01-validation-tcp`（listen `11001`）、`forward-server-01-production-tcp`（listen `10001`）。

游戏服默认回源 TCP `7777` / UDP `7778`；上游防火墙只放行网关回源 IP。

### ACL 速记

```yaml
acl:
  deny: ["1.2.3.4/32"]   # 黑名单
  allow: []              # 非空 = 严格白名单模式
```

SSH 不受 ACL 约束。改 ACL 后执行 `firewall apply`（无需 `reload` Envoy）。

---

## 日常命令

```bash
# 诊断 / 检查
relaygate doctor
relaygate smoke [HOST]
relaygate canary [HOST]          # 对指定主机做验证探测
relaygate baseline

# 配置落地
relaygate validate
relaygate apply                  # 首次 / 全量
relaygate reload                 # resources→Envoy：backup→drain→restart→ready
relaygate firewall check|apply
relaygate rollback [STAMP]

# 摘流（配合云 LB）
relaygate drain fail|ok|status

# 升级 / 多机
relaygate upgrade [--drain]
GATEWAYS=gateway-01,gateway-02 relaygate fleet   # 默认升到最新 Release
```

| 场景 | 命令 |
|------|------|
| 上游启停 | `relaygate server enable\|disable <server>` |
| 启用全部正式入口 | `relaygate server enable --all-production` |
| ACL | `relaygate acl list\|add\|remove deny\|allow CIDR` |
| 游戏档位 profile | `relaygate profile list\|show\|apply NAME` |
| 变更摘要 | `relaygate changes [--limit N]` |
| Panel | `sudo relaygate panel install\|uninstall` |
| 版本 | `relaygate version` |

完整子命令说明：`relaygate help`。

---

## Panel（:9000）

默认 `PANEL_BIND=0.0.0.0:9000`（对外监听；gateway nft 放行 `9000/tcp`）。浏览器打开：

```bash
http://<GATEWAY_PUBLIC_IP>:9000
```

仅本机访问时改 `PANEL_BIND=127.0.0.1:9000`，并可用 SSH 隧道：`ssh -p <GATEWAY_SSH_PORT> -L 9000:127.0.0.1:9000 root@<GATEWAY_PUBLIC_IP>`。

| 页 | 做什么 |
|----|--------|
| 总览 | 限速指标、转发限速 Top |
| 上游 / 转发 | CRUD；入口类型、listen、开关；「启用正式入口」= 开正式转发 |
| ACL 集合 | 黑白名单（改后需应用防火墙） |
| 配置 | 整份 `resources.yaml` 预览 / 编辑 / 导出 |
| 应用 | 变更摘要；**应用配置** 与 **应用防火墙** 分按钮 |
| 运维 | 诊断、摘流、冒烟/验证、防火墙检查、运维档位 |
| 变更 | 备份历史与回滚 |
| 监控 | 侧栏新窗口打开 Grafana（同源反代 `/grafana/`） |

- `PANEL_ROLE=standby` 时拒写
- 写操作审计：`$RELAYGATE_DATA_DIR/panel-audit.log`

---

## 监控与日志

链路：**Envoy TCP access** → Fluent Bit → Loki → Grafana（经 Panel `/grafana/` 反代）。

| 节点 | `.env` 中 `COMPOSE_PROFILES` |
|------|------------------------------|
| 主管理 | `with-grafana,with-loki,with-logs` |
| 从节点 | `with-logs`（并设 `LOKI_HOST=<中心私网>`） |

启用步骤、LogQL、与 Prometheus 对齐的排查见 **[docs/logging-playbook.md](docs/logging-playbook.md)**。看板：**TCP Session Logs**。

本地日志：`${RELAYGATE_DATA_DIR}/envoy/logs/tcp-access.json`。

---

## 双活维护与升级

```text
玩家 → 云 L4 LB
         ├─ gateway-01（ENABLE_PANEL=1, PANEL_ROLE=primary）
         └─ gateway-02（ENABLE_PANEL=0, PANEL_ROLE=standby）
```

### 单节点安全变更

```bash
relaygate doctor
relaygate drain fail             # 等 DRAIN_WAIT（默认 30s，对齐 NLB 3×10s HC）
# 控制台确认 target unhealthy 后：
relaygate reload                 # 或 upgrade --drain
relaygate drain ok               # 若未自动恢复
relaygate smoke
```

`DRAIN_WAIT` 写在 `.env`（见 [`.env.example`](.env.example)）。NLB 模板：[`packaging/terraform/nlb/`](packaging/terraform/nlb/)。

### 升级 / 回滚 / 卸载

```bash
sudo relaygate upgrade --drain              # 默认最新 Release；可加 RELAYGATE_VERSION=vX.Y.Z
relaygate rollback
sudo PURGE=1 bash install.sh --uninstall
```

---

## 排障速查

| 现象 | 先查 |
|------|------|
| `/ready` 非 LIVE / LB 不健康 | `relaygate doctor`；是否误摘流 `drain status` |
| 转发不通 | 转发是否 `enabled`；入口类型是否开错；上游 TCP/UDP 端口；`smoke` / `canary` |
| 改了 YAML 不生效 | 是否走了对应 Apply（`reload` vs `firewall apply`） |
| Panel 打不开 | `PANEL_BIND`、防火墙 9000、systemd `relaygate-panel` |
| 无会话日志 | `COMPOSE_PROFILES`、Fluent Bit/Loki 就绪；见 [logging-playbook](docs/logging-playbook.md) |
| 限速过猛 / ACL 误伤 | `defaults` 限速字段、`acl.deny`/`allow`；SSH 不受 ACL 影响 |

### 关键路径

| 路径 | 用途 |
|------|------|
| `/opt/relaygate` | 安装前缀 |
| `$RELAYGATE_DATA_DIR/resources.yaml` | 业务意图 |
| `$RELAYGATE_DATA_DIR/envoy/` | Envoy 生成配置与日志 |
| `$RELAYGATE_DATA_DIR/firewall/` | nft 生成物 |
| `$RELAYGATE_DATA_DIR/backups/` | 变更备份（回滚） |
| `/etc/relaygate/secrets/` | Panel / Grafana 密码文件 |

---

## 术语（运维对照）

RelayGate 面向 **南北向** 边缘转发（玩家 → 网关 → 游戏服），不做东西向服务网格。对外产品用语对齐 Envoy / L4 惯例；YAML / CLI 标识符保持兼容，**不改字段名**。

### 推荐对外用语

| 中文（对外） | English | YAML / 运行标识 | 含义 |
|--------------|---------|-----------------|------|
| **下游** / 客户端 / 玩家 | Downstream / Client / Player | （无独立资源） | 连入网关的一侧（Envoy *downstream*） |
| **入口** | Entry | `rules[].entry` + `listen_port` | 玩家侧接入点：类型 + 监听端口 + 协议 |
| **验证入口** / **正式入口** | Validation / Production entry | `validation` / `production` | 入口类型；可并行；**不是**金丝雀阶段名 |
| **Listener（监听器）** | Listener | 生成名 `ingress-{forward名}` | Envoy 绑定的 listen；由一条转发生成 |
| **上游**（口语可称游戏服） | Upstream | `servers[]`，名如 `server-01` | 网关回源目标；域内常对应区服/房间服实例 |
| **转发** | Forward | `rules[]`，名如 `forward-…` | 一条「入口 → 上游」的 L4 映射 |
| **ACL** | ACL / Access list | `acl.deny` / `acl.allow` | 谁能连上入口（nft） |
| **限速指标** | Rate-limit metric | `rl_<forward名>` | Envoy 本地限速 stat |

受众提示：对运维/平台用 **上游 / Upstream**；对玩法/发行侧可用 **游戏服** 作同义说明，避免再引入「后端节点」「Target Group」等第二套主词。

### 与配置字段的映射（兼容）

| 产品用语 | 保留的标识符 | 展示层怎么叫 |
|----------|--------------|--------------|
| 上游 | `servers` / `server` / CLI `relaygate server` | Panel：**上游** / **Upstreams**（勿改 YAML key） |
| 转发 | `rules` / 名 `forward-*` | Panel：**转发** / **Forwards** |
| 入口类型 | `entry` | **验证** / **正式**（Validation / Production） |
| 默认回源端口 | `defaults.backend_tcp_port` / `backend_udp_port` | 文档称「默认上游端口」；`backend_*` 为历史字段名 |
| Envoy cluster | 生成 cluster（每上游×协议） | 产品层仍叫上游；本产品固定单目标，不强调 Cluster / Pool |

### 行业对照（选型结论）

| 常见叫法 | 本产品取舍 |
|----------|------------|
| Envoy Downstream / Upstream / Listener / Cluster | **采用** Downstream、Upstream、Listener；Cluster 仅实现细节 |
| HAProxy Frontend / Backend | 入口 ≈ Frontend；上游 ≈ Backend——对外统一用 Upstream，少说 Backend |
| NGINX `upstream {}`、云 LB Target Group | 概念接近 `servers`；因无跨成员 LB，不对外主推 Target Group / Pool member |
| Ingress（K8s）/ VIP / 公网 Endpoint | VIP/公网 IP 属云 LB；本产品用 **入口 + Listener**，避免与 K8s Ingress 混淆 |
| Route / Virtual host（偏 L7） | L4 固定转发用 **转发（Forward）**，不用 Route 作主词 |
| 区服 / 房间服 | **域内同义**于上游实例，写在说明里即可，不作 YAML 主词 |

### 不建议混用的旧说法

| 避免 | 原因 | 改用 |
|------|------|------|
| 把游戏服叫「下游」 | 与 Envoy *downstream*=客户端相反 | **上游** / 游戏服 |
| 「上游节点」与「网关节点」并列且不加限定 | 「节点」易指双活 gateway-01/02 | 上游用 **上游**；机器用 **网关** |
| 入口 / Listener / Ingress 三词互换 | Ingress 易联想到 K8s | **入口**（意图）+ **Listener**（运行态） |
| 把 validation/production 叫金丝雀 / 灰度阶段 | 二者是并行入口类型，不是流水线阶段 | **验证入口** / **正式入口** |
| 主推 Backend、Route、Cluster、Target Group | 与现有 Panel/文档主词分叉 | 文档映射表中作同义即可 |
| 把 `servers` 改名叫 `upstreams`（破坏兼容） | 标识符稳定优先 | 仅展示层叫 Upstream |

---


## License

[MIT](LICENSE) © 2026 RelayGate
