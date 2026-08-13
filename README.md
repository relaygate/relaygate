# RelayGate

基于 **Envoy** 的通用 **L4 TCP/UDP 网关**。用一份 Rules-first 的 `resources.yaml` 描述上游与转发；日常运维靠 `relaygate` CLI 与 Panel（`:9000`）。

```text
客户端（下游）→ 云 L4 LB（可选）→ 网关入口 / Listener → Envoy → 上游服务
```

| 概念 | 说明 |
|------|------|
| **上游** `upstreams` | 回源目标（地址 + TCP/UDP 端口）；固定单目标，非多成员 LB |
| **转发** `forwards` | 入口（类型 + 监听端口 + 协议）→ 某一上游；命名 `forward-{upstream}-{entry}-{proto}` |
| **入口类型** `entry` | `validation`（验证）/ `production`（正式），可并行；回退 = 关正式转发 |
| **运行态** | 默认 `/opt/relaygate/data`（可用 `RELAYGATE_DATA_DIR` 覆盖） |

**已知边界：** 固定目标转发（不做多上游成员 LB）· UDP 无可靠主动健康检查 · 上游看到的是网关源 IP · 网关非抗 DDoS（大流量需云高防）· 默认 `PROXY_PROTOCOL=off`（公网直连；有云 LB 发 PROXY 时再开，见 [logging-playbook](docs/logging-playbook.md)）· 安全四域与落地顺序见 [security-domains](docs/security-domains.md)

**延伸阅读：** [能力边界](docs/envoy-capability-roadmap.md) · [安全领域](docs/security-domains.md) · [机群运维](docs/fleet-ops.md) · [机群架构](docs/fleet-scale-control-plane.md) · [热更新](docs/hot-update-xds.md) · [中心观测](packaging/observability/README.md)

---

## 安装

支持 Ubuntu / Debian / RHEL / Rocky / Alma / CentOS Stream / Fedora / Amazon Linux（`amd64` / `arm64`，systemd）。

`install.sh` 只做 bootstrap：下载 release tar → 解压到 `/opt/relaygate` → 调用 CLI。Compose、Panel、防火墙由 `relaygate` 完成。

