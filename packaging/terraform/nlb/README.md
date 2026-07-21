# AWS NLB for RelayGate (TCP+UDP active-active)

Overview, HA topology, drain, and backend allowlists: root [`README.md`](../../../README.md)（「双活」「游戏后端放行」「L4 维护窗口」）.

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
- Game servers must allowlist **both** gateway source IPs.

## Drain ↔ NLB timing

本模板 `health_check`：`unhealthy_threshold = 3`，`interval = 10` → **30s**。

| RelayGate | NLB / ops |
|-----------|-----------|
| `DRAIN_WAIT`（`.env`，默认 **30**） | 建议 ≥ `unhealthy_threshold × interval`（本模板 = 30） |
| `relaygate drain fail` | POST `/healthcheck/fail` → 等窗口 → 控制台确认 target unhealthy |
| `relaygate reload` | resources/Envoy：drain → restart → poll `/ready` → undrain |
| `relaygate upgrade --drain` | 二进制/packaging：drain → `install.sh --upgrade` → undrain |
| `relaygate doctor` | `DRAIN_WAIT` 过短且有双活/NLB 迹象时硬失败 |

产品**不接**云 SDK：摘流确认请在控制台或现有 Terraform/CLI 完成。

```bash
relaygate doctor
relaygate drain fail    # 提示 NLB 核对项；过短 DRAIN_WAIT 会 WARN
# …变更 / reload / upgrade…
relaygate drain ok
relaygate smoke
```
