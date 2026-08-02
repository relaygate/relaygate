# RelayGate 热更新（xDS）工程方案

> 状态：设计稿（可开工）  
> 源码基准：`/root/relaygate`  
> 生产对照：`/opt/relaygate`（仅行为对照，非改动目标）  
> 日期：2026-08-02

---

## 1. 目标与非目标

### 1.1 目标

| 变更类型 | 期望 |
|----------|------|
| 加/改/删上游（`servers`：address / tcp.port / udp.port / enabled） | **零断连**热推（CDS） |
| 加/改/开关转发规则（`rules`：enabled、target server、listen_port） | **尽量零断连**热推（LDS）；见 §5.7 边界 |
| Envoy 侧 defaults（idle timeout、local rate limit、max_connections、health_check） | 热推（随 CDS/LDS 资源重写） |
| Panel「应用配置」/ `relaygate reload` 常规路径 | 默认走热更新，**不** `docker restart` / `--force-recreate` |

**「零断连」定义（验收口径）**：Envoy **进程与容器 PID 不变**；与本次变更无关的已建立 TCP 长连接在 Apply 前后持续可读可写；admin `/ready` 保持 LIVE；不执行 NLB drain（除非显式 hard reload）。

### 1.2 非目标（仍允许 / 必须进程级重启）

| 变更 | 原因 | 路径 |
|------|------|------|
| Envoy **镜像/二进制**升级 | 进程替换不可避免 | `upgrade` / compose pull + recreate（可双活 drain） |
| **Bootstrap** 变更：admin 地址/端口、xDS 控制面地址、node/cluster 身份、concurrency 启动参数 | 静态启动配置，xDS 管不到 | Hard reload（保留现有 drain→restart） |
| Compose 网络/卷/命令行（`--concurrency`、挂载路径） | 容器规格 | Hard reload / `apply` |
| `PROXY_PROTOCOL` 等若误放进 bootstrap | 应放进 LDS；若误放 bootstrap 则硬重启 | 见 Phase 1 约束 |
| 主机 nft / ACL | 本就不走 Envoy | 继续 `firewall apply`（与热更新正交） |
| 摘流迁移已建立长连接 | **做不到**；drain 只摘新流 | 双活剧本不变 |

### 1.3 明确不做的备选主路径

- **不以 hot_restart 为主路径**（见 §2.2）。
- 不引入独立远程 Istiod/全局控制面；每台网关 **本机 loopback ADS** 即可。

---

## 2. 推荐架构（主路径：本机 ADS = CDS + LDS）

### 2.1 选定方案

**主路径：Envoy bootstrap 仅保留 admin + xDS 集群；Listeners/Clusters 全部经 ADS（Aggregated Discovery Service）下发 CDS + LDS。**

控制面嵌入 **Panel 同进程**（`relaygate panel` / systemd），监听 `127.0.0.1:<XDS_PORT>`（建议默认 `18000`）。Envoy 容器 `network_mode: host`，可直连宿主 loopback。

