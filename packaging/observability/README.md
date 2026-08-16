# 集中观测与边缘采集

主控做监控+日志中心；节点用 Alloy 上报指标。指标来自 Envoy `/stats/prometheus`；主控日志用 **Grafana Alloy** → Loki。

## 角色默认

| 角色 | 本机跑什么 | 说明 |
|------|------------|------|
| **主控** | Envoy + Prometheus + node-exporter + Grafana + Loki + Alloy + Alertmanager | 接收节点 remote_write |
| **节点** | Envoy + Alloy（+ agent） | scrape + remote_write；无本机 Prometheus/Grafana/Loki |

## 节点指标上报

```bash
# 节点 .env（install.sh node / setup 默认写入）
# COMPOSE_PROFILES=node
# ALLOY_CONFIG_FILE=config.node.alloy
PROMETHEUS_REMOTE_WRITE_URL=http://203.0.113.10:9000/api/agent/metrics/write
```

主控 Panel（`/api/agent/metrics/write`）校验节点令牌后转发到本机 Prometheus remote_write（loopback）。
节点上：`relaygate render --observability`（同步 Bearer token）后重建 Alloy。

## 告警（主控）

- 规则：`packaging/prometheus/rules/gateway-alerts.yml`（及 log-capacity）
- Prometheus 在主控渲染时写入 `alerting → 127.0.0.1:9093`
- Alertmanager：`packaging/alertmanager/alertmanager.yml`（默认 receiver **不外发**，可在 UI `http://127.0.0.1:9093` 查看）
- 配 webhook / email：编辑 alertmanager.yml 的 `receivers`，`docker compose ... up -d --force-recreate alertmanager`

验证：

```bash
curl -sS http://127.0.0.1:9090/api/v1/rules | head
curl -sS http://127.0.0.1:9093/-/healthy
```

## 日志（Alloy → Loki）

见仓库 `docs/logging-playbook.md`（开发/深度运维）。

## 看板

Grafana 文件夹 RelayGate：`gateway-overview`（L4 连接/上游健康/限流/吞吐）与 `tcp-session-logs`。

## 采集路径（当前）

| 信号 | 路径 |
|------|------|
| 主控日志 | Alloy → Loki |
| 节点指标 | Alloy scrape + remote_write（Bearer → `/api/agent/metrics/write`） |
| 主控指标 | Prometheus scrape 本机 Envoy + node-exporter |

## 可选：独立 scrape 主机

本目录 `compose.yaml` + `prometheus.yml` 供专用观测机多网关 scrape（非机群默认）。默认机群用节点 remote_write。

```bash
cp packaging/observability/prometheus.yml /path/to/observability/prometheus.yml
# 编辑 targets 后：
cd packaging/observability && docker compose up -d
```

## 标签约定

| label | 含义 |
|-------|------|
| `gateway` | 与 `GATEWAY_NAME` / `resources.yaml` meta 一致 |
| `role` | `gateway`（Envoy admin）/ `host`（unix exporter）/ `observability` |
| `host` | 人类可读主机名（通常等于 gateway 名） |

单机开发：`relaygate render --observability` 从 `packaging/prometheus/prometheus.yml.tpl` 渲染主控 scrape 配置；节点同步 Alloy Bearer token。

## 扩容 checklist

1. 主控 `fleet join` 接入节点
2. 节点 `relaygate render --observability` 且 Alloy 在跑
3. 主控 `relaygate fleet publish`；节点自行拉取
4. Grafana 按 `gateway` 过滤

机群说明见 [docs/fleet-ops.md](../../docs/fleet-ops.md)。
