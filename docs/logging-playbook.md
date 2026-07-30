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
| `rule` / `upstream` / `flags` / `duration_ms` / `conn_id` | JSON / LogQL `\| json` | 查询时解析；`conn_id` = Envoy `%CONNECTION_ID%` |

## 真实客户端 IP 与排障关联

### 目标拆解

| 目标 | 要什么 | 本仓库现状 / 做法 |
|------|--------|-------------------|
| **真实客户端 IP** | TCP access 的 `downstream`（及 nft 限速/ACL 所见源 IP）= 玩家公网 IP | 字段已是 `%DOWNSTREAM_REMOTE_ADDRESS%`；值取决于链路（见下） |
| **排障关联** | 同一会话能对齐到网关、转发、时间窗、指标 | label `gateway` + JSON `rule`/`conn_id`/`ts`；Prometheus 同 `gateway` 时间对齐 |

### `downstream` 实际是什么

```text
【产品主路径】公网直连暴露（PROXY_PROTOCOL=off，默认）：
  玩家 ──TCP──▶ Envoy → downstream = 玩家 IP（TCP peer）
  多上游 = 网关 rules 转发，前面没有云 LB → 不需要、也不应开 PROXY

NLB 仅 preserve、不发 PROXY：
  玩家 → NLB ──源 IP 保留──▶ Envoy → 保持 off → downstream = 玩家 IP

NLB 发 PROXY v2（PROXY_PROTOCOL=v2，端口仅对 LB）：
  玩家 → NLB ──PROXY──▶ Envoy → downstream = 头内客户端 IP

兼容模式 v2-compat（有头用头、无头用 peer）：
  仅当入口已锁 LB 网段；【禁止】公网直连开 compat（可伪造 PROXY 头）
```

### 能否「自判断」？

**安装时无法可靠自动判断**前面有没有云 LB（不要用「上游数量」推断）。产品策略：

| 做法 | 说明 |
|------|------|
| **默认 off** | 适合直连暴露；安全优先 |
| **用户显式配置** | 有 LB 发 PROXY 时再写 `v2` / `v2-compat` |
| **弱启发（仅文档）** | 若部署了 `packaging/terraform/nlb` 且 `enable_proxy_protocol_v2=true`，则网关应对齐 `v2`；**从不**因多上游或公网 IP 自动开 PROXY/compat |

### 配置项（默认 `PROXY_PROTOCOL=off`）

| 变量 / 模式 | 默认 | 行为 |
|-------------|------|------|
| `PROXY_PROTOCOL=off` | **是** | 无 PROXY filter；`downstream`=TCP peer |
| `PROXY_PROTOCOL=v2`（或 `on`） | | 强制 PROXY v2；无头则失败 |
| `PROXY_PROTOCOL=v1` | | 强制 PROXY v1 |
| `PROXY_PROTOCOL=v2-compat`（或 `compat`） | | v2 + `allow_requests_without_proxy_protocol` |
| `PROXY_PROTOCOL=v2` + `PROXY_PROTOCOL_ALLOW_WITHOUT=1` | | 同上兼容模式 |

Envoy 兼容语义：无 PROXY 头 → 放行并用 TCP peer；有头 → 用头内 IP。

**安全边界（大字）：**

- **公网直连暴露端口 → 必须 `off`。** 即使 compat 能「连上」，攻击者仍可发送伪造 PROXY 头，骗过 access log；若把 `downstream` 用于 ACL/封禁则更危险。
- **`v2` / `v2-compat` 仅当转发口只对 LB 网段开放时使用。** Envoy 不按 CIDR 校验谁能发 PROXY 头。
- 多上游、双活、网关自身转发 **都不**构成开启 PROXY 的理由。

对齐步骤（可选前置云 LB）：

1. SG/nft：转发口仅 LB。  
2. NLB TG：`enable_proxy_protocol_v2=true`。  
3. 网关：`PROXY_PROTOCOL=v2`（迁移可短暂 `v2-compat`）→ `relaygate reload`。  
4. Loki 确认 `downstream`；确认无公网直打转发口。

