# RelayGate

基于 Envoy 的游戏网关产品。支持单机起步，并演进为 **双活网关 + 云 L4 LB**（见 [`docs/HA.md`](docs/HA.md)）。Go 工具链：

- **数据面**：Envoy v1.39.0（TCP / UDP 固定目标转发）
- **产品二进制**：`relaygate`（渲染、规则切换与 Panel，Panel 监听 `127.0.0.1:8080`）
- **监控面**：Prometheus + Grafana + node_exporter（仅监听 127.0.0.1；可接集中监控）
- **限流**：Envoy TCP 本地连接限速 + nftables UDP PPS / TCP 新建连接限速
- **资产源**：[`config/resources.yaml`](config/resources.yaml)（GitOps 单一配置源）
- **实例身份**：由 `.env` 中 `GATEWAY_NAME` / `GATEWAY_PUBLIC_IP` 驱动（同一套 compose 模板可部署多台）

## 拓扑

```text
玩家
  → [可选] Cloud L4 LB (TCP+UDP)
  → gateway-01 / gateway-02:10001–10010 (TCP/UDP)
  → Envoy
  → server-01 … server-10

管理面（本机，建议仅 primary 开 Panel）
  Panel         :8080
  Envoy Admin   :9901
  Prometheus    :9090
  Grafana       :3000
  node_exporter  :9100
```

## 目录

```text
config/resources.yaml          # 唯一资产源
cmd/relaygate                  # 唯一入口：render / server / panel / version
internal/                      # Go 业务包
web/                           # Panel 模板与静态资源
gateway/generated/envoy.yaml   # 生成的 Envoy 配置
deploy/                        # compose + 监控 + 防火墙 + sysctl
scripts/                       # Shell 编排
docs/
panel/README.md                # Panel 说明
```

命名约定：

| 名称 | 含义 |
|------|------|
| `gateway-01` / `gateway-02` | 网关主机（由 `GATEWAY_NAME` 指定） |
| `server-01`…`server-10` | 游戏后端 |
| `listener-rule-…` | Envoy Listener |
| `cluster-server-NN-tcp/udp-game` | Envoy Cluster |
| `rule-canary-server-01-*` | 旁路测试 11001 |

## Server 端配置（游戏后端）

Server 指真正跑游戏逻辑的机器（`server-01` … `server-10`）。网关只做转发，**游戏进程本身仍安装在 Server 上**；客户端不再直连 Server，而是连 `gateway-01` 的入口端口。

默认约定（可在 `config/resources.yaml` 修改）：

| 项 | 默认值 | 说明 |
|----|--------|------|
| 游戏 TCP | `7777` | Server 本机监听 |
| 游戏 UDP | `7778` | Server 本机监听 |
| 健康检查 | TCP `7777` | 网关对 Server 做 TCP connect 探测 |
| 网关公网 IP | `107.149.191.37` | 下称 `GATEWAY_IP`，请改成你的真实值 |
| 玩家入口（示例 server-01） | `GATEWAY_IP:10001` TCP/UDP | canary 为 `11001` |

流量关系：

```text
玩家 → gateway-01:10001 (TCP/UDP)
     → Envoy
     → server-01:7777 (TCP) / :7778 (UDP)
```

配置原则：

1. 游戏进程监听 `0.0.0.0` 或内网网卡（不要只绑 `127.0.0.1`，否则网关连不上）。
2. **防火墙只放行来自网关回源 IP 的游戏端口**，禁止公网直连 Server。双活时必须放行 **所有** 网关 IP（见 [`docs/HA.md`](docs/HA.md)）。
3. 游戏日志里看到的客户端源 IP 通常是网关 IP（当前未开 TPROXY / PROXY Protocol）。
4. 在网关的 `config/resources.yaml` 填入该 Server 的真实 IP，并执行 `./bin/relaygate render` 后 Apply/部署（仅在 primary Panel / Git 写入，避免双写）。

---

### Linux Server

以 `server-01`、TCP `7777`、UDP `7778`、网关 `107.149.191.37` 为例。

#### 1. 确认游戏进程监听

```bash
ss -lntup | grep -E '7777|7778'
# 期望看到 0.0.0.0:7777 / 0.0.0.0:7778（或对应内网 IP），而不是 127.0.0.1
```

若游戏配置支持绑定地址，请设为 `0.0.0.0` 或内网 IP。

#### 2. 防火墙：仅允许网关访问游戏端口

**nftables（推荐）**

