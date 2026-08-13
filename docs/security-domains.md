# 安全领域（Security domains）

贡献者 / 深度运维说明。**本版起无兼容层**：旧领域/组件混用命名、旧状态字段、旧策略 id、旧 `security.policies[]` 键与旧 preview JSON 键一律废弃，以本文与当前代码为准。

产品主文案只用**领域简称**；`sysctl` / `nftables` / `Envoy` 等是**执行组件**，仅出现在脚本、CLI 高级帮助与实现细节。

## 结构决策（2026-08）

### 问题

1. **准入（allowlist）与限速/加固正交**：行业上源 ACL 属访问控制；新建连接限速、UDP PPS、SYN 加固、网关 `max_connections` 属容量/滥用防护。场景模板几乎只调防护档位，名单改动少且独立。
2. **命名债**：`allowlist` / `udp_limit` 缺 `firewall_` 前缀；`conn_limit` 实为网关 circuit breaker，易被误读成主机 conntrack。
3. **type 与 id 不一致**：曾用 `new_conn_limit_firewall` 等「机制_领域」倒序 type，与领域前缀 id 对不齐。

### 备选

| 方案 | 做法 | 利 | 弊 |
|------|------|----|----|
| A（采用） | `security.access` 与 `security.protections[]` 同级；防护 id 领域前缀；type≡id | ACL 与防护语义清晰；场景合并默认可不碰名单；与落地顺序（ACL→限速）一致 | YAML/API/UI 结构破坏面大 |
| B | 仍放单一 `policies[]`，仅改名为 `firewall_allowlist` 等 | 改动面小、一套 catalog | 继续把准入与限速塞进同一袋；场景易误覆盖名单 |
| C | `access` 同级但保留键名 `policies` | 少一词迁移 | 「policies」是否含 access 含糊；与「防护」产品语言不对齐 |

### 主方案（A）

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

- **不**把 access 留在 protections 列表（即使加 `firewall_` 前缀也不够：生命周期与合并规则不同）。
- **不**做 `policies` 别名或旧 id 双读。
- **params 约定**：产品契约仍用真实键（如 `tcp_per_ip`）；Go 内置策略用 typed 字段 + tag。未知键进 `PolicyParams.Extra`，Load→Save / API 往返保留；内置 `Effective*` / 适配器只读 typed 字段，Extra 不参与生效计算（留给自定义 type→适配器）。`security.access` 保持强类型，不改为 map。

### 节点身份：`GATEWAY_NAME` 与 `gateway.name`

节点身份以安装环境变量 **`GATEWAY_NAME`** 为准；本机 `resources.yaml` 可选写入 **`gateway.name`**（拉取后由 agent 盖章）。**已删除** `meta.gateway_name`（无双读）；旧 YAML 中该键被忽略。机群发布剥离 `gateway.name` / `public_ip`。升级后请重新发布机群包，节点用本机 env 承接身份。

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

## 命名表（新旧对照）

| 旧 id / 位置 | 新 | type（≡id） | 领域 | 执行面 |
|--------------|-----|-------------|------|--------|
| `security.policies[]` | `security.protections[]` | — | — | — |
| `allowlist`（policies 成员） | `security.access`（`enabled`/`deny`/`allow`） | — | 防火墙 | nft ACL |
| `kernel_syn` / type `kernel` | `kernel_syn` | `kernel_syn` | 内核 | sysctl |
| （无） | `nic_egress_shape` | `nic_egress_shape` | 网卡 | tc egress TBF |
| （无） | `nic_ingress_police` | `nic_ingress_police` | 网卡 | tc ingress police |
| `firewall_new_conn_limit` / type `new_conn_limit_firewall` | `firewall_new_conn_limit` | `firewall_new_conn_limit` | 防火墙 | nft new TCP |
| `udp_limit` | `firewall_udp_limit` | `firewall_udp_limit` | 防火墙 | nft UDP PPS |
| `gateway_new_conn_limit` / type `new_conn_limit_gateway` | `gateway_new_conn_limit` | `gateway_new_conn_limit` | 网关 | Envoy local_ratelimit |
| `conn_limit` | `gateway_conn_limit` | `gateway_conn_limit` | 网关 | Envoy max_connections |

旧 id（含 `allowlist`、`conn_limit`、`udp_limit`、`sysctl_syn`、`nft_*`、`envoy_*`）与旧 type、旧键 `security.policies` **不再识别**。

### 网卡域模型