依赖：引入 [`envoyproxy/go-control-plane`](https://github.com/envoyproxy/go-control-plane)（与镜像 `envoyproxy/envoy:v1.39.0` 对齐的 API 版本）。

### 2.2 为何不用 / 何时用 hot restart

| | xDS CDS/LDS（主） | hot_restart（备） |
|--|-------------------|-------------------|
| 已建立连接 | 进程不变，无关连接保留 | 父子进程 + 共享内存交棒；Docker/`host` 网络下 **IPC、drain、epoch 编排复杂**，易踩坑 |
| 实现落点 | 复用现有 `render` 产物 → Snapshot；Panel 已是常驻进程 | 需改 compose command、`--restart-epoch`、parent/child、失败回退 |
| 适用 | 日常 resources 变更（上游/规则/限速） | Envoy **二进制**小版本热替换（可选后期）；**不**作为 Apply 日常路径 |
| 与现注释一致 | `core/ops/reload.go` 已写明 Prefer xDS | 保留为备选备注即可 |

**结论**：日常 Apply → xDS；进程/bootstrap/镜像 → 现有 hard reload（可双活 drain）。

### 2.3 数据流

```text
┌─────────────┐  写 resources.yaml   ┌──────────────────┐
│ Panel / CLI │ ──────────────────► │ DataDir/resources │
└──────┬──────┘                     └────────┬─────────┘
       │ Apply / reload                      │
       ▼                                     ▼
┌──────────────────────────────────────────────────────┐
│ ops.HotApply / SnapshotPublisher                     │
│  1. backup + ChangeSummary（已有）                   │
│  2. resources.Validate（已有）                       │
│  3. render → Cluster/Listener protobuf + bootstrap   │
│  4. envoy --mode validate（临时合并 YAML 或等价）    │
│  5. AtomicSnapshot(version++) → ADS cache            │
│  6. 等待 ACK / 超时 → 成功或回滚上一 version         │
└───────────────────────────┬──────────────────────────┘
                            │ gRPC ADS (127.0.0.1:18000)
                            ▼
┌──────────────────────────────────────────────────────┐
│ Envoy（docker, host net, PID 不变）                  │
│  bootstrap: admin + cluster xds_cluster              │
│  dynamic: CDS clusters + LDS listeners               │
│  已建立连接绑定旧 filter chain / upstream socket；   │
│  新连接走新快照                                      │
└──────────────────────────────────────────────────────┘
```

**现有连接如何保留**：不杀容器、不 `docker restart`；CDS 更新 endpoint 时，已建立 upstream 连接通常继续到 idle close；仅被删除的 cluster/listener 上的流才会 drain。与变更无关的 listener 上的长连接应存活。

### 2.4 与 drain / 双活

| 路径 | drain？ |
|------|---------|
| HotApply（xDS） | **否**。勿摘流；摘流不能迁移已建立连接，热更新也不需要摘新流。 |
| HardReload（现 `ReloadTo`） | **是**（保持现状）：`/healthcheck/fail` → wait → restart → ready → ok。双活维护窗口用。 |
| 防火墙 apply | 不变；与 Envoy 热更新独立。 |

---

## 3. 与现有代码的衔接

### 3.1 现状（真实行为）

| 环节 | 文件 | 行为 |
|------|------|------|
| 模型 | `core/resources/resources.go` | `Resources{Meta,Gateway,Defaults,ACL,Servers,Rules}`；校验端口/协议/上游引用 |
| Diff / 分流 | `core/resources/diff.go` | `ChangeSummary.Classify()` → `NeedsReload` / `NeedsFirewall` |
| 渲染 | `core/render/generate.go` | **整份** Envoy YAML：`admin` + `static_resources.{listeners,clusters}`；命名 `upstream-{server}-{proto}` / `ingress-{rule}` |
| Apply 全量部署 | `core/ops/apply.go` | seed → backup → validate → compose up（首次/全量，非热更新对象） |
| Reload | `core/ops/reload.go` | backup → render → validate → **drain** → `docker restart` 或 `compose up --force-recreate` → ready → undrain |
| Panel | `core/panel/server.go` `apiApply` | 二次确认 `RELOAD_ENVOY` → `ops.ReloadCapture` |
| Preview | `apiApplyPreview` | 返回 `needs_reload` / `needs_firewall` |
| UI | `ui/src/pages/ApplyPage.tsx` + i18n | 风险文案已中性（「现有连接」）；确认语 `RELOAD_ENVOY` |
| Compose | `packaging/compose.yaml` | Envoy `network_mode: host`，挂载 `DataDir/envoy/envoy.yaml`，command `-c /etc/envoy/envoy.yaml`，**无** xDS/`--drain-time-s` 热更新专用项 |
| 生产对照 | `/opt/relaygate/data/envoy/envoy.yaml` | 与源码 render 一致：纯 `static_resources`，无 `dynamic_resources` |
| xDS/ADS/SDS | **无实现痕迹** | 仅 `reload.go` roadmap 注释与 UI「规划中」文案 |

### 3.2 关键复用点（勿推倒重来）

1. **继续以 `resources.yaml` 为唯一意图源**；nft 仍走 `NeedsFirewall` / `firewall apply`。
2. **复用** `render` 里 cluster/listener **结构生成逻辑**（`renderTCPCluster` / `renderTCPListener` 等），拆成：
   - `RenderBootstrap(r)` → 静态 admin + xds_cluster + `dynamic_resources`
   - `RenderCDS(r)` / `RenderLDS(r)` → 动态资源（map 或 protobuf）
3. **复用** `BackupWithSummary`、`Validate`、`Classify`；扩展 Classify 输出 `ApplyMode: hot | hard`。
4. **Hard path 完整保留** `ReloadTo`，改名语义为 hard，避免半吊子改坏生产回退。

### 3.3 新增 / 修改模块清单

| 包路径 | 职责 | Phase |
|--------|------|-------|
| `docs/hot-update-xds.md` | 本方案（已落盘） | 0 |
| `core/xds/`（新） | ADS gRPC server、Snapshot cache、version、ACK 等待、回滚 | 1 |
| `core/xds/snapshot.go` | `Resources` → go-control-plane `cache.Snapshot`（CDS+LDS） | 1 |
| `core/render/bootstrap.go`（新） | 最小 bootstrap YAML | 1 |
| `core/render/generate.go` | 拆分 static→dynamic；保留 `Write` 兼容路径或改为「bootstrap + 旁路 dump」 | 1 |
| `core/ops/hot_apply.go`（新） | `HotApplyTo`：backup→validate→publish→ACK；失败回滚 snapshot | 1 |
| `core/ops/reload.go` | 入口分流：`Reload` → Hot 优先，不可热则 Hard；保留 `HardReloadTo` | 1–2 |
| `core/resources/diff.go` | `ApplySurface` 增加 `NeedsHardReload` / `CanHotApply` | 1 |
| `core/panel/server.go` | preview 返回 `apply_mode`；apply 按模式选确认语 | 2 |
| `core/cli/cli.go` | `reload` 默认热更新；`reload --hard` 强制旧路径 | 1 |
| `packaging/compose.yaml` | **无需改 command 即可收 xDS**（仍 `-c bootstrap`）；可选挂载不变 | 1* |
| `.env` / `core/ops/env.go` | `XDS_PORT`、`XDS_ENABLED`（迁移开关） | 1 |
| `core/host/panel.go` / systemd | 确保 Panel 先于 Envoy 就绪，或 Envoy 容忍 xDS 短暂不可达 | 1 |
| `ui/.../ApplyPage.tsx` + i18n | 热更新确认 `HOT_APPLY`；hard 仍 `RELOAD_ENVOY`；风险分级 | 2 |
| `core/ops/validate.go` | 校验「bootstrap+动态合并」或 protobuf 等价配置 | 1 |
| 测试 | `core/xds/*_test.go`、`render` golden、集成：长连接不断 | 1–2 |

\*Phase 1 前置：生产 Envoy 必须换成 **bootstrap 形态**的 `envoy.yaml`（一次 hard recreate 迁移窗口）；之后日常不再 recreate。

### 3.4 控制面进程选型（已定）

- **嵌入 Panel**：零新容器、与 Apply API 同生命周期、权限已在宿主。
- CLI `relaygate reload`：若 Panel 未运行，则：
  - **推荐**：`reload` 短暂自起 in-process xDS、推快照、等 ACK、退出时 **不关** Envoy（Envoy 已持有配置）；或
  - 要求生产 **Panel 常驻**（与现 systemd 一致），CLI 经 Unix socket/HTTP localhost 调 Panel 内部 HotApply。
- **选定**：生产以 Panel 常驻为准；CLI 调用 `ops.HotApply` 时检测本机 ADS：若无服务则 **进程内临时起 ADS 并 `SetSnapshot`，且写 disk bootstrap 指向该端口**——迁移完成后 bootstrap 固定指 Panel，CLI 只 HTTP 调 Panel（Phase 2 收紧）。

Phase 1 务实做法：`ops.HotApply` 与 Panel 共用 `xds.Server` 单例接口；Panel `Start` 时 Listen；CLI 若连不上 `127.0.0.1:XDS_PORT` 则 fallback **HardReload** 并打明确日志（避免静默失败）。

---

## 4. 分阶段落地计划

### Phase 0 — 方案冻结与分流骨架（0.5–1 人天）

**改动**

- 本文件合入评审。
- `ApplySurface` 扩展字段（可先只加类型与单测，**不改** `ReloadTo` 行为）：
  - `CanHotApply bool`
  - `NeedsHardReload bool`（meta.admin_*、envoy_image、或显式 bootstrap 相关）
- Feature flag：`XDS_ENABLED=0`（默认关）时行为与现网完全一致。

**验收**

- 全量单测绿；`XDS_ENABLED=0` 时 Panel Apply / `reload` 仍 drain+restart。
- 无生产 compose/envoy 行为变化。

**回滚**：删 flag 相关代码或保持默认关。  
**风险**：极低。

---

### Phase 1 — 骨架接通：加/改上游与规则走 CDS/LDS（主里程碑，3–5 人天）

**改动内容**

1. **Bootstrap 渲染**：`admin` + static cluster `xds_cluster`（STRICT_DNS/STATIC → `127.0.0.1:XDS_PORT`）+ `dynamic_resources.ads_config` / `cds_config`/`lds_config` 走 ADS。
2. **`core/xds`**：go-control-plane Aggregated Discovery；`SetSnapshot(nodeID, snap)`；nodeID = `GATEWAY_NAME`（与 compose `--service-node` 一致）。
3. **Panel 启动时**拉起 xDS Listen；从当前 `resources.yaml` 发布 **初始快照**（否则 Envoy 空配置）。
4. **`HotApplyTo`**：backup → render snapshot → validate → `SetSnapshot` → wait ACK（超时可配，建议 10–15s）。
5. **`Reload` 分流**（`XDS_ENABLED=1` 且 bootstrap 已迁移）：
   - `CanHotApply && !NeedsHardReload` → `HotApplyTo`（**无 drain**）
   - 否则 → 现有 `HardReloadTo`（drain+restart）
6. **一次性迁移**：文档/脚本说明维护窗口执行一次 hard reload 写入 bootstrap（或 `relaygate reload --migrate-xds`）。
7. Compose：**一般无需改 command**；确认 host 网络与 admin 端口。可选增加 `--drain-time-s` / `--parent-shutdown-time-s` 仅当后续做 hot_restart 备选时再加。

**验收（必须）**

```bash
# 1) 建立长连接（示例：经某 production TCP listen）
# 2) 仅改无关上游 address 或新增无关 server+disabled rule → HotApply
# 3) 连接仍存活；docker inspect PID 不变；日志无 restart
ss -tpn | grep <conn>
docker inspect -f '{{.State.Pid}}' ${GATEWAY_NAME}-envoy   # Apply 前后相同
curl -sS 127.0.0.1:9901/ready | grep LIVE
```

**回滚**

- `XDS_ENABLED=0` + 用旧逻辑 `render.Write` 全量 static + `HardReloadTo`。
- 保留 backups；`relaygate rollback` 仍走 hard recreate（Phase 1 可暂不热回滚 disk）。

**风险**

- 迁移窗口一次断连（预期、可双活 drain）。
- Panel 挂掉后 Envoy **继续用最后快照**（正常）；新 Apply 需 Panel；需监控 Panel。
- ACK 超时误判 → 勿自动 hard restart；只报错并保留上一快照。

---

### Phase 2 — 产品化：确认语、回滚、边界文案（2–3 人天）

**改动**

- Preview：`apply_mode: "hot"|"hard"|"none"`；UI 分级 Alert。
- 确认语：热更新 `HOT_APPLY`；硬重启仍 `RELOAD_ENVOY`。
- Snapshot 持久化：`DataDir/envoy/snapshots/<version>.json`（或 protobuf bin）；失败自动 `SetSnapshot(prev)`。
- Listener 删除/改端口：文案诚实说明「该 listen 上现有连接会进入 listener drain」。
- CLI：`reload --hard`；默认热更新。
- doctor：检查 xDS 端口 listening、Envoy `cds`/`lds` 统计、与 Panel 版本一致。

**验收**：UI/API 确认语与模式一致；故意坏配置被 validate 拦住且旧连接不断；ACK 失败回滚后业务端口仍可用。

**回滚**：UI 仍可强制 hard；flag 关闭。  
**风险**：文案/确认语两套，需测试覆盖 `ops_test` / ApplyPage。

---

### Phase 3 — 加固与观测（1–2 人天）

- Prometheus：`relaygate_xds_snapshot_version`、`xds_ack_latency`、`hot_apply_total{result=}`。
- 混沌：xDS 短暂不可用、推重复 version、快速连续 Apply。
- （可选）EDS 拆分：多 endpoint 时再拆；当前单 IP STATIC 可继续 CDS 内联 `load_assignment`。
- 文档更新 README「Apply 分流」表。

---

## 5. 关键设计细节

### 5.1 Bootstrap 最小形态（示意）

```yaml
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 9901, protocol: TCP }
  # access_log 保持现网

node:
  cluster: ${GATEWAY_NAME}
  id: ${GATEWAY_NAME}

static_resources:
  clusters:
    - name: xds_cluster
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: xds_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 18000 }
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}

dynamic_resources:
  ads_config:
    api_type: GRPC
    transport_api_version: V3
    grpc_services:
      - envoy_grpc: { cluster_name: xds_cluster }
  cds_config: { ads: {}, resource_api_version: V3 }
  lds_config: { ads: {}, resource_api_version: V3 }
```

业务 `listeners` / `clusters`（upstream-*）**不得**再出现在 bootstrap `static_resources`。

### 5.2 配置快照版本号 / 一致性

- Version：单调递增字符串（建议 `strconv.FormatUint` 或 `time.Format` + counter）。
- 单次 Apply **一个 Snapshot** 同时含完整 CDS + LDS（go-control-plane 一致性：同 version 原子切换）。
- `node.id` = `GATEWAY_NAME`；多网关互不共享控制面。
- Disk：`resources.yaml` 仍为意图源；Snapshot 为派生物；backup 继续备份 resources + 可选 bootstrap。

### 5.3 推送前校验

顺序（失败即停，不 bump 给 Envoy 的活跃 version）：

1. `resources.Validate()`
2. 渲染 protobuf / 等价 YAML
3. **合成校验**：将 CDS+LDS 临时写成完整 config 跑现有 `ValidateEnvoyContainer`（`--mode validate`），或使用 go-control-plane 的转换 + envoy 校验镜像
4. 可选：拒绝「删除最后一个 enabled listener」等已有规则

Validate **失败**：不调用 `SetSnapshot`；已建立连接零影响。

### 5.4 失败自动回滚

| 失败点 | 行为 |
|--------|------|
| Validate 失败 | 不推送 |
| ADS NACK / ACK 超时 | `SetSnapshot(previousVersion)`；API 返回错误；**不** docker restart |
| Panel 崩溃 | Envoy 保持最后 ACK 配置 |
| 运维要回到旧 resources | 恢复 yaml 后 HotApply；若控制面全挂 → HardReload 保底 |

### 5.5 与 drain 的关系

- HotApply：**禁止**自动 drain。
- HardReload：保持现实现。
- 双活 + 必须 hard 的变更：文档要求先 `drain fail`。

### 5.6 Apply UI/API 确认语

| 模式 | Preview | 确认语 | 文案要点 |
|------|---------|--------|----------|
| 热更新 | `apply_mode=hot` | `HOT_APPLY` | 不重启 Envoy；无关现有连接应保留；改/删的 listen 上连接可能 drain |
| 硬重启 | `apply_mode=hard` | `RELOAD_ENVOY` | 现有全文案（断全部现有连接） |
| 仅防火墙 | `needs_firewall` | `YES_FLUSH_NFTABLES` | 不变 |
| 无变更 | none | — | 按钮禁用 |

Phase 1 可暂不改 UI 确认语（仍 `RELOAD_ENVOY` 但后端已热更新）——**不推荐长期**；Phase 2 必须拆开，避免操作者误以为会断连而不敢发版，或误以为硬重启却走了热更新。

### 5.7 Listener / UDP / TCP 边界（诚实说明）

| 操作 | Envoy 行为（预期） | 连接影响 |
|------|-------------------|----------|
| CDS：改无关 upstream 的 address/port | 更新 cluster；无关 listener 不动 | **无关长连接保留** |
| CDS：改**当前**连接所用 upstream endpoint | 已建立连接通常仍走旧 socket；**新连接**走新地址；旧 upstream 无流量后随 idle 回收 | 不保证「会话内切上游」；不断开是常见情况，但是 **best-effort**，验收以「进程不杀 + 无关连接存活」为准 |
| CDS：删除某 upstream cluster | 引用它的 listener 应先改掉；若仍引用则 NACK | 发布顺序靠同一 snapshot 一致性 |
| LDS：**新增** listener（新端口） | 新绑定 | 现有连接无影响 |
| LDS：**删除**或 **改 listen_port** | 旧 listener drain | **该端口上现有连接会进入 drain/断开**（TCP）；需文案说明 |
| LDS：改 filter 参数（rate limit、idle_timeout、tcp_proxy cluster 名） | in-place listener 更新常触发 **listener warm/drain** | 可能影响**该** listener 上连接；尽量把「仅改 upstream」做成只动 CDS，listener 的 `cluster` 字段不变 |
| UDP listener 更新 | UDP 无长连接语义；session 由 idle_timeout 表征 | 更新可能导致该端口 UDP session 重置；验收侧重 TCP 长连接 |
| 禁用 rule（enabled false） | 等同移除 listener | 该 listen 上连接 drain |

**产品策略**：Phase 1 优先保证「加上游 / 改无关上游 address / 加新转发端口」零影响；「改当前规则的 listen 或关掉规则」允许该规则连接受影响，但 **整机不重启**。

### 5.8 命名与资源稳定性

保持现有稳定名（热更新友好）：

- Cluster：`upstream-{server}-{tcp|udp}`（`render.UpstreamClusterName`）
- Listener：`ingress-{forwardName}`（`render.IngressListenerName`）

禁止 Apply 时无意义改名（会导致 delete+add，放大 drain）。

### 5.9 PROXY_PROTOCOL / meta

- `PROXY_PROTOCOL*` → 继续进 **LDS listener_filters**（现 `renderTCPListener`），可热更新。
- `meta.admin_port` / `admin_address` → **NeedsHardReload**。
- `meta.envoy_image` → 改 `.env`/`ENVOY_IMAGE` + compose recreate，不走 HotApply。

---

## 6. 工作量与里程碑

| 阶段 | 粗估 | 可合并产出 |
|------|------|------------|
| Phase 0 | 0.5–1 人天 | 本文档 + Classify 扩展 + flag（默认关） |
| Phase 1 | 3–5 人天 | xds 包、bootstrap、HotApply、Reload 分流、迁移说明、核心单测 |
| Phase 2 | 2–3 人天 | UI/API 确认语、自动回滚、doctor、文案 |
| Phase 3 | 1–2 人天 | 指标、混沌、README |

**合计约 7–11 人天**（含联调与迁移演练）。

### 第一个可合并 PR（Phase 0 + Phase 1 最小竖切）建议包含

1. `docs/hot-update-xds.md`（本文）
2. `core/resources`：`CanHotApply` / `NeedsHardReload` + 单测
3. `core/render`：`RenderBootstrap` + 动态资源导出（单测 golden）
4. `core/xds`：可启动的 ADS + 内存 Snapshot（unit test 用 cache）
5. `core/ops/hot_apply.go` + `Reload` 在 `XDS_ENABLED=1` 时分流；默认 `0` **行为零变化**
6. 简短 `docs/xds-migrate.md`：生产一次迁移步骤（双活 drain → 写 bootstrap → 启 Panel xDS → hard 一次 → 验证 → 此后 HotApply）

**不要**在第一 PR 强改默认 UI 确认语；不要删除 `HardReloadTo`。

---

## 7. 验证计划

### 7.1 场景 A — 改无关上游（金标准）

1. 客户端长连接打到 `forward-server-01-production-tcp`。
2. 记录 Envoy PID、`/stats` 中该连接相关计数。
3. 修改 `server-02` address（或新增 `server-03`）→ Panel Apply / `reload`（热更新）。
4. **期望**：连接存活；PID 不变；无 docker restart 日志；`/ready` LIVE。

### 7.2 场景 B — 改当前上游 endpoint

1. 长连接在 server-01。
2. 改 server-01 `address` 或 `tcp.port` → HotApply。
3. **期望**：进程不杀；已建立连接 **多数情况**仍可用直到对端/idle；**新连接**到新地址；文档不承诺会话内无缝切上游。

### 7.3 场景 C — 坏配置

1. 推送非法 resources（端口冲突 / 空 enabled rules）→ Validate 失败。
2. **期望**：不 bump 活跃 snapshot；好连接不断。
3. （Phase 2）模拟 NACK：推明显非法 protobuf → 回滚上一 version；业务 listen 仍在。

### 7.4 场景 D — 改/删当前 listener

1. 禁用当前 rule 或改 `listen_port`。
2. **期望**：整机不重启；**该**端口连接断开或 drain；其他端口连接保留。

### 7.5 场景 E — Hard 路径仍可用

1. `XDS_ENABLED=0` 或 `reload --hard` 或改 `admin_port`。
2. **期望**：仍 drain + restart；二次确认 `RELOAD_ENVOY`。

### 7.6 建议命令片段

```bash
# 连接存活探针（示例）
pid1=$(docker inspect -f '{{.State.Pid}}' gateway-01-envoy)
# ... HotApply ...
pid2=$(docker inspect -f '{{.State.Pid}}' gateway-01-envoy)
test "$pid1" = "$pid2"

curl -sS 127.0.0.1:9901/ready
curl -sS 127.0.0.1:9901/stats | grep -E 'cluster.xds_cluster|update_success|update_rejected'

# Panel 日志应出现 hot_apply ok，而非 docker restart
journalctl -u relaygate-panel -n 50 --no-pager
```

---

## 8. Phase 1 前置检查清单（生产）

- [ ] Panel systemd 常驻，且先于/并行于 Envoy；xDS 端口仅绑定 `127.0.0.1`
- [ ] `GATEWAY_NAME` = Envoy `--service-node` / `--service-cluster`
- [ ] 防火墙 **不要**对公网暴露 `XDS_PORT` / admin
- [ ] 迁移窗口：双活则先 `drain fail`，写入 bootstrap，启 xDS，**一次** hard recreate，`drain ok`
- [ ] 确认镜像仍为 Envoy v3 API（当前 v1.39.0）
- [ ] 回滚演练：`XDS_ENABLED=0` + 全量 static render + hard reload

---

## 9. 决策摘要（供评审勾选）

1. **主路径 = 本机 ADS（CDS+LDS）**；hot restart 不作日常 Apply。  
2. **控制面 = Panel 内嵌**；CLI 连不上则 Phase 1 fallback hard。  
3. **默认 flag 关闭合并**；迁移后开 `XDS_ENABLED=1`。  
4. **HotApply 不 drain**；HardReload 保持 drain。  
5. **第一阶段验收焦点**：加/改无关上游、加转发端口 → PID 不变 + 长连接存活。  

评审通过后即可按 §6 第一个 PR 开工。