```bash
# 按需安装：Debian/Ubuntu
# apt-get install -y nftables

GATEWAY_IP=107.149.191.37
TCP_PORT=7777
UDP_PORT=7778

nft add table inet game_server 2>/dev/null || true
nft add chain inet game_server input '{ type filter hook input priority 0; policy accept; }' 2>/dev/null || true

# 先删同名旧规则（若存在）可忽略报错
nft insert rule inet game_server input ip saddr $GATEWAY_IP tcp dport $TCP_PORT accept comment \"allow-gateway-tcp\"
nft insert rule inet game_server input ip saddr $GATEWAY_IP udp dport $UDP_PORT accept comment \"allow-gateway-udp\"
nft insert rule inet game_server input tcp dport $TCP_PORT drop comment \"deny-public-tcp\"
nft insert rule inet game_server input udp dport $UDP_PORT drop comment \"deny-public-udp\"

# 持久化（发行版不同，任选其一）
# Debian/Ubuntu: nft list ruleset > /etc/nftables.conf && systemctl enable --now nftables
# 或放入 /etc/nftables.d/game-server.nft 后 include
```

**firewalld**

```bash
GATEWAY_IP=107.149.191.37
firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=${GATEWAY_IP} port port=7777 protocol=tcp accept"
firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=${GATEWAY_IP} port port=7778 protocol=udp accept"
# 若端口已对 public 开放，请移除公网放行，避免绕过网关
firewall-cmd --permanent --remove-port=7777/tcp 2>/dev/null || true
firewall-cmd --permanent --remove-port=7778/udp 2>/dev/null || true
firewall-cmd --reload
```

**ufw**

```bash
GATEWAY_IP=107.149.191.37
ufw allow from ${GATEWAY_IP} to any port 7777 proto tcp
ufw allow from ${GATEWAY_IP} to any port 7778 proto udp
ufw deny 7777/tcp
ufw deny 7778/udp
ufw reload
```

运维 SSH、内网管理端口请单独放行，不要被上面的 deny 误伤。

#### 3. 从网关侧连通性自检

在 **gateway-01** 上执行：

```bash
# TCP
nc -vz <server-01-ip> 7777
# UDP（仅能确认发出，需游戏或抓包验证回包）
echo test | nc -u -w2 <server-01-ip> 7778
```

#### 4. 云厂商安全组（若有）

入站规则示例：

- 协议 TCP，端口 `7777`，源 = `GATEWAY_IP/32`
- 协议 UDP，端口 `7778`，源 = `GATEWAY_IP/32`
- 不要对 `0.0.0.0/0` 开放游戏端口

---

### Windows Server

同样以 TCP `7777`、UDP `7778`、网关 `107.149.191.37` 为例。建议用 **管理员 PowerShell**。

#### 1. 确认游戏进程监听

```powershell
netstat -ano | findstr "7777 7778"
# 或
Get-NetTCPConnection -LocalPort 7777 -ErrorAction SilentlyContinue
Get-NetUDPEndpoint -LocalPort 7778 -ErrorAction SilentlyContinue
```

确认监听地址不是仅 `127.0.0.1`。若游戏有配置文件，将 Bind/Listen 设为 `0.0.0.0` 或主机内网 IP。

#### 2. Windows 防火墙：仅允许网关

```powershell
$GatewayIP = "107.149.191.37"
$TcpPort   = 7777
$UdpPort   = 7778

# 允许网关 → 游戏端口
New-NetFirewallRule -DisplayName "Allow-Gateway-Game-TCP" `
  -Direction Inbound -Action Allow -Protocol TCP -LocalPort $TcpPort `
  -RemoteAddress $GatewayIP -Profile Any

New-NetFirewallRule -DisplayName "Allow-Gateway-Game-UDP" `
  -Direction Inbound -Action Allow -Protocol UDP -LocalPort $UdpPort `
  -RemoteAddress $GatewayIP -Profile Any

# 阻断其它来源访问游戏端口（规则名可自定义）
New-NetFirewallRule -DisplayName "Deny-Public-Game-TCP" `
  -Direction Inbound -Action Block -Protocol TCP -LocalPort $TcpPort `
  -RemoteAddress Any -Profile Any

New-NetFirewallRule -DisplayName "Deny-Public-Game-UDP" `
  -Direction Inbound -Action Block -Protocol UDP -LocalPort $UdpPort `
  -RemoteAddress Any -Profile Any
```

说明：Windows 防火墙按规则优先级与匹配顺序生效；若已有更宽的「允许任何入站」规则盖住游戏端口，请先禁用或收紧那些规则。可用：

```powershell
Get-NetFirewallRule -DisplayName "*Game*" | Format-Table DisplayName, Direction, Action, Enabled
```

删除示例规则：

```powershell
Remove-NetFirewallRule -DisplayName "Allow-Gateway-Game-TCP","Allow-Gateway-Game-UDP","Deny-Public-Game-TCP","Deny-Public-Game-UDP" -ErrorAction SilentlyContinue
```

#### 3. 从网关侧连通性自检