RelayGate 是**单一产品**，按角色安装两个组件之一（详见 [产品表面 · 双组件](docs/product-surface-agent.md#0-对内双组件单一产品--非双产品线)）：

| 角色 | 环境模板 | 启用什么 |
|------|----------|----------|
| **主控** | [`packaging/control/env.example`](packaging/control/env.example) | Panel（意图/发布/节点名册）+ 可选本机转发与中心观测 |
| **节点** | [`packaging/node/env.example`](packaging/node/env.example) | Envoy + agent（`CONTROL_URL` / `AGENT_TOKEN_FILE`）；`ENABLE_PANEL=0` |

**节点 vs agent：** 「节点」是机群角色（`install.sh node`）；**agent** 是节点上的拉取/心跳进程（`relaygate agent` / systemd `relaygate-agent`）。用户文案说节点；命令与服务名保留 agent。

### 一键安装 / 升级

```bash
# 1) 安装主控（首启默认密码 relaygate，生产务必改密）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- control

# 2) 安装节点：优先用主控 `fleet join` / Panel「接入」生成的一行
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- node --control http://203.0.113.10:9000 \
      --name gateway-02 --token '<token>'

# 3) 升级主控 / 节点（同一命令；读现有 .env，保留角色与 DataDir）
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo bash -s -- upgrade
# 已安装本机也可: sudo /opt/relaygate/bin/relaygate upgrade [--drain]
```

固定版本或本地包：

```bash
curl -fsSL https://raw.githubusercontent.com/relaygate/relaygate/master/install.sh \
  | sudo RELAYGATE_VERSION=v0.1.0 bash -s -- control

sudo RELAYGATE_TAR=/path/relaygate-v0.1.0-linux-amd64.tar.gz bash install.sh control
```

### 安装变量

| 变量 | 默认 / 说明 |
|------|-------------|
| `RELAYGATE_VERSION` | 默认最新 Release；可设具体 tag（勿用 `master` / `main`） |
| `RELAYGATE_TAR` | 本地 tar.gz，跳过下载 |
| `RELAYGATE_INSTALL_DIR` | `/opt/relaygate` |
| `RELAYGATE_DATA_DIR` | `$INSTALL_DIR/data` |
| `GATEWAY_NAME` / `GATEWAY_PUBLIC_IP` | 网关身份 |
| `GATEWAY_SSH_PORT` | 安装/配置时指定（常见 `22`；示例见 [`packaging/shared/env.example`](packaging/shared/env.example)） |
| `ENABLE_PANEL` | 主控 `1`；节点 `0`（由 `control` / `node` 子命令默认） |
| `ENABLE_GRAFANA` | 主控常用 `1`；节点勿启中心栈 |
| `PANEL_BIND` | `0.0.0.0:9000`（对外；可改 `127.0.0.1:9000`） |
| `APPLY_FIREWALL` | `0`（安装时只校验防火墙） |
| `CONTROL_URL` / `AGENT_TOKEN_FILE` | 节点组件必填（见 `node.env.example`；`node` 子命令写入） |

### 首启检查

```bash
relaygate setup --noninteractive   # 若安装器未跑完
relaygate diag
relaygate smoke
```

### Panel UI

`ui/dist` 是构建产物（gitignore），不进仓库。Panel 读安装前缀下的 `ui/dist`（默认 `/opt/relaygate/ui/dist`）。

| 场景 | 怎么得到 UI |
|------|-------------|
| **生产 / 一键安装** | Release tar 在 `make dist` 时已打包 `ui/dist` |
| **源码开发** | `make ui`，再 `make panel` / `relaygate panel` |
| **`FROM_SOURCE=1`** | 只构建 Go 二进制；需事先生成 `ui/dist`，否则安装校验失败 |

---

## 配置正式业务

真相源：`$RELAYGATE_DATA_DIR/resources.yaml`（模板见 [`packaging/shared/resources.example.yaml`](packaging/shared/resources.example.yaml)）。可用 Panel 或直接改 YAML。

1. 添加**上游**（地址、TCP/UDP 端口）
2. 添加**转发**：先开 `validation` 验证入口，测通后再开 `production` 正式入口
3. **应用配置**（Envoy）→ `smoke` / 验证
4. 需要 ACL / 主机侧放行时再 **应用防火墙**

| 改了什么 | 怎么应用 |
|----------|----------|
| 上游 / 转发 / 网关限速等 | Panel「应用配置」或 `relaygate reload`（首次全量用 `apply`） |
| ACL / 仅防火墙 | Panel「应用防火墙」或 `relaygate firewall apply` |
| 二进制 / packaging | `relaygate upgrade [--drain]` 或 `install.sh upgrade` |

| `entry` | 用途 |
|---------|------|
| `validation` | 验证；旁路验收 |
| `production` | 正式放量；可与验证并行 |

命名示例：`forward-server-01-validation-tcp`（listen `11001`）、`forward-server-01-production-tcp`（listen `10001`）。

```yaml
acl:
  deny: ["1.2.3.4/32"]   # 黑名单
  allow: []              # 非空 = 严格白名单模式
```

SSH 不受 ACL 约束。改 ACL 后执行 `firewall apply`（无需 `reload` Envoy）。

**网关防护（可选）：** 领域与落地顺序见 [security-domains](docs/security-domains.md)；攻击×策略见 [`packaging/security/`](packaging/security/)（[threat-analysis.md](packaging/security/threat-analysis.md)；`profile apply tcp-longlived`；防火墙仍走上方「应用防火墙」）。**Out of scope：** 外部 RLS 全局限速。

---

## 日常命令

```bash
relaygate diag
relaygate smoke [HOST]
relaygate canary [HOST]
relaygate baseline

relaygate validate
relaygate apply                  # 首次 / 全量
relaygate reload                 # 优先热更新
relaygate reload --hard          # 硬重启
relaygate firewall check|apply
relaygate rollback [STAMP]

relaygate drain fail|ok|status
relaygate upgrade [--drain]

relaygate server enable|disable <server>
relaygate server enable --all-production
relaygate acl list|add|remove deny|allow CIDR
relaygate profile list|show|apply NAME
relaygate changes [--limit N]
relaygate panel install|uninstall
relaygate version
```

完整子命令：`relaygate help`。

---

## Panel（:9000）

默认 `PANEL_BIND=0.0.0.0:9000`。浏览器：`http://<GATEWAY_PUBLIC_IP>:9000`  
仅本机：`PANEL_BIND=127.0.0.1:9000`，可用 SSH 隧道。

| 页 | 做什么 |
|----|--------|
| 总览 | 限速指标、转发限速 Top |
| 上游 / 转发 | CRUD；入口类型、listen、开关 |
| ACL 集合 | 黑白名单（改后需应用防火墙） |
| 配置 | 整份 `resources.yaml` 预览 / 编辑 / 导出 |
| 应用 | 变更摘要；**应用配置** 与 **应用防火墙**；**发布到机群** |
| 机群 | 节点名册、接入、退役、发布概况 |
| 运维 | 诊断、摘流、冒烟/验证、防火墙检查、运维档位 |
| 变更 | 备份历史与回滚 |
| 监控 | 侧栏打开 Grafana（同源反代 `/grafana/`） |

- `PANEL_ROLE=standby` 时拒写
- 写操作审计：`$RELAYGATE_DATA_DIR/panel-audit.log`

---

## 监控与日志

链路：**Envoy TCP access** → Fluent Bit → Loki → Grafana（经 Panel `/grafana/` 反代）。

| 角色 | `.env` 中 `COMPOSE_PROFILES` |
|------|------------------------------|
| 主控 | `with-grafana,with-loki,with-logs` |
| 节点 | `with-logs`（并设 `LOKI_HOST=<中心私网>`） |

启用步骤与 LogQL 见 **[docs/logging-playbook.md](docs/logging-playbook.md)**。看板：**TCP Session Logs**。  
本地日志：`${RELAYGATE_DATA_DIR}/envoy/logs/tcp-access.json`。

---

## 双活维护与升级

多节点见 **[机群架构](docs/fleet-scale-control-plane.md)** 与 **[机群运维](docs/fleet-ops.md)**；菜单/确认词见 **[产品表面](docs/product-surface-agent.md)**。

```text
客户端 → 云 L4 LB
           ├─ gateway-01 主控（control.env · ENABLE_PANEL=1；可选本机转发）
           └─ gateway-02 网关节点（node.env · ENABLE_PANEL=0 · agent 拉取）
```

机群名册面向 **网关节点**；主控本机转发走「配置应用」，不要用退役下线主控。

```bash
relaygate diag
relaygate drain fail             # 等 DRAIN_WAIT（默认 30s）
# 控制台确认 target unhealthy 后：
relaygate reload                 # 或 upgrade --drain
relaygate drain ok
relaygate smoke
```

`DRAIN_WAIT` 见 [`packaging/shared/env.example`](packaging/shared/env.example)。NLB：[`packaging/terraform/nlb/`](packaging/terraform/nlb/)。

```bash
sudo relaygate upgrade --drain
relaygate rollback
sudo PURGE=1 bash install.sh --uninstall
```

---

## 排障速查

| 现象 | 先查 |
|------|------|
| `/ready` 非 LIVE / LB 不健康 | `relaygate diag`；是否误摘流 `drain status` |
| 转发不通 | 转发是否 `enabled`；入口类型；上游端口；`smoke` / `canary` |
| 改了 YAML 不生效 | 是否走了对应 Apply（`reload` vs `firewall apply`） |
| Panel 打不开 | `PANEL_BIND`、防火墙 9000、systemd `relaygate-panel` |
| 无会话日志 | `COMPOSE_PROFILES`、Fluent Bit/Loki；见 [logging-playbook](docs/logging-playbook.md) |
| 限速过猛 / ACL 误伤 | `defaults` 限速字段、`acl.deny`/`allow`；SSH 不受 ACL 影响 |

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

对外对齐 Envoy / L4；YAML / API / Panel 使用统一中性字段名。

| 中文（对外） | English | YAML / 运行标识 |
|--------------|---------|-----------------|
| **下游** / 客户端 | Downstream / Client | （无独立资源） |
| **入口** | Entry | `forwards[].entry` + `listen_port` |
| **验证入口** / **正式入口** | Validation / Production | `validation` / `production` |
| **Listener** | Listener | `ingress-{forward名}` |
| **上游** | Upstream | `upstreams[]` |
| **转发** | Forward | `forwards[]` |
| **安全策略** | Security policy | `security.policies[]`（含 allowlist deny/allow） |
| **默认上游端口** | Default upstream port | `defaults.default_upstream_tcp_port` / `default_upstream_udp_port` |
| **业务标识** | Service name | `meta.service_name` |

---

## 贡献与安全

- 参与开发：[CONTRIBUTING.md](CONTRIBUTING.md)（含 DCO）
- 变更记录：[CHANGELOG.md](CHANGELOG.md)
- 安全披露：[SECURITY.md](SECURITY.md)
- 行为准则：[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

## License

[MIT](LICENSE) © 2026 RelayGate
