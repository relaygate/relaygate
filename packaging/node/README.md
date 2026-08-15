# 节点组件资产

「节点」是安装角色（`install.sh node`）；本机常驻的拉取/心跳进程叫 **agent**（`relaygate agent` / systemd `relaygate-agent`）。

## 默认行为（精简）

节点默认 **仅** Compose 起 **Envoy**，加上 systemd **agent** 向主控心跳 / 拉取配置 / 本机热更新。机群在线与已应用版本由 agent 上报主控 Panel，**不**依赖节点本机 Grafana / Loki / Prometheus。

- **不需要** `MINIMAL=1`（该开关面向主控小规格精简）
- 默认 `COMPOSE_PROFILES` 为空：不启本机 Prometheus、node-exporter、Grafana、Loki、Fluent Bit

| 可选 profile | 作用 |
|--------------|------|
| `with-metrics` | 本机 Prometheus + node-exporter；可经 `PROMETHEUS_REMOTE_WRITE_URL` remote_write 到主控 |
| `with-logs` | Fluent Bit 采集边缘 TCP access → 中心 Loki（`LOKI_HOST`） |

## 资产与接入

- `env.example`：复制为产品根 `.env`（`PANEL_ENABLED=0`，填写 `CONTROL_URL` 与 `AGENT_TOKEN_FILE`）
- **推荐**：主控 `fleet join` / Panel「接入」生成一句话命令，目标机 root 执行即可安装并启动 `relaygate-agent`
- 常驻：`relaygate agent run`（或 systemd `relaygate-agent`：心跳 + 拉配置 + 本机热更新）

一键安装（示例）/ 升级：

```bash
# 接入（令牌来自主控 fleet join；默认精简，无需 MINIMAL=1）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-02 --token '<token>'

# 可选：安装时即开边缘指标和/或日志
# COMPOSE_PROFILES=with-metrics
# COMPOSE_PROFILES=with-logs
# COMPOSE_PROFILES=with-metrics,with-logs

# 升级（保留 .env / 节点角色）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
```

已装节点若升级后需要继续本机 Prometheus：在 `.env` 设 `COMPOSE_PROFILES=with-metrics`，再 `relaygate render --observability` 与 compose `--profile with-metrics up -d`。详见根目录 [CHANGELOG.md](../../CHANGELOG.md) Breaking 节与 [observability/README.md](../observability/README.md)。
