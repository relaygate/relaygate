# Alloy（日志采集）

Compose profile **`with-logs`** 使用 [Grafana Alloy](https://grafana.com/docs/alloy/) 采集本机 Envoy `tcp-access.json`，推送到中心 Loki。

| 负责 | 不负责（本阶段） |
|------|------------------|
| TCP access → Loki（替代 Fluent Bit） | scrape Envoy / node_exporter |
| 异常 flags / 短会话优先保留 + `LOG_SAMPLE_RATE` | remote_write 到主控（仍由本机 Prometheus） |

指标路径见 [`../observability/README.md`](../observability/README.md)「采集统一的下一步」。
