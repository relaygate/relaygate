# 集中 Prometheus scrape 模板

本目录供 **中心观测栈**（`packaging/observability/compose.yaml`）使用，抓取多台网关的 Envoy / node_exporter 指标。

## 快速开始

1. 复制并按环境编辑 targets：

   ```bash
   cp packaging/observability/prometheus.yml /path/to/observability/prometheus.yml
   # 将 gateway-01.internal / gateway-02.internal 改为 VPN 内网 IP 或 SSH 隧道本地端口
   ```

2. 启动观测栈：

   ```bash
   cd packaging/observability && docker compose up -d
   ```

3. 每台网关边缘 Prometheus（可选）通过 `.env` 的 `PROMETHEUS_REMOTE_WRITE_URL` 联邦到中心。

## 标签约定

| label | 含义 |
|-------|------|
| `gateway` | 与 `GATEWAY_NAME` / `resources.yaml` meta 一致 |
| `role` | `gateway`（Envoy admin）/ `host`（node_exporter）/ `observability` |
| `host` | 人类可读主机名（通常等于 gateway 名） |

单机开发：`relaygate render --observability` 从 `packaging/prometheus/prometheus.yml.tpl` 渲染 **本机** `DataDir/prometheus/prometheus.yml`（仅 scrape 127.0.0.1）。

## 热更新指标

Panel `GET /api/status/xds` 返回本机热更新计数器。边缘以 Envoy `envoy_*` 与会话指标为主。

## 扩容 checklist

加一台网关节点后：

1. 用主控 `fleet join` 接入节点并启动 agent  
2. 在本目录 `prometheus.yml` 增加对应 `targets` 与 `gateway` label  
3. 主控 `relaygate fleet publish`；节点自行拉取对齐  
4. Grafana 看板按 `gateway` 变量过滤（已有 `gateway-overview`）

机群说明见 [docs/fleet-ops.md](../../docs/fleet-ops.md)。
