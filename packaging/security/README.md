# 网关安全防护（Security · 运维）

通用 **四层（L4）** 南北向网关。防护按 **攻击类型** 组织为可开关的 **安全策略（security.access / security.protections[]）**；配置真相源为 `resources.yaml` 的 `security` 段。

领域命名与落地顺序见 [docs/security-domains.md](../../docs/security-domains.md)。

**Long-lived TCP 铁律：** 新建连接限速只打 **SYN / ct state new**；`gateway.nft` 对 `established,related` 先 accept。禁止对已建立会话做 **PPS** 限速。

**Out of scope：** 经外部 **Rate Limit Service（RLS）**（如 Redis）的 **全局限速** — 本产品不做。

| 文档 | 内容 |
|------|------|
| **[threat-analysis.md](threat-analysis.md)** | 攻击类型 T1–T12、策略对照表、分层矩阵 |
| 下文 | 执行层、策略怎么用、应用路径 |

---

## 安全领域（产品）vs 执行组件

产品主文案只用领域简称：**内核 / 防火墙 / 网卡 / 网关**。执行组件（sysctl、nftables、tc、Envoy）仅脚本与高级帮助。完整约定见 [docs/security-domains.md](../../docs/security-domains.md)。

```text
resources.yaml  security.access / security.protections[]
        │
        ├─ 防火墙 ─ access、firewall_new_conn_limit、firewall_udp_limit
        │         → relaygate firewall apply（Panel：应用安全策略）
        │         → 节点 agent 拉取后可自动应用（见 SECURITY_AUTO_APPLY）
        │         执行：nftables
        │
        ├─ 网关 ─── gateway_conn_limit、gateway_new_conn_limit
        │         → HotApply / reload；节点 agent 拉取后 HotApply
        │         执行：Envoy
        │
        ├─ 内核 ─── kernel_syn
        │         → relaygate security apply-kernel --verify（内核 · sysctl）
        │         → 节点 agent 拉取后可自动应用（主控默认关闭）
        │
        └─ 网卡 ─── nic_egress_shape、nic_ingress_police
                  → relaygate security apply-nic --verify（网卡 · tc；可同时落地已启用项）
                  → 节点 agent 拉取后可自动应用（主控默认关闭；均未启用则 skipped）
                  执行：tc（出口 TBF + 入向 police）；过滤仍主要 nft；XDP 不作默认
```

落地顺序：**内核 → 网卡 → 防火墙 → 网关**。入站路径上防火墙在网卡下游。

- **防火墙**：来源 allow/deny、新建 TCP 限速、UDP PPS。`source-acl` 不是独立中间层，只是 `security.access` 的 deny/allow。
- **内核**：SYN cookies 等；保存 YAML 不会自动改内核，须 apply-kernel 或节点自动落地。
- **网卡**：业务口**出口**整形与**入向** police；`device` 空则探测默认路由口；`rate` 示例 `3mbit`（对齐主机带宽，勿按高防口放开）。入向 police 管入向字节/带宽，**不替代**高防；大包洪水主防仍在高防。关闭策略**不**自动删 qdisc/police。
- **「应用安全策略」** 在 UI 上仅指 **防火墙**；勿与内核、网卡或网关 reload 混在同一危险按钮。
- **SECURITY_AUTO_APPLY**：未设置时，`PANEL_ENABLED=0`（纯节点）默认自动应用主机侧（内核/网卡/防火墙）；`PANEL_ENABLED=1`（主控）默认关闭。显式 `1`/`0` 可覆盖。
- **与 `APPLY_FIREWALL` 分层**：`APPLY_FIREWALL=1` 只用于**安装/CLI 一次性**应用防火墙；运行时节点拉取后的自动落地只用 `SECURITY_AUTO_APPLY`（二者勿混用、勿指望互为别名）。

---

## 网卡整形 / 入向限速（apply-nic）运维

适用：低带宽主机（示例 **约 3 Mbps**）上限制本机**出站**突发与/或**入向**字节速率，减轻薄链路被占满；**不是**入站抗 DDoS / 高防替代。过滤/限速仍用防火墙（nft）。**勿在主控管理口随意 apply**（`PANEL_ENABLED=1` 默认也不自动落地）。

### 配置

