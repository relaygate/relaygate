# 节点组件资产

「节点」是安装角色（`install.sh node`）；本机常驻的拉取/心跳进程叫 **agent**（`relaygate agent` / systemd `relaygate-agent`）。

## 默认行为（精简但上报指标）

节点**不做本地监控中心**（无 Grafana / Loki），但**要向主控发指标**。默认 Compose 起：

| 组件 | 作用 |
|------|------|
| **Envoy** | L4 转发 |
| **Prometheus** + **node-exporter**（`with-metrics`） | scrape 本机 Envoy/主机，经 `PROMETHEUS_REMOTE_WRITE_URL` remote_write 到主控 |
| **systemd agent** | 心跳 / 配置拉取 / 本机热更新（机群在线状态） |

- 安装按角色（`install.sh node`），**无需**手传 `with-*`，也**不需要** `MINIMAL=1`
- 默认 `COMPOSE_PROFILES=with-metrics`：启本机 Prometheus + node-exporter；**不**启 Grafana、Loki、Fluent Bit
- `install.sh` / setup 在已有 `CONTROL_URL` 时写入 `PROMETHEUS_REMOTE_WRITE_URL=…/api/agent/metrics/write`

| 可选 profile | 作用 |
|--------------|------|
| `with-logs` | Fluent Bit 采集边缘 TCP access → 中心 Loki（`LOKI_HOST`）；与 `with-metrics` 并用：`with-metrics,with-logs` |

## 资产与接入

- `env.example`：复制为产品根 `.env`（`PANEL_ENABLED=0`，填写 `CONTROL_URL` 与 `AGENT_TOKEN_FILE`）
- **推荐**：主控 `fleet join` / Panel「接入」生成一句话命令，目标机 root 执行即可安装并启动 `relaygate-agent`
- 常驻：`relaygate agent run`（或 systemd `relaygate-agent`：心跳 + 拉配置 + 本机热更新）

一键安装（示例）/ 升级：

```bash
# 接入（令牌来自主控 fleet join；默认 with-metrics → 指标上报主控）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-02 --token '<token>'

# 可选：再开边缘 TCP 日志
# COMPOSE_PROFILES=with-metrics,with-logs

# 升级（保留 .env / 节点角色；缺 with-metrics 时 setup 会补上）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
```

指标路径与渲染说明见 [observability/README.md](../observability/README.md) 与根目录 [CHANGELOG.md](../../CHANGELOG.md)。
