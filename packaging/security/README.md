# 网关安全防护（Security · 运维）

通用 **四层（L4）** 南北向网关。防护按 **攻击类型** 组织为可开关的 **安全策略（security.policies[]）**；配置真相源为 `resources.yaml` 的 `security` 段。

领域命名与落地顺序（**本版无兼容、以当前代码为准**）见 [docs/security-domains.md](../../docs/security-domains.md)。

**Long-lived TCP 铁律：** 新建连接限速只打 **SYN / ct state new**；`gateway.nft` 对 `established,related` 先 accept。禁止对已建立会话做 **PPS** 限速。

**Out of scope：** 经外部 **Rate Limit Service（RLS）**（如 Redis）的 **全局限速** — 本产品不做。

| 文档 | 内容 |
|------|------|
| **[threat-analysis.md](threat-analysis.md)** | 攻击类型 T1–T12、策略对照表、分层矩阵 |
| 下文 | 执行层、策略怎么用、应用路径 |

---

## 安全领域（产品）vs 执行组件

产品主文案只用领域简称：**内核 / 防火墙 / 网卡（预留）/ 网关**。执行组件（sysctl、nftables、Envoy）仅脚本与高级帮助。完整约定见 [docs/security-domains.md](../../docs/security-domains.md)。

```text
resources.yaml  security.policies[]
        │
        ├─ 防火墙 ─ allowlist、firewall_new_conn_limit、udp_limit
        │         → relaygate firewall apply（Panel：应用安全策略）
        │         → 节点 agent 拉取后可自动应用（见 SECURITY_AUTO_APPLY）
        │         执行：nftables
        │
        ├─ 网关 ─── conn_limit、gateway_new_conn_limit
        │         → HotApply / reload；节点 agent 拉取后 HotApply
        │         执行：Envoy
        │
        ├─ 内核 ─── kernel_syn
        │         → relaygate security apply-kernel --verify（内核 · sysctl）
        │         → 节点 agent 拉取后可自动应用（主控默认关闭）
        │
        └─ 网卡 ─── （预留，未实现）
```

落地顺序：**内核 →（网卡 skip）→ 防火墙 → 网关**。入站路径上防火墙在网卡下游。

- **防火墙**：来源 allow/deny、新建 TCP 限速、UDP PPS。`source-acl` 不是独立中间层，只是 `allowlist` 的 `params`。
- **内核**：SYN cookies 等；保存 YAML 不会自动改内核，须 apply-kernel 或节点自动落地。
- **「应用安全策略」** 在 UI 上仅指 **防火墙**；勿与内核或网关 reload 混在同一危险按钮。
- **SECURITY_AUTO_APPLY**：未设置时，`ENABLE_PANEL=0`（纯节点）默认自动应用主机侧；`ENABLE_PANEL=1`（主控）默认关闭。显式 `1`/`0` 可覆盖。

---

## 攻击类型 → 策略 → 应用

| 攻击（Threat） | 策略 ID | type | 领域 | 参数（params） | 保存后如何生效 |
|----------------|---------|------|------|----------------|----------------|
| **T1** SYN Flood | `kernel_syn` | kernel | 内核 | （脚本常量） | `apply-sysctl-harden.sh --apply` |
| **T1/T4** 新建连接洪泛（防火墙） | `firewall_new_conn_limit` | new_conn_limit_firewall | 防火墙 | `tcp_per_ip`、`burst` | **应用安全策略** |
| **T1/T4** 新建连接洪泛（网关） | `gateway_new_conn_limit` | new_conn_limit_gateway | 网关 | `per_sec`、`burst` | reload |
| **T2** 连接耗尽 | `conn_limit` | conn_limit | 网关 | `max_connections` | reload |
| **T5** 扫描 / 探测 | `allowlist` | allowlist | 防火墙 | `deny` / `allow` CIDR | **应用安全策略** |
| **T6** UDP 反射 / 噪声 | `udp_limit` | udp_limit | 防火墙 | `udp_pps_per_ip`、`udp_burst` | **应用安全策略** |

每条策略含：`id`、`type`、`enabled`、`attack_tags[]`、`params`。关闭某策略时渲染层使用「放行」等效值，**不伤 established 长连接**。

### 示例配置片段

```yaml
security:
  policies:
    - id: kernel_syn
      type: kernel
      enabled: true
      attack_tags: [T1]
    - id: firewall_new_conn_limit
      type: new_conn_limit_firewall
      enabled: true
      attack_tags: [T1, T4]
      params:
        tcp_per_ip: 30/second
        burst: 60
    - id: gateway_new_conn_limit
      type: new_conn_limit_gateway
      enabled: true
      attack_tags: [T1, T4]
      params:
        per_sec: 200
        burst: 400
    - id: conn_limit
      type: conn_limit
      enabled: true
      attack_tags: [T2]
      params:
        max_connections: 1024
    - id: allowlist
      type: allowlist
      enabled: true
      attack_tags: [T5]
      params:
        deny: []
        allow: []
    - id: udp_limit
      type: udp_limit
      enabled: true
      attack_tags: [T6]
      params:
        udp_pps_per_ip: 500/second
        udp_burst: 1000
```

完整模板见 `packaging/shared/resources.example.yaml`。

---

## 用户操作路径

### Panel

1. **安全策略**（`/security`）— 开关、参数、allowlist deny/allow 名单；保存后点 **应用安全策略**（nft）
2. **配置应用** — Envoy/转发相关变更：本机应用（reload）；nft 待办显示为「安全策略」

### CLI

