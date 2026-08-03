# 机群运维（发布 · 接入 · 退役）

主控 Panel **机群管理**（`/fleet`）。只读角色不可发布/接入/退役。运维工具（`/ops`）仅本机诊断、摘流、探测、防火墙、档位。

正式路径：**主控发布 → 节点上的 agent 拉取 → 本机热更新**。

**节点 vs agent：** 节点 = 安装角色（`install.sh node`）；agent = 该机上的拉取/心跳守护进程（`relaygate agent` / `relaygate-agent`）。二者并存，不是两套产品角色。

## 安装角色

| 角色 | 环境模板 | 要点 |
|------|----------|------|
| **主控** | [`packaging/control/env.example`](../packaging/control/env.example) | `ENABLE_PANEL=1`；可选本机转发与中心观测 |
| **节点** | [`packaging/node/env.example`](../packaging/node/env.example) | `ENABLE_PANEL=0`；`CONTROL_URL` + `AGENT_TOKEN_FILE`；本机 `agent run` |

## 发布到机群

1. 建议先在主控 **配置应用 → 本机应用**（热更新或硬重启，确认输入「确认」/`Confirm`）
2. **发布到机群**，确认输入「确认」/`Confirm`
3. 节点 Agent 自拉并本机热更新；未跟上显示「未对齐」

```bash
relaygate fleet publish                 # 或 RELAYGATE_CONFIRM=Confirm
relaygate fleet status
relaygate agent pull                    # 节点侧拉一次
relaygate agent run                     # 节点常驻
```

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
| GET | `/api/ops/fleet/status` | 对齐状态 / 版本 |
| POST | `/api/ops/fleet/publish` | `{ "confirm": "Confirm" }`（或「确认」） |
| POST | `/api/ops/fleet/join` | `{ "name": "gateway-02" }` → `join_command` |
| POST | `/api/ops/fleet/leave` | `{ "confirm": "Confirm", "name": "…" }`（或「确认」） |
| GET | `/api/agent/config` | 节点拉取（Bearer） |
| POST | `/api/agent/heartbeat` | 节点心跳（Bearer） |
