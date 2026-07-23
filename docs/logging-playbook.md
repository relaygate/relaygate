# TCP Access 日志：启用与排查 Playbook

边缘用 **Fluent Bit** 采集 Envoy `tcp-access.json`（NDJSON），写入中心 **Loki**；Panel / Grafana **不**自建日志库，也**不**嵌 Envoy Admin。查询走 Grafana Explore（或 Panel「会话日志」深链）。

## 架构

```text
Envoy → ${RELAYGATE_DATA_DIR}/envoy/logs/tcp-access.json
                 ↓ (Fluent Bit, 每机 with-logs)
            Loki :3100（with-loki，可与 Grafana 同机）
                 ↓
         Grafana datasource uid=loki → Explore / 看板 TCP Session Logs
```

| 角色 | COMPOSE_PROFILES | 说明 |
|------|------------------|------|
| 主管理节点 | `with-grafana,with-loki,with-logs` | 本机 Loki + 采集 + Grafana |
| 从节点 / 边缘 | `with-logs` | 仅 Fluent Bit；`LOKI_HOST=<中心私网>` |
| 专用可观测主机 | 见 `packaging/observability` + profile `with-loki` | 边缘推中心 |

## 启用（P0）

1. `.env`（示例见仓库 `.env.example` / `gateway-01.env.example`）：

```bash
COMPOSE_PROFILES=with-grafana,with-loki,with-logs
# 同机可省略；从节点必填中心地址
# LOKI_HOST=127.0.0.1
# LOKI_PORT=3100
# LOKI_RETENTION_PERIOD=168h   # 对照 packaging/loki/loki-config.yml retention_period
# LOG_SAMPLE_RATE=1.0         # P2：正常会话采样；异常 flags/短会话始终全量
# LOG_SHORT_SESSION_MS=2000
```

2. 安装 logrotate（`apply` 在 root 下自动渲染）：

```bash
sudo ./bin/relaygate apply
# 或手工：
# sed "s|@DATA_DIR@|$RELAYGATE_DATA_DIR|g" \
#   packaging/logrotate/envoy-tcp-access \
#   | sudo tee /etc/logrotate.d/relaygate-envoy-tcp-access
```

3. 拉起并检查：

```bash
docker compose -f packaging/compose.yaml --env-file .env up -d
curl -sS http://127.0.0.1:3100/ready
curl -sS 'http://127.0.0.1:3100/loki/api/v1/label/job/values'
# 期望含 envoy-tcp-access
```

Grafana provisioning 已含 Loki datasource（`uid: loki`）与看板 **TCP Session Logs**。

## 标签约定

| 字段 | 位置 | 说明 |
|------|------|------|
| `job` | label | 固定 `envoy-tcp-access` |
| `gateway` | label | 必选，低基数实例名 |
| `downstream`（客户端 IP） | **仅 JSON 内容** | **禁止**做 label |
| `rule` / `upstream` / `flags` / `duration_ms` | JSON / LogQL `\| json` | 查询时解析 |

## LogQL 速查（对齐 Prometheus）

| 意图 | LogQL | 对照 Prometheus |
|------|-------|-----------------|
| 某网关近 1h | `{job="envoy-tcp-access", gateway="gateway-01"}` | `up{job="envoy", gateway="gateway-01"}` |
| 按客户端 IP | `{job="envoy-tcp-access", gateway="gateway-01"} \|= \`"downstream":"203.0.113.9\` | 主机侧 ACL / `HostNetworkErrors` |
| 按转发规则 | `{job="envoy-tcp-access"} \| json \| rule="forward-server-01-production-tcp"` | `envoy_local_rate_limit_*` / cluster 健康 |
| 按上游 | `{job="envoy-tcp-access"} \| json \| upstream=~"10.0.0.11:.*"` | `envoy_cluster_membership_healthy{envoy_cluster_name="upstream-server-01-tcp"}` |
| 异常 flags | `{job="envoy-tcp-access"} \| json \| flags != "-"` | `EnvoyNoHealthyUpstream`、`EnvoyCircuitBreakerOpen` |
| 短会话 | `{job="envoy-tcp-access"} \| json \| duration_ms < 2000` | UDP/TCP 错误率、限速命中 |

Explore 深链（经 Panel 反代）：`/grafana/explore?...`（Panel「监控」页可生成）。

## 排查步骤（与指标对齐）

1. **Prometheus**：目标网关 `up`、上游健康、限速 / 熔断告警是否触发。  
2. **Explore**：同一 `gateway` + 时间窗，先 `flags != "-"`，再按 `rule` / `upstream` 收窄。  
3. **IP**：用 `|=` 子串匹配 `downstream`（勿建 label）。结合 nft ACL。  
4. **看板**：Grafana → RelayGate → **TCP Session Logs**（异常 / 短会话 / rule TopN）。  
5. **容量**：根分区与 `DataDir/envoy/logs`；logrotate；`LOKI_RETENTION` / `LOG_SAMPLE_RATE`。

## 多机（P1）

从节点 `.env`：

```bash
COMPOSE_PROFILES=with-logs
LOKI_HOST=10.0.0.1   # 中心私网；勿对公网暴露 3100
LOKI_PORT=3100
# 勿开 with-grafana / with-loki（由中心承担）
```

中心需能从边缘访问 `LOKI_HOST:3100`（VPN / 私网安全组）。边缘只跑 Fluent Bit，不落中心以外的第二份 Loki（除非刻意冷备）。

## UDP access 结论（P1）

当前 Envoy UDP proxy **未**配置与 TCP 同级的 FileAccessLog NDJSON。UDP 会话指标已由 Prometheus（`envoy_udp_*`）覆盖；若补 UDP access：

- **成本**：需改 `core/render` UDP listener，并评估 PPS 下的日志量（通常远高于 TCP 连接日志）。  
- **建议**：默认不上 UDP access；故障优先看指标 + nft / 抓包。若业务强依赖按 IP 审计 UDP，再以独立文件 + 更高采样率接入同一 `job` 或 `job=envoy-udp-access`。

## 采样与保留（P2）

| 变量 | 默认 | 含义 |
|------|------|------|
| `LOG_SAMPLE_RATE` | `1.0` | 正常会话保留概率；`flags≠-` 与 `duration_ms < LOG_SHORT_SESSION_MS` 始终保留 |
| `LOG_SHORT_SESSION_MS` | `2000` | 短会话阈值 |
| Loki `retention_period` | `168h` | 见 `packaging/loki/loki-config.yml` |

告警草案：`packaging/loki/rules/fake/tcp-access-alerts.yml`（Loki ruler）；磁盘类继续用 Prometheus `HostDiskAlmostFull`。
