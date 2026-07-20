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

| RelayGate | NLB / ops |
|-----------|-----------|
| `DRAIN_WAIT`（`.env`） | 建议 ≥ `unhealthy_threshold × interval` |
| `relaygate drain fail` | POST `/healthcheck/fail` → 等窗口 → 控制台确认 target unhealthy |
| `relaygate reload` | 内置 drain → restart → poll `/ready` → undrain |
| `relaygate doctor` | 打印 DRAIN_WAIT、/ready、双活角色与高防回源清单 |

产品**不接**云 SDK：摘流确认请在控制台或现有 Terraform/CLI 完成。

```bash
relaygate doctor
relaygate drain fail    # 提示 NLB 核对项
# …变更 / reload…
relaygate drain ok
relaygate smoke
```
