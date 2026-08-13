# L4 网关安全防护：攻击类型 → 策略 → 开关 → 应用

面向通用 **四层（L4）TCP/UDP** 南北向网关（含 **TCP 长连接 / long-lived TCP**）。

**产品边界（out of scope）：** 外部 **RLS / Redis 全局限速**；**带宽型 DDoS** 以云清洗为主。

**Long-lived TCP：** 主机与 Envoy 限速仅针对 **新建连接（ct state new / local_ratelimit）**；**established** 须先放行，禁止对整会话做 PPS 限速。场景档位见 `packaging/profiles/tcp-longlived.yaml`（按小包主导业务放宽 idle/并发；流量对照 [packet-size-traffic-analysis.md](../../docs/packet-size-traffic-analysis.md)）。

---

## 1. 策略目录（Policy catalog）

配置载体：`resources.yaml` → `security.policies[]`（`id`、`type`、`enabled`、`params`）。

| ID | 中文名 | English | Threat | 领域 | 生效方式 |
|----|--------|---------|--------|------|----------|
| `kernel_syn` | SYN 洪泛加固 | SYN flood hardening | T1 | 内核 | `apply-sysctl-harden.sh --apply` |
| `firewall_new_conn_limit` | 防火墙新建连接限速 | Firewall new-connection rate limit | T1, T4 | 防火墙 | `firewall apply` |
| `gateway_new_conn_limit` | 网关新建连接限速 | Gateway new-connection rate limit | T1, T4 | 网关 | `reload` |
| `conn_limit` | 并发连接上限 | Connection limit | T2 | 网关 | `reload` |
| `allowlist` | 来源访问控制 | Source access control | T5 | 防火墙 | `firewall apply` |
| `udp_limit` | UDP 包速限制 | UDP PPS limit | T6 | 防火墙 | `firewall apply` |

关闭策略时的行为（不伤 established）：

| 策略 | 禁用时 |
|------|--------|
| `firewall_new_conn_limit` | 防火墙使用极高 new 速率 |
| `gateway_new_conn_limit` | 网关省略本地新建连接限速过滤器 |
| `conn_limit` | 网关使用极高 `max_connections` |
| `allowlist` | 忽略严格 allow/deny（仍保留默认 drop 未开放端口） |
| `udp_limit` | 防火墙 UDP 使用极高 PPS |
| `kernel_syn` | 仅 YAML 标记；是否卸载内核叠加由运维在本机处理 |

---

## 2. 攻击种类（Threats T1–T12）

| ID | 攻击 Attack | 推荐策略 | 备注 |
|----|-------------|----------|------|
| T1 | SYN Flood | `kernel_syn` + `firewall_new_conn_limit` + `gateway_new_conn_limit` | 加固握手；勿动 established |
| T2 | 连接耗尽 | `conn_limit` + 新建连接限速 | idle 勿过短误杀长连 |
| T3 | 慢连接（Slowloris-style） | `conn_limit`（辅助） | 主要靠 idle / 上游超时 |
| T4 | 新建连接洪泛 | `firewall_new_conn_limit` + `gateway_new_conn_limit` | 防火墙 new + 网关本地限速 |
| T5 | 端口扫描 / 探测 | `allowlist` + 默认 drop | SSH 独立限速 |
| T6 | UDP 反射 / 放大 | `udp_limit` | 非抗 volumetric |
| T7 | 带宽型 DDoS | —（云侧） | 本机减负 only |
| T8 | 凭证滥用 | —（ops） | token / 确认词 / standby |
| T9 | 配置篡改 / 漂移 | —（ops） | 确认词、`firewall check` |
| T10 | 横向移动 | —（上游 SG） | 最小放行 |
| T11 | 管理面暴露 | `gateway.nft` 内置 | Panel 绑本机或 OPS 源 |
| T12 | drain / HC 干扰 | —（运维） | 维护窗口摘流 |

---

## 3. 领域矩阵（摘要）

| 攻击 | 内核 | 防火墙 | 网关 | 本产品策略 |
|------|------|--------|------|------------|
| T1 | ● SYN cookies | ○ | — | `kernel_syn` |
| T2 | ○ FD | ● new RL | ● conn limit | `firewall_new_conn_limit`, `gateway_new_conn_limit`, `conn_limit` |
| T4 | ○ | ● | ● local RL | `firewall_new_conn_limit`, `gateway_new_conn_limit` |
| T5 | — | ● ACL | — | `allowlist` |
| T6 | — | ● UDP PPS | ○ | `udp_limit` |
| T7 | — | — | — | **out of scope**（云清洗） |

领域 vs 执行组件、落地顺序见 [docs/security-domains.md](../../docs/security-domains.md)。完整矩阵见 [README.md](README.md).

---

## 4. 明确不做（out of scope）

| 手段 | 态度 |
|------|------|
| Global RLS（Redis 等） | **out of scope** |
| Envoy delay-reject 引擎 | **out of scope** |
| 对已建立 TCP 做主机 PPS 限速 | **禁止** |
| 本机独自抗 volumetric DDoS | **不指望** |

---

## 术语表（Glossary）

| 中文 | English |
|------|---------|
| 纵深防御 | defense in depth |
| SYN 洪泛 | SYN Flood |
| 新建连接 | new connections (`ct state new`) |
| 已建立连接 | established |
| 连接耗尽 | connection exhaustion |
| 本机限速 | local rate limit (Envoy) |
| 允许名单 / 拒绝名单 | allowlist / denylist |
