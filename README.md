# RelayGate

[MIT License](LICENSE)

基于 **Envoy** 的游戏 **L4 TCP/UDP 网关**。一份 `resources.yaml`（Rules-first）描述上游与转发规则；日常运维用二进制 `relaygate` 与 Panel（`:9000`）。

```text
玩家 → 云 L4 LB（可选）→ 网关入口端口 → Envoy → 上游 server:7777/7778
```

| 概念 | 说明 |
|------|------|
| 上游（`servers`） | 游戏服地址与 TCP/UDP 端口 |
| 转发规则（`rules`） | 入口端口 → 某上游某协议；命名 `forward-{server}-{entry}-{proto}` |
| 入口类型 | `validation`（验证）/ `production`（正式），可并行；回退=关正式规则 |
| 运行态 | 安装默认 `/opt/relaygate/data`（可用 `RELAYGATE_DATA_DIR` 覆盖） |

---

## 安装

支持 Ubuntu / Debian / RHEL / Rocky / Alma / CentOS Stream / Fedora / Amazon Linux（`amd64`/`arm64`，systemd）。

`install.sh` 只做 bootstrap：下载 release tar → 解压到 `/opt/relaygate` → 调用 CLI。Compose、Panel、防火墙由 `relaygate` 完成。

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh -o /tmp/relaygate-install.sh
sudo RELAYGATE_VERSION=v0.1.0 bash /tmp/relaygate-install.sh
```

生产请固定 **Release tag**（不要用 `master` / `latest`）。本地包：

```bash
sudo RELAYGATE_TAR=/path/relaygate-v0.1.0-linux-amd64.tar.gz bash /tmp/relaygate-install.sh
```

| 变量 | 说明 |
|------|------|
| `RELAYGATE_VERSION` | GitHub Release tag（推荐） |
| `RELAYGATE_TAR` | 本地 tar.gz，跳过下载 |
| `RELAYGATE_INSTALL_DIR` | 默认 `/opt/relaygate` |
| `RELAYGATE_DATA_DIR` | 运行态；默认 `$INSTALL_DIR/data` |
| `GATEWAY_NAME` / `GATEWAY_PUBLIC_IP` | 网关身份 |
| `GATEWAY_SSH_PORT` | 默认 `30455` |
| `ENABLE_PANEL` / `ENABLE_GRAFANA` | 默认 1 |
| `APPLY_FIREWALL` | 默认 0（安装时只校验防火墙） |

首启检查：

```bash
relaygate setup --noninteractive   # 若安装器未跑完
relaygate doctor
relaygate smoke
```

---

## 配置正式业务

真相源：`$RELAYGATE_DATA_DIR/resources.yaml`（模板见仓库 `resources.example.yaml`）。可用 Panel 或直接改 YAML。

### 推荐流程

1. 添加**上游**（地址、TCP/UDP 端口）
2. 添加**转发规则**：先开 `validation` 验证入口，测通后再开 `production` 正式入口
3. **应用配置**（Envoy）→ `smoke` / 验证
4. 需要 ACL / 主机侧放行时再 **应用防火墙**（nft）

### Apply 分流（重要）

改完意图后按变更类型选命令，不要混用：

| 改了什么 | 怎么应用 |
|----------|----------|
| 上游 / 转发规则 / Envoy 限速等 | Panel「应用配置」或 `relaygate reload`（首次全量用 `apply`） |
| ACL / 仅 nftables | Panel「应用防火墙」或 `relaygate firewall apply` |
| 二进制 / packaging | `relaygate upgrade [--drain]` 或 `install.sh --upgrade` |

### 入口类型

| `entry` | 中文 | 典型用途 |
|---------|------|----------|
| `validation` | 验证 | 旁路验收；默认可先开 |
| `production` | 正式 | 对玩家正式放量；与验证可同时开 |

命名示例：`forward-server-01-validation-tcp`（listen `11001`）、`forward-server-01-production-tcp`（listen `10001`）。游戏服默认回源 TCP `7777` / UDP `7778`；上游防火墙只放行网关回源 IP。

### ACL 速记

```yaml
acl:
  deny: ["1.2.3.4/32"]   # 黑名单
  allow: []              # 非空 = 严格白名单模式
```

SSH 不受 ACL 约束。改 ACL 后执行 `firewall apply`。

---

## 日常命令

```bash
# 诊断 / 检查
relaygate doctor
relaygate smoke
relaygate canary [HOST]          # 对指定主机做验证探测

# 配置落地
relaygate validate
relaygate apply                  # 首次 / 全量
relaygate reload                 # resources→Envoy：backup→drain→restart→ready
relaygate firewall check|apply
relaygate rollback [STAMP]

# 摘流（配合云 LB）
relaygate drain fail|ok|status

