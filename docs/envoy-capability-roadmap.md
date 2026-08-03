# RelayGate × Envoy 能力评估与功能扩充路线图

> **状态：已产品决策拍板（2026-08-03）**  
> **范围：产品边界与分期；不改业务代码**  
> **产品定位：通用 L4 南北向网关**（基于 Envoy；多节点舰队；本机 ADS 热更新）  
> **相关文档：** [热更新 xDS](hot-update-xds.md) · [主管控与伸缩](fleet-scale-control-plane.md) · [舰队运维](fleet-ops.md) · [日志 Playbook](logging-playbook.md) · [README](../README.md)

### 产品边界一句话

1. **只做 L4**：TCP/UDP 固定目标转发（入口 Listener → 单上游 Cluster）；默认不做 L7 业务网关。
2. **一条动态配置主路径**：本机 ADS = CDS + LDS；endpoint 内联 Cluster；**默认无 EDS / RDS / SDS**；不上 Istiod。
3. **固定单上游**：一逻辑上游一地址；多成员 LB / 独立 EDS **非目标**，除非另开产品需求。
4. **弹性只做薄产品化**：Outlier / 熔断仅暴露少量 `defaults` 字段与档位，不做策略引擎。
5. **准入以主机 nft 为真相源**；TLS/SDS、ext_authz、WASM、Tracing/Tap 默认非目标或远期可选。
6. **机群**：Primary 一键扩容（灌钥+安装+同步+探测）；NLB/TG **人工**；不接云 SDK；Secondary 不做完整 Panel。长期稳态配置分发以 **Agent 拉取** 为主、SSH 仅 bootstrap/应急（[fleet-scale 专章](fleet-scale-control-plane.md#fleet-control-plane-strategy)，已拍板 2026-08-03）。
7. **观测**：中心 Loki + 多 gateway Prometheus；不每节点一套 Grafana/Loki。
8. **写操作须二次确认**；敏感确认词独立、不复用。

---

## 0. 执行摘要

1. RelayGate 已把 Envoy 用成 **L4 TCP/UDP 固定目标转发器**：入口 Listener → 单上游 Cluster，外加本机限速、连接上限、TCP 健康检查与可选 PROXY。
2. 动态配置主路径为 **本机 ADS = CDS + LDS**；endpoint 仍内联在 Cluster（**无独立 EDS**）；**无 RDS / SDS**。
3. 主机侧 **nft ACL / 每 IP 限速** 与 Envoy 正交，是边缘准入与抗滥用的第一道闸。
4. 舰队侧：**Management Primary + 同质双活数据面**、配置同步与伸缩向导已拍板并部分落地；不引入全局 Istiod。
5. 最大产品缺口不在「再堆 L7」，而在 **L4 稳态与可运维性**：热更新/迁移打磨、观测对齐、少量弹性 defaults 产品化。
6. **明确不做**：完整服务网格、默认 L7、云 SDK 内嵌、跨上游成员会话 LB、每节点一套 Panel/Grafana、默认 TLS/SDS/ext_authz/WASM。
7. 扩充原则：**价值 × 复杂度** 分期；能进 Panel/CLI、可验收优先；少层次、少协议分叉、默认一条主路径。
8. 下文 §6 为 **已拍板** 决策清单（含「为什么」）；实现另开工程任务，不在本文改代码。

---

## 1. Envoy 能力全景（与本产品相关度）

相关度：**高** = 应对齐或已用；**中** = 分期可选；**低** = 暂缓；**不适用** = 越界或与架构冲突。

### 1.1 L4：转发与连接语义


| 能力                                         | 相关度        | 说明                                                              |
| ------------------------------------------ | ---------- | --------------------------------------------------------------- |
| TCP Proxy                                  | **高**      | 核心数据面；已用 `tcp_proxy`                                            |
| UDP Proxy                                  | **高**      | 已用 `udp_proxy`；无可靠主动健康检查（已知边界）                                  |
| 连接数限制（circuit breakers / connection limit） | **高**      | 已用 cluster `max_connections` 等；Listener 级 connection_limit 未产品化 |
| 空闲超时                                       | **高**      | TCP/UDP idle timeout 已进 defaults                                |
| PROXY Protocol（v1/v2）                      | **高**      | `.env` 可控；默认 off（公网直连）；前置云 LB 时再开                               |
| 原 IP（下游可见 / 上游看到的源）                        | **高 / 边界** | 日志/ACL 侧依赖 TCP peer 或 PROXY；**上游侧看到网关源 IP** 为已知边界（非 Envoy 独有）   |
| original_dst / 透明代理                        | **低**      | 非南北向固定转发主路径                                                     |
| SO_ORIGINAL_DST / TPROXY                   | **低**      | 运维复杂度高，非默认目标                                                    |




### 1.2 上游：健康、均衡、弹性


| 能力                                   | 相关度       | 说明                                                              |
| ------------------------------------ | --------- | --------------------------------------------------------------- |
| Active health check（TCP）             | **高**     | 已用；UDP-only 上游不探活                                               |
| HTTP/gRPC health check               | **低～不适用** | L4 产品默认不依赖应用层探针                                                 |
| Outlier detection（被动剔除）              | **中**     | **P1 薄产品化**：少量 defaults；默认关或极保守；非策略引擎                         |
| Circuit breaking（熔断阈值）               | **高**     | 已有连接/挂起上限；P1 补运维文案与告警，不扩复杂档位                                   |
| Retry（偏 L7）                          | **不适用**   | TCP 字节流重试语义弱；不做应用层重试                                            |
| 负载均衡策略（RR / least_request / Maglev…） | **低**     | 代码写了 ROUND_ROBIN，但 **单 endpoint**；多成员 LB **已拍板不做**               |
| EDS（独立 Endpoint 发现）                  | **低 / 非目标** | 默认 CDS 内联；**仅当「多成员上游」另开产品需求再评估**；本期不做                        |
| STRICT_DNS / LOGICAL_DNS             | **低**     | 现多为静态 IP；域名上游非 MVP                                             |




### 1.3 流量治理与准入


| 能力                             | 相关度        | 说明                                       |
| ------------------------------ | ---------- | ---------------------------------------- |
| Local rate limit（网络过滤器）        | **高**      | TCP 本地令牌桶已用；UDP 主要靠主机 nft PPS            |
| Global rate limit（外部 RLS）      | **低**      | 多一依赖；nft + 本地限速 + 云防护足够；**不做**           |
| 主机 nft 限速 / ACL                | **高**（产品侧） | 边缘治理真相源                                  |
| Envoy RBAC（网络层）                | **低**      | 与 nft 重叠；**默认不做**，避免双真相                  |
| ext_authz                      | **低 / 非目标** | 默认不引入；远期仅在 nft 不足时单独评估                   |
| IP tagging / internal listener | **低**      | 非刚需                                      |




### 1.4 TLS


| 能力            | 相关度         | 说明                         |
| ------------- | ----------- | -------------------------- |
| 下游 TLS 终结     | **低 / 远期**  | 许多部署在云 LB 或客户端自带加密；默认不强开   |
| 上游 TLS / 回源加密 | **低 / 远期**  | 内网明文常见；合规场景另开需求            |
| SDS（证书动态下发）   | **低 / 远期**  | 无 TLS 产品面则不做               |
| mTLS          | **不适用**     | 南北向边缘少见；东西向网格场景不适用本产品      |




### 1.5 L7（对本 L4 产品）


| 能力                           | 相关度      | 说明                       |
| ---------------------------- | -------- | ------------------------ |
| HTTP Connection Manager / 路由 | **非目标**  | 越界：完整应用网关                |
| RDS                          | **非目标**  | 依赖 L7                    |
| gRPC / HTTP2 业务代理            | **非目标**  | 控制面 gRPC（xDS）已用，不等于业务 L7 |
| JWT / CORS / Lua 路由          | **非目标**  | 应用层能力                    |
| 可选「旁路 L7 健康页 / 管理入口」         | **非目标**  | 已拍板冻结；需求转交专用应用网关         |




### 1.6 可观测


| 能力                      | 相关度     | 说明                                  |
| ----------------------- | ------- | ----------------------------------- |
| Access log（文件 / JSON）   | **高**   | TCP access JSON 已落地；UDP 会话日志较弱      |
| Stats → Prometheus      | **高**   | Envoy stats + 限速告警规则已有；xDS 指标进程内已暴露 |
| Tracing（Zipkin/OTLP 等）  | **低**   | L4 跨度信息有限；默认关、不进主 Panel             |
| ALS（Access Log Service） | **低**   | 已有文件→Fluent Bit→Loki；ALS 非刚需        |
| Admin `/ready` `/stats` | **高**   | drain、健康检查、doctor 依赖                |




### 1.7 动态配置


| 能力                    | 相关度       | 说明                           |
| --------------------- | --------- | ---------------------------- |
| CDS + LDS + ADS       | **高**     | 主路径；已实现本机 ADS                |
| EDS                   | **非目标**   | 见上游节；默认不做                    |
| RDS / SDS             | **非目标 / 远期** | RDS 不做；SDS 随 TLS（远期）       |
| Runtime（feature flag） | **低**     | 产品用 resources + HotApply 更清晰 |
| Tap                   | **低**     | 排障利器，默认不对运维开放（流量敏感）          |




### 1.8 高可用（Envoy + 产品侧）


| 能力                  | 相关度       | 说明                                                    |
| ------------------- | --------- | ----------------------------------------------------- |
| Hot restart（进程交棒）   | **低（备选）** | **已拍板：不作日常 Apply 主路径**；镜像升级走 drain + recreate         |
| Listener / 连接 drain | **高**     | Hot 改删 listen 会 drain 该口；Hard/缩容走 `/healthcheck/fail` |
| 双活舰队 + 可选云 L4 LB    | **高（产品）** | 配置热更与 TG 伸缩解耦（已拍板）                                    |
| 全局共享控制面             | **不适用**   | 不做 Istiod；Envoy 只连本机 ADS                              |




### 1.9 扩展


| 能力        | 相关度     | 说明              |
| --------- | ------- | --------------- |
| WASM      | **非目标** | 运维面与供应链成本高；默认不做 |
| Lua（偏 L7） | **非目标** |                 |
| ext_proc  | **非目标** | 外挂处理面；非 L4 MVP  |


---



## 2. 现状对照表


| 能力                          | Envoy 支持           | 本项目现状                                      | 缺口                                 | 分期                  |
| --------------------------- | ------------------ | ------------------------------------------ | ---------------------------------- | ------------------- |
| TCP/UDP L4 转发               | 成熟                 | `tcp_proxy` / `udp_proxy`；规则→Listener 1:1  | UDP 可观测与会话语义偏弱                     | P0 观测打磨；能力本身已够      |
| 固定单上游                       | Cluster + endpoint | `servers` 单 address；内联 `load_assignment`   | 无多成员池（**已拍板边界**）                   | 保持；EDS **不做**       |
| 空闲超时                        | 支持                 | `tcp_idle_timeout` / `udp_idle_timeout`    | 分转发覆盖 thrash 少                     | P0 文档/档位            |
| 连接上限                        | circuit_breakers 等 | `max_connections` / `max_pending_requests` | 缺「快满」运维告警与文案                       | P1（薄）               |
| PROXY / 原 IP                | listener filter    | `PROXY_PROTOCOL` 默认 off；文档与 NLB 模板已对齐      | 误开 compat 的风险需持续强调                 | P0 文案/检查            |
| 本地限速                        | local_ratelimit    | TCP Envoy 令牌桶 + nft 每 IP                   | 无分转发覆盖；UDP 主要靠 nft                 | P1（可选、保持简单）         |
| ACL                         | 网络 RBAC 或主机防火墙     | **nft deny/allow**（非 Envoy RBAC）           | Envoy 内 RBAC **不做**                | 非目标                 |
| TCP 健康检查                    | active HC          | defaults.health_check + TCP                | UDP-only 无主动 HC；被动 outlier 未开      | P1 outlier（薄）       |
| 熔断 / 异常剔除                   | CB + outlier       | 仅 CB 阈值                                    | outlier 未产品化；失败率可见性不足              | P1（少量 defaults）     |
| 负载均衡多成员                     | EDS + lb_policy    | 名义 ROUND_ROBIN、实际单点                        | **已拍板不做**                          | 非目标                 |
| xDS CDS/LDS/ADS             | 全家桶                | 本机 ADS；HotApply；Hard 回退                    | 迁移/doctor/确认语/舰队瘦 ADS 持续打磨         | **P0**              |
| EDS / RDS / SDS             | 支持                 | **无**                                      | 默认全不做；SDS 仅远期 TLS                  | 非目标 / 远期            |
| 下游/上游 TLS                   | 支持                 | **未接**                                     | 默认不强开                              | 远期可选                |
| Access log                  | 丰富                 | TCP JSON → Fluent Bit → Loki               | 字段可增强；UDP/采样策略细化                   | P0 对齐；P3 增强         |
| Metrics                     | stats sink         | Prom + 限速告警 + xDS 进程指标                     | 统一 scrape/看板与 HotApply 指标进中心       | P0                  |
| Tracing / ALS               | 支持                 | **未接**                                     | 默认非目标                              | 非目标                 |
| Drain / 双活                  | admin + 产品 SOP     | drain fail/ok；NLB 剧本；舰队 sync/scale         | 缩容等待长连接的诚实窗口                       | P0（SOP）             |
| Hot restart                 | 支持                 | **明确不作为日常路径**                              | —                                  | 非目标                 |
| WASM / ext_authz / ext_proc | 支持                 | **未接**                                     | 默认不做                               | 非目标                 |
| 舰队同步 / 伸缩                   | 产品侧                | 同步与一键扩缩容（L2+）已落地                          | NLB 仍人工；不接云 SDK                    | 对齐舰队文档              |


---



## 3. 明确不做 / 慎做（保持产品边界）

与已拍板决策（热更新、主管控、本文 §6）对齐：

### 3.1 明确不做（默认永久非目标，除非另开产品线）


| 项                                     | 原因                                        |
| ------------------------------------- | ----------------------------------------- |
| 完整服务网格 / Istiod / 远程共享 xDS            | Envoy 只连本机 ADS；中心挂不影响已落地快照                |
| 每节点完整 Panel 或每节点 Grafana/Loki         | 仅 Management Primary；Secondary 瘦控制面 + 日志船 |
| 默认 L7 应用网关（HTTP 路由、JWT、CORS、RDS）     | 产品是 L4 南北向转发，不是应用入口                       |
| 多成员上游 LB / 独立 EDS / STRICT_DNS MVP    | 改变「固定目标」语义与排障模型；CDS 内联已够                   |
| 用容器重启冒充热更新                            | Hot 与 Hard 分流已定                           |
| 产品内嵌云 SDK（自动改 TG/ASG）                 | NLB 变更维持控制台 / Terraform；伸缩 L2            |
| 跨上游成员会话迁移 / 「连接跟着上游走」                 | LB drain 只摘新流；长连接不迁移                      |
| deploy 私钥进 Panel 库 / 多 Primary 互写     | 安全与一致性                                    |
| WASM / Lua / ext_proc / ext_authz 默认开放 | 供应链、延迟与双真相成本                              |
| Global rate limit 服务                  | 多一依赖；nft + 本地限速足够                        |
| Envoy 充当抗 DDoS 主防线                    | 大流量依赖云高防 / 主机限速                           |
| Tracing / ALS / Tap 进主 Panel          | L4 价值有限；流量敏感                              |



### 3.2 慎做（默认不开；须另开需求单）


| 项                       | 慎做原因                          |
| ----------------------- | ----------------------------- |
| 多成员上游 + EDS             | 仅当明确产品需求；本期冻结                 |
| 下游/上游 TLS + SDS         | 证书与合规责任上移；许多部署已在外侧终结          |
| 公网入口开启 PROXY compat     | 可伪造源 IP；仅信任 LB 网段             |
| Envoy RBAC 与 nft 并行     | 双真相；优先强化 nft                  |
| hot_restart 日常化         | host 网络 + Docker 编排复杂；已否决为主路径 |


---



## 4. 功能扩充路线图（分期）

分期按 **用户价值 × 实现/运维复杂度**。人天为粗估，供排期，非承诺。  
**MVP = P0 + P1（收窄）**；P2 起为远期/冻结，不阻塞主路径。

### P0 — MVP 夯实：热更新 / 舰队 / 观测（可运营收口）


| 维度       | 内容                                                                                                                    |
| -------- | --------------------------------------------------------------------------------------------------------------------- |
| **用户价值** | 改上游/转发时少断连；舰队配置一致；出问题能按网关/转发定位                                                                                        |
| **范围**   | xDS 迁移检查清单与 doctor；Hot/Hard 确认语与 preview 一致；舰队漂移可重试；TCP 会话日志与 Prom 告警对齐；PROXY/直连安全检查；文档与 Panel 文案中性运维化                |
| **不做**   | 新 Envoy 协议面（EDS/TLS/L7）；不扩策略引擎                                                                                        |
| **验收口径** | 无关长连接 + Envoy PID 在 HotApply 后不变；`fleet-sync` 后 hash 一致或标红可重试；Grafana 能按 `gateway`+`rule` 查会话；误开公网 PROXY compat 有明确警示 |
| **依赖**   | 已有 ADS/HotApply/舰队同步；生产完成 bootstrap 迁移（见 [xds-migrate](xds-migrate.md)）                                               |
| **风险**   | 未迁移节点仍走 Hard；文案与真实模式不一致导致误操作                                                                                          |
| **粗估**   | 持续迭代，约 2–4 人天收口「可运营」                                                                                                  |




### P1 — MVP L4 薄增强（少量 defaults，可进 Panel）


| 维度       | 内容                                                                                                                          |
| -------- | --------------------------------------------------------------------------------------------------------------------------- |
| **用户价值** | 上游偶发抖动时少雪崩；连接/限速「快满」可告警；运维能在配置页调少数档位                                                                                     |
| **范围（收窄）** | ① Outlier detection：**默认关或极保守**，仅 2–4 个 defaults 字段（非策略引擎） ② 熔断/连接上限：运维文案 + 「快满」告警 ③（可选）分转发本地限速覆盖——若实现复杂则降级只做全局 defaults |
| **明确砍掉** | 独立 EDS、多 endpoint、STRICT_DNS、Global RLS、Envoy RBAC                                                                           |
| **验收口径** | 打开 outlier 后，人为上游间歇失败时新连接避开不健康目标；关闭时行为与现网一致；Panel/CLI 仅暴露少量字段；敏感改动仍二次确认                                                     |
| **依赖**   | P0 热更新稳定                                                                                                                     |
| **风险**   | 过激 outlier 误剔除 → 默认关/保守阈值                                                                                                   |
| **粗估**   | 2–4 人天                                                                                                                      |




### P2 — 远期可选（不排入默认路线；另开需求）


| 维度       | 内容                                                         |
| -------- | ---------------------------------------------------------- |
| **定位**   | **非 MVP**；TLS/SDS、ext_authz 等默认 **不做**                     |
| **触发条件** | 明确合规回源加密或 nft 准入不足的书面需求                                     |
| **若做则约束** | TLS 默认关；SDS 证书目录规范；**禁止** nft + Envoy RBAC + ext_authz 三重默认开启 |
| **粗估**   | 需求批准后再估（曾估 4–8 人天，不承诺排期）                                    |




### P3 — 可观测增强（P0 之后按需）


| 维度       | 内容                                                                                |
| -------- | --------------------------------------------------------------------------------- |
| **用户价值** | 更快定位「哪条转发、哪个下游、是否限速/上游失败」                                                         |
| **范围**   | access log 字段增强；UDP 会话/统计补强；xDS ACK/版本进中心看板；**不做**默认 Tracing/ALS                   |
| **验收口径** | Playbook 级排障：告警 → 网关 → 转发 → 会话日志一条链路；禁止客户端 IP 作高基数 Prom label                      |
| **依赖**   | 现有中心 Loki/Prom 拓扑                                                                 |
| **粗估**   | 2–4 人天                                                                            |




### P4 — L7 书面冻结


| 维度       | 内容                                                         |
| -------- | ---------------------------------------------------------- |
| **拍板**   | **冻结为非目标**：HTTP/gRPC 业务代理、RDS、JWT、CORS；需求转交专用应用网关         |
| **不做**   | 管理面旁路 HTTP Listener（易滑向「顺便做路由」）                            |
| **验收口径** | 文档与 Panel 无默认 L7 入口；路线图不再开「可选旁路」分支                          |
| **粗估**   | 0（决策已定）                                                    |




### 分期总览

```text
P0 夯实热更/舰队/观测 ──► P1 L4 薄弹性（defaults）
                              │
                              └────────► P3 观测增强（按需）
远期另开需求 ─────────────────────────► P2 TLS/准入（默认不做）
已冻结 ───────────────────────────────► P4 L7 非目标
```

与舰队文档关系：舰队 P0–P4（身份/观测/xDS/同步/伸缩）是 **机群能力**；本文 P0–P1 是 **Envoy 数据面 MVP**。二者并行时，**数据面 P0 与舰队热更/同步共用优先级**。机群侧已拍板：Primary 一键扩容、NLB 人工、中心观测，以及稳态 **Agent 拉取**（SSH 降级）——与本文 D-E10/D-E11 一致；细节见 [fleet-scale 专章](fleet-scale-control-plane.md#fleet-control-plane-strategy)。

---



## 5. 与 Panel / 机群管理的产品映射


| 能力方向                     | 配置应用 / 上游 / 转发                         | 机群管理（运维·舰队）           | 仅 CLI / 高级                 |
| ------------------------ | -------------------------------------- | --------------------- | -------------------------- |
| 上游 CRUD、启用/停用            | ✅ 上游页                                  | —                     | `relaygate server …`       |
| 转发、验证/正式入口、listen        | ✅ 转发页                                  | —                     | YAML / CLI                 |
| defaults：超时、连接上限、本地限速、HC | ✅ 配置页或档位                               | —                     | `profile apply`；高级字段可折叠    |
| 应用配置（Hot/Hard）           | ✅ 应用页；确认词 `HOT_APPLY` / `RELOAD_ENVOY` | 同步前本机应已 Apply         | `reload` / `reload --hard` |
| ACL / 主机限速               | ✅ ACL + 应用防火墙                          | 各节点各自 firewall（随同步意图） | `firewall apply`           |
| PROXY_PROTOCOL           | 高级/环境（非日常表单）                           | 舰队同质 `.env` 约定        | `.env` 编辑                  |
| Outlier / 熔断（P1 薄）       | ✅ defaults 折叠区（少量字段）                  | —                     | 档位 YAML                    |
| 多成员 EDS                   | ❌ 不做                                   | —                     | —                          |
| TLS / SDS / ext_authz    | ❌ 默认不做（远期另开）                          | —                     | —                          |
| 舰队同步、漂移                  | —                                      | ✅ 机群；`FLEET_SYNC`     | `fleet-sync`               |
| 伸缩 Expand/Shrink         | —                                      | ✅ 向导；TG/NLB **人工**    | playbook API / 脚本          |
| 摘流 drain                 | ✅ 运维页                                  | 缩容步骤内                 | `drain fail|ok`            |
| 诊断 doctor / smoke        | ✅ 运维                                   | 节点接入检查清单              | CLI                        |
| xDS 端口 / bootstrap 迁移    | —                                      | —                     | ✅ `xds-migrate`、doctor     |
| Tracing / Tap / WASM     | —                                      | —                     | ❌ 默认关，不进主路径               |
| Grafana / 会话日志           | 监控入口（深链）                               | 中心仅 Primary           | —                          |


**原则：** 改「业务意图」（上游/转发/限速）走 **配置应用**；改「谁在舰队里、是否接流量」走 **机群管理**；改「进程/bootstrap/密钥/实验开关」留 **CLI/高级**，并保持二次确认。standby / 只读角色禁止写操作，须引导到 Management Primary。

---



## 6. 决策摘要（已拍板 · 2026-08-03）

原则：**产品化（可验收、可进 Panel/CLI）** × **结构简单（一条主路径）** × **少复杂逻辑**。

| ID | 拍板 | 内容 | 为什么（简单 / 产品化） |
|----|------|------|-------------------------|
| **D-E1** | **是** | 主路径保持 **L4 固定目标转发**；不做默认多成员上游 LB | 排障模型简单；一上游一地址可验收 |
| **D-E2** | **是** | 动态配置 = **本机 ADS（CDS+LDS）**；不上 Istiod / 远程共享 xDS | 少一层控制面；中心挂不影响已落地快照 |
| **D-E3** | **是** | 日常 **HotApply**；镜像/bootstrap **Hard + drain**；不以 hot_restart 为日常路径 | 一条 Apply 主路径；避免 Docker/host 交棒复杂度 |
| **D-E4** | **是** | **P0** 先打磨迁移/doctor/确认语/舰队一致/观测，再开新特性 | 可运营优先于堆 Envoy 能力 |
| **D-E5** | **是（收窄）** | **P1** 仅 outlier + 熔断/限速 **薄产品化**（少量 defaults）；**EDS 默认不做** | 用户能感到「少雪崩」；不做策略引擎与协议分叉 |
| **D-E6** | **是** | ACL 真相源 = **主机 nft**；Envoy RBAC / ext_authz **默认不做** | 避免双/三真相；边缘准入一条路径 |
| **D-E7** | **是（降级）** | TLS/SDS = **远期可选、默认安装不强开**；不进 MVP | 多数部署外侧已终结；证书面显著增复杂 |
| **D-E8** | **是（冻结）** | **默认不做** L7 业务网关；P4 书面冻结，无旁路分支 | 防范围蠕变；产品定位清晰 |
| **D-E9** | **是** | Tracing / ALS / WASM / Tap：**默认关**，不进主 Panel | 运维主路径保持短；敏感面不默认暴露 |
| **D-E10** | **是** | 观测 = **中心 Loki + 多 gateway Prom**；禁客户端 IP 高基数 label | 与舰队 D7 一致；不每节点一套栈 |
| **D-E11** | **是** | 云 TG/ASG：**不接云 SDK**；Primary 一键扩容，**NLB 人工** | 与舰队 D8/D11 一致；自动化停在可控边界 |
| **D-E12** | **是** | 新危险操作须 **独立确认词**；Hot/Hard/同步/防火墙/摘流/伸缩词不复用 | 防误操作；面向运维二次确认 |

---



## 附录 A — 代码与配置锚点（只读索引）


| 区域       | 路径                                                    | 与评估的关系                                 |
| -------- | ----------------------------------------------------- | ---------------------------------------- |
| 意图模型     | `core/resources/resources.go`                         | servers/rules/defaults/acl             |
| 静态/动态渲染  | `core/render/generate.go` `dynamic.go` `bootstrap.go` | TCP/UDP cluster+listener；ADS bootstrap |
| xDS      | `core/xds/`                                           | ADS、Snapshot、ACK 指标                    |
| HotApply | `core/ops/hot_apply.go` `reload.go`                   | Hot/Hard 分流                            |
| 舰队       | `core/ops/fleet_sync.go` `scale_*.go`                 | 同步与伸缩                                  |
| Compose  | `packaging/compose.yaml`                              | Envoy host 网络、日志卷                      |
| 意图示例     | `packaging/shared/resources.example.yaml`          | 默认超时/限速/HC                             |
| 环境       | `packaging/shared/env.example`                    | `XDS_*` `PROXY_PROTOCOL` profiles      |




## 附录 B — 已关闭的评审问题

| 问题 | 拍板结论 |
|------|----------|
| 是否要一个逻辑上游多个地址？ | **否（本期）**；冻结 EDS/多成员；专注 outlier 与限速薄产品化 |
| 边缘是否必须在网关终结 TLS？ | **否（默认）**；继续外侧终结；TLS/SDS 远期另开 |
| 准入以 nft 为唯一真相是否够？ | **是（默认 12 个月）**；不并行默认开启 Envoy RBAC/ext_authz |
| P0 未收口前是否排 P2 TLS？ | **否**；P2 不进默认路线 |

---

**文档结束。** 决策已拍板；实现阶段另开工程方案与任务，不在本文改业务代码。与 [fleet-scale-control-plane.md](fleet-scale-control-plane.md)、[hot-update-xds.md](hot-update-xds.md) 冲突时，以本文「简单产品化」边界为准（CDS 内联、无默认 EDS/L7/TLS）。
