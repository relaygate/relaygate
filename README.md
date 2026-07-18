# gateway-01 Envoy 游戏网关

单机 `gateway + panel` 落地：

- **数据面**：Envoy v1.39.0（TCP / UDP 固定目标转发）
- **监控面**：Prometheus + Grafana + node_exporter（仅监听 127.0.0.1）
- **限流**：Envoy TCP 本地连接限速 + nftables UDP PPS / TCP 新建连接限速
- **资产源**：[`config/resources.yaml`](config/resources.yaml)（语义化命名，禁止散落裸 IP）

## 拓扑

```text
玩家
  → gateway-01:10001–10010 (TCP/UDP)
  → Envoy
  → server-01 … server-10

管理面（本机）
  Envoy Admin :9901
  Prometheus  :9090
  Grafana     :3000
  node_exporter :9100
```

命名约定：

| 名称 | 含义 |
|------|------|
| `gateway-01` | 当前网关主机 |
| `server-01`…`server-10` | 游戏后端 |
| `listener-rule-…` | Envoy Listener |
| `cluster-server-NN-tcp/udp-game` | Envoy Cluster |
| `rule-canary-server-01-*` | 旁路测试 11001 |

## 快速开始

### 1. 填资产

编辑 `config/resources.yaml`，将 `server-01`～`server-10` 的 `address` 改为真实 IP。

默认仅启用 **canary** 规则（`11001/TCP+UDP → server-01`），production `10001–10010` 默认关闭。

### 2. 本地/服务器准备

```bash
pip3 install -r requirements.txt
cp .env.example .env
# 编辑 GRAFANA_ADMIN_PASSWORD
chmod 600 .env
```

### 3. 校验与部署

```bash
bash scripts/validate.sh
bash scripts/deploy.sh
bash scripts/collect_baseline.sh
bash scripts/canary_test.sh 127.0.0.1
```

### 4. 访问 Grafana

```bash
ssh -p 30455 -L 3000:127.0.0.1:3000 root@107.149.191.37
# 浏览器打开 http://127.0.0.1:3000
```

### 5. 分批上线

见 [`docs/MIGRATE.md`](docs/MIGRATE.md)。

```bash
python3 scripts/enable_server.py server-01
bash scripts/deploy.sh
```

## 常用命令

| 操作 | 命令 |
|------|------|
| 重新渲染配置 | `python3 scripts/render_config.py` |
| 校验 | `bash scripts/validate.sh` |
| 部署 | `bash scripts/deploy.sh` |
| 回滚 | `bash scripts/rollback.sh` |
| 应用防火墙 | `sudo bash scripts/apply_firewall.sh` |
| Canary 探测 | `bash scripts/canary_test.sh <ip>` |

## 目录结构

```text
config/resources.yaml          # 唯一资产源
scripts/render_config.py       # 渲染 Envoy + nft 端口
envoy/generated/envoy.yaml     # 生成的 Envoy 配置
compose.yaml                   # Envoy + 监控栈
prometheus/                    # 抓取与告警
grafana/provisioning/          # 数据源与仪表盘
firewall/gateway.nft           # 语义化 nftables
sysctl/gateway-01.conf         # 内核参数（安装为 /etc/sysctl.d/99-gateway-01.conf）
docs/BASELINE.md               # 主机基线
docs/MIGRATE.md                # 迁移手册
```

## 安全提醒

1. 勿把 root 密码放进仓库或聊天；尽快改密并改用 SSH 密钥
2. Admin / Prometheus / Grafana / node_exporter **不得**对公网开放
3. 应用 nftables 前务必保留 SSH `30455` 放行，并准备控制台
4. Envoy 不是抗 DDoS 产品；大流量攻击需要云厂商高防

## 已知边界

- 固定目标：无跨服负载均衡；熔断只停止向故障服转发
- UDP 无可靠主动健康检查：靠错误指标与告警
- 后端默认看到网关源 IP；保留玩家源 IP 需后续 TPROXY / PROXY Protocol
- 当前 `gateway-01` 为单点，稳定后再扩 `gateway-02`
