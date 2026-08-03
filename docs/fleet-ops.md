# Panel 机群运维（发布 · 接入 · 退役）

> **主控**上的 Panel **机群管理**页（`/fleet`）；只读角色不可触发发布/接入/退役。  
> **运维工具**（`/ops`）仅保留本机：诊断、摘流、冒烟/探测、防火墙、档位。  
> **产品表面**见 [product-surface-agent.md](product-surface-agent.md)；控制面策略见 [机群控制面策略](fleet-scale-control-plane.md#fleet-control-plane-strategy)。  
> 正式路径：**主控发布配置版本 → 节点 Agent 拉取 → 本机热更新**。SSH / `fleet-sync` **不**作为产品主词。

## 安装角色

| 角色 | 环境模板 | 要点 |
|------|----------|------|
| **主控** | [`packaging/control/env.example`](../packaging/control/env.example) | `ENABLE_PANEL=1` `PANEL_ROLE=primary`；可选本机转发与中心观测 |
| **节点** | [`packaging/node/env.example`](../packaging/node/env.example) | `ENABLE_PANEL=0`；`PRIMARY_URL` + `AGENT_TOKEN_FILE`；`relaygate agent run` |

## 节点列表与对齐状态

1. 打开 **机群管理**。
2. 表格列：**对齐状态**（已对齐 / 未对齐 / 离线 / 未授权）来自节点心跳与已应用版本。
3. **发布概况**显示当前发布版本；写动作以 **配置应用 → 发布到机群** 为准。

## 发布到机群

1. 建议先在主控 **配置应用 → 本机应用**（`HOT_APPLY` / `RELOAD_ENVOY`）。
2. 再 **发布到机群**，确认词 `PUBLISH_FLEET`。
3. 各网关节点 Agent 随后自拉并本机热更新；未跟上显示「未对齐」。

等价 CLI（主控宿主）：

```bash
relaygate fleet publish                 # 需确认 PUBLISH_FLEET 或 RELAYGATE_CONFIRM=PUBLISH_FLEET
relaygate fleet status
```

节点侧：

```bash
relaygate agent pull                    # 拉一次
relaygate agent run                     # 常驻：心跳 + 定时拉取
```

## 接入节点 / 退役节点

### 接入节点

1. **机群管理 → 接入节点**：填写节点名。
2. 确认词 `FLEET_JOIN`：生成一次性令牌与引导说明（目标机用 `node.env.example` 安装并 `agent run`）。
3. **剩余人工**：云控制台/Terraform 将新节点 **register** 到 NLB Target Group（不接云 API）。

### 退役节点

1. **机群管理 → 退役节点**：选择名册中的节点。
2. 确认词 `FLEET_LEAVE`：从名册移除并吊销代理令牌。
3. **剩余人工**：NLB/TG **deregister**。

## API 摘要

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/ops/fleet` | 节点名册 |
| GET | `/api/ops/fleet/status` | 对齐状态 / 版本 |
| POST | `/api/ops/fleet/publish` | body: `{ "confirm": "PUBLISH_FLEET" }` |
| POST | `/api/ops/fleet/join` | body: `{ "confirm": "FLEET_JOIN", "name": "gateway-02" }` |
| POST | `/api/ops/fleet/leave` | body: `{ "confirm": "FLEET_LEAVE", "name": "gateway-02" }` |
| GET | `/api/agent/config` | 节点拉取当前发布（Bearer 令牌） |
| POST | `/api/agent/heartbeat` | 节点心跳（Bearer 令牌） |

只读角色对 `publish` / `join` / `leave` 返回 403，并引导至主控。

**已废除（勿再使用）**：`FLEET_SYNC`、`SCALE_EXPAND`、`SCALE_SHRINK`、`fleet-sync`、`/api/ops/fleet-sync`、SSH 扩缩容向导作为产品主路径。