在 **gateway-01（Linux）** 上：

```bash
nc -vz <windows-server-ip> 7777
```

在 **另一台 Windows** 上（可选）：

```powershell
Test-NetConnection -ComputerName <windows-server-ip> -Port 7777
```

#### 4. 云厂商安全组 / Windows 云主机

与 Linux 相同：安全组入站仅允许 `GATEWAY_IP` 访问 `7777/tcp`、`7778/udp`，不要对全网开放。

---

### 网关侧登记 Server

在仓库 `config/resources.yaml` 中填写（或用 Panel → Servers 保存）：

```yaml
servers:
  - name: server-01
    address: 203.0.113.10   # 该 Server 真实 IP（Windows 或 Linux）
    tcp_port: 7777
    udp_port: 7778
    health_check_port: 7777
    enabled: true
```

然后：

```bash
./bin/relaygate render
# 或 Panel → Apply
bash scripts/deploy.sh   # 或 bash scripts/reload_envoy.sh
```

玩家 / 客户端应连接：

| 用途 | 地址 |
|------|------|
| 生产 server-01 | `GATEWAY_IP:10001`（TCP + UDP，按游戏需要） |
| Canary 验证 | `GATEWAY_IP:11001` |
| 不要再直连 | `server-01:7777/7778` |

## 快速开始（单台网关）

### 1. 填资产

编辑 `config/resources.yaml`，将 `server-01`～`server-10` 的 `address` 改为真实 IP（先完成上一节 Server 端放行）。

默认仅启用 **canary**（`11001/TCP+UDP → server-01`），production `10001–10010` 默认关闭。

### 2. 构建与准备

```bash
# 需要 Go 1.22+
bash scripts/build.sh
# Windows: powershell -File scripts/validate.ps1

cp .env.example .env
# 或双活：cp deploy/env/gateway-01.env.example .env
# 编辑 GATEWAY_NAME、GRAFANA_ADMIN_PASSWORD、PANEL_ADMIN_PASSWORD
chmod 600 .env
```

### 3. 校验与部署

```bash
bash scripts/validate.sh
bash scripts/deploy.sh
bash scripts/smoke_test.sh
bash scripts/collect_baseline.sh
bash scripts/canary_test.sh 127.0.0.1
```

Compose 约定（仓库根）：

```bash
docker compose -f deploy/compose.yaml --env-file .env up -d --build
```

多网关高可用、L4 LB、GitOps 分批与回滚：见 [`docs/HA.md`](docs/HA.md)。

### 4. 访问 Panel / Grafana

```bash
ssh -p 30455 -L 8080:127.0.0.1:8080 -L 3000:127.0.0.1:3000 root@107.149.191.37
# Panel   http://127.0.0.1:8080
# Grafana http://127.0.0.1:3000
```

### 5. 分批上线

见 [`docs/MIGRATE.md`](docs/MIGRATE.md)。

```bash
./bin/relaygate server enable server-01
bash scripts/deploy.sh
```

## 常用命令

| 操作 | 命令 |
|------|------|
| 构建二进制 | `bash scripts/build.sh` |
| 重新渲染 | `./bin/relaygate render` |
| 仅校验 | `./bin/relaygate render --check-only` |
| 启用 production | `./bin/relaygate server enable server-01` |
| 启动 Panel | `PANEL_ADMIN_PASSWORD=... ./bin/relaygate panel` |
| 查看版本 | `./bin/relaygate version` |
| 部署 | `bash scripts/deploy.sh` |
| 冒烟 | `bash scripts/smoke_test.sh` |
| Drain / Undrain | `bash scripts/drain_gateway.sh fail\|ok` |
| 分批双活部署 | `bash scripts/deploy_multi.sh` |
| 回滚 | `bash scripts/rollback.sh` |
| 应用防火墙 | `sudo bash scripts/apply_firewall.sh` |
| Canary 探测 | `bash scripts/canary_test.sh <ip>` |

## 安全提醒

1. 勿把 root / Panel / Grafana 密码放进仓库
2. Admin / Panel / Prometheus / Grafana / node_exporter **不得**对公网开放
3. 应用 nftables 前务必保留 SSH `30455` 放行
4. Envoy 不是抗 DDoS 产品；大流量攻击需要云厂商高防

## 已知边界

- 固定目标：无跨服负载均衡；熔断只停止向故障服转发
- UDP 无可靠主动健康检查：靠错误指标与告警
- 后端默认看到网关源 IP；保留玩家源 IP 需后续 TPROXY / PROXY Protocol
- 双活依赖云 L4 LB + 两台网关回源放行；`PANEL_ROLE` 仅为运维约定（Panel 不读），standby 通过不启 `with-panel` 防双写（详见 [`docs/HA.md`](docs/HA.md)）
