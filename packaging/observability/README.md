# 集中观测与边缘采集

主控做**监控+日志中心**；节点**上报指标**、可选推送 TCP access 日志。指标继续吃 Envoy `/stats/prometheus`；采集侧日志统一 **Grafana Alloy**。

## 角色默认（安装按 control / node，不必手传 with-*）

| 角色 | 本机跑什么 | 说明 |
|------|------------|------|
| **主控** | Envoy + Prometheus + node-exporter + Grafana + Loki + Alloy + Alertmanager | 观测全开；接收节点 remote_write |
| **节点** | Envoy + Prometheus + node-exporter（+ systemd agent） | 精简 = 无本地 Grafana/Loki；指标 remote_write 到主控 |
| **节点 + 边缘日志** | 上列再加 Alloy | `.env` 增加日志 profile（内部名 `with-logs`）并设 `LOKI_HOST` |

内部 Compose profile（`COMPOSE_PROFILES`）仅实现细节；`GRAFANA_ENABLED` / `MINIMAL` 为兼容开关，**主控不要用 MINIMAL 精简**。

## 节点指标上报

```bash
# 节点 .env（install.sh node / setup 默认写入）
# COMPOSE_PROFILES 由角色生成（含 with-metrics）
PROMETHEUS_REMOTE_WRITE_URL=http://203.0.113.10:9000/api/agent/metrics/write
```

主控 Panel（`/api/agent/metrics/write`）校验节点令牌后转发到本机 Prometheus remote_write（loopback）。
然后在节点上：`relaygate render --observability` 并重建 Prometheus（见下）。

## 告警（主控）

- 规则：`packaging/prometheus/rules/gateway-alerts.yml`（及 log-capacity）
- Prometheus 在主控渲染时写入 `alerting → 127.0.0.1:9093`
- Alertmanager：`packaging/alertmanager/alertmanager.yml`（默认 receiver **不外发**，可在 UI `http://127.0.0.1:9093` 查看）
- 配 webhook / email：编辑 alertmanager.yml 的 `receivers`，`docker compose ... up -d --force-recreate alertmanager`

验证：

```bash
curl -sS http://127.0.0.1:9090/api/v1/rules | head
curl -sS http://127.0.0.1:9093/-/healthy
# 人为制造 up==0 或看 Alertmanager UI 是否出现 firing（配好 receiver 后才有外发）
```

## 日志（Alloy → Loki）

见 [docs/logging-playbook.md](../../docs/logging-playbook.md)。`with-logs` 现为 Alloy，不再启动 Fluent Bit。

## 看板

Grafana 文件夹 RelayGate：`gateway-overview`（L4 连接/上游健康/限流/吞吐）与 `tcp-session-logs`。
可另从 Grafana.com 导入社区 Envoy 看板作补充；产品默认看板已覆盖运维主路径。

## 采集统一的下一步

| 已统一 | 本阶段仍分离 |
|--------|----------------|
| 日志：Alloy 替 Fluent Bit | 节点指标：本机 Prometheus scrape + remote_write |
| 主控 Alertmanager 可投递 | 节点不用 Alloy 兼任 scrape（避免与现有 Bearer/`/api/agent/metrics/write` 路径并行两套） |

后续若 Alloy `prometheus.scrape` + `prometheus.remote_write`（Bearer）在节点验证稳定，可再去掉边缘 Prometheus/node-exporter，由 Alloy 一并承担。

## 可选：独立 scrape 主机

本目录 `compose.yaml` + `prometheus.yml` 供专用观测机多网关 scrape（非机群默认路径）。默认机群用节点 remote_write。

```bash
cp packaging/observability/prometheus.yml /path/to/observability/prometheus.yml
# 编辑 targets 后：
cd packaging/observability && docker compose up -d
```

## 标签约定

| label | 含义 |
|-------|------|
| `gateway` | 与 `GATEWAY_NAME` / `resources.yaml` meta 一致 |
| `role` | `gateway`（Envoy admin）/ `host`（node_exporter）/ `observability` |
| `host` | 人类可读主机名（通常等于 gateway 名） |

单机开发：`relaygate render --observability` 从 `packaging/prometheus/prometheus.yml.tpl` 渲染本机 scrape 配置。

## 扩容 checklist

1. 主控 `fleet join` 接入节点（默认指标上报）
2. 节点 `relaygate render --observability` 且 metrics profile 在跑
3. 主控 `relaygate fleet publish`；节点自行拉取
4. Grafana 按 `gateway` 过滤

机群说明见 [docs/fleet-ops.md](../../docs/fleet-ops.md)。