```yaml
- id: nic_egress_shape
  type: nic_egress_shape
  enabled: true
  params:
    device: ""      # 空=探测默认路由口；或显式 eth0 / ens3
    rate: 3mbit     # 对齐主机出站，勿按高防 10M
- id: nic_ingress_police
  type: nic_ingress_police
  enabled: true
  params:
    device: ""
    rate: 3mbit     # 入向带宽 police；大包洪水主防仍在高防
```

也可 Panel「添加防护策略」选 `[网卡] 出口整形` / `[网卡] 入向限速`，保存后再在**目标节点**落地。

### 应用与校验（须确认词）

```bash
# 先看将作用的口（可选）
ip -o route show default

# 应用 + 校验（同时落地已启用的 nic_*；交互确认词：确认 / Confirm；或非交互）
sudo RELAYGATE_CONFIRM=Confirm relaygate security apply-nic --verify

# 只读核对
tc qdisc show dev eth0                    # 出口期望含 tbf rate≈3Mbit；入向期望含 ingress
tc filter show dev eth0 parent ffff:      # 入向期望含 police rate≈3Mbit
```

特权 helper：`sudo /usr/lib/relaygate/relaygate-apply nic-shape-apply`（内部带 `RELAYGATE_CONFIRM=Confirm`）。

### 回滚

关闭 `nic_egress_shape` / `nic_ingress_police` **不会**自动清除已有 qdisc/police。手动恢复：

```bash
# 出口：删 root qdisc（会去掉 TBF；通常回到内核默认）
sudo tc qdisc del dev eth0 root

# 入向：删 ingress qdisc（一并去掉 police filter）
sudo tc qdisc del dev eth0 ingress

tc qdisc show dev eth0
```

若误在错误口上整形/限速，立刻对**该口**执行对应 `tc qdisc del`；SSH 若走同一口且已被严重限速，优先走带外/控制台。

### 非目标（网卡域）

- XDP / 多口拓扑
- 关闭策略时自动清 qdisc/police
- 主控 eth0 当试验场
- 替代云高防做体量清洗
---

## 攻击类型 → 策略 → 应用

| 攻击（Threat） | 策略 ID | type | 领域 | 参数（params） | 保存后如何生效 |
|----------------|---------|------|------|----------------|----------------|
| **T1** SYN Flood | `kernel_syn` | kernel_syn | 内核 | （脚本常量） | `relaygate security apply-kernel --verify` |
| **T7** 带宽占满（本机减负 · 出口） | `nic_egress_shape` | nic_egress_shape | 网卡 | `device`、`rate` | `relaygate security apply-nic --verify` |
| **T7** 带宽占满（本机减负 · 入向） | `nic_ingress_police` | nic_ingress_police | 网卡 | `device`、`rate` | `relaygate security apply-nic --verify` |
| **T1/T4** 新建连接洪泛（防火墙） | `firewall_new_conn_limit` | firewall_new_conn_limit | 防火墙 | `tcp_per_ip`、`burst` | **应用安全策略** |
| **T1/T4** 新建连接洪泛（网关） | `gateway_new_conn_limit` | gateway_new_conn_limit | 网关 | `per_sec`、`burst` | reload |
| **T2** 连接耗尽 | `gateway_conn_limit` | gateway_conn_limit | 网关 | `max_connections` | reload |
| **T5** 扫描 / 探测 | `security.access` | — | 防火墙 | `deny` / `allow` CIDR | **应用安全策略** |
| **T6** UDP 反射 / 噪声 | `firewall_udp_limit` | firewall_udp_limit | 防火墙 | `udp_pps_per_ip`、`udp_burst` | **应用安全策略** |

每条策略含：`id`、`type`、`enabled`、`attack_tags[]`、`params`。关闭某策略时渲染层使用「放行」等效值，**不伤 established 长连接**。

### 示例配置片段

```yaml
security:
  access:
    enabled: true
    deny: []
    allow: []
  protections:
    - id: kernel_syn
      type: kernel_syn
      enabled: true
      attack_tags: [T1]
    - id: firewall_new_conn_limit
      type: firewall_new_conn_limit
      enabled: true
      attack_tags: [T1, T4]
      params:
        tcp_per_ip: 30/second
        burst: 60
    - id: gateway_new_conn_limit
      type: gateway_new_conn_limit
      enabled: true
      attack_tags: [T1, T4]
      params:
        per_sec: 200
        burst: 400
    - id: gateway_conn_limit
      type: gateway_conn_limit
      enabled: true
      attack_tags: [T2]
      params:
        max_connections: 1024
    - id: firewall_udp_limit
      type: firewall_udp_limit
      enabled: true
      attack_tags: [T6]
      params:
        udp_pps_per_ip: 500/second
        udp_burst: 1000
    - id: nic_egress_shape
      type: nic_egress_shape
      enabled: false
      attack_tags: [T7]
      params:
        device: ""
        rate: 3mbit
    - id: nic_ingress_police
      type: nic_ingress_police
      enabled: false
      attack_tags: [T7]
      params:
        device: ""
        rate: 3mbit
```

