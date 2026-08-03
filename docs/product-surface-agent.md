# RelayGate：Agent 改造下的产品表面（菜单 · 指令 · 术语）

> **状态：工程落地中（2026-08-03）**  
> **前提（已拍板）**：日常机群 = **发布配置版本 + 节点 Agent 拉取**；SSH **不**作为正式产品能力（菜单 / 主命令 / 默认文案均不出现）。  
> 控制面与数据流见 [fleet-scale-control-plane.md — 机群控制面策略](fleet-scale-control-plane.md#fleet-control-plane-strategy) · 产品规则见 `.cursor/rules/relaygate-product.mdc`  
> **产品表面（本文）为准**；与旧 SSH / `fleet-sync` 文案冲突时，以本文终态覆盖。

---

## 0. 对内双组件（单一产品 · 非双产品线）

RelayGate **只有一个产品**。对内按安装角色拆成两个**组件**（同一二进制 / 同一发行包，不同 `.env` 与启用进程）：

```text
RelayGate（单一产品）
├─ 主控组件（control）
│    Panel UI + 意图编辑 / 发布 / 节点名册
│    + 可选本机转发（Compose Envoy）
│    模板：packaging/control/env.example
└─ 节点组件（node）
     agent + 本机热更新控制 + Envoy
     无完整 Panel（ENABLE_PANEL=0）
     模板：packaging/node/env.example
     需 PRIMARY_URL + AGENT_TOKEN_FILE
```

| 维度 | 主控 control | 节点 node |
|------|--------------|-----------|
| 安装意图 | 可写管理面 + 发布源 | 数据面齐步对齐 |
| Panel | `ENABLE_PANEL=1` `PANEL_ROLE=primary` | `ENABLE_PANEL=0`（无侧栏） |
| 日常机群动作 | `fleet publish` / `join` / `leave` / `status` | `agent run` / `agent pull` |
| 观测 | 中心 Grafana/Loki（可选本机） | 日志船出站；不启完整观测栈 |

**不是**「控制面产品 + 数据面产品」两条线；文档 / UI / CLI 不出现双产品线叙事。Standby 只读是误启 Panel 时的写保护兜底，**不是**推荐的节点形态。

---

## 1. 设计原则（彻底重构 · 无兼容层）

| # | 原则 | 说明 |
|---|------|------|
| P1 | **彻底重构，不做兼容** | 产品表面**不**保留「旧 SSH 推送 / fleet-sync 主语义 / 双路径」作为正式能力。无「应急 SSH」「legacy」「高级推送」菜单或主命令。排障走宿主手册 / `doctor`，不占产品 IA。 |
| P2 | **日常机群 = 发布 + 拉取** | 运维在主控**发布**配置版本；各网关节点上的 Agent **拉取 → 本机落盘 → 本机热更新**。正式文案只讲这条链路。 |
| P3 | **SSH 不出现在正式表面** | 首次装机引导可在文档/安装脚本中出现（一次性命令），但 **Panel 侧栏、机群页、CLI usage 主树** 不出现 SSH / 灌钥 / 推送等主词。 |
| P4 | **Panel 给人，CLI 给脚本** | Panel = 可写主控上的运维 UI；CLI = 自动化与宿主脚本。Secondary（网关节点）**无 Panel**。 |
| P5 | **术语中性、四字菜单** | 通用 L4 网关语汇（下游 / 上游 / 入口 / 转发 / 机群）；侧栏中文**四字**；禁止游戏向与客户黑话。 |
| P6 | **敏感操作二次确认** | 写操作 / 危险操作须显式确认词（或等价环境开关）；Standby / 只读禁止写，引导至主控。新危险操作**独立确认词**，勿复用。 |
| P7 | **结构简单** | 一页一事；机群页内四区块即可；配置应用页只区分「本机」与「机群发布」两个动作。 |

---

## 2. 术语表（旧 → 新 · 不兼容保留旧名作主词）

| 旧称 / 现状主词 | 对外主词（中文） | 对外主词（英文/CLI） | 对内实现名（若保留） | 备注 |
|-----------------|------------------|----------------------|----------------------|------|
| fleet-sync / 同步配置（推送） | **发布配置**（使机群可见新版本） | `fleet publish` | 发布 API / version bump（仅代码） | **废除** `fleet-sync`、`FLEET_SYNC` 作产品主词 |
| 同步（synced） | **已对齐** | aligned / in sync（状态文案） | status enum（仅代码） | 节点已应用当前发布版本 |
| drift | **未对齐** | drifted | drift（仅代码） | 心跳版本 ≠ 当前发布 |
| unreachable | **离线** | offline | — | 长时间无心跳 |
| inventory / gateways.env | **节点名册** | node registry | `nodes.yaml` / 注册表（仅代码）；旧 `gateways.env` 不进 UI | 展示名册，不展示 SSH 字段 |
| expand / 扩容接入 | **接入节点** | `fleet join` | bootstrap 流程（仅代码） | 生成引导、节点报到；非「SSH 灌钥向导」主叙事 |
| shrink / 缩容下线 | **退役节点** | `fleet leave` | leave / revoke（仅代码） | 摘流提示 + 名册移除 + 吊销凭证 |
| 灌钥 / push-fleet-key | （正式表面**不出现**） | — | bootstrap 脚本（仅安装文档） | 非产品菜单项 |
| xds serve | **本机数据面控制**（用户少见） | `agent run` 内嵌，或 `gateway dataplane` | `xds serve`（仅代码/systemd） | 不对运维主推 `xds` 子命令 |
| xds apply / HotApply | **热更新**（本机） | 本机应用 / hot apply | HotApply（仅代码） | 确认词仍 `HOT_APPLY` |
| Hard reload | **硬重启**（本机） | `reload --hard` | HardReload（仅代码） | 确认词 `RELOAD_ENVOY` |
| Management Primary | **主控** | primary / management primary | `PANEL_ROLE=primary`（仅代码） | 唯一可写意图源 + Panel |
| Secondary / 数据面节点 | **网关节点** | gateway node / secondary | Secondary（仅代码/模板） | 无完整 Panel |
| 舰队 / Fleet | **机群** | fleet | fleet（包名可留） | UI 统一「机群」 |
| DataDir | **数据目录** | data directory | `DataDir`（仅代码/日志折叠） | 用户文案勿裸露路径常量名 |
| resources.yaml | **业务配置** / **意图配置** | intent / resources | `resources.yaml`（文件名可保留） | 「发布」对象即此意图的版本化快照 |
| 舰队 Agent | **节点代理** | agent | fleet agent（仅代码） | 注册 / 心跳 / 拉配置 |
| 本机瘦 xDS / ADS | **本机控制面** | local dataplane control | ADS / xDS（仅代码） | Envoy 只连本机 loopback |
| Standby | **只读角色** | standby | `PANEL_ROLE=standby`（仅代码） | 拒写，引导主控 |

**禁止作为产品主词（正式表面）**：`fleet-sync`、SSH 推送、应急通道、legacy、双路径、inventory、DataDir、游戏/玩家/区服等。

---

## 3. Panel 信息架构（最终态）

> 仅 **主控** 提供完整 Panel。网关节点默认 `ENABLE_PANEL=0`，无侧栏。

### 3.1 侧栏（四字中文 + 路由建议）

| 顺序 | 菜单（四字） | 路由 | 职责（一句话） |
|------|--------------|------|----------------|
| 1 | 状态总览 | `/` | 本机与机群健康一眼看清 |
| 2 | 上游管理 | `/servers` | 上游服务增删与启用 |
| 3 | 转发规则 | `/rules` | 入口端口 → 上游转发 |
| 4 | 访问控制 | `/acl` | 下游访问 ACL |
| 5 | 配置编辑 | `/config` | 编辑意图（落盘，未应用） |
| 6 | 配置应用 | `/apply` | **本机应用** 与 **发布到机群** |
| 7 | 机群管理 | `/fleet` | 节点 · 发布 · 接入 · 退役 |
| 8 | 运维工具 | `/ops` | **仅本机**诊断 / 摘流 / 探测 / 防火墙 / 档位 |
| 9 | 变更历史 | `/changes` | 本机配置变更与回滚 |
| 10 | 监控面板 | （外链 Grafana） | 中心观测，非第二套控制面 |
| — | 退出登录 | — | — |

侧栏顺序相对现状：**机群管理**上移到运维工具之前（机群是主路径，运维是本机工具），避免「先同步再找机群」的旧心智。

### 3.2 机群管理页内区块（`/fleet`）

**不要**：「应急 SSH」「legacy」「推送配置」「灌钥」入口。

| 区块 | 四字/标题 | 内容 |
|------|-----------|------|
| A | **节点列表** | 名称、角色（主控/节点）、在线、已对齐/未对齐/离线/未授权、已应用版本；只读心跳与版本 |
| B | **发布概况** | 当前发布版本 / 时间 / 操作者摘要；快捷跳转「配置应用 → 发布到机群」；本页可放只读，写动作以配置应用页为准（避免双入口写） |
| C | **接入节点** | 填写节点名 → 生成**一次性加入令牌 / 引导命令**（用户在目标机执行安装脚本）；节点 Agent 报到后出现在列表。NLB/TG register **人工提示**（不接云 API） |
| D | **退役节点** | 选节点 → 可选提示摘流 → 名册移除 + 吊销 Token；NLB deregister **人工提示** |

接入叙事：**生成引导 → 节点自助安装并报到 → 自动拉取当前发布**；不讲 SSH 矩阵。

### 3.3 配置应用（`/apply`）：两个动作

| 动作（对外） | 做什么 | 确认词 | 不做什么 |
|--------------|--------|--------|----------|
| **本机应用** | 主控本机：热更新（默认）或硬重启 | `HOT_APPLY` / `RELOAD_ENVOY` | 不自动发布到机群 |
| **发布到机群** | 将当前意图打成**新发布版本**；各节点 Agent 随后自拉并本机热更新 | `PUBLISH_FLEET` | 不 SSH、不 scp、不远程 reload；本机是否已应用由运维自行保证（文案提示：建议先本机应用再发布） |

文案要点（人话）：

- 本机应用：只影响**这台主控上的数据面**。  
- 发布到机群：全机群网关节点将拉取该版本；部分节点未跟上显示「未对齐」，可等待或查该节点，**不会**自动全局硬重启。

### 3.4 运维工具边界（`/ops` · 仅本机）

保留：诊断、摘流（`DRAIN_FAIL` / `DRAIN_OK`）、冒烟/探测、防火墙检查/应用、档位应用（`APPLY_PROFILE`）。  
**移除**：任何机群同步 / 扩缩容（已迁入机群管理）；不出现跨机推送。

### 3.5 删除 / 合并的现有入口

| 现状 | 终态处理 |
|------|----------|
| 机群页「同步配置」+ `FLEET_SYNC` | **删除**；改为配置应用「发布到机群」+ `PUBLISH_FLEET` |
| 机群页「扩容接入」SSH 向导语义 | **改写**为「接入节点」：令牌/引导命令，无灌钥主路径 UI |
| 机群页「缩容下线」 | **改名**「退役节点」；确认词见 §5 |
| 运维工具中的机群相关（若有） | **删除**，只留本机 |
| 任何「SSH 推送 / 应急同步」UI | **不提供** |
| CLI `fleet-sync` 在帮助中的主条目 | **废除**（见 §4） |
| 用户可见的 `xds serve` | **降级**：并入 `agent` / systemd，usage 主树不展示 |

---

## 4. CLI 命令树（最终态 · breaking）

> CLI 给人话 help；脚本可非交互 + 环境确认变量。与 Panel **同一确认词**。

### 4.1 推荐主树（产品命令）

```text
首启与健康
  relaygate setup …
  relaygate diag …
  relaygate version

配置（意图）
  relaygate render|validate|…
  relaygate server|acl|profile|changes …

数据面（本机）
  relaygate apply
  relaygate reload              # 默认热/硬分流
  relaygate reload --hard       # 需确认：RELOAD_ENVOY 或 RELAYGATE_CONFIRM=RELOAD_ENVOY
  relaygate rollback …
  relaygate drain fail|ok|status
  relaygate upgrade …

机群（主控宿主）
  relaygate fleet status                 # 节点在线 / 对齐 / 版本
  relaygate fleet publish                # 发布当前意图版本 → 确认 PUBLISH_FLEET
  relaygate fleet join                   # 创建接入令牌 / 打印引导命令
  relaygate fleet leave <name>           # 退役节点 → 确认 FLEET_LEAVE

节点代理（网关节点 / 亦可主控侧调试）
  relaygate agent run                    # 常驻：心跳 + 拉版本 + 触发本机热更新（内嵌或联动本机 ADS）
  relaygate agent pull                   # 拉一次并应用（脚本/排障）

防火墙 / Panel（本机）
  relaygate firewall …
  relaygate panel …
```

### 4.2 废除或降级为非产品命令

| 现状命令 | 终态 | 说明 |
|----------|------|------|
| `relaygate fleet-sync` | **废除**（产品表面与 usage 删除） | 由 `fleet publish` + 节点 `agent pull` 取代；工程迁移期可直接删入口，**不做**长期别名 |
| `relaygate fleet`（旧：inventory 批量 upgrade） | **重做语义**或改名 | 旧「分批 drain→upgrade→smoke」若保留，迁到 `upgrade --fleet` 或文档级宿主剧本，**勿**与 `fleet status|publish|join|leave` 抢同一主词而不加说明 |
| `relaygate xds serve` | **非产品主命令** | systemd/`agent run` 内部调用；高级调试可留隐藏子命令，不进默认 usage |
| `relaygate xds apply` | **非产品主命令** | 本机热更新走 `reload`（无 `--hard`）或 Agent 拉取后内部调用 |

### 4.3 确认词与环境变量（与 Panel 对齐）

| 操作 | 交互确认 | 非交互（脚本） |
|------|----------|----------------|
| 发布到机群 | 输入 `PUBLISH_FLEET` | `RELAYGATE_CONFIRM=PUBLISH_FLEET`（或专用 `FLEET_PUBLISH_CONFIRM=`，二选一，实施时定一种并写死） |
| 本机热更新 | `HOT_APPLY` | 同上模式 |
| 本机硬重启 | `RELOAD_ENVOY` | 已有 `--hard` 习惯则仍须确认开关 |
| 接入节点（写名册/发令牌） | `FLEET_JOIN` | 环境确认 |
| 退役节点 | `FLEET_LEAVE` | 环境确认 |
| 吊销节点令牌（若独立于退役） | `REVOKE_AGENT` | 环境确认 |

**废除**作为主确认词：`FLEET_SYNC`。  
**改名对齐产品动词**：`SCALE_EXPAND` → `FLEET_JOIN`；`SCALE_SHRINK` → `FLEET_LEAVE`（不保留旧词别名）。

---

## 5. 确认词最终表（全部独立 · 人话说明）

| 确认词 | 场景 | 人话风险说明（确认框主文案方向） |
|--------|------|----------------------------------|
| `HOT_APPLY` | 本机热更新 | 将备份并通过本机控制面热推送配置（不摘流、不重启 Envoy）。无关长连接应保留；改/删的入口上连接可能被排空。 |
| `RELOAD_ENVOY` | 本机硬重启 | 将备份、渲染并硬重启本机 Envoy；本机现有会话会中断；摘流窗口内新流量可能被负载均衡摘除。 |
| `YES_FLUSH_NFTABLES` | 本机防火墙应用 | 将清空并加载本机防火墙规则；出错可能影响远程登录——请保持当前会话并备好云控制台。 |
| `PUBLISH_FLEET` | 发布到机群 | 将当前业务配置发布为机群新版本；各网关节点将自行拉取并在本机热更新。部分节点未对齐时可重试排查，不会自动全局硬重启。 |
| `FLEET_JOIN` | 接入节点 | 将创建节点身份与一次性加入凭证/引导；请在目标主机完成安装。负载均衡挂载仍须按清单人工处理。 |
| `FLEET_LEAVE` | 退役节点 | 将从机群名册移除该节点并吊销其代理凭证；若勾选摘流，将停止向其分配新连接。负载均衡摘除仍须人工处理。 |
| `REVOKE_AGENT` | 单独轮换/吊销令牌 | 将使该节点代理凭证立即失效；节点需重新接入后才能继续拉取配置。 |
| `DRAIN_FAIL` | 本机摘流 | 本机停止承接负载均衡新连接；已建立连接通常仍保留；误操作会导致本网关不再接受新连接。 |
| `DRAIN_OK` | 本机恢复接流 | 恢复本机探活，使负载均衡重新分配新连接。 |
| `APPLY_PROFILE` | 本机档位 | 仅写入本机默认参数，不会立刻重启 Envoy；之后仍须「本机应用」才会生效。 |
| `ROLLBACK` | 本机回滚 | 回滚到指定变更并重建数据面（会打断现有连接）。 |

**已废除（勿再用于产品主路径）**：`FLEET_SYNC`、`SCALE_EXPAND`、`SCALE_SHRINK`。

---

## 6. 工程迁移说明（给开发 · 极短）

1. **一次性 breaking**：文档、i18n/侧栏、CLI usage、API 路径与确认词**同一迭代改完**；**不做**双命令别名、双确认词、双 UI 入口长期并存。  
2. **推荐实施顺序**：  
   1. **先改产品表面命名与 IA**（本文：菜单四字、文案、确认词常量、CLI 树骨架、删除 sync 入口文案）——即使 Agent API 尚未就绪，也避免继续强化推送心智；发布动作可先接「写版本文件 + 列表展示」，拉配置随后跟上。  
   2. **再落地 Agent API**（注册 / 心跳 / 拉版本 / 本机 HotApply）与 `agent run`。  
   3. 接入/退役改为令牌引导；删除 SSH expand/sync 代码路径。  
3. 与 [机群控制面策略](fleet-scale-control-plane.md#fleet-control-plane-strategy)：技术竖切（安装键、Agent/xDS 边界、MVP）以专章为准；**产品命名、菜单、确认词以本文为准**（无 SSH 应急 UI；确认词 `PUBLISH_FLEET` 等）。  
4. Standby 拒写：对 `publish` / `join` / `leave` / 本机写操作一律 403，文案引导主控。

---

## 7. 决策摘要（已按「彻底重构」代用户拍板）

- [x] **不做兼容层**：正式产品表面无 SSH 推送、无 `fleet-sync` 主语义、无双路径/legacy 入口。  
- [x] **日常链路**：主控发布版本 + 节点 Agent 拉取 + 本机热更新。  
- [x] **Panel 仅主控**；网关节点无 Panel；运维工具仅本机。  
- [x] **侧栏四字中文**；机群页区块 = 节点 / 发布 / 接入 / 退役。  
- [x] **配置应用两动作**：本机应用（`HOT_APPLY`/`RELOAD_ENVOY`）与发布到机群（`PUBLISH_FLEET`）。  
- [x] **CLI 主树**：`fleet status|publish|join|leave` + `agent run|pull`；废除 `fleet-sync` 产品位；`xds *` 降为非产品。  
- [x] **确认词独立**：`PUBLISH_FLEET` / `FLEET_JOIN` / `FLEET_LEAVE` / `REVOKE_AGENT`；废除 `FLEET_SYNC`、`SCALE_EXPAND`、`SCALE_SHRINK` 作主词。  
- [x] **迁移顺序**：先表面命名与 IA，再 Agent API 竖切。  
- [x] **术语中性**：主控 / 网关节点 / 机群 / 节点名册 / 数据目录；旧实现名可留代码。

---

**一句话**：运维只在主控编辑并**发布**配置，节点自己**拉取**对齐；菜单与命令围绕「本机应用 / 机群发布 / 接入 / 退役」四个动词展开，旧推送叙事整体退役。
