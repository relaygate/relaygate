# Alloy（日志采集）

主控 / 节点的 Compose 中，**Alloy** 采集本机 Envoy `tcp-access.json` 并推送到中心 Loki（主控随 `control`；节点可选 `alloy`）。

| 负责 | 不负责（本阶段） |
|------|------------------|
| TCP access → Loki（替代 Fluent Bit） | scrape Envoy / node_exporter |
| 异常 flags / 短会话优先保留 + `LOG_SAMPLE_RATE` | remote_write 到主控（仍由本机 Prometheus） |

指标路径见 [`../observability/README.md`](../observability/README.md)「采集统一的下一步」。
