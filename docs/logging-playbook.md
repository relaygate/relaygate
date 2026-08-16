# TCP Access 日志：启用与排查 Playbook

边缘用 **Grafana Alloy** 采集 Envoy `tcp-access.json`（NDJSON），写入中心 **Loki**；Panel / Grafana **不**自建日志库。查询走 Grafana Explore（或 Panel「会话日志」深链）。

## 架构

```text
Envoy → ${RELAYGATE_DATA_DIR}/envoy/logs/tcp-access.json
                 ↓ (Alloy：主控 control)
            Loki :3100（主控）
                 ↓
         Grafana datasource uid=loki → Explore / 看板 TCP Session Logs

指标：Envoy /stats/prometheus →（主控）Prometheus；（节点）Alloy remote_write → 主控
告警（主控）：Prometheus rules → Alertmanager :9093
```

| 角色 | 默认行为 | 说明 |
|------|----------|------|
| 主控 | 观测全开 | Prometheus + Loki + Alloy（日志）+ Grafana + Alertmanager |
| 节点 | 上报指标 | Envoy + Alloy（指标 remote_write）；无本机 Prometheus/Grafana/Loki |

## 启用

1. `.env`（示例见 `packaging/shared/env.example` / `control.env.example`）：

```bash
# 主控：setup 默认已写入全开观测 profiles
# LOKI_HOST=127.0.0.1   # 同机可省略
# LOKI_PORT=3100
# LOG_SAMPLE_RATE=1.0   # 正常会话采样；异常 flags / 短会话始终全量
```

2. 安装 logrotate（`apply` 在 root 下自动渲染）：

```bash
sudo ./bin/relaygate apply
```

3. 拉起并检查：

```bash
docker compose -f packaging/compose.yaml --env-file .env up -d
curl -sS http://127.0.0.1:3100/ready
curl -sS 'http://127.0.0.1:3100/loki/api/v1/label/job/values'
# 期望含 envoy-tcp-access
```

## 采样

| 行为 | 说明 |
|------|------|
| 默认 | **全量**保留（`LOG_SAMPLE_RATE=1.0`） |
| 降采样 | Alloy `stage.match` 不支持 `| json`；请在 Loki/Explore 用 `flags` / `duration_ms` 过滤，或改 `packaging/alloy/config.alloy` |

## 标签约定

| 字段 | 位置 | 说明 |
|------|------|------|
| `job` | label | 固定 `envoy-tcp-access` |
| `gateway` | label | 必选，低基数实例名 |
| `downstream`（客户端 IP） | **仅 JSON 内容** | **禁止**做 label |
| `rule` / `upstream` / `flags` / `duration_ms` / `conn_id` | JSON / LogQL `\| json` | 查询时解析 |

## 真实客户端 IP 与排障关联

| 目标 | 要什么 | 做法 |
|------|--------|-------------------|
| **真实客户端 IP** | TCP access 的 `downstream` | 字段已是 `%DOWNSTREAM_REMOTE_ADDRESS%`；值取决于链路（见下） |
| **排障关联** | 同一会话对齐网关/转发/时间窗 | label `gateway` + JSON `rule`/`conn_id`/`ts`；Prometheus 同 `gateway` |

### `downstream` 实际是什么

```text
公网直连（PROXY_PROTOCOL=off，默认）：
  客户端 ──TCP──▶ Envoy → downstream = 客户端 IP（TCP peer）

NLB 仅 preserve、不发 PROXY：
  客户端 → NLB ──源 IP 保留──▶ Envoy → 保持 off → downstream = 客户端 IP

NLB 发 PROXY v2（PROXY_PROTOCOL=v2，端口仅对 LB）：
  客户端 → NLB ──PROXY──▶ Envoy → downstream = 头内客户端 IP
```

| 做法 | 说明 |
|------|------|
| **默认 off** | 适合直连暴露；安全优先 |
| **用户显式配置** | 有 LB 发 PROXY 时再写 `v2` / `v2-compat` |

**安全边界：** 公网直连暴露端口必须 `off`；`v2` / `v2-compat` 仅当转发口只对 LB 网段开放。

### 排障关联字段

| 字段 | 来源 | 用途 |
|------|------|------|
| `gateway` | Alloy label（`GATEWAY_NAME`） | 与 Prom `gateway=` 对齐 |
| `rule` | Envoy JSON | 对齐限速 / cluster |
| `conn_id` | `%CONNECTION_ID%` | 同连接多条日志关联 |
| `downstream` | 见上 | **禁止**做 Loki label |

LogQL 示例：

```logql
{job="envoy-tcp-access", gateway="gateway-01"} | json | conn_id="12345"
{job="envoy-tcp-access"} | json | flags != "-"
```

## 多机

TCP access 日志默认在**主控** Alloy → 本机 Loki。节点 Alloy 只做指标 remote_write（`config.node.alloy`）。

## UDP access

当前 Envoy UDP proxy **未**配置与 TCP 同级的 FileAccessLog NDJSON。UDP 会话指标已由 Prometheus / Alloy（`envoy_udp_*`）覆盖。

## 告警

- Prometheus：`packaging/prometheus/rules/gateway-alerts.yml` → Alertmanager
- Loki ruler 草案：`packaging/loki/rules/fake/tcp-access-alerts.yml`（已指 Alertmanager；外发仍看 receivers）
