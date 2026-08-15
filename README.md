# RelayGate

基于 **Envoy** 的通用 **L4 TCP/UDP 网关**：主控管意图与发布，节点拉配置并本机转发。日常用 `relaygate` CLI 与 Panel（`:9000`）。

```text
客户端（下游）→ 云 L4 LB（可选）→ 网关 Listener → Envoy → 上游
```

## 安装

支持 Ubuntu / Debian / RHEL / Rocky / Alma / CentOS Stream / Fedora / Amazon Linux（`amd64` / `arm64`，systemd）。

`install.sh`：下载 Release tar → 解压到 `/opt/relaygate` → 调用 CLI。角色只有 **主控** / **节点**（不是 agent）。

| 角色 | 安装 | 做什么 |
|------|------|--------|
| **主控** | `install.sh control` | Panel、意图/发布、节点名册；可选本机转发与中心观测 |
| **节点** | `install.sh node …` | Envoy + **agent**（拉取/心跳）；需 `CONTROL_URL` 与接入令牌 |

**节点 vs agent：** 「节点」是安装角色；**agent** 是节点上的守护进程（`relaygate agent` / `relaygate-agent`）。

```bash
# 主控（首启默认密码 relaygate，生产务必改密）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- control

# 节点：优先用主控 `fleet join` / Panel「接入」生成的一行
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-01 --token '<token>'

# 升级（读现有 .env，保留角色）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
# 或: sudo /opt/relaygate/bin/relaygate upgrade [--drain]
```

固定版本：`RELAYGATE_VERSION=vX.Y.Z`；本地包：`RELAYGATE_TAR=/path/to.tar.gz`。环境模板：[`packaging/control/env.example`](packaging/control/env.example)、[`packaging/node/env.example`](packaging/node/env.example)。

默认会拉哪些容器：**始终** Envoy。节点侧安全（限连/防火墙/ACL）走本机 CLI/配置，不依赖观测容器。机群在线/版本状态由节点上的 **agent 心跳上报主控**（Panel 汇总）；时序指标由节点本机 Prometheus **remote_write** 到主控。

| 角色 | 默认 | 说明 |
|------|------|------|
| **节点** | 精简但上报指标 | Compose：`with-metrics`（Envoy + Prometheus + node-exporter）+ systemd agent；**无** Grafana / Loki / Fluent Bit；安装按角色，无需手传 `with-*` |
| **主控** | 监控+日志全开 | `with-metrics` + Grafana / Loki / Fluent Bit；**无需 / 不推荐** `MINIMAL=1` |

首次安装慢点通常在装 Docker 与拉镜像（主控更重；节点默认拉 Envoy + Prometheus）。节点若要边缘 TCP 日志：安装后可在 `.env` 把 `COMPOSE_PROFILES` 设为 `with-metrics,with-logs`（可选）。

```bash
# 节点（默认 with-metrics → 指标上报主控；无需 MINIMAL / 手传 with-*）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-02 --token '<token>'

# 主控（默认全开观测；勿依赖 MINIMAL 精简）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- control
```

首启：`relaygate diag` · `relaygate smoke`。Panel：`http://<GATEWAY_PUBLIC_IP>:9000`（可改 `PANEL_BIND`）。

### 常用环境变量（按组）

| 分组 | 变量 |
|------|------|
| 安装 / 路径 / 版本 | `RELAYGATE_VERSION` · `RELAYGATE_TAR` · `RELAYGATE_INSTALL_DIR` · `RELAYGATE_DATA_DIR` · `RELAYGATE_SECRETS_DIR` |
| 本机节点身份 | `GATEWAY_NAME` · `GATEWAY_PUBLIC_IP` · `GATEWAY_SSH_PORT` |
| Panel / 观测 | `PANEL_ENABLED` · `PANEL_BIND` · `PANEL_ROLE` · `GRAFANA_ENABLED` · `COMPOSE_PROFILES`（`MINIMAL` 仅兼容，主控不推荐） |
| 机群连接（节点） | `CONTROL_URL` · `AGENT_TOKEN` / `AGENT_TOKEN_FILE` · `PROMETHEUS_REMOTE_WRITE_URL` |
| 安全落地（分层） | `APPLY_FIREWALL`（安装/CLI 一次性）· `SECURITY_AUTO_APPLY`（节点拉取后自动应用主机侧） |

