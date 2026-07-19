# RelayGate 多网关高可用（双活 + L4 LB）

本文描述从单机 `gateway-01` 演进为 **active-active 双网关** 的运维模型。配置仍以 Git 中的 `config/resources.yaml` 为单一配置源（GitOps）。

## 目标拓扑

```text
玩家
  → Cloud L4 LB (TCP+UDP, 端口透传, source_ip 会话保持)
      ├─ gateway-01 (Envoy)  ──┐
      └─ gateway-02 (Envoy)  ──┼→ server-01 … server-10
                               │
集中 Prometheus/Grafana ←─────┘（或边缘 remote_write）

管理面（建议）
  Panel 仅在 gateway-01（primary）以 systemd 二进制写入配置
  gateway-02 为 standby 数据面（不启 Panel）
```

## 1. 同一模板部署两台

每台机器使用**同一套** `deploy/compose.yaml` / `scripts/*`，仅 `.env` 不同：

| 主机 | 示例 env | Panel | Grafana |
|------|----------|-------|---------|
| gateway-01 | `deploy/env/gateway-01.env.example` | systemd（`ENABLE_PANEL=1`） | `with-grafana` |
| gateway-02 | `deploy/env/gateway-02.env.example` | 关闭（`ENABLE_PANEL=0`） | 关闭 |

```bash
# 在 gateway-01
cp deploy/env/gateway-01.env.example .env && chmod 600 .env
# 编辑 GATEWAY_PUBLIC_IP / 密码（生产从密钥管理注入）
bash scripts/build.sh
bash scripts/deploy.sh
# Panel（若手动部署）:
sudo bash scripts/install_panel_service.sh
bash scripts/smoke_test.sh

# 在 gateway-02（同步同一 Git 提交）
cp deploy/env/gateway-02.env.example .env && chmod 600 .env
bash scripts/deploy.sh
bash scripts/smoke_test.sh
```

`GATEWAY_NAME` 会驱动：

- 容器名：`${GATEWAY_NAME}-envoy` 等
- Envoy `--service-cluster` / `--service-node`
- Prometheus `external_labels.gateway` 与 scrape `gateway`/`host` 标签
- sysctl 安装路径：`/etc/sysctl.d/99-${GATEWAY_NAME}.conf`

渲染可观测性配置：

```bash
bash scripts/render_observability.sh .env
```

## 2. 云 L4 LB（Terraform）

模板目录：[`deploy/terraform/nlb/`](../deploy/terraform/nlb/)

- AWS Network Load Balancer，**TCP + UDP** 监听游戏端口 `10001–10010` 与 canary `11001`
- Target group **source_ip stickiness**（会话保持）
- 健康检查：HTTP `GET /ready` 到 Envoy admin 口（默认 `9901`），或 TCP 探测同一端口
- **禁止**把 admin/Panel/Grafana 暴露到公网；健康检查仅允许 VPC CIDR

```bash
cd deploy/terraform/nlb
cp terraform.tfvars.example terraform.tfvars   # 填入 VPC/实例，勿提交密钥
terraform init
terraform plan
# 生产 apply 需显式审批
```

无云凭据时：保留上述模板即可；用 `terraform plan` 在有权限的环境验证。

### 健康检查与摘流

| 动作 | 命令 | 效果 |
|------|------|------|
| 探针 | `curl -fsS http://127.0.0.1:9901/ready` | 期望 `LIVE` |
| Drain | `bash scripts/drain_gateway.sh fail` | `/healthcheck/fail`，LB 摘流 |
| Undrain | `bash scripts/drain_gateway.sh ok` | 恢复 `/ready` |

Compose 内 Docker healthcheck 同样探测 `/ready`。

### 游戏服回源放行（必做）

后端看到的源 IP 是**发起转发的网关 IP**。双活后必须放行 **两台** 网关：

```bash
# Linux nftables 示例
GATEWAY_IPS="{ 107.149.191.37, 203.0.113.20 }"
TCP_PORT=7777
UDP_PORT=7778
nft insert rule inet game_server input ip saddr $GATEWAY_IPS tcp dport $TCP_PORT accept
nft insert rule inet game_server input ip saddr $GATEWAY_IPS udp dport $UDP_PORT accept
```

