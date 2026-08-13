# 节点组件资产

「节点」是安装角色（`install.sh node`）；本机常驻的拉取/心跳进程叫 **agent**（`relaygate agent` / systemd `relaygate-agent`）。

- `env.example`：复制为产品根 `.env`（`PANEL_ENABLED=0`，填写 `CONTROL_URL` 与 `AGENT_TOKEN_FILE`）
- **推荐**：主控 `fleet join` / Panel「接入」生成一句话命令，目标机 root 执行即可安装并启动 `relaygate-agent`
- 常驻：`relaygate agent run`（或 systemd `relaygate-agent`：心跳 + 拉配置 + 本机热更新）

一键安装（示例）/ 升级：

```bash
# 接入（令牌来自主控 fleet join）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-02 --token '<token>'

# 升级（保留 .env / 节点角色）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
```
