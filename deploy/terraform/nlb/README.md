# AWS NLB for RelayGate (TCP+UDP active-active)

See [`docs/HA.md`](../../../docs/HA.md) for architecture, health checks, rollback, and backend allowlists.

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
- Drain a gateway before maintenance: `bash scripts/drain_gateway.sh fail` so `/ready` fails and NLB removes the target.
- Game servers must allowlist **both** gateway source IPs.
