# Alloy

| 角色 | 配置 | 作用 |
|------|------|------|
| **主控** | `config.alloy` | Envoy TCP access → 本机 Loki |
| **节点** | `config.node.alloy` | scrape Envoy `/stats/prometheus` + 主机指标（`prometheus.exporter.unix`），Bearer `remote_write` 到主控 |

`setup` 按角色写入 `ALLOY_CONFIG_FILE`。节点令牌：`render --observability` 同步 `AGENT_TOKEN_FILE` → `DataDir/prometheus/agent.token`。

指标与主控栈见 [`../observability/README.md`](../observability/README.md)。
