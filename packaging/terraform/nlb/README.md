# AWS NLB for RelayGate (TCP+UDP active-active)

Overview, HA topology, drain, and upstream allowlists: root [`README.md`](../../../README.md)（「双活」「上游放行」「L4 维护窗口」）.

## Quick start

```bash
cp terraform.tfvars.example terraform.tfvars
# fill vpc_id, subnets, gateway instance IDs — never commit secrets/credentials
terraform init
terraform plan
# terraform apply  # requires explicit approval in production
```

AWS credentials: environment variables, shared config, or CI OIDC. Do **not** put keys in this directory.

## Notes

- UDP target groups use TCP health checks on Envoy admin/`/ready` port (AWS requirement).
- Drain a gateway before maintenance: `relaygate drain fail` so `/ready` fails and NLB removes the target.
- Upstream services must allowlist **both** gateway source IPs.

## Client IP（真实客户端 IP）

**产品主路径是公网直连网关端口**（`PROXY_PROTOCOL=off`）。本 NLB 模板是**可选**前置 LB；默认 **`enable_proxy_protocol_v2 = false`**，与网关默认一致。

| 路径 | 配置 |
|------|------|
| **直连暴露**（无本 NLB / 客户端直连网关） | 网关 `PROXY_PROTOCOL=off`；勿开 TG PROXY |
| NLB + **preserve client IP**、不发 PROXY | 两边 PROXY 保持 **off** |
| NLB + **PROXY v2**（关 preserve / PrivateLink 等） | TF `enable_proxy_protocol_v2=true` **且** 网关 `PROXY_PROTOCOL=v2`；转发口 **仅对 LB** |
| 迁移混跑 | 网关 `PROXY_PROTOCOL=v2-compat`（或 `v2`+`ALLOW_WITHOUT=1`）；**入口仍必须只信 LB，公网禁止 compat** |

```hcl
# terraform.tfvars — 仅当 NLB 对目标发 PROXY v2 时
# enable_proxy_protocol_v2 = true
```

```bash
# 网关 .env（直连默认）
PROXY_PROTOCOL=off
# 与上 NLB PROXY 同步时：
# PROXY_PROTOCOL=v2
```

「多个上游」由 RelayGate 转发实现，**不**表示需要 PROXY。安全与 LogQL：见 [`docs/logging-playbook.md`](../../../docs/logging-playbook.md)。

## Drain ↔ NLB timing

本模板 `health_check`：`unhealthy_threshold = 3`，`interval = 10` → **30s**。

| RelayGate | NLB / ops |
|-----------|-----------|
| `DRAIN_WAIT`（`.env`，默认 **30**） | 建议 ≥ `unhealthy_threshold × interval`（本模板 = 30） |
| `relaygate drain fail` | POST `/healthcheck/fail` → 等窗口 → 控制台确认 target unhealthy |
| `relaygate reload` | resources/Envoy：drain → restart → poll `/ready` → undrain |
| `relaygate upgrade --drain` | 二进制/packaging：drain → `install.sh --upgrade` → undrain |
| `relaygate diag` | `DRAIN_WAIT` 过短且有双活/NLB 迹象时硬失败 |

产品**不接**云 SDK：摘流确认请在控制台或现有 Terraform/CLI 完成。

```bash
relaygate diag
relaygate drain fail    # 提示 NLB 核对项；过短 DRAIN_WAIT 会 WARN
# …变更 / reload / upgrade…
relaygate drain ok
relaygate smoke
```
