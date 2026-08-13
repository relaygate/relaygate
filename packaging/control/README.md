# 主控组件资产

- `env.example`：复制为产品根 `.env`（`PANEL_ENABLED=1`；Panel 可写角色）
- 中心 Grafana/Loki 配置仍位于 `packaging/grafana`、`packaging/loki`（Compose profile）

一键安装 / 升级：

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- control

curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
```

首启默认管理员密码：`relaygate`（生产务必改密）。