采用与内核/防火墙/网关一致的 **`security.protections[]` + `nic_` 前缀**（type≡id），**不**另开 `security.nic` 顶层块：

- 策略 id：`nic_egress_shape`（出口整形）、`nic_ingress_police`（入向 police）
- params 产品键：`device`（业务口名；空=探测默认路由口）、`rate`（如 `3mbit`）
- 默认：**关闭**（`DefaultSecurity`）；场景如 `tcp-longlived` 可显式开启并对齐主机约 3 Mbps（出/入向）
- 落地：`relaygate security apply-nic [--verify]`（同时 apply 已启用的 `nic_*`）；节点 AfterPull 在内核之后、防火墙之前；**主控 `PANEL_ENABLED=1` 默认不自动 apply**（同 `SECURITY_AUTO_APPLY`）
- 边界：管入向**带宽**减负；大包洪水主防仍在高防；不替代云清洗

未采用独立 `security.nic` 块的原因：合并/ catalog / Panel JSON 参数编辑已与 protections 对齐；口对象用 `device` 表达即可，后续若要多口可扩列表而不破坏 type≡id。

## 场景合并规则

Profile / Panel 场景合并（`MergeSecurityInto`）：

| 场景 | 碰 `access`？ | 碰 `protections`？ |
|------|---------------|-------------------|
| `default-l4` / `tcp-longlived` / `tcp-short-burst` / `udp-heavy` | 否（YAML 不写 `access`） | 是（限速/并发/可选网卡出/入向档位） |
| `strict-allowlist` | 是（示例 allow CIDR） | 是（可同时收紧限速） |
| `host-harden-only` | 是（`enabled: false`） | 是（关防火墙/网关/网卡防护，保留 `kernel_syn`） |

规则：**仅当 profile 显式带 `security.access` 时覆盖名单**；缺省键则保留节点当前 access，避免选「长连接档」冲掉运维维护的 allow/deny。

## API / 状态字段（无双读）

- Preview / catalog 的 `layer` 与 surfaces：`kernel` / `firewall` / `nic` / `gateway`。
- Preview 内容块 JSON 键：`kernel` / `nic` / `firewall` / `gateway`（无 `sysctl` / `nft` / `envoy` / `tc` 作顶层键）。
- Diff 字段前缀：`security.protections.<id>…`；access 变更用 `security.access…` 或 `+deny` / `+allow` 摘要。
- 节点 `security-apply-status.json`：顶层键与 `module` / `failed_at` 同为领域名；含 `nic`（未启用或主控跳过时为 skipped）。
- **不提供**对旧键 `policies` / `sysctl` / `nftables` / `envoy` 的读取兼容。

## CLI

| 命令 | 含义 |
|------|------|
| `relaygate security apply-kernel [--verify]` | 落地内核 |
| `relaygate security apply-nic [--verify]` | 落地已启用的网卡出口整形 / 入向 police（tc） |
| `relaygate security kernel-conf` | 渲染内核叠加到 stdout |
| `relaygate firewall apply` | 落地防火墙（含 access + 防火墙域 protections） |
| `relaygate reload` | 落地网关 |

特权 helper（`packaging/systemd/relaygate-apply`）对应动作：`kernel-harden-apply` → `security apply-kernel`；`nic-shape-apply` → `security apply-nic`。

旧子命令与旧 id **已删除**。

## 破坏面清单

| 面 | 影响 |
|----|------|
| YAML | `resources.yaml`、`resources.example.yaml`、`packaging/profiles/*.yaml` |
| Go | `core/resources`、`profile`、`dataplane`、`panel`、`render`、相关测试 |
| Panel API | 安全保存 / preview / allowlist 投影字段路径 |
| UI | `securityPolicies.ts`、Security 页、i18n policy keys |
| Docs | 本文、`packaging/security/*`、CHANGELOG |
| CLI 文案 | 若提及旧 id / `policies` |

## 非目标

- 外部 RLS / Redis 全局限速；本机独自抗 volumetric DDoS。
- 对已建立 TCP 做主机 PPS 限速（禁止）。
- 网卡域 XDP / 多口复杂拓扑 — 当前为 tc 单业务口出口整形 + 入向 police；关闭策略不自动清 qdisc/police。
- 旧命名 / 双写 / deprecated 别名 — **不做**。

运维手册与策略目录见 [`packaging/security/README.md`](../packaging/security/README.md)、[`threat-analysis.md`](../packaging/security/threat-analysis.md)。
