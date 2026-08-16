# 节点组件资产

安装角色 `node`；本机守护进程为 **agent**（`relaygate agent` / `relaygate-agent`）。

## 默认行为

节点不做本地监控中心（无 Prometheus / Grafana / Loki / Alertmanager），向主控上报指标。默认 Compose：

| 组件 | 作用 |
|------|------|
| **Envoy** | L4 转发 |
| **Alloy** | scrape Envoy + 主机指标，经 `PROMETHEUS_REMOTE_WRITE_URL` remote_write 到主控 |
| **systemd agent** | 心跳 / 配置拉取 / 本机热更新 |

## 资产与接入

- `env.example`：复制为产品根 `.env`（`PANEL_ENABLED=0`，填写 `CONTROL_URL` 与 `AGENT_TOKEN_FILE`）
- **推荐**：主控 `fleet join` / Panel「接入」生成一句话命令
- 常驻：`relaygate agent run`（或 systemd `relaygate-agent`）

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-02 --token '<token>'

# 升级
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
```

指标与告警见 [observability/README.md](../observability/README.md)。
