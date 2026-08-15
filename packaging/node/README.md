# 节点组件资产

「节点」是安装角色（`install.sh node`）；本机常驻的拉取/心跳进程叫 **agent**（`relaygate agent` / systemd `relaygate-agent`）。

## 默认行为（精简但上报指标）

节点**不做本地监控中心**（无 Grafana / Loki / Alertmanager），但**要向主控发指标**。默认 Compose 起：

| 组件 | 作用 |
|------|------|
| **Envoy** | L4 转发 |
| **Prometheus** + **node-exporter** | scrape 本机 Envoy/主机，经 `PROMETHEUS_REMOTE_WRITE_URL` remote_write 到主控 |
| **systemd agent** | 心跳 / 配置拉取 / 本机热更新（机群在线状态） |

- 安装按角色（`install.sh node`），一般不必手改 Compose profile
- `install.sh` / setup 在已有 `CONTROL_URL` 时写入 `PROMETHEUS_REMOTE_WRITE_URL=…/api/agent/metrics/write`

| 可选 | 作用 |
|------|------|
| 边缘 TCP 日志 | Alloy → 中心 Loki（设 `LOKI_HOST`；内部 profile `with-logs`） |

## 资产与接入

- `env.example`：复制为产品根 `.env`（`PANEL_ENABLED=0`，填写 `CONTROL_URL` 与 `AGENT_TOKEN_FILE`）
- **推荐**：主控 `fleet join` / Panel「接入」生成一句话命令
- 常驻：`relaygate agent run`（或 systemd `relaygate-agent`）

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-02 --token '<token>'

# 升级（保留 .env / 节点角色）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
```

指标与告警说明见 [observability/README.md](../observability/README.md)。
