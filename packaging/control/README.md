# 主控组件资产

- `env.example`：复制为产品根 `.env`（`ENABLE_PANEL=1` `PANEL_ROLE=primary`）
- 中心 Grafana/Loki 配置仍位于 `packaging/grafana`、`packaging/loki`（Compose profile）

一键安装 / 升级：

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo env ENABLE_PANEL=1 NONINTERACTIVE=1 bash -s -- -y

curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- --upgrade -y
```

首启默认管理员密码：`relaygate`（生产务必改密）。