云安全组：入站源改为两台网关的公网或私网 `/32`，不要 `0.0.0.0/0`。

## 3. GitOps：一份配置 → CI → 分批部署

```text
Git (config/resources.yaml)
  → CI: gofmt / vet / test / relaygate render / compose config / trivy
  → 构建镜像（tag = git SHA，禁止 latest）
  → matrix 分批：gateway-01 → gateway-02
       每台：drain → sync → reload_envoy → undrain → smoke
```

工作流：[`.github/workflows/ci.yml`](../.github/workflows/ci.yml)

运维机手工分批：

```bash
cp deploy/inventory/gateways.env.example deploy/inventory/gateways.env
# 填写 HOST_gateway_01 / HOST_gateway_02
GATEWAYS=gateway-01,gateway-02 bash scripts/deploy_multi.sh
```

### Panel 在多节点下的定位

| 角色 | 建议 | 说明 |
|------|------|------|
| **primary**（gateway-01） | 启 Panel（`ENABLE_PANEL=1` → systemd `relaygate-panel`） | 唯一写入 `resources.yaml` 的入口；Apply 后走 Git 提交或同步脚本 |
| **standby**（gateway-02） | **不启 Panel**（`ENABLE_PANEL=0`） | 只跑 Envoy；配置靠 GitOps 下发，避免双写冲突 |
| 只读从节点（可选） | Panel 只读 | 若未来加只读模式，可挂从节点；当前未实现时勿开第二个写 Panel |

**`PANEL_ROLE` 仅为运维约定**：`.env` 中的 `primary` / `standby` 供人与脚本识别角色；**Panel 进程不读取该变量**，也不据此拒绝写 API。防双写靠「standby 不启 Panel」，而不是进程内开关。Compose 中已无 `with-panel` profile。

**冲突风险**：若两台都开 Panel 并同时改 Server，会分叉本地 `resources.yaml`。请只在 primary 修改，再经 Git 推到各节点。

## 4. 集中监控

两种形态（可并存）：

1. **集中 scrape**：[`deploy/monitoring/`](../deploy/monitoring/) 抓取两台 `:9901` / `:9100`（内网或隧道）
2. **remote_write**：各网关 `.env` 设置 `PROMETHEUS_REMOTE_WRITE_URL`

所有指标带唯一 `gateway` 标签。Grafana dashboard：`RelayGate Multi-Gateway Overview`（变量 `$gateway`）。

告警规则：`deploy/prometheus/rules/gateway-alerts.yml`（含 `GatewayPartialOutage`）。

## 5. 部署验证 / 冒烟

单台：

```bash
bash scripts/smoke_test.sh 127.0.0.1
bash scripts/canary_test.sh <LB_DNS_OR_IP>
curl -fsS http://127.0.0.1:9901/ready
docker ps --format '{{.Names}}' | grep "${GATEWAY_NAME}-envoy"
```

双活：

```bash
# 分别在两台
bash scripts/drain_gateway.sh status
# 经 LB 测 canary
bash scripts/canary_test.sh <nlb-dns>
```

## 6. 回滚流程（逐台）

**原则：一次只回滚一台，另一台继续承接 LB 流量。**

```bash
# --- 在故障/待回滚网关 ---
bash scripts/drain_gateway.sh fail          # 1. 摘流
bash scripts/rollback.sh                    # 2. 恢复 backups/latest 并 recreate envoy
bash scripts/smoke_test.sh                  # 3. 冒烟
bash scripts/drain_gateway.sh ok            # 4. 重新接入

# --- 再处理另一台（如需）---
# 重复上述步骤
```

指定备份戳：

```bash
bash scripts/rollback.sh 20260719-153000
```

验证命令：

```bash
curl -fsS http://127.0.0.1:9901/ready
curl -fsS http://127.0.0.1:9090/api/v1/query?query=up
docker compose -f deploy/compose.yaml --env-file .env ps
```

## 7. 密钥与安全

- `.env` / `terraform.tfvars` / SSH 私钥 **不入库**；CI 使用 GitHub Environments + Secrets / OIDC
- 镜像 tag 使用 **git SHA**，禁止生产 `latest`
- Admin `9901` 仅本机或 VPC；公网只暴露游戏端口（经 NLB）
