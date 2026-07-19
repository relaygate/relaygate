# RelayGate 安全清单

## 上线前必做

1. **更换已泄露的 root 密码**（曾在聊天中明文出现）
2. 配置 SSH 公钥登录：
   ```bash
   mkdir -p ~/.ssh && chmod 700 ~/.ssh
   echo 'ssh-ed25519 AAAA... your-key' >> ~/.ssh/authorized_keys
   chmod 600 ~/.ssh/authorized_keys
   ```
3. 加固 SSH（确认密钥可登录后再改）：
   ```text
   Port 30455
   PermitRootLogin prohibit-password
   PasswordAuthentication no
   PubkeyAuthentication yes
   ```
4. `.env` 权限 `600`，永不提交仓库（含 `GRAFANA_ADMIN_PASSWORD`、`PANEL_ADMIN_PASSWORD`）
5. 确认管理端口仅本机监听：`9000 / 9901 / 9090 / 3000 / 9100`（对外只需隧道 **9000**）
6. 游戏后端防火墙仅允许网关回源 IP；双活时放行 **全部** `GATEWAY_PUBLIC_IP`（见 [`HA.md`](HA.md)）
7. 生产密钥走密钥管理 / CI Secrets，不要提交 `.env` / `terraform.tfvars`
8. 容器镜像使用不可变 tag（git SHA），禁止生产 `latest`

## Panel 安全模型（systemd 二进制）

- 专用用户/组 `relaygate`，**不在** `docker` 组；Panel **不**挂载 `docker.sock`
- 可写路径仅：`config/resources.yaml`、`gateway/generated/`、`deploy/firewall/generated/`、`backups/`
- 二进制 / scripts / web：root-owned，Panel 不可改
- Apply：仅允许 `sudo -n /usr/local/libexec/relaygate/apply`（sudoers 白名单）；helper 为 root:root 0755
- systemd：`ProtectSystem=strict`、`PrivateTmp`、`UMask=0077` 等；**不**启用 `NoNewPrivileges` / `RestrictSUIDSGID`（否则破坏 sudo helper）
- 密钥：`panel_admin_password` 为 `root:relaygate` 0640；`grafana_admin_password` 仅 root 0600

## 防火墙

- 使用 `sudo bash scripts/apply_firewall.sh`（规则在 `deploy/firewall/`）
- 应用前保留现有 SSH 会话 + 云控制台
- SSH `30455` 必须放行

## 日志

- Compose 已限制 json-file 大小
- Panel：`journalctl -u relaygate-panel`
- 可选安装 `deploy/logrotate/envoy-gateway` 到 `/etc/logrotate.d/`

## 访问 Panel / Grafana

Panel 为唯一管理出口：会话 Cookie 鉴权后反代 `GRAFANA_URL`（固定 loopback，禁止开放代理）。
Grafana 绑定 `127.0.0.1:3000`，匿名 Viewer 无法从公网或未隧道主机直达。

```bash
ssh -p 30455 \
  -L 9000:127.0.0.1:9000 \
  root@107.149.191.37
# http://127.0.0.1:9000/monitoring 或 /grafana/
```

不要把 Panel / Grafana 直接暴露到 `0.0.0.0`。不要单独隧道 3000。