### 排障关联字段

| 字段 | 来源 | 用途 |
|------|------|------|
| `gateway` | Fluent Bit label（`GATEWAY_NAME`） | 与 Prom `gateway=` 对齐 |
| `rule` | Envoy JSON | 对齐 `ingress-{rule}` / 限速 stat |
| `conn_id` | `%CONNECTION_ID%` | 同连接多条日志 / 人工粘贴关联（非跨机全局 ID） |
| `ts` / `duration_ms` | Envoy | 与告警时间窗对齐 |
| `downstream` | 见上 | **禁止**做 Loki label |

#### 按 `conn_id` 查询（Grafana / Explore）

1. Panel → Grafana → **TCP Session Logs**；日志行旁展示 **`conn_id`**（以及 `rule` / `downstream` / `flags`）。  
2. 顶部变量 **conn_id**：填入数值后看面板「按 conn_id」；`.*` = 不过滤。  
3. Explore / LogQL：

```logql
{job="envoy-tcp-access", gateway="gateway-01"} | json | conn_id="12345"
{job="envoy-tcp-access", gateway="gateway-01"} |= `"conn_id":"12345"`
```

注意：`conn_id` 仅在同一 Envoy 进程生命周期内有意义；reload 后会重新编号。

### 做不到（明确边界）

- **游戏服（上游）仍见网关出口 IP**（README 已知边界）。要让上游见玩家 IP 需 TPROXY/SNAT 回程等更大改动，不在本方案范围。  
- **UDP access NDJSON** 仍未默认开启（见下节）；UDP 直连时源 IP 已是 peer。  
- `conn_id` **不是**跨网关 / 跨 reload 的全局 trace id。  
- **不会**根据上游数量或公网暴露自动开启 PROXY。

## LogQL 速查（对齐 Prometheus）

| 意图 | LogQL | 对照 Prometheus |
|------|-------|-----------------|
| 某网关近 1h | `{job="envoy-tcp-access", gateway="gateway-01"}` | `up{job="envoy", gateway="gateway-01"}` |
| 按客户端 IP | `{job="envoy-tcp-access", gateway="gateway-01"} \|= \`"downstream":"203.0.113.9\` | 主机侧 ACL / `HostNetworkErrors` |
| 按连接 ID | `{job="envoy-tcp-access", gateway="gateway-01"} \| json \| conn_id="12345"` | 同机同进程内会话粘贴关联 |
| 按转发 | `{job="envoy-tcp-access"} \| json \| rule="forward-server-01-production-tcp"` | `envoy_local_rate_limit_*` / cluster 健康 |
| 按上游 | `{job="envoy-tcp-access"} \| json \| upstream=~"10.0.0.11:.*"` | `envoy_cluster_membership_healthy{envoy_cluster_name="upstream-server-01-tcp"}` |
| 异常 flags | `{job="envoy-tcp-access"} \| json \| flags != "-"` | `EnvoyNoHealthyUpstream`、`EnvoyCircuitBreakerOpen` |
| 短会话 | `{job="envoy-tcp-access"} \| json \| duration_ms < 2000` | UDP/TCP 错误率、限速命中 |

Explore 深链（经 Panel 反代）：`/grafana/explore?...`（运维在 Grafana Explore 中查询即可）。

## 排查步骤（与指标对齐）

1. **Prometheus**：目标网关 `up`、上游健康、限速 / 熔断告警是否触发。  
2. **Explore**：同一 `gateway` + 时间窗，先 `flags != "-"`，再按 `rule` / `upstream` 收窄。  
3. **IP**：用 `|=` 子串匹配 `downstream`（勿建 label）。结合 nft ACL。  
4. **看板**：Grafana → RelayGate → **TCP Session Logs**（异常 / 短会话 / **conn_id** / rule TopN）。  
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