完整模板见 `packaging/shared/resources.example.yaml`。

---

## 用户操作路径

### Panel

1. **安全策略**（`/security`）— 开关、参数、access deny/allow 名单；保存后点 **应用安全策略**（nft）
2. **配置应用** — Envoy/转发相关变更：本机应用（reload）；nft 待办显示为「安全策略」

### CLI

```bash
relaygate security list          # 含 access 名单
relaygate security kernel-conf   # 按配置渲染内核（sysctl）叠加（stdout）
relaygate security apply-kernel --verify   # 内核（sysctl）：按配置应用并校验
relaygate security apply-nic --verify      # 网卡（tc）：出口整形 / 入向 police 并校验
relaygate security verify        # 校验内核 / 网卡 / 防火墙 / 网关 ready

relaygate validate
relaygate reload                 # 网关（含 gateway_new_conn_limit）
sudo relaygate firewall apply    # 防火墙（与 Panel「应用安全策略」同效）

relaygate profile apply tcp-longlived
# 可选主机侧（节点或明确测试环境；主控默认勿乱 apply）:
# sudo RELAYGATE_CONFIRM=Confirm relaygate security apply-kernel --verify
# sudo RELAYGATE_CONFIRM=Confirm relaygate security apply-nic --verify
```

节点 agent：`relaygate agent run` 拉取成功后按序落地 **内核 → 网卡 → 防火墙 → 网关**（主机侧受 `SECURITY_AUTO_APPLY` / `PANEL_ENABLED` 约束）；失败不更新 applied-version。状态见 DataDir `security-apply-status.json`（领域键 `kernel` / `nic` / `firewall` / `gateway`）与 journal。
`nft-newconn-syn.snippet.nft` 仅对照说明，**禁止**用它 flush 正式规则集。

---

## 应用场景（packaging/profiles）

Panel **安全策略**页可选择场景模板填入 `security.protections` 与相关 defaults（须保存后才写入磁盘）：

| 模板 | scenario | 说明 |
|------|----------|------|
| `default-l4` | default_l4 | 通用 L4 默认 |
| `tcp-longlived` | tcp_longlived | TCP 长连接（低带宽约 3 Mbps；宽 idle/并发、出口/入向 3mbit；新建/UDP 稳态偏紧但 burst 留重连余量，防误杀） |
| `tcp-short-burst` | tcp_short_burst | 高并发短连接 |
| `udp-heavy` | udp_heavy | UDP 包率偏高 |
| `strict-allowlist` | strict_allowlist | 严格 allow 名单（示例 CIDR 须替换） |
| `host-harden-only` | host_harden_only | 仅 kernel_syn，关闭防火墙/网关/网卡业务限速 |

CLI：`relaygate profile apply <name>` 会写入 defaults + 合并 security 参数。

---

## 配置预览

安全策略页 **预览生效结果**（`GET/POST /api/security/preview`）展示：

- **落地顺序**：内核 → 网卡 → 防火墙 → 网关
- **内核**：`kernel_syn` 启用时显示 harden 片段（高级区可见 sysctl 键）
- **网卡**：`nic_egress_shape` / `nic_ingress_police` 启用时显示 device/rate
- **防火墙**：`forward-ports` / INPUT 链摘录（高级区可写 nft 文件名）
- **网关**：`max_connections`、本地新建连接限速等从 policies 推导
- **策略与领域**：每条 policy 落在哪一域（防火墙与网关的新建连接限速为独立策略）

未保存的编辑可通过 POST 携带 `access` + `protections[]` 预览。

---

## 落地顺序（防火墙与网关两道新建连接限速）

