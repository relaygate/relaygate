# RelayGate：主管控 + xDS 热更新 + 弹性伸缩

> **状态：已评审拍板（2026-08-02）；P3 舰队同步 / P4 Panel 一键扩缩容（L2+，NLB 仍人工）已落地**  
> **机群控制面策略：已拍板（2026-08-03）**——稳态以 **Agent 拉取（pull）** 为主路径；SSH 仅首次接入引导与应急（见下文专章）  
> 基线：`/data/relaygate`（README / xDS 设计稿 / Panel / CLI / inventory / NLB / 观测）  
> 相关工程方案：[热更新 xDS](hot-update-xds.md) · [Envoy 能力路线图（已拍板）](envoy-capability-roadmap.md) · [日志 Playbook](logging-playbook.md) · [舰队运维 Panel](fleet-ops.md) · [README 双活维护](../README.md#双活维护与升级)

---

## 执行摘要

1. **Management Primary（本机）** 做配置/观测/运维中心；**Data-plane Fleet** 为同质 **active-active** 数据面（含 Primary 本机 Envoy），进出可选云 L4 NLB——不是冷备待命。
2. **配置变更 ≠ 伸缩**：意图经 Primary 发布版本 → 各节点 **Agent 拉取 → 本机落盘 → 本机 ADS HotApply**；节点加减走 **NLB/TG + drain**。勿把容器重启当热更新。
3. **不做 N 套 Panel（D13）**：仅 Primary 跑完整 Panel UI/API；Secondary = Envoy + **瘦本机 xDS / Agent**（注册、心跳、拉配置）+ 日志船；Grafana/Loki **中心一份**，不每台一套。
4. **不引入全局 Istiod**；Envoy 只连本机 `127.0.0.1` ADS。
5. **日常同步不依赖 Primary 持全舰队 root 私钥**；SSH 降级为 bootstrap（装 Agent / 灌节点凭证）与应急。当前 P3 仍以 SSH 推送落地，按分期迁到 Agent 拉取。
6. 路线：**P0→P1→P2→P3→P4**（MVP）；**P5 ASG/云 API 可选非默认**。伸缩第一期目标 **L2 半自动**；NLB 变更维持人工/Terraform（不接云 SDK）。
7. **与数据面能力边界对齐（2026-08-03）**：Envoy 侧坚持 L4 + 本机 ADS（CDS+LDS 内联 endpoint）；**默认不做 EDS / L7 / TLS·SDS / WASM**；观测继续中心 Loki+Prom。细节与分期见 [envoy-capability-roadmap.md](envoy-capability-roadmap.md)。机群本文只约束「谁管配置、怎么扩缩」；数据面「开哪些 Envoy 特性」以该路线图为准。

---

## 决策摘要（已通过）

- [x] **D1** Management Primary + Data-plane Fleet + 可选 NLB；节点对等接流量（备用 = active-active，非冷备）。
- [x] **D2** 中心意图 + 每节点本机 ADS；不引入全局 Istiod。
- [x] **D3** Hot 不 drain；Hard / 升级 / 缩容才 drain。
- [x] **D4** 意图源 = Primary `resources.yaml`；**过渡期**可用 SSH 推送 + 远程 HotApply（P3 已落地）。
- [x] **D5** Deploy 身份 = `relaygate_fleet` + BatchMode（灌钥见 [`packaging/scripts/push-fleet-key.sh`](../packaging/scripts/push-fleet-key.sh)）——用于 bootstrap / 应急，非稳态日常同步主路径。
- [x] **D6** 若误启 Panel，保留 `PANEL_ROLE=standby` 拒写，禁止双主。
- [x] **D7** 中心 Loki + 多 gateway Prom；伸缩时半自动更新 scrape。
- [x] **D8** 第一期伸缩成熟度 **L2**（向导+脚本+人工 TG）；**L3 ASG 仅 P5 可选**。
- [x] **D9** 路线：P0 → P1 → P2 → P3 → P4。
- [x] **D10** 舰队部分失败可重试，不自动全局 HardRestart。
- [x] **D11** 产品不接云 SDK；NLB 变更维持 Terraform/控制台。
- [x] **D12** 验收：Hot 时 Envoy PID / 无关长连接不变；Expand / Shrink 可重复演练。
- [x] **D13 不做 N 套 Panel**：仅 Primary 完整 Panel；Secondary = Envoy + 瘦 xDS / Agent + 日志船；无每节点 Grafana/Loki 中心栈。管理与观测集中在 Management Primary。
- [x] **D14 机群控制面（2026-08-03）**：稳态以 **Secondary Agent 拉取** 为主路径；SSH 仅首次接入引导与应急。详见专章。

---

## 机群控制面策略（已拍板 · 2026-08-03）

<a id="fleet-control-plane-strategy"></a>

> **入口说明**：长期机群「谁下发配置、日常是否靠 SSH」以本节为准；伸缩剧本、inventory、P3/P4 操作步骤见后文各章。不另拆独立策略文，避免文档碎片。

### 拍板原文

1. **稳态主路径 = Agent 拉取（pull）**：Secondary 上的 Agent 向 Management Primary **注册 / 心跳 / 拉取配置版本** → 本机落盘 `resources.yaml`（或等价意图）→ **本机瘦 xDS HotApply**。Envoy 始终只连本机 ADS。
2. **SSH 降级**：仅用于 **bootstrap**（安装 Agent、灌节点凭证）与 **应急**（Agent 不可用时人工介入）。日常配置同步 **不**要求 Primary 长期持有全舰队 root 私钥并主动 scp。
3. **与已拍板边界对齐**：不做 N 套 Panel（D13）；不引入全局 Istiod / 远程共享 xDS；NLB/TG 变更维持人工或 Terraform；产品 **不接云 SDK**。
4. **分期落地**（与现网 P3 SSH 推送并存，再切换默认）：
   - **A 并存**：保留 SSH `fleet-sync`；Agent 拉取可灰度，漂移检测同时覆盖两种通道。
   - **B 默认 Agent**：Panel/CLI「同步配置」以拉取/催促拉取为主；SSH 推送降为高级/应急开关。
   - **C 收紧 SSH**：日常运维不再依赖全舰队 deploy 私钥在线；私钥仅 bootstrap 窗口与受控应急使用。
5. **现状说明**：P3/P4 已落地的同步与扩容向导仍走 SSH 推送——属过渡实现，**不改变**本节长期方向；工程实现另开任务，本文不改代码。

### 稳态数据流（目标）

```text
[意图] Primary resources.yaml（版本 / hash）
   ↓ Secondary Agent：注册 · 心跳 · 拉配置版本
[节点磁盘] 同质意图落盘
   ↓ 本机瘦 xDS HotApply
[本机 ADS] Snapshot → Envoy ACK（仅 127.0.0.1）
   ↓（正交）
[NLB TG] 是否接新下游流量（人工 / Terraform）
```

### 角色对照（稳态）

| 角色 | 做什么 | 不做什么 |
|------|--------|----------|
| Management Primary | 唯一可写意图源；接受 Agent 注册/心跳；发布配置版本；中心观测 | 日常不靠持全舰队 root 钥推文件；不跑全局 Istiod |
| Secondary Agent | 注册、心跳、拉版本、落盘、触发本机 HotApply | 不跑完整 Panel；不对外暴露可写管理面 |
| SSH / deploy 钥 | 首次接入、装 Agent、灌凭证；应急推送 | 不作为稳态日常 sync 主路径 |
| NLB / TG | 节点进出流量成员（人工） | 产品内不调云 API |

### 与 D13 / 本机 ADS / NLB / 云边界

- **D13**：Secondary 仍是 Envoy + 瘦控制面（Agent / 本机 ADS）+ 日志船；观测中心一份。
- **本机 ADS**：拉取只解决「意图如何到节点磁盘」；应用到 Envoy 的路径不变——本机 HotApply。
- **NLB**：扩缩容与配置分发正交；TG register/deregister 仍人工。
- **不接云 SDK**：Agent 通道是产品内机群控制，不是云厂商控制面替代。

### 安装与连接

#### Primary 暴露什么

| 出口 | 拍板 | 说明 |
|------|------|------|
| **管理 UI + 运维 API** | 内网 HTTPS（或同机反代），默认与 Panel 同源 | 运维浏览器入口；Standby 拒写不变 |
| **舰队 Agent 入口** | MVP：同一 Panel 进程挂节点 API（如 `/api/agent/...`）；勿对公网裸奔 | 不另起控制面集群；进阶可拆独立内网监听（仍同主机） |
| **本机 ADS / Envoy admin** | **仅 loopback** | 永不对舰队开放 |
| **Loki / Grafana** | 中心一份；边缘只出站推日志 | Secondary 不启完整观测栈（D13） |

网络：Agent → Primary 走管理网 / VPC；安全组只放行节点源 → Primary 管理口。公网只保留业务数据面（及可选 NLB）。

#### 运维可见的接入链路

```text
[运维在 Primary Panel]
  预创建节点名 → 生成一次性 Token / 引导命令（或 Expand bootstrap）
        ↓
[新节点：数据面 + 舰队 Agent + 本机瘦 xDS]
  持有 PRIMARY_URL + 节点 Token
        ↓
① 注册 / 首次报到（名称、能力、本机身份）
② 拉取当前发布配置版本 → 写入本机 resources.yaml
③ 本机 HotApply（瘦 xDS → Envoy ACK）
④ 周期心跳：在线、已应用版本/hash、简要健康
        ↓
[机群列表] 已连接 / 已同步 / 漂移 / 离线
```

- **凭证**：每节点独立 Token（`AGENT_TOKEN_FILE`，权限收紧）；Primary 只存可校验的哈希或密文，**不**把全舰队 root SSH 私钥当日常通道。
- **配置版本**：Primary 在「发布」时生成单调版本号（或 content hash）；Agent 对比本地已应用版本，不一致则拉取全文（MVP 整份 `resources.yaml`，不做差分）。
- **加入模型**：主控预创建节点并下发 Token（不允许匿名自助入群）。
- **发布粒度**：舰队同质一个当前版本（与今日同质同步一致）；不做「仅部分节点」多版本并行。
- **推送时效**：MVP 定时拉取即可；发布后轻量唤醒（长轮询 / webhook）为可选增强，不阻塞主路径。

#### 舰队 Agent 与本机瘦 xDS

| 职责 | 舰队 Agent | 本机瘦 xDS |
|------|------------|------------|
| 跨机拿意图 | ✅ | ❌ |
| 写本机 `resources.yaml` | ✅ | ❌（只读盘发布快照） |
| 对 Envoy 推 ADS | ❌ | ✅ |
| 心跳 / 机群状态 | ✅ | ❌（可提供本机 xDS 就绪供 Agent 汇总） |

Envoy **永远**只连本机 ADS。Agent **不**替代 ADS，也 **不**把 Envoy 指到远程 Primary。

#### TLS 分期

| 分期 | 传输 | 鉴权 | 适用 |
|------|------|------|------|
| **MVP** | 内网 HTTP 可接受（管理网隔离） | 节点 Bearer Token | 私有 VPC、无跨租户 |
| **进阶** | HTTPS（内网证书或私有 CA） | Token 保留 | 默认加固 |
| **加强** | mTLS（节点证书） | 证书 + 可选 Token | 合规 / 多租户边界 |

MVP **不**阻塞在 mTLS；安装与文档须写清「勿把 Agent 口暴露公网」。

#### Secondary 安装清单（目标）

| 包含 | 不包含（默认） |
|------|----------------|
| `bin/relaygate`（含 `xds serve`、reload、doctor、smoke、**agent**） | 完整 Panel UI 常驻 / 可写管理面 |
| Compose：Envoy +（可选）日志船 `with-logs` + 本机 exporter | 本机 Grafana / Loki 中心栈 |
| systemd：`relaygate-xds`（或等价）+ **`relaygate-agent`** | `relaygate-panel`（`ENABLE_PANEL=0`） |
| `.env` + `AGENT_TOKEN_FILE` + 数据目录骨架 | Primary 的 inventory 私钥、全量舰队密钥 |

同一 release tar；安装器按角色开关组件（已有 `ENABLE_PANEL`），增量启用 Agent 单元。首次装机可用一次性引导命令或云用户数据把「数据面 + Agent」装上并注入凭证；之后配置变更走 Agent 拉取。正式产品表面不提供 SSH 日常同步（见 [product-surface-agent.md](product-surface-agent.md)）。

**用户步骤（人话）**：① 主控 Panel → 机群：预创建节点 / 生成加入 Token；② 目标机执行一次性引导（Panel 给出的命令 / 预置镜像）；③ 节点自动报到并首次拉取后显示「已对齐」；④ 若用 NLB，仍人工挂目标组。

### 配置与密钥

| 位置 | 文件 / 数据 | 作用 |
|------|-------------|------|
| Primary | `resources.yaml` | **唯一业务意图源**（保持不变） |
| Primary | 节点注册表（MVP 文件，如 `data/fleet/nodes.yaml`） | 名称、角色、Token 摘要、最后心跳、已报版本 |
| Primary | 配置发布版本（如 `data/fleet/config-version`） | 当前发布号 / hash、时间、操作者 |
| Primary | `inventory/gateways.env` | 过渡期 SSH 通讯录（bootstrap / 应急）；身份字段迁到注册表后，日常路径不再要求 SSH 字段 |
| Secondary `.env` | `GATEWAY_NAME`、`PRIMARY_URL`、`AGENT_TOKEN_FILE`、`XDS_ENABLED=1`、`ENABLE_PANEL=0`、`LOKI_HOST` 等 | 在现有 secondary 模板上增量；无完整 Panel / Grafana 管理员密码 |

| 密钥 | 位置 | 勿 |
|------|------|-----|
| Panel 管理员密码 | Primary `/etc/relaygate/secrets` | 进 Git / 进 Secondary |
| 舰队 deploy SSH 私钥 | **仅** Primary 宿主（bootstrap / 应急） | 日常同步路径；Panel DB；inventory 明文 |
| 节点 Agent Token | **该节点**文件 + Primary 侧校验材料 | Primary 存可逆明文全集 |
| 业务数据面证书（若有） | 节点本地 / 另案 | 与 Agent Token 混用同一文件 |

### 失败行为与确认词

| 情况 | 机群列表 | 用户下一步 |
|------|----------|------------|
| 长时间无心跳 | **离线** | 查管理网与安全组；节点 `doctor`；确认 Agent 服务在跑 |
| Primary 短暂不可用 | 保持上次已应用配置继续转发；可标「暂无法报到」 | 恢复后自动续心跳；无需重启 Envoy |
| 版本冲突 / 校验失败 | **配置异常 / 未同步** | 在 Primary 重新发布；或应急对照 hash |
| Token 错 / 吊销 | **未授权** | 主控轮换该节点 Token，重新下发引导 |
| 拉取成功但本机 HotApply 失败 | **已下载、应用失败** | 看节点日志；修好后 Agent 重试或「重试应用」 |
| 部分节点未跟上 | **漂移** | 查该节点拉取/应用；**不**自动全局 Hard 重启 |

| 操作 | 是否需确认 | 确认词 |
|------|------------|--------|
| Panel **发布**新配置版本（舰队可见） | **是** | `PUBLISH_FLEET`（产品表面见 [product-surface-agent.md](product-surface-agent.md)；**勿**复用 `FLEET_SYNC`） |
| 本机 HotApply | 已有 | `HOT_APPLY` |
| 节点接入 / 退役 / 吊销 Token | **是** | `FLEET_JOIN` / `FLEET_LEAVE` / `REVOKE_AGENT`（以产品表面为准） |
| 硬重启 / 防火墙 | 已有 | `RELOAD_ENVOY` / … |
| Agent 自动拉取已发布版本 | **否**（执行已确认的发布） | — |

Standby / 只读角色：**禁止**发布与扩缩容；引导到 Management Primary。正式产品表面（菜单 / 主命令）**不**保留 SSH 推送或「应急 sync」入口——见产品表面文；SSH 仅出现在安装引导与宿主排障手册。

### MVP 竖切（工程实现时）

避免：差分协议、独立 Agent 微服务、DB、mTLS、云注册、替换 NLB 流程。

1. Primary：文件版节点表 +「发布配置版本」（整份 `resources.yaml` + version/hash）+ 确认词 `PUBLISH_FLEET`。
2. Secondary：systemd 常驻 Agent：Token 鉴权 → 轮询拉版本 → 落盘 → 调现有本机 HotApply / `xds apply`。
3. 心跳：版本 + 在线；Panel 机群列表：**已对齐 / 未对齐或异常 / 离线**。
4. 安装：Secondary 模板增加 `PRIMARY_URL` + Token 文件；接入引导注入；首次装机可走一次性命令（文档/脚本，非 Panel 日常菜单）。
5. 产品表面废除 `fleet-sync` 主语义；排障不占正式 IA。

**明确不做（MVP）**：全局 Istiod、Envoy 直连 Primary、N 套 Panel、Agent 替代 ADS、配置加密差分、自动 NLB register。

---

## 0. 概念澄清

| 原话倾向 | 准确含义 | 采用 |
|----------|----------|------|
| 「备用网关」 | active-active 同质数据面 | Fleet 成员，流量对等 |
| 「主网关」 | 管理面 primary，非「只主接点流量」 | **Management Primary** |
| 「弹性伸缩」 | 同质节点进出 LB TG + 配置同质下发 | Expand / Shrink |
| 「统一配置管理」 | 意图源 → 分发 → 本机 xDS → LB 成员 | 分层，见 §5 |
| 「热更新」 | Envoy PID 不变的 CDS/LDS | HotApply；非 `docker restart` |

```text
下游客户端 → [可选 NLB] → Fleet{gw-01…gw-N} Envoy → 上游服务
                              ↑
                 Management Primary：意图 / 观测 / 运维触发
```

---

## 1. 目标架构

### 1.1 进程矩阵（Primary vs Secondary）

| 组件 | Management Primary | Secondary（数据面） |
|------|--------------------|---------------------|
| **Envoy** | ✅ 接流量（与舰队对等） | ✅ 接流量 |
| **完整 Panel**（UI/API 可写） | ✅ `ENABLE_PANEL=1` `PANEL_ROLE=primary` | ❌ **禁止**作默认；勿每台一套 |
| **本机 ADS（xDS）** | ✅ 可嵌 Panel，或同进程 | ✅ **瘦进程**：常驻 Agent（拉配置 + 本机 ADS），或 Apply 时短时拉起 ADS → 推快照 → 退出 |
| **Grafana / Loki 中心** | ✅（`with-grafana,with-loki`） | ❌ 不启中心栈 |
| **日志船**（Fluent Bit） | ✅ `with-logs` | ✅ `with-logs` → `LOKI_HOST=<中心>` |
| **Prometheus** | 中心 scrape / 联邦多 `gateway` | 可本机 exporter；targets 在中心登记 |
| **Deploy SSH** | bootstrap / 应急持 `relaygate_fleet`；**稳态日常 sync 不依赖**全舰队 root 私钥 | 仅接受公钥，BatchMode；Agent 拉取为主（见专章） |

典型 env：主控见 `packaging/control/env.example`；节点见 `packaging/node/env.example`（`ENABLE_PANEL=0`、`PRIMARY_URL`、`AGENT_TOKEN_FILE`、`COMPOSE_PROFILES=with-logs`）。历史 SSH inventory：[gateways.env.example](../packaging/shared/gateways.env.example)（不进正式菜单）。

**若误启 Panel**：`PANEL_ROLE=standby` 拒写（D6）——兜底，**不是** Secondary 推荐形态。

### 1.2 逻辑图

```text
下游客户端 → [云 L4 NLB 可选] ┬→ Envoy gw-01 ← 本机 ADS（Panel 内嵌或同宿主）
                             ├→ Envoy gw-02 ← Agent 拉配置 + 本机瘦 xDS
                             └→ Envoy gw-N  ← 同上

Management Primary（本机）
  Panel UI/API ──写──► resources.yaml（意图源 · 版本/hash）
       │                      ▲
       │ 本机 HotApply        │ Agent：注册 / 心跳 / 拉配置版本
       ▼                      │
  本机 ADS :18000        各 Secondary 落盘 + 本机瘦 ADS HotApply
       │                      │
       └──────── Envoy 仅连 127.0.0.1 ADS（永不直连远程 Istiod）

过渡期：仍可用 SSH scp + 远程 HotApply（P3）；目标见「机群控制面策略」专章。
观测：边缘 Fluent Bit → 中心 Loki；Prom 多 gateway；Grafana 仅 Primary/中心
```

### 1.3 HotApply vs Hard / drain

| 操作 | drain？ | 说明 |
|------|---------|------|
| HotApply（xDS） | **否** | 无关长连接应保留 |
| HardReload / 镜像升级 | **是** | 双活维护窗口；见 [README 双活](../README.md#双活维护与升级) |
| 节点出群 / Shrink | **是** | 只摘**新流**；已建立连接不迁移 |
| `firewall apply` | — | 与 Envoy 热更新正交 |

细设计：[docs/hot-update-xds.md](hot-update-xds.md)。

### 1.4 为何不用全局 Istiod

- 多一控制面集群与暴露面；本产品已有单意图源 `resources.yaml`。
- Envoy 持最后快照：中心挂不影响已落地数据面。
- 伸缩在 NLB/TG，与 ADS 解耦。第一期：**中心意图 + 每节点本机 ADS**。

---

## 2. 与现有能力（精炼）

| 判定 | 能力 |
|------|------|
| **复用** | `apply` / `drain` / `fleet`（升级）/ `firewall` / standby 拒写 / 主从 Loki profiles / inventory / `push-fleet-key.sh` / NLB Terraform |
| **半有** | `reload`（仍 drain→restart）→ 需 xDS 分流；Prom 多机 targets 手工；inventory 缺产品化 CRUD |
| **需建（阻塞 MVP 后）** | Agent 拉取主路径（D14：注册/心跳/拉版本）；收紧日常 SSH；漂移检测覆盖 pull |
| **非目标** | 云 SDK / 默认 ASG（P5 可选） |

---

## 3. 功能包 F1–F6

| ID | 名称 | 用户故事（一句话） | 验收要点 |
|----|------|-------------------|----------|
| **F1** | 机群库存 | Primary 维护网关列表、SSH、角色 | 与 `gateways.env` / `fleet` / 灌钥同一 inventory |
| **F2** | 节点接入 | 灌钥 → install → standby env → 日志/监控注册 | BatchMode OK；Loki 带 `gateway=`；doctor/smoke 绿 |
| **F3** | 配置统一 | Primary 发布意图版本 → 各节点 Agent 拉取（过渡期 SSH 推送）→ 本机 HotApply | hash/version 一致；PID 不变；失败可重试 |
| **F4** | 流量成员 | Expand 进 TG / Shrink 先 drain | 演练 +1/-1；Hot **不**替代 drain |
| **F5** | 观测统一 | Primary 按 `gateway` 看指标与会话日志 | 新节点进 scrape；无第二套 Loki/Grafana |
| **F6** | 滚动 Hard | 串行 drain→变更→smoke | 有 NLB 时始终一侧接新流 |

Secondary 落地约束（贯穿 F2/F3）：**不**装完整 Panel；只部署 Envoy + 瘦 xDS（或事件型）+ `with-logs`。

---

## 4. 伸缩剧本与成熟度

### 4.1 成熟度

| 级别 | 形态 | 阶段 |
|------|------|------|
| L0 | 手工 SSH + 控制台 | 现状 |
| L1 | 脚本：灌钥 / install / sync / HotApply / prom 片段 | P0–P3 |
| **L2** | Panel/CLI 向导 + **人工**点云 TG | **P4（第一期目标）** |
| L3 | ASG / 云 API | P5 可选 |

### 4.2 Expand（Panel 一键接入 · L2+）

Primary Panel **机群管理 → 扩容接入**（`POST /api/ops/scale/expand`，确认词 `SCALE_EXPAND`）自动完成：

1. 准备 VM；**上游放行新网关源 IP**（人工前置）  
2. Bootstrap → [`push-fleet-key.sh`](../packaging/scripts/push-fleet-key.sh) → BatchMode（Panel 自动化；密码/旧钥仅请求体）  
3. `install.sh` + Secondary `.env`（`ENABLE_PANEL=0`、`with-logs`、`LOKI_HOST`→Primary）  
4. 写入 inventory → 同步同质 `resources.yaml` → 远端 reload / 瘦 ADS HotApply  
5. 连通性探测（远端 doctor/smoke）  
6. **仍人工**：控制台/TF **register** TG → HC 健康（不接云 API）  

镜像与网络预置下，目标压到数分钟～十余分钟（非秒级 ASG）。操作说明见 [fleet-ops.md](fleet-ops.md)。

### 4.3 Shrink

Primary Panel **机群管理 → 缩容下线**（`POST /api/ops/scale/shrink`，确认词 `SCALE_SHRINK`）：

1. 远程 `drain fail` → 等 `DRAIN_WAIT`（可选，默认开）  
2. **仍人工**：确认 unhealthy → **deregister**（不接云 API）  
3. 等待长连接自然结束（无法 LB 迁移）  
4. 从 inventory 移除；Prom target / 上游放行收紧可另做  

---

## 5. 一致性模型

| 议题 | 拍板 |
|------|------|
| 意图源 | Primary `DataDir/resources.yaml`（Git 可选备份，防双真相漂移） |
| 分发（目标） | Secondary **Agent 拉取**版本 → 本机落盘 → 本机 HotApply / Hard（D14） |
| 分发（过渡） | Primary **推送** SSH + 远程 HotApply（P3 已落地；分期 A→B→C 收紧） |
| ADS 边界 | Envoy **只**连本机 `127.0.0.1`；不指远程 Primary xDS / Istiod |
| Secondary 控制面 | 瘦 Agent（注册/心跳/拉配置）+ 本机 ADS（D13 / D14） |
| 拒写 | standby 兜底；配置只经 Primary 意图源与分发通道 |
| 部分失败 | 标红可重试；不自动全局 HardRestart（D10） |
| ACL/nft | 每节点 `firewall apply`，与 xDS 分流 |

```text
[意图] Primary resources.yaml（版本 / hash）
   ↓ Agent 拉取（目标）· 或 SSH 推送（过渡）
[节点磁盘] 同质 resources.yaml
   ↓ HotApply
[本机瘦 ADS] Snapshot → Envoy ACK
   ↓（正交）
[NLB TG] 是否接新下游流量
```

---

## 6. 路线图 P0–P5

### 6.0 P0 操作入口（身份 / inventory / 灌钥）

1. **填 inventory**：复制 [`gateways.env.example`](../packaging/shared/gateways.env.example) → `DataDir/inventory/gateways.env`（或 setup 生成路径）；填 `HOST_*` / `SSH_*` / `REMOTE_DIR_*`。**勿**把 deploy 私钥写入仓库或 inventory。
2. **灌 fleet 钥**（在 Management Primary 上执行，目标为已知主机）：
   ```bash
   # 单机（已有登录钥）
   BOOTSTRAP_IDENTITY=~/.ssh/id_ed25519 ./packaging/scripts/push-fleet-key.sh root@<host>
   # 或按 inventory 批量
   ./packaging/scripts/push-fleet-key.sh --inventory /opt/relaygate/data/inventory/gateways.env
   ```
3. **对齐 fleet SSH**：脚本成功后提示的 `SSH_OPTS` 与 `relaygate fleet` 一致（需含 `-i ~/.ssh/relaygate_fleet` 与 `BatchMode=yes`）：
   ```bash
   export SSH_OPTS="-o StrictHostKeyChecking=accept-new -o BatchMode=yes -i $HOME/.ssh/relaygate_fleet"
   GATEWAYS=gateway-01,gateway-02 relaygate fleet
   ```
4. **角色**：主控用 `packaging/control/env.example`（`ENABLE_PANEL=1` `PANEL_ROLE=primary`）；节点用 `packaging/node/env.example`（`ENABLE_PANEL=0`、`PRIMARY_URL`、`AGENT_TOKEN_FILE`）。xDS 热更新开关见 `.env` 的 `XDS_ENABLED`（**默认 `1`**；未迁移 bootstrap 时用 `reload --hard` 或设 `0`）。

| 阶段 | 范围 | 依赖 | 验收 | 粗估 | 明确不做 |
|------|------|------|------|------|----------|
| **P0** 身份与 inventory | deploy 钥、灌钥、主从角色、inventory 填实 | SSH 策略 | BatchMode；`fleet` 可达 | 1–2 人天 | 密钥进 Git |
| **P1** 观测并网 | 从节点→中心 Loki；多 gateway Prom | P0 | 两节点日志+指标 | 1–2 人天 | 每节点 Grafana/Loki |
| **P2** 单机 xDS HotApply | 对齐 [hot-update-xds](hot-update-xds.md) Phase 0–1；Primary 可嵌 Panel | Panel/宿主 | PID + 无关连接不变 | 3–5 人天 | Istiod；hot_restart 主路径 |
| **P3** 舰队同步 | Primary→N 机推送 + 远程 HotApply；**Secondary 瘦 xDS**；Panel/CLI/API | P0+P2 | 两节点同质热更；漂移检测 | 2–4 人天 | 多 Primary 互写；N 套 Panel |
| **P4** 伸缩 | Panel 一键接入（灌钥+安装+探测）；NLB 仍人工/TF | P1+P3 | Expand/Shrink 演练 | 2–3 人天 | 云 SDK register |
| **P5** 可选 ASG | 自动 in/out + drain 钩子 | P4 | 另估 | **默认不做** |

- P0∥P1 可与 P2 并行；**P3 强依赖 P2**（及 Secondary 瘦 ADS 形态）。  
- MVP = **P0–P4**；合计约 **9–16 人天**（含单机 xDS，不含 P5）。

### 6.1 实现进度（2026-08-02）

| 阶段 | 状态 | 落地要点 |
|------|------|----------|
| P3 舰队同步 | **已落地（SSH 推送）** | `relaygate fleet-sync`；`POST /api/ops/fleet-sync`、`GET /api/ops/fleet/status`；Panel **机群管理**列表 + 同步。长期改为 Agent 拉取（D14），见专章分期 A→B→C |
| P4 伸缩 | **已落地（L2+）** | `POST /api/ops/scale/expand\|shrink`（灌钥/安装/同步/探测；NLB 仍人工）；`GET .../playbook` 为步骤预览；UI 在 **机群管理**（非运维工具） |

操作说明见 [fleet-ops.md](fleet-ops.md)。

---

## 7. 风险与非目标

### 7.1 主要风险

| 风险 | 缓解 |
|------|------|
| 长连接不随 LB 迁移 | SOP 诚实窗口；drain 只摘新流 |
| 上游未放行新源 IP | Expand 检查清单 |
| 双主 / N 套 Panel | 仅一 Primary；D13；standby 兜底 |
| 密钥进库 / 私钥面过大 | 私钥仅 Primary 宿主；inventory 无密；稳态改为 Agent 拉取后日常不再依赖全舰队 root 钥在线 |
| 误 restart 当热更新 | `HOT_APPLY` / Hard 文案分流 |
| 舰队漂移 | 版本/hash 汇总；禁止假成功 |
| xDS/admin 暴露 | 仅 loopback |

### 7.2 非目标（第一期）

- 全局 Istiod / 远程共享 xDS  
- **每节点完整 Panel 或每节点 Grafana/Loki**  
- 用容器重启冒充热更新  
- 产品内嵌云 SDK  
- 跨服上游 LB / 会话迁移  
- 多 Primary 活跃写  
- deploy 私钥进 Panel DB  

---

## 附录 — 文档与代码索引

| 路径 | 用途 |
|------|------|
| [docs/hot-update-xds.md](hot-update-xds.md) | 本机 ADS、Hot/Hard、非 Istiod |
| [docs/envoy-capability-roadmap.md](envoy-capability-roadmap.md) | 数据面能力边界与分期（已拍板；默认无 EDS/L7） |
| [docs/logging-playbook.md](logging-playbook.md) | 主从 Loki；中心观测 |
| [docs/fleet-ops.md](fleet-ops.md) | Panel 机群管理：同步 / 扩容接入 / 缩容下线（P3/P4） |
| [docs/fleet-agent-strategy.md](fleet-agent-strategy.md) | **跳转页**（正文已并入上文「机群控制面策略」专章） |
| [README.md §双活维护与升级](../README.md#双活维护与升级) | drain / NLB / 升级窗口 |
| `core/ops/fleet_sync.go` | Primary→舰队 scp + 远程 reload；drift / skip local |
| `core/ops/scale_playbook.go` | Expand/Shrink 自动化步骤预览 |
| `core/ops/scale_expand.go` / `scale_shrink.go` | Panel 一键扩容/缩容 |
| [gateways.env.example](../packaging/shared/gateways.env.example) | 舰队 inventory |
| [packaging/scripts/push-fleet-key.sh](../packaging/scripts/push-fleet-key.sh) | deploy 灌钥 → BatchMode |
| `packaging/terraform/nlb/` | NLB / drain 时机 |
| `packaging/observability/prometheus.yml` | 多 gateway scrape |

---

**纠正一句（对齐用户意图）**：以当前主机为管理面与观测中心，把多台同质 Envoy 编成可伸缩双活舰队；配置意图由节点 **Agent 拉取**后走每机**瘦** xDS 热更新，流量伸缩走 LB 目标组——两者一起自动化；SSH 只做接入引导与应急；**不是**冷备、**不是**每台一套 Panel、**不是**全局服务网格控制面。
