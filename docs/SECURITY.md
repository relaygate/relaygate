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
5. 确认管理端口仅本机监听：`8080 / 9901 / 9090 / 3000 / 9100`
6. 游戏后端防火墙仅允许网关回源 IP；双活时放行 **全部** `GATEWAY_PUBLIC_IP`（见 [`HA.md`](HA.md)）
7. 生产密钥走密钥管理 / CI Secrets，不要提交 `.env` / `terraform.tfvars`
8. 容器镜像使用不可变 tag（git SHA），禁止生产 `latest`

## 防火墙

- 使用 `sudo bash scripts/apply_firewall.sh`（规则在 `deploy/firewall/`）
- 应用前保留现有 SSH 会话 + 云控制台
- SSH `30455` 必须放行

## 日志

- Compose 已限制 json-file 大小
- 可选安装 `deploy/logrotate/envoy-gateway` 到 `/etc/logrotate.d/`

## 访问 Panel / Grafana

```bash
ssh -p 30455 \
  -L 8080:127.0.0.1:8080 \
  -L 3000:127.0.0.1:3000 \
  root@107.149.191.37
```

不要把 Panel / Grafana 直接暴露到 `0.0.0.0`。
