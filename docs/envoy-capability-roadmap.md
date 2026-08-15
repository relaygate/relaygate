# RelayGate 能力边界

基于 Envoy 的通用 **L4 TCP/UDP** 南北向网关。相关：[热更新](hot-update-xds.md) · [机群架构](fleet-scale-control-plane.md) · [机群运维](fleet-ops.md) · [日志](logging-playbook.md)

## 做什么

- **L4 固定转发**：入口 Listener → 单上游；不做多成员 LB / EDS
- **本机 ADS**：CDS + LDS，endpoint 内联 Cluster；无 RDS / SDS / Istiod
- **日常热更新**（HotApply）；镜像 / bootstrap / admin 元数据走硬重启 + 摘流
- **准入**：主机 nft ACL / 每 IP 限速为真相源
- **机群**：主控发布 → 节点 Agent 拉取 → 本机热更新；NLB/TG 人工；不接云 SDK
- **观测**：中心 Loki + 多 gateway Prometheus；不每节点一套 Grafana/Loki
- **写操作**须独立确认词（见 Panel / CLI）

## 明确不做

多上游成员 LB · 默认 L7 业务网关 · TLS/SDS · ext_authz · WASM · Tracing · 完整服务网格 · 每节点完整 Panel · **外部全局限速（Redis / Rate Limit Service）**（行业可选，产品不接）· **TCP 魔数 / 首包 payload 匹配 / 等下游首包再 dial 上游**（自研协议粗筛放高防或前置 HAProxy 等；本产品用 nft ACL、新建连接限速、cluster `max_connections`、可选 tc 整形做本机减负）

## 已知边界

- UDP 无可靠主动健康检查
- 上游看到的是网关源 IP
- Envoy 非抗 DDoS（大流量需云高防）
- 默认 `PROXY_PROTOCOL=off`（公网直连；有云 LB 发 PROXY 时再开）
- 主机准入与每 IP **新建**连接限速以 nftables 为真相源；TCP 长连接勿对已建立会话做 PPS 限速（见 `packaging/security/`）

可选薄能力：少量 `defaults` 上的 outlier / 熔断档位（非策略引擎）；本机 Envoy `local_ratelimit`（按新连接令牌桶）。
