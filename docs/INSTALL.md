# RelayGate 一键安装

安装器面向独立 Linux 网关主机，自动完成环境检查、依赖与 Docker
Compose v2 安装、源码获取、镜像及二进制构建、配置渲染、sysctl、数据面容器启动、
Panel systemd 服务（默认）和 readiness 检查。它不会连接其他服务器，也不会默认改写主机防火墙。

## 支持矩阵

- Ubuntu、Debian
- RHEL、Rocky Linux、AlmaLinux、CentOS Stream
- systemd
- `amd64`（x86_64）、`arm64`（aarch64）
- 最低约 1 GiB 内存、4 GiB 可用磁盘
- Docker Compose v2.20 或更高版本

其他发行版、非 systemd 环境和 32 位架构会明确退出。安装器必须以 root
运行；`--dry-run` 允许非 root 检查。

## 部署形态（默认）

| 组件 | 运行方式 |
|------|----------|
| Panel | 宿主 Linux 二进制 + `relaygate-panel.service`（`127.0.0.1:9000`） |
| Envoy / Prometheus / Grafana / node_exporter | Docker Compose（`deploy/compose.yaml`） |

Panel **不是** Compose 服务，不挂载 `docker.sock`，用户 `relaygate` 也不在 `docker` 组。
Apply 经 root-owned helper + sudoers 白名单调用 `scripts/reload_envoy.sh`。

## 一条命令安装

先审阅脚本，再在目标主机运行：

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh -o /tmp/relaygate-install.sh
less /tmp/relaygate-install.sh
sudo bash /tmp/relaygate-install.sh
```

如果接受直接管道执行，也可以：

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh | sudo bash
```

脚本从 `/dev/tty` 读取交互输入，不会与 `curl | bash` 的标准输入冲突。默认安装
到 `/opt/relaygate`。未指定 `RELAYGATE_VERSION` 时读取仓库根目录的
`RELEASE`；当前值为 `master`。生产环境建议在发布 tag 可用后显式固定 tag
或提交 SHA，避免分支移动：

```bash
sudo RELAYGATE_VERSION=<tag-or-commit> bash /tmp/relaygate-install.sh
```

## 安装时询问

交互模式会询问：

1. gateway 名称；
2. 公网 IPv4（尝试自动探测，仍会显示供确认）；
3. 是否启用 Panel（systemd 二进制）；
4. 是否启用 Grafana（Compose `with-grafana`）；
5. 是否应用 nftables（默认否；选择是后仍需输入完整确认短语）。

SSH 端口优先通过 `sshd -T` 检测，无法检测时安全回退为 `22`。也可以显式
设置 `GATEWAY_SSH_PORT`。管理服务始终绑定到 `127.0.0.1`。

主要环境变量：

```text
RELAYGATE_INSTALL_DIR    默认 /opt/relaygate
RELAYGATE_VERSION        tag、branch 或 commit；默认读取 RELEASE
RELAYGATE_REPO_URL       默认 https://github.com/relaygate/relaygate.git
RELAYGATE_SECRETS_DIR    默认 /etc/relaygate/secrets
GATEWAY_NAME             默认 gateway-01
GATEWAY_PUBLIC_IP        公网 IPv4；非交互模式必填
GATEWAY_SSH_PORT         自动检测，无法检测时为 22
ENABLE_PANEL             1/0，默认 1（systemd，非 Compose）
ENABLE_GRAFANA           1/0，默认 1
NONINTERACTIVE           1/0
APPLY_FIREWALL           1/0，默认 0
```

## 非交互安装

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh |
  sudo env \
    NONINTERACTIVE=1 \
    RELAYGATE_VERSION=master \
    GATEWAY_NAME=gateway-01 \
    GATEWAY_PUBLIC_IP=107.149.191.37 \
    GATEWAY_SSH_PORT=30455 \
    ENABLE_PANEL=1 \
    ENABLE_GRAFANA=1 \
    APPLY_FIREWALL=0 \
    bash
