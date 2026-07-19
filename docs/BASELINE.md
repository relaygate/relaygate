# RelayGate 系统基线

> 首次部署前在目标主机上只读采集并回填本文件。  
> 产品：RelayGate；主机语义名：`gateway-01`（gateway + observability panel）

## 1. 主机身份

| 项 | 值 |
|----|-----|
| 语义名 | `gateway-01` |
| 公网 IP | `107.149.191.37` |
| SSH 端口 | `30455` |
| 角色 | gateway + panel（Envoy + Prometheus + Grafana） |
| 采集时间 | _待填写_ |
| 采集人 | _待填写_ |

## 2. 系统与硬件（部署前采集）

在主机上执行：

```bash
uname -a
cat /etc/os-release
nproc
free -h
df -h /
ip -br addr
ip route
ss -lntup
systemctl is-active docker || true
docker --version || true
sysctl net.core.somaxconn net.ipv4.ip_local_port_range net.core.rmem_max net.core.wmem_max
nft list ruleset 2>/dev/null || iptables -S 2>/dev/null || true
```

| 项 | 值 |
|----|-----|
| OS | _待填写_ |
| Kernel | _待填写_ |
| CPU 核数 | _待填写_ |
| 内存 | _待填写_ |
| 根分区可用 | _待填写_ |
| 主网卡 | _待填写_ |
| Docker | _待填写_ |
| 现有防火墙 | _待填写_ |

## 3. 端口规划

| 用途 | 协议 | 端口 | 绑定 | 备注 |
|------|------|------|------|------|
| SSH | TCP | 30455 | 0.0.0.0 | 现有运维入口，部署防火墙时必须保留 |
| Envoy Admin | TCP | 9901 | 127.0.0.1 | 仅本机，供 Prometheus 抓取 |
| Prometheus | TCP | 9090 | 127.0.0.1 | 管理面 |
| Grafana | TCP | 3000 | 127.0.0.1 | 首期 SSH 隧道访问 |
| node_exporter | TCP | 9100 | 127.0.0.1 | 主机指标 |
| 游戏入口 TCP | TCP | 10001–10010 | 0.0.0.0 | 对应 server-01～server-10 |
| 游戏入口 UDP | UDP | 10001–10010 | 0.0.0.0 | 与 TCP 同号，协议独立 |
| 旁路测试 TCP | TCP | 11001 | 0.0.0.0 | 仅 canary，验证后再切生产 |
| 旁路测试 UDP | UDP | 11001 | 0.0.0.0 | 仅 canary |

## 4. 后端游戏服资产

| 语义名 | 后端 IP | TCP 端口 | UDP 端口 | 健康检查 | 状态 |
|--------|---------|----------|----------|----------|------|
| server-01 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-02 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-03 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-04 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-05 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-06 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-07 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-08 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-09 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |
| server-10 | _待填写_ | 7777 | 7778 | TCP connect :7777 | placeholder |

说明：

- 真实 IP 只写在 [`config/resources.yaml`](../config/resources.yaml)，本表用于沟通与盘点。
- 默认假设后端游戏端口为 `7777/TCP`、`7778/UDP`；若实际不同，先改 `resources.yaml` 再重新渲染。
- UDP 无独立健康端口时不做伪造 UDP 健康检查，只依赖转发错误指标与告警。

## 5. 入口 → 后端映射（固定目标）

| 规则名 | 入口 | 集群 | 目标 |
|--------|------|------|------|
| rule-server-01-tcp-game | `gateway-01:10001/TCP` | `cluster-server-01-tcp-game` | `server-01:7777` |
| rule-server-01-udp-game | `gateway-01:10001/UDP` | `cluster-server-01-udp-game` | `server-01:7778` |
| rule-server-02-tcp-game | `gateway-01:10002/TCP` | `cluster-server-02-tcp-game` | `server-02:7777` |
| … | … | … | … |
| rule-server-10-udp-game | `gateway-01:10010/UDP` | `cluster-server-10-udp-game` | `server-10:7778` |
| rule-canary-server-01-tcp | `gateway-01:11001/TCP` | `cluster-server-01-tcp-game` | `server-01:7777` |
| rule-canary-server-01-udp | `gateway-01:11001/UDP` | `cluster-server-01-udp-game` | `server-01:7778` |

## 6. 安全基线检查清单

- [ ] 已更换此前在聊天中暴露的 root 密码
- [ ] 已配置 SSH 公钥登录，并禁用密码登录（或至少禁用 root 密码）
- [ ] Envoy Admin / Prometheus / Grafana / node_exporter 仅监听 127.0.0.1
- [ ] 游戏后端防火墙仅允许 `gateway-01` 访问
- [ ] 部署 nftables 前已确认 SSH `30455` 放行规则存在
- [ ] `.env` 仅存在于服务器，权限 `600`，未提交仓库

## 7. 内核建议参数（部署时应用）

```bash
# 仓库: deploy/sysctl/gateway-01.conf
# 安装: /etc/sysctl.d/99-gateway-01.conf  （99- = sysctl.d 加载顺序，越大越晚生效）
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 250000
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.ip_local_port_range = 10240 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
fs.file-max = 2097152
```

## 8. 已知风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| 单机 `gateway-01` 单点 | 主机故障全站不可用 | 首期接受；稳定后扩 `gateway-02` |
| 后端源 IP 变为网关 IP | 游戏反作弊/封禁失效 | 后续 TPROXY / PROXY Protocol |
| UDP 无主动健康检查 | 故障发现偏慢 | 监控丢包/超时并告警 |
| 无云厂商高防 | 大流量 DDoS 打满带宽 | 采购高防 IP / 清洗 |
