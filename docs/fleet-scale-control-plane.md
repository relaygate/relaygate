# 机群架构（主控 · 网关节点）

单一产品、两个安装角色。日常运维步骤见 [fleet-ops.md](fleet-ops.md)；菜单与确认词见 [product-surface-agent.md](product-surface-agent.md)。

```text
客户端 → 云 L4 LB（可选）
           ├─ gateway-01 主控（Panel + 发布 + 名册；可选本机转发）
           └─ gateway-02… 网关节点（Envoy + agent；无完整 Panel）
```

## 角色

| | 主控 | 网关节点 |
|--|------|----------|
| 模板 | `packaging/control/env.example` | `packaging/node/env.example` |
| 启用 | `ENABLE_PANEL=1` | `ENABLE_PANEL=0`；`PRIMARY_URL` + `AGENT_TOKEN_FILE` |
| 职责 | 唯一可写意图源、发布、名册、可选中心观测 | 拉取配置、本机热更新、转发 |

## 稳态数据流

1. 主控编辑业务配置并（建议）先本机应用
2. **发布到机群**（确认词 `PUBLISH_FLEET`）
3. 节点 Agent 拉取 → 落盘 → 本机 HotApply
4. Envoy 只连本机 loopback ADS（`127.0.0.1`），不指远程主控

配置变更与流量伸缩解耦：NLB/TG 增删目标由人工或 Terraform 完成。

## 摘流与硬重启

| 场景 | 行为 |
|------|------|
| 日常热更新 | 不摘流、不重启 Envoy；无关长连接通常保留 |
| 硬重启 / 升级 / 退役前 | 先 `drain fail`，再操作，再 `drain ok` |
| 长连接 | 不随云 LB 目标组迁移自动断开 |

## 失败态（运维）

| 状态 | 含义 | 下一步 |
|------|------|--------|
| 已对齐 | 已应用当前发布版本 | — |
| 未对齐 | 版本落后或应用失败 | 查节点日志；`agent pull` / 修配置后重发 |
| 离线 | 长时间无心跳 | 查网络、agent 服务、令牌 |
| 未授权 | 令牌无效 | 重新 `fleet join` |

## 确认词（机群相关）

`PUBLISH_FLEET` · `FLEET_LEAVE` · 本机 `HOT_APPLY` / `RELOAD_ENVOY` · 摘流 `DRAIN_FAIL` / `DRAIN_OK`

接入 `fleet join` 无需确认词（一句话安装命令）。
