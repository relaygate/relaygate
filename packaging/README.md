# packaging/ — 安装与运行资产

按 **主控 / 节点 / 共享** 划分（单一产品，非双产品线）：

| 目录 | 角色 | 内容 |
|------|------|------|
| [`control/`](control/) | 主控组件 | `env.example`（Panel + 发布/名册）；中心观测相关见 `grafana/` `loki/` `observability/` |
| [`node/`](node/) | 节点组件 | `env.example`（`ENABLE_PANEL=0`、`CONTROL_URL`、`AGENT_TOKEN_FILE`） |
| [`shared/`](shared/) | 共用 | 默认 `.env`、resources、历史 inventory 模板 |
| 根下 Compose/采集 | 共用运行时 | `compose.yaml`、`prometheus/`、`fluent-bit/`、`firewall/`、`sysctl/`、`profiles/` |
| [`systemd/`](systemd/) | 宿主服务 | Panel（主控）；节点用 `relaygate agent run`（可自建 unit） |
| [`terraform/`](terraform/) | 可选云 LB | NLB 人工挂载清单 |

安装示例：

```bash
# 主控
cp packaging/control/env.example .env && chmod 600 .env

# 节点
cp packaging/node/env.example .env && chmod 600 .env
relaygate agent run
```

