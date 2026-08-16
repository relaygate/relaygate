# 安全领域（Security domains）

贡献者 / 深度运维说明。产品主文案用**领域简称**；`sysctl` / `nftables` / `Envoy` 等是执行组件（脚本与高级帮助）。

## 结构

```yaml
security:
  access:                 # 防火墙域：来源访问控制（非 protections 成员）
    enabled: true
    deny: []
    allow: []
  protections:            # 内核 / 防火墙限速 / 网关限速与并发
    - id: kernel_syn
      type: kernel_syn
      enabled: true
      params: { ... }
```

- access 与 protections 同级；防护 id 领域前缀，type≡id。
- **params**：产品键（如 `tcp_per_ip`）；未知键进 `PolicyParams.Extra`（往返保留，不参与内置生效计算）。`security.access` 强类型。

### 节点身份：`GATEWAY_NAME` 与 `gateway.name`

节点身份以 **`GATEWAY_NAME`** 为准；本机 `resources.yaml` 可选 **`gateway.name`**（拉取后由 agent 盖章）。机群发布剥离 `gateway.name` / `public_ip`。

---

## 四域与简称

| 全称 | 简称中 | 简称英 | 执行组件（不进产品主文案当模块名） |
|------|--------|--------|--------------------------------------|
| 主机内核加固 | 内核 | Kernel | sysctl 等 |
| 主机防火墙 | 防火墙 | Firewall | nftables |
| 网卡与入口路径 | 网卡 | NIC | ip / tc（默认整形）；XDP 不作默认 |
| 网关转发 | 网关 | Gateway | Envoy |

## 入站路径 vs 落地顺序

```text
入站路径（流量方向）:
  线缆 → 网卡 → 防火墙 → 协议栈/内核行为 → 网关

落地 / 校验顺序（agent 拉取后、运维应用）:
  内核 → 网卡 → 防火墙 → 网关
```

防火墙链内（**准入在限速前**）：

```text
established,related accept
→ access deny
→ access allow（strict）
→ firewall_new_conn_limit（TCP new）
→ firewall_udp_limit
→（随后）网关 local_ratelimit / gateway_conn_limit
```

网卡域：**出口整形**（`nic_egress_shape` → tc egress TBF）与 **入向 police**（`nic_ingress_police` → tc ingress）。启用时落地；关闭则状态记 `skipped`（**不**自动删除已有 qdisc/police，需运维手动回滚）。过滤仍主要走防火墙（nft）；**不做** XDP（与「过滤 nft、整形/限速 tc」一致）。体量型入站清洗属云高防；本机入向 police 只做主机侧带宽减负，**不替代**高防。

## 产品文案 vs 执行组件

| 场景 | 用语 |
|------|------|
| Panel / 确认框 / 状态 / 预览主文案 | `[内核]` / `[网卡]` / `[防火墙]` / `[网关]`（及对应英文） |
| CLI 主帮助 | 领域名；子命令 `apply-kernel` / `apply-nic` / `kernel-conf`；帮助可括注执行细节 |
| 脚本路径、日志折叠细节、高级区 | 可写 sysctl / tc / nftables / Envoy |

**禁止：** 把 nft / nftables / Envoy /「数据面」/ sysctl / tc 当作产品三/四件套模块名；领域与组件混用并列。

## 防护 id

| id | 领域 | 执行面 |
|----|------|--------|
| `security.access`（`enabled`/`deny`/`allow`） | 防火墙 | nft ACL |
| `kernel_syn` | 内核 | sysctl |
| `nic_egress_shape` | 网卡 | tc egress TBF |
| `nic_ingress_police` | 网卡 | tc ingress police |
| `firewall_new_conn_limit` | 防火墙 | nft new TCP |
| `firewall_udp_limit` | 防火墙 | nft UDP PPS |
| `gateway_new_conn_limit` | 网关 | Envoy local_ratelimit |
| `gateway_conn_limit` | 网关 | Envoy max_connections |

`protections[]` 中 type≡id；`access` 与 protections 同级。

### 网卡域

- params：`device`（空=默认路由口）、`rate`（如 `3mbit`）
- 默认关闭；`tcp-longlived` 等场景可显式开启
- 落地：`relaygate security apply-nic [--verify]`；AfterPull 在内核之后、防火墙之前；主控默认不自动 apply
- 管入向带宽减负，不替代高防

## 场景合并规则

Profile / Panel 场景合并（`MergeSecurityInto`）：

| 场景 | 碰 `access`？ | 碰 `protections`？ |
|------|---------------|-------------------|
| `default-l4` / `tcp-longlived` / `tcp-short-burst` / `udp-heavy` | 否（YAML 不写 `access`） | 是（限速/并发/可选网卡出/入向档位） |
| `strict-allowlist` | 是（示例 allow CIDR） | 是（可同时收紧限速） |
| `host-harden-only` | 是（`enabled: false`） | 是（关防火墙/网关/网卡防护，保留 `kernel_syn`） |

规则：**仅当 profile 显式带 `security.access` 时覆盖名单**；缺省键则保留节点当前 access，避免选「长连接档」冲掉运维维护的 allow/deny。

## API / 状态字段

- Preview / catalog 的 `layer` 与 surfaces：`kernel` / `firewall` / `nic` / `gateway`。
- Preview 内容块 JSON 键：`kernel` / `nic` / `firewall` / `gateway`。
- Diff 字段前缀：`security.protections.<id>…`；access 用 `security.access…` 或 `+deny` / `+allow`。
- 节点 `security-apply-status.json`：顶层键为领域名；含 `nic`（未启用或主控跳过时为 skipped）。

## CLI

| 命令 | 含义 |
|------|------|
| `relaygate security apply-kernel [--verify]` | 落地内核 |
| `relaygate security apply-nic [--verify]` | 落地已启用的网卡出口整形 / 入向 police（tc） |
| `relaygate security kernel-conf` | 渲染内核叠加到 stdout |
| `relaygate firewall apply` | 落地防火墙（含 access + 防火墙域 protections） |
| `relaygate reload` | 落地网关 |

特权 helper：`kernel-harden-apply` → `security apply-kernel`；`nic-shape-apply` → `security apply-nic`。

## 非目标

- 外部 RLS / Redis 全局限速；本机独自抗 volumetric DDoS。
- 对已建立 TCP 做主机 PPS 限速（禁止）。
- 网卡域 XDP / 多口复杂拓扑 — 当前为 tc 单业务口出口整形 + 入向 police；关闭策略不自动清 qdisc/police。

运维手册与策略目录见 [`packaging/security/README.md`](../packaging/security/README.md)、[`threat-analysis.md`](../packaging/security/threat-analysis.md)。