| 顺序 | 领域 | 执行细节 | 策略 |
|------|------|----------|------|
| 1 | 内核 | sysctl SYN cookies / backlog | kernel_syn |
| 2 | 网卡 | tc 出口整形 / 入向 police（可选） | nic_egress_shape、nic_ingress_police |
| 3 | 防火墙 | established,related accept | — |
| 4 | 防火墙 | access deny | access |
| 5 | 防火墙 | access allow strict | access |
| 6 | 防火墙 | 新建 TCP 每 IP 限速 | firewall_new_conn_limit |
| 7 | 防火墙 | UDP 每 IP PPS | firewall_udp_limit |
| 8 | 网关 | listener 本地令牌桶 | gateway_new_conn_limit |
| 9 | 网关 | cluster max_connections | gateway_conn_limit |

`firewall_new_conn_limit` 与 `gateway_new_conn_limit` 为 **两条独立策略**，可分别开关与调参；若两者均启用，须先后通过。

---

## 与 profiles / 脚本的关系

| 组件 | 作用 | 与策略的关系 |
|------|------|----------------|
| `packaging/profiles/*.yaml` | 批量写入 `defaults` + `security.protections` 参数 | 不自动改策略开关 |
| `packaging/sysctl/gateway.conf` | 基线 somaxconn / 缓冲 | `relaygate setup --sysctl` |
| `sysctl-tcp-harden.conf` | SYN cookies 等 | 对应 `kernel_syn` |
| `gateway.nft` + render | 正式 nft 规则 | 读取 `security.protections` 有效值 |

---

## TCP 长连接场景安全策略

套用档位：`relaygate profile apply tcp-longlived`（或 Panel **安全策略** → 场景「TCP 长连接」）。

**示例假设：** 主机真实出/入口约 **3 Mbps**（高防口或约 10 Mbps，整形应对齐主机 3M，勿按 10M）。长连接：高 idle、稳态新建不高，但会有多客户端同时重连 / 发布后抖动 / 校验+生产口 / TCP+UDP 同业务。

**防误杀（低带宽）：** 出口整形与入向 police 可跟主机 **3mbit**；连接与 UDP 限速勿按「极严防扫」砍到误杀重连——**稳态可低于 default-l4，短时 burst 须留足**（数秒内几十～上百新建、UDP 心跳余量）。

依据 [包长对照](../../docs/packet-size-traffic-analysis.md)：正常多为 **0–199 字节**小包（established 心跳/短载荷），攻击侧常见 **近 MTU 大包单峰**（体量型，走云清洗）。因此本档位：

| 项 | default-l4 | tcp-longlived | 说明 |
|----|------------|---------------|------|
| `tcp_idle_timeout` | 3600s | **14400s** | 稀疏小包下勿过早掐断长连 |
| `udp_idle_timeout` | 120s | **180s** | 略放宽 |
| `max_pending_requests` | 256 | **1024** | 重连潮缓冲 |
| health_check interval/timeout | 10s / 2s | **15s / 3s** | 薄链路少探测、略容错 |
| `gateway_conn_limit.max_connections` | 1024 | **4096** | 宽 idle 槽位；勿无依据砍并发 |
| `firewall_new_conn_limit` | 30/s · 60 | **20/s · 80** | 仅 new；稳态偏紧，burst 覆盖 NAT 重连潮 |
| `gateway_new_conn_limit` | 200 · 400 | **80 · 160** | 每 listener；突发留发布后重连余量 |
| `firewall_udp_limit` | 500/s · 1000 | **250/s · 500** | 按 3M 小包 PPS 留心跳余量，勿过紧 |
| `nic_egress_shape` | 默认关 | **开 · 3mbit** | 口级 tc 出口；对齐主机而非高防 10M |
| `nic_ingress_police` | 默认关 | **开 · 3mbit** | 口级 tc 入向 police；不替代高防 |

**铁律不变：** `established,related` 先 accept；禁止对已建会话做 PPS。近 MTU 体量攻击 **不是** 靠本档限速清洗。场景 **不写 `access`**，合并时保留运维名单。

调高 `max_connections` 后请核对 Prometheus `EnvoyConnectionsNearLimit`（本档约 **3277**=4096×80%）。

建议操作顺序：

```bash
relaygate profile apply tcp-longlived
# 热更新本机应用（Panel/CLI，须确认词）— 网关侧
# 节点或明确测试环境再落地主机侧（主控默认跳过自动 apply）:
# sudo RELAYGATE_CONFIRM=Confirm relaygate security apply-kernel --verify
# sudo RELAYGATE_CONFIRM=Confirm relaygate security apply-nic --verify
# 若改了防火墙策略: sudo relaygate firewall apply（须确认词）
```
