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
     需 PRIMARY_URL + AGENT_TOKEN_FILE
```

| | 主控 | 节点 |
|--|------|------|
| Panel | `ENABLE_PANEL=1` `PANEL_ROLE=primary` | `ENABLE_PANEL=0` |
| 机群动作 | `fleet publish` / `join` / `leave` / `status` | `agent run` / `agent pull` |
| 观测 | 可选中心 Grafana/Loki | 日志出站；不启完整观测栈 |

只读角色（standby）拒写并引导主控；不是推荐的节点形态。

日常链路：**主控发布配置版本 → 节点 Agent 拉取 → 本机热更新**。

## 1. 术语（终态）

| 对外 | English / CLI | 说明 |
|------|---------------|------|
| 主控 | control / primary | 唯一可写意图源 + Panel |
| 网关节点 | gateway node | Envoy + agent |
| 机群 | fleet | UI 用「机群」 |
| 发布配置 | `fleet publish` | 使机群可见新版本 |
| 已对齐 / 未对齐 / 离线 | aligned / drifted / offline | 节点相对发布版本 |
| 接入 / 退役 | `fleet join` / `leave` | 名册 + 令牌 |
| 热更新 / 硬重启 | hot apply / `reload --hard` | 确认词 `HOT_APPLY` / `RELOAD_ENVOY` |
| 上游 / 转发 / 入口 | upstream / forward / entry | L4 产品主词 |

## 2. Panel 侧栏（主控）

| 菜单 | 路由 | 职责 |
|------|------|------|
| 状态总览 | `/` | 本机与机群健康 |
| 上游管理 | `/servers` | 上游增删与启用 |
| 转发规则 | `/rules` | 入口 → 上游 |
| 访问控制 | `/acl` | 下游 ACL |
| 配置编辑 | `/config` | 编辑意图（落盘） |
| 配置应用 | `/apply` | 本机应用 · 发布到机群 |
| 机群管理 | `/fleet` | 节点 · 发布概况 · 接入 · 退役 |
| 运维工具 | `/ops` | 本机诊断 / 摘流 / 探测 / 防火墙 / 档位 |
| 变更历史 | `/changes` | 本机回滚 |
| 监控面板 | Grafana | 中心观测 |

**机群页**：节点列表 · 发布概况 · 接入节点 · 退役节点  
**配置应用**：本机应用（热/硬）与发布到机群分按钮。

## 3. CLI 主树

```text
relaygate fleet status|publish|join|leave
relaygate agent run|pull|install
relaygate reload [--hard]
relaygate apply | firewall | drain | diag | …
```

`xds *` 为工程入口，不对运维主推。

## 4. 确认词

| 操作 | 确认词 |
|------|--------|
| 热更新 | `HOT_APPLY` |
| 硬重启 | `RELOAD_ENVOY` |
| 防火墙 | `YES_FLUSH_NFTABLES` |
| 发布到机群 | `PUBLISH_FLEET` |
| 退役节点 | `FLEET_LEAVE` |
| 应用档位 | `APPLY_PROFILE` |
| 摘流 / 恢复 | `DRAIN_FAIL` / `DRAIN_OK` |
| 回滚 | `ROLLBACK` |

接入 `fleet join` **无需**确认词。