```

`NONINTERACTIVE=1` 时公网 IP 必须显式提供。Panel/Grafana 默认启用但仅监听
本机。无论交互与否，`APPLY_FIREWALL` 默认都为 `0`。

## Panel 与前端布局

| 路径 | 说明 |
|------|------|
| `internal/panel` | Go Panel 服务（会话鉴权、API、Grafana 固定目标反代） |
| `web/` | 模板与静态资源（`favicon.svg` 等） |
| `deploy/compose.yaml` | 数据面；可选 `with-grafana`（无 `with-panel`） |
| `deploy/systemd/` | `relaygate-panel.service`、apply helper、sudoers |
| `cmd/relaygate panel` | 进程入口（`/opt/relaygate/bin/relaygate panel`） |

Grafana 通过 Panel 的 `/grafana/` 反代与 `/monitoring` 内嵌访问；`GRAFANA_URL`
须为 loopback `http(s)`。Compose 默认 `GF_SERVER_ROOT_URL=/grafana/` 与
`serve_from_sub_path=true`，适配 SSH 隧道到 `localhost:9000`。

手动安装/更新 Panel 服务（幂等）：

```bash
sudo bash /opt/relaygate/scripts/install_panel_service.sh
sudo bash /opt/relaygate/scripts/uninstall_panel_service.sh
```

## 密钥

安装器为 Panel 和 Grafana 分别生成随机强密码，保存在：

```text
/etc/relaygate/secrets/panel_admin_password   # root:relaygate 0640
/etc/relaygate/secrets/grafana_admin_password # root:root 0600
```

目录权限为 `0750`（`root:relaygate`），普通用户不可列目录；Grafana 文件仅
root/容器可读。systemd 设置 `PANEL_ADMIN_PASSWORD_FILE`。Compose 以只读文件
挂载密钥目录供 Grafana 使用。为兼容旧部署，原有 `PANEL_ADMIN_PASSWORD` 和
`GRAFANA_ADMIN_PASSWORD` 环境变量仍可使用，但新安装不会把密码写入 `.env`。

这是一种单机 bootstrap 文件密钥方案，不是集中式密钥管理器。更高安全等级
环境应把同一路径挂载替换为 Vault、云密钥服务或受控配置管理投递，并定期轮换。
不得将密钥或 `.env` 提交到 Git。

## 防火墙安全模型

仓库的 nftables 模板包含 `flush ruleset`，错误应用可能导致 SSH 断开。因此：

- 安装器默认只渲染并运行 `nft -c` 语法检查；
- 规则会用检测/指定的 SSH 端口替换模板值；
- 应用前备份完整 ruleset 到 `/root/nft-backup-*.nft`；
- 同时生成 root-only 恢复脚本；
- 交互应用必须输入 `YES_FLUSH_NFTABLES`；
- 非交互应用除了 `APPLY_FIREWALL=1`，还必须设置
  `FIREWALL_CONFIRM=YES_FLUSH_NFTABLES`。

强烈建议保持当前 SSH 会话、准备云控制台，并在应用后立即从新终端验证：

```bash
ssh -p 30455 root@107.149.191.37
```

显式应用示例：

```bash
sudo APPLY_FIREWALL=1 GATEWAY_SSH_PORT=30455 \
  bash /opt/relaygate/scripts/apply_firewall.sh
```

## 验证与访问

安装结束会检查：

- `docker compose config`；
- Envoy 配置与 `/ready`；
- nftables 语法；
- `docker compose ps`；
- Panel `/login`（启用时，systemd）；
- Grafana `/api/health`（启用时）；
- Prometheus/Envoy 冒烟检查。

手动复查：

```bash
cd /opt/relaygate
sudo docker compose -f deploy/compose.yaml --env-file .env ps
sudo systemctl status relaygate-panel
sudo journalctl -u relaygate-panel -n 100 --no-pager
sudo bash scripts/smoke_test.sh
curl -fsS http://127.0.0.1:9901/ready
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9000/login
```

管理端不直接暴露公网，使用 SSH 隧道（仅 Panel 9000；Grafana 经 Panel 反代）：

```bash
ssh -p 30455 \
  -L 9000:127.0.0.1:9000 \
  root@107.149.191.37