## 常用操作

真相源：`$RELAYGATE_DATA_DIR/resources.yaml`（默认 `/opt/relaygate/data`；模板 [`packaging/shared/resources.example.yaml`](packaging/shared/resources.example.yaml)）。

1. 配置**上游**与**转发**（`validation` 验证入口 → `production` 正式入口）
2. **本机应用**：Panel「配置应用」或 `relaygate reload`（首次全量用 `apply`）
3. **机群发布**（主控）：`relaygate fleet publish`（确认词：`确认` / `Confirm`；Panel 配置应用页不再提供发布）；节点 agent 自行拉取并应用。某台需立即对齐：机群页该节点「同步」或 `relaygate fleet sync <name>`（仅该节点）
4. **安全**：名单与防护在 `security.access` / `security.protections`（Panel「安全策略」）；改防火墙后 `relaygate firewall apply`；内核相关用 `relaygate security apply-kernel`。四域与落地顺序见 [security-domains](docs/security-domains.md)

| 改了什么 | 怎么落地 |
|----------|----------|
| 上游 / 转发 / 网关侧策略 | `reload`（本机）或 `fleet publish`（机群）；单节点立即对齐用 `fleet sync <name>` |
| 防火墙名单 / 主机防火墙策略 | `firewall apply` |
| 二进制 / packaging | `upgrade` / `install.sh upgrade` |

```bash
relaygate diag
relaygate validate
relaygate apply                  # 首次 / 全量
relaygate reload                 # 优先热更新
relaygate reload --hard          # 硬重启（会断连）
relaygate firewall check|apply
relaygate security list|verify
relaygate fleet status|publish|sync|join|leave
relaygate drain fail|ok|status
relaygate upgrade [--drain]
relaygate smoke [HOST]
```

完整子命令：`relaygate help`。维护时可先 `drain fail`，再 `reload` / `upgrade --drain`，最后 `drain ok`。

会话日志与 Grafana（主控）：见 [logging-playbook](docs/logging-playbook.md)。

## 已知边界

- 固定单目标转发（**不做**多上游成员 LB）
- UDP **无**可靠主动健康检查
- 上游看到的是网关源 IP
- **非**抗 DDoS；大流量需云高防等外部能力。正确分层：外部抗量/粗筛 → 本机限连与 ACL / 可选 tc 整形 → 上游鉴权；**不要**指望只升回源带宽
- **不做** TCP 魔数 / 首包指纹 /「等下游数据再 dial」；自研协议粗筛放高防或前置专用组件（如 HAProxy inspect），本产品保持 Envoy L4 转发
- 默认 `PROXY_PROTOCOL=off`（有云 LB 发 PROXY 时再开）
- 网卡 `nic_*` 为**主机减负**（示例档位常对齐约 3mbit），**不**替代高防，也**不**单独充当「攻击下回源 &lt; N Mbps」验收工具；回源侧应配合 `security.access` 只放行高防/内网 CIDR

深度运维 / 设计：[机群运维](docs/fleet-ops.md) · [能力边界](docs/envoy-capability-roadmap.md) · [中心观测](packaging/observability/README.md)

## 贡献与安全

- 参与开发：[CONTRIBUTING.md](CONTRIBUTING.md)（含 DCO）
- 变更记录：[CHANGELOG.md](CHANGELOG.md)
- 安全披露：[SECURITY.md](SECURITY.md)
- 行为准则：[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

## License

[MIT](LICENSE) © 2026 RelayGate
