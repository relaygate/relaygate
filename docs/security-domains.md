# 安全领域（Security domains）

贡献者 / 深度运维说明。**本版起无兼容层**：旧领域/组件混用命名、旧状态字段、旧策略 id 与旧 preview JSON 键一律废弃，以本文与当前代码为准。

产品主文案只用**领域简称**；`sysctl` / `nftables` / `Envoy` 等是**执行组件**，仅出现在脚本、CLI 高级帮助与实现细节。

## 四域与简称

| 全称 | 简称中 | 简称英 | 执行组件（不进产品主文案当模块名） |
|------|--------|--------|--------------------------------------|
| 主机内核加固 | 内核 | Kernel | sysctl 等 |
| 主机防火墙 | 防火墙 | Firewall | nftables |
| 网卡与入口路径 | 网卡 | NIC | ip / tc / XDP 等（**预留，未实现**） |
| 网关转发 | 网关 | Gateway | Envoy |

## 入站路径 vs 落地顺序

```text
入站路径（流量方向）:
  线缆 → 网卡 → 防火墙 → 协议栈/内核行为 → 网关

落地 / 校验顺序（agent 拉取后、运维应用）:
  内核 → 网卡 → 防火墙 → 网关
```

防火墙在网卡**下游**。当前未实现网卡域时：落地顺序为 **内核 → 防火墙 → 网关**，状态里对网卡记 `skipped`（预留位）。

## 产品文案 vs 执行组件

| 场景 | 用语 |
|------|------|
| Panel / 确认框 / 状态 / 预览主文案 | `[内核]` / `[防火墙]` / `[网关]`（及对应英文） |
| CLI 主帮助 | 领域名；子命令 `apply-kernel` / `kernel-conf`；帮助可括注执行细节「内核（sysctl）」 |
| 脚本路径、日志折叠细节、高级区 | 可写 sysctl / nftables / Envoy |

**禁止：** 把 nft / nftables / Envoy /「数据面」/ sysctl 当作产品三/四件套模块名；领域与组件混用并列。

## 与 `security.policies[]` 映射（本版 id / type）

| 策略 id | type | 领域 |
|---------|------|------|
| `kernel_syn` | `kernel` | 内核 |
| `firewall_new_conn_limit` | `new_conn_limit_firewall` | 防火墙 |
| `allowlist` | `allowlist` | 防火墙 |
| `udp_limit` | `udp_limit` | 防火墙 |
| `gateway_new_conn_limit` | `new_conn_limit_gateway` | 网关 |
| `conn_limit` | `conn_limit` | 网关 |

（无策略落在网卡域；预留。）

旧 id（`sysctl_syn` / `nft_new_conn_limit` / `envoy_new_conn_limit`）与旧 type（`sysctl` / `new_conn_limit_nft` / `new_conn_limit_envoy`）**不再识别**，须按本表改写 `resources.yaml` / profiles。

## API / 状态字段（无双读）

- Preview / catalog 的 `layer` 与 surfaces：`kernel` / `firewall` / `nic` / `gateway`。
- Preview 内容块 JSON 键：`kernel` / `firewall` / `gateway`（无 `sysctl` / `nft` / `envoy`）。
- 节点 `security-apply-status.json`：顶层键与 `module` / `failed_at` 同为领域名；含 `nic`（当前恒为 skipped）。
- **不提供**对旧键 `sysctl` / `nftables` / `envoy` 或旧 module 值的读取兼容。

## CLI

| 命令 | 含义 |
|------|------|
| `relaygate security apply-kernel [--verify]` | 落地内核 |
| `relaygate security kernel-conf` | 渲染内核叠加到 stdout |
| `relaygate firewall apply` | 落地防火墙 |
| `relaygate reload` | 落地网关 |

旧子命令 `apply-sysctl` / `sysctl-conf` **已删除**。

## 非目标

- 外部 RLS / Redis 全局限速；本机独自抗 volumetric DDoS。
- 对已建立 TCP 做主机 PPS 限速（禁止）。
- 网卡域能力（XDP/tc 等）— 仅占位，本阶段不实现。
- 旧命名 / 双写 / deprecated 别名 — **不做**。

运维手册与策略目录见 [`packaging/security/README.md`](../packaging/security/README.md)、[`threat-analysis.md`](../packaging/security/threat-analysis.md)。