```bash
relaygate security list          # 含 allowlist 名单
relaygate security kernel-conf   # 按配置渲染内核（sysctl）叠加（stdout）
relaygate security apply-kernel --verify   # 内核（sysctl）：按配置应用并校验
relaygate security verify        # 校验内核 / 防火墙 / 网关 ready

relaygate validate
relaygate reload                 # 网关（含 gateway_new_conn_limit）
sudo relaygate firewall apply    # 防火墙（与 Panel「应用安全策略」同效）
# 兼容脚本（优先读 resources.yaml）：
sudo ./packaging/security/apply-sysctl-harden.sh --apply --verify

relaygate profile apply tcp-longlived
```

节点 agent：`relaygate agent run` 拉取成功后按序落地 **内核 →（网卡 skip）→ 防火墙 → 网关**（主机侧受 `SECURITY_AUTO_APPLY` / `ENABLE_PANEL` 约束）；失败不更新 applied-version。状态见 DataDir `security-apply-status.json`（领域键 `kernel` / `nic` / `firewall` / `gateway`）与 journal。
`nft-newconn-syn.snippet.nft` 仅对照说明，**禁止**用它 flush 正式规则集。

---

## 应用场景（packaging/profiles）

Panel **安全策略**页可选择场景模板填入 `security.policies` 与相关 defaults（须保存后才写入磁盘）：

| 模板 | scenario | 说明 |
|------|----------|------|
| `default-l4` | default_l4 | 通用 L4 默认 |
| `tcp-longlived` | tcp_longlived | TCP 长连接场景安全策略（小包基线，放宽 idle/并发） |
| `tcp-short-burst` | tcp_short_burst | 高并发短连接 |
| `udp-heavy` | udp_heavy | UDP 包率偏高 |
| `strict-allowlist` | strict_allowlist | 严格 allow 名单（示例 CIDR 须替换） |
| `host-harden-only` | host_harden_only | 仅 kernel_syn，关闭防火墙/网关业务限速 |

CLI：`relaygate profile apply <name>` 会写入 defaults + 合并 security 参数。

---

## 配置预览

安全策略页 **预览生效结果**（`GET/POST /api/security/preview`）展示：

- **落地顺序**：内核 →（网卡预留）→ 防火墙 → 网关
- **内核**：`kernel_syn` 启用时显示 harden 片段（高级区可见 sysctl 键）
- **防火墙**：`forward-ports` / INPUT 链摘录（高级区可写 nft 文件名）
- **网关**：`max_connections`、本地新建连接限速等从 policies 推导
- **策略与领域**：每条 policy 落在哪一域（防火墙与网关的新建连接限速为独立策略）

未保存的编辑可通过 POST 携带 `policies[]` 预览。

---

## 落地顺序（防火墙与网关两道新建连接限速）

| 顺序 | 领域 | 执行细节 | 策略 |
|------|------|----------|------|
| 1 | 内核 | sysctl SYN cookies / backlog | kernel_syn |
| — | 网卡 | （预留） | — |
| 2 | 防火墙 | established,related accept | — |
| 3 | 防火墙 | allowlist deny | allowlist |
| 4 | 防火墙 | allowlist allow strict | allowlist |
| 5 | 防火墙 | 新建 TCP 每 IP 限速 | firewall_new_conn_limit |
| 6 | 防火墙 | UDP 每 IP PPS | udp_limit |
| 7 | 网关 | listener 本地令牌桶 | gateway_new_conn_limit |
| 8 | 网关 | cluster max_connections | conn_limit |

`firewall_new_conn_limit` 与 `gateway_new_conn_limit` 为 **两条独立策略**，可分别开关与调参；若两者均启用，须先后通过。

---

## 与 profiles / 脚本的关系

| 组件 | 作用 | 与策略的关系 |
|------|------|----------------|
| `packaging/profiles/*.yaml` | 批量写入 `defaults` + `security.policies` 参数 | 不自动改策略开关 |
| `packaging/sysctl/gateway.conf` | 基线 somaxconn / 缓冲 | `relaygate setup --sysctl` |
| `sysctl-tcp-harden.conf` | SYN cookies 等 | 对应 `kernel_syn` |
| `gateway.nft` + render | 正式 nft 规则 | 读取 `security.policies` 有效值 |

---

## TCP 长连接场景安全策略

套用档位：`relaygate profile apply tcp-longlived`（或 Panel **安全策略** → 场景「TCP 长连接」）。

依据 [包长对照](../../docs/packet-size-traffic-analysis.md)：正常多为 **0–199 字节**小包（established 心跳/短载荷），攻击侧常见 **近 MTU 大包单峰**（体量型，走云清洗）。因此本档位：

| 项 | default-l4 | tcp-longlived | 说明 |
|----|------------|---------------|------|
| `tcp_idle_timeout` | 3600s | **14400s** | 稀疏小包下勿过早掐断长连 |
| `conn_limit.max_connections` | 1024 | **4096** | 提高并发槽位 |
| `max_pending_requests` | 256 | **1024** | 配套 pending |
| `firewall_new_conn_limit` | 30/s · 60 | **40/s · 80** | 仅 new；略放宽以容纳重连潮 |
| `gateway_new_conn_limit` | 200 · 400 | **150 · 300** | 仍低于短连接洪泛档；约束握手滥用 |

**铁律不变：** `established,related` 先 accept；禁止对已建会话做 PPS。近 MTU 体量攻击 **不是** 靠本档限速清洗。

调高 `max_connections` 后请核对 Prometheus `EnvoyConnectionsNearLimit`（默认按 1024 的 80%=800；本档约 **3277**）。辅助脚本：`packaging/security/apply-tcp-longlived.sh`。