```

浏览器打开 `http://127.0.0.1:9000` → Overview / Monitoring。Grafana 在
`/monitoring` iframe 或 `/grafana/`，需先登录 Panel。匿名 Viewer 仅因 Grafana
绑 loopback 而不能绕过 Panel；编辑仪表盘请在 `/grafana/login` 用 admin 登录。

如果 readiness 失败，Compose 与 Panel journal 摘要保存到
`/var/log/relaygate/install-failure.log`。

## Panel 运维命令

```bash
sudo systemctl status relaygate-panel
sudo journalctl -u relaygate-panel -f
sudo systemctl restart relaygate-panel
```

## Dry run

`--dry-run` 完成 OS、架构、systemd、容量和输入检查，打印将执行的系统操作，
但不安装软件、不复制源码、不写配置、不应用 sysctl、不启动容器/Panel，也不应用
防火墙：

```bash
sudo GATEWAY_PUBLIC_IP=107.149.191.37 \
  NONINTERACTIVE=1 bash install.sh --dry-run
```

## 升级、回滚与幂等

重复普通安装不会覆盖现有实例，而会要求使用 `--upgrade`：

```bash
sudo RELAYGATE_VERSION=<new-tag-or-commit> \
  bash /opt/relaygate/install.sh --upgrade
```

升级保留 `.env`、`config/resources.yaml` 和 `/etc/relaygate/secrets`，更新二进制、
systemd unit/helper/sudoers 并 restart Panel；关键配置备份到
`/opt/relaygate/backups/installer-<timestamp>`。升级失败后可用：

```bash
cd /opt/relaygate
sudo bash scripts/rollback.sh
# Panel 仍异常时检查:
sudo journalctl -u relaygate-panel -n 200 --no-pager
# 必要时用旧版本再跑 --upgrade，或恢复 backups/installer-* 后重装服务
```

如果需要回到指定源码版本，重新以旧 tag/commit 执行 `--upgrade`，然后运行
上述回滚命令恢复配置。Docker 命名卷不会在升级中删除。

## 卸载

默认卸载停止 Panel systemd 与 Compose 容器，移除 unit/helper/sudoers 与本安装
对应的 sysctl 文件，保留配置、密钥、镜像、数据卷与用户 `relaygate`：

```bash
sudo bash /opt/relaygate/install.sh --uninstall
```

如果曾显式应用 nftables，卸载不会擅自覆盖后来产生的防火墙变更。安装器会
显示安装前 ruleset 的 root-only 恢复脚本路径；请审阅后手动执行。

永久删除必须显式增加 `--purge` 并再次输入确认短语：

```bash
sudo bash /opt/relaygate/install.sh --uninstall --purge
```

非交互永久删除还必须设置：

```bash
sudo NONINTERACTIVE=1 PURGE_CONFIRM=DELETE_RELAYGATE_DATA \
  bash /opt/relaygate/install.sh --uninstall --purge
```

`--purge` 会删除 Compose 数据卷、安装目录、本机密钥与 `relaygate` 用户，无法撤销。

## 已知限制

- 当前没有正式 release tag，`RELEASE` 暂时指向 `master`；生产使用者应显式
  固定审核过的提交 SHA，待正式发布后改为固定 tag。
- 安装器通过 Docker multi-stage 构建获得 Linux `relaygate`，首次构建需要
  能访问 Docker Hub 和 Go module 源（`Dockerfile.panel` 仍作 builder）。
- 主机云安全组不由脚本修改；游戏入口和 SSH 端口仍需在云平台正确配置。
- 文件密钥适合单机 bootstrap；集中式生产环境应接入外部密钥管理器。
