# 节点组件资产

- `env.example`：复制为产品根 `.env`（`ENABLE_PANEL=0`，填写 `PRIMARY_URL` 与 `AGENT_TOKEN_FILE`）
- **推荐**：主控 `fleet join` / Panel「接入」生成一句话命令，目标机 root 执行即可安装并启动 `relaygate-agent`
- 常驻：`relaygate agent run`（或 systemd `relaygate-agent`：心跳 + 拉配置 + 本机热更新）

一键安装（示例）/ 升级：

```bash
# 接入（令牌来自主控 fleet join）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo env PRIMARY_URL='http://203.0.113.10:9000' GATEWAY_NAME=gateway-02 \
      AGENT_TOKEN='<token>' ENABLE_PANEL=0 NONINTERACTIVE=1 bash -s -- -y

# 升级（保留 .env / agent 角色）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- --upgrade -y
```