# 升级 / 多机
relaygate upgrade [--drain]
GATEWAYS=gateway-01,gateway-02 RELAYGATE_VERSION=v0.1.0 relaygate fleet
```

| 场景 | 命令 |
|------|------|
| 上游启停 | `relaygate server enable\|disable <server>` |
| ACL | `relaygate acl list\|add\|remove …` |
| 游戏档位 profile | `relaygate profile list\|show\|apply` |
| 变更摘要 | `relaygate changes [--limit N]` |
| Panel 安装 | `sudo relaygate panel install\|uninstall` |

---

## Panel（:9000）

默认 `PANEL_BIND=127.0.0.1:9000`（loopback）。经 SSH 隧道访问：

```bash
ssh -p 30455 -L 9000:127.0.0.1:9000 root@<GATEWAY_PUBLIC_IP>
# 浏览器打开 http://127.0.0.1:9000
```

| 页 | 做什么 |
|----|--------|
| 总览 | 限速指标、转发规则限速 Top |
| 上游 / 转发规则 | CRUD；入口类型、listen、开关；「启用正式入口」= 开正式转发 |
| ACL 集合 | 黑白名单（改后需应用防火墙） |
| 配置 | 整份 `resources.yaml` 预览 / 编辑 / 导出 |
| 应用 | 变更摘要；**应用配置** 与 **应用防火墙** 分按钮 |
| 运维 | 诊断、摘流、冒烟/验证、防火墙检查、运维档位 |
| 变更 | 备份历史与回滚 |
| 监控 | 侧栏新窗口打开 Grafana（同源反代 `/grafana/`）；日志在 Explore 查 |

`PANEL_ROLE=standby` 时拒写。写操作审计：`DataDir/panel-audit.log`。

---

## 监控与日志

链路：**Envoy TCP access** → Fluent Bit → Loki → Grafana（经 Panel `/grafana/` 反代）。

| 节点 | `.env` 中 `COMPOSE_PROFILES` |
|------|------------------------------|
| 主管理 | `with-grafana,with-loki,with-logs` |
| 从节点 | `with-logs`（并设 `LOKI_HOST=<中心私网>`） |

启用步骤、LogQL、与 Prometheus 对齐的排查见 **[docs/logging-playbook.md](docs/logging-playbook.md)**。看板：**TCP Session Logs**。

本地日志文件：`${RELAYGATE_DATA_DIR}/envoy/logs/tcp-access.json`。

---

## 双活维护与升级

```text
玩家 → 云 L4 LB
         ├─ gateway-01（ENABLE_PANEL=1, PANEL_ROLE=primary）
         └─ gateway-02（ENABLE_PANEL=0, PANEL_ROLE=standby）
```

```bash
relaygate doctor
relaygate drain fail             # 等 DRAIN_WAIT（默认 30s，对齐 NLB 3×10s HC）
# 控制台确认 target unhealthy 后：
relaygate reload                 # 或 upgrade --drain
relaygate drain ok               # 若未自动恢复
relaygate smoke
```

`DRAIN_WAIT` 写在 `.env`（见 `.env.example`）。NLB 模板：[`packaging/terraform/nlb/`](packaging/terraform/nlb/)。

```bash
# 本机升级 / 回滚 / 卸载
sudo RELAYGATE_VERSION=v0.1.0 relaygate upgrade --drain
relaygate rollback
sudo PURGE=1 bash install.sh --uninstall
```

---

## 排障速查

| 现象 | 先查 |
|------|------|
| `/ready` 非 LIVE / LB 不健康 | `relaygate doctor`；是否误摘流 `drain status` |
| 转发不通 | 规则是否 `enabled`；入口类型是否开错；上游 TCP/UDP 端口；`smoke` / `canary` |
| 改了 YAML 不生效 | 是否走了对应 Apply（`reload` vs `firewall apply`） |
| Panel 打不开 | SSH 隧道、`PANEL_BIND`、systemd `relaygate-panel` |
| 无会话日志 | `COMPOSE_PROFILES`、Fluent Bit/Loki 就绪；见 [logging-playbook](docs/logging-playbook.md) |
| 限速过猛 / ACL 误伤 | `defaults` 限速字段、`acl.deny`/`allow`；SSH 不受 ACL 影响 |

关键路径：

| 路径 | 用途 |
|------|------|
| `/opt/relaygate` | 安装前缀 |
| `$RELAYGATE_DATA_DIR/resources.yaml` | 业务意图 |
| `$RELAYGATE_DATA_DIR/envoy/` | Envoy 生成配置与日志 |
| `$RELAYGATE_DATA_DIR/firewall/` | nft 生成物 |
| `$RELAYGATE_DATA_DIR/backups/` | 变更备份（回滚） |
| `/etc/relaygate/secrets/` | Panel / Grafana 密码文件 |

---

## 术语（运维对照）

| 中文 | 标识 | 一句话 |
|------|------|--------|
| 上游 | `server-*` | 游戏服 |
| 转发规则 | `forward-*`（YAML `rules[]`） | 入口 → 上游 |
| 验证 / 正式 | `validation` / `production` | 入口类型，不是「金丝雀阶段」 |
| 入口 Listener | `ingress-*` | 由转发规则生成 |
| 限速指标 | `rl_<forward名>` | Envoy 本地限速 stat |
| ACL 集合 | `ACL_DENY` / `ACL_ALLOW` | nft 黑白名单 |

---

## 已知边界

- 固定目标转发，不做跨服负载均衡
- UDP 无可靠主动健康检查
- 上游看到的是网关源 IP
- Envoy 非抗 DDoS；大流量需云高防

---

## License

[MIT](LICENSE) © 2026 RelayGate
