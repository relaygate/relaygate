# 机群运维（发布 · 单节点同步 · 接入 · 退役）

主控 Panel **机群管理**（`/fleet`）。只读角色不可发布/同步/接入/退役。运维工具（`/ops`）仅本机诊断、摘流、探测、防火墙、档位。

正式路径：**主控发布 → 节点 agent 拉取 → 本机热更新**。立刻对齐某台用**单节点同步**（不广播）。

## 安装角色

| 角色 | 环境模板 | 要点 |
|------|----------|------|
| **主控** | [`packaging/control/env.example`](../packaging/control/env.example) | `PANEL_ENABLED=1`；可选本机转发与中心观测 |
| **节点** | [`packaging/node/env.example`](../packaging/node/env.example) | `PANEL_ENABLED=0`；`CONTROL_URL` + `AGENT_TOKEN_FILE`；Compose `node`（Envoy + Alloy 指标） |

机群在线/版本由 agent 心跳与配置拉取上报；指标经 `POST /api/agent/metrics/write` remote_write。

## 发布到机群

Panel **配置应用**页只做本机落地，**不提供**「发布到机群」按钮（避免与按台同步路径混淆）。

1. 建议先在主控 **配置应用 → 本机应用**（热更新或硬重启，确认输入「确认」/`Confirm`）
2. 在主控执行 **`relaygate fleet publish`**（确认「确认」/`Confirm`；或 `POST /api/ops/fleet/publish`）— 只提升 desired 版本；节点按拉取周期自行对齐
3. 节点 Agent 自拉并本机热更新；未跟上显示「未对齐」

```bash
relaygate fleet publish                 # 或 RELAYGATE_CONFIRM=Confirm
relaygate fleet status
relaygate agent pull                    # 节点侧拉一次
relaygate agent run                     # 节点常驻
```

## 单节点同步

发布后若某台未跟上、或希望逐台滚动对齐（降低全机群同时断连风险）：

1. **机群管理 → 节点行「同步」**（确认「确认」/`Confirm`）
2. 主控仅标记该节点；其 agent 下次心跳收到 `pull_now` 后立即拉取并本机落地
3. **不影响其他节点**

```bash
relaygate fleet sync gateway-02         # 或 RELAYGATE_CONFIRM=Confirm
```

主控本机转发请用「本机应用」，不可对主控名册条目做机群同步。

## 接入节点

1. **机群管理 → 接入节点**：填写名称，生成接入命令（无需确认词）
2. 在目标主机以 root 执行该命令
3. 若用云 LB：在控制台/Terraform 将新节点 register 到 Target Group

```bash
relaygate fleet join gateway-02
```

升级（主控与节点同一命令，读现有 `.env`）：

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
```

## 退役节点

1. **机群管理 → 退役节点**（不可退役主控角色）
2. 确认输入「确认」/`Confirm`：名册移除并吊销令牌
3. 若用云 LB：deregister 该目标

## API 摘要

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/ops/fleet` | 节点名册 |
| GET | `/api/ops/fleet/status` | 对齐状态 / 版本（含 `sync_pending`） |
| POST | `/api/ops/fleet/publish` | `{ "confirm": "Confirm" }`（或「确认」）— 提升 desired 版本 |
| POST | `/api/ops/fleet/sync` | `{ "confirm": "Confirm", "name": "gateway-02" }` — 单节点立即拉取标记 |
| POST | `/api/ops/fleet/join` | `{ "name": "gateway-02" }` → `join_command` |
| POST | `/api/ops/fleet/leave` | `{ "confirm": "Confirm", "name": "…" }`（或「确认」） |
| GET | `/api/agent/config` | 节点拉取（Bearer；成功后清除该节点 sync 标记） |
| POST | `/api/agent/heartbeat` | 节点心跳（Bearer）；响应可含 `pull_now` |
