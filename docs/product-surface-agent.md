# 产品表面（菜单 · CLI · 确认词）

单一产品、两个组件。机群运维步骤见 [fleet-ops.md](fleet-ops.md)；架构见 [fleet-scale-control-plane.md](fleet-scale-control-plane.md)。

## 0. 对内双组件（单一产品 · 非双产品线）

```text
RelayGate（单一产品）
├─ 主控组件（control）
│    Panel + 意图编辑 / 发布 / 节点名册
│    + 可选本机转发
│    模板：packaging/control/env.example
└─ 节点组件（node）
     agent + 本机热更新 + Envoy
     无完整 Panel（ENABLE_PANEL=0）
     模板：packaging/node/env.example
     需 CONTROL_URL + AGENT_TOKEN_FILE
```

| | 主控 | 节点 |
|--|------|------|
| Panel | `ENABLE_PANEL=1`（可写） | `ENABLE_PANEL=0` |
| 机群动作 | `fleet publish` / `join` / `leave` / `status` | `agent run` / `agent pull` |
| 观测 | 可选中心 Grafana/Loki | 日志出站；不启完整观测栈 |

只读角色拒写并引导主控；不是推荐的节点形态。

日常链路：**主控发布配置版本 → 节点 agent 拉取 → 本机热更新**。

## 1. 术语（终态）

| 对外 | English / CLI | 说明 |
|------|---------------|------|
| 主控 | control | 唯一可写意图源 + Panel；安装：`install.sh control` |
| 网关节点 | node | 机群中的转发角色（一台机器）；安装：`install.sh node --control …` |
| agent | `relaygate agent` · `relaygate-agent` | **节点上的**拉取/心跳守护进程（不是安装角色）；包 `core/agent`；API `/api/agent/*` |
| 机群 | fleet | UI 用「机群」 |
| 发布配置 | `fleet publish` | 使机群可见新版本 |
| 已对齐 / 未对齐 / 离线 | aligned / drifted / offline | 节点相对发布版本 |
| 接入 / 退役 | `fleet join` / `leave` | 名册 + 令牌 |
| 热更新 / 硬重启 | hot apply / `reload --hard` | 二次确认输入「确认」/`Confirm` |
| 上游 / 转发 / 入口 | upstream / forward / entry | L4 产品主词 |

**分层：** 用户文案与安装子命令用「主控 / 节点」；工程侧进程、systemd、CLI 子命令、令牌 env 保留 `agent`。关系：**节点 = 跑 agent 的机器**。勿把安装改成 `agent`，也勿为统一而 rename `core/agent`。

勿再用 primary / secondary 作为用户可见产品角色名。

## 2. Panel 侧栏（主控）

| 菜单 | 路由 | 职责 |
|------|------|------|
| 状态总览 | `/` | 本机与机群健康 |
| 上游管理 | `/upstreams` | 上游增删与启用 |
| 转发规则 | `/forwards` | 入口 → 上游 |
| 安全策略 | `/security` | 统一 `security.policies`（含来源访问控制） |
| 配置编辑 | `/config` | 编辑意图（落盘） |
| 配置应用 | `/apply` | 本机应用 · 发布到机群 |
| 运维工具 | `/ops` | 本机诊断 / 摘流 / 探测 / 防火墙 / 档位 |
| 变更历史 | `/changes` | 本机回滚 |
| 机群管理 | `/fleet` | 节点 · 发布概况 · 接入 · 退役（机群页；置于本机项下、监控上） |
| 监控面板 | Grafana | 中心观测 |

**机群页**：节点列表 · 发布概况 · 接入节点 · 退役节点  
**配置应用**：本机应用（热/硬）与发布到机群分按钮；风险说明只在各操作二次确认框。

## 3. CLI 主树

```text
relaygate fleet status|publish|join|leave
relaygate agent run|pull|install
relaygate reload [--hard]
relaygate apply | firewall | drain | diag | …
```

`xds *` 为工程入口，不对运维主推。

## 4. 确认词

所有需二次确认的敏感操作统一输入：

| 语言 | 确认词 |
|------|--------|
| 中文 | `确认`（精确） |
| 英文 | `Confirm`（大小写敏感） |

Panel / API / CLI 均接受两者（避免语言切换踩坑）。非交互：`RELAYGATE_CONFIRM=Confirm` 或 `FIREWALL_CONFIRM=Confirm`。

涉及操作：热更新、硬重启、防火墙应用、发布到机群、退役节点、应用档位、摘流/恢复、回滚。

接入 `fleet join` **无需**确认词。

**已废除**的按操作指令词：`HOT_APPLY`、`RELOAD_ENVOY`、`YES_FLUSH_NFTABLES`、`PUBLISH_FLEET`、`FLEET_LEAVE`、`APPLY_PROFILE`、`DRAIN_FAIL` / `DRAIN_OK`、`ROLLBACK`，以及更早的 `FLEET_SYNC` / `SCALE_*`。

## 5. Panel 按钮风险色

| 色 | variant | 典型操作 |
|----|---------|----------|
| 红 | `destructive` | 硬重启本机应用、应用防火墙、退役、摘流、回滚、删除上游 |
| 橙 | `caution` | 热更新本机应用、发布到机群、恢复承接、应用档位 |
| 灰 | `outline` / `secondary` | 刷新、接入命令、保存、检查、诊断、查看 |
