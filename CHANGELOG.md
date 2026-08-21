# Changelog

本文件记录 RelayGate **用户可见**的重要变更（Keep a Changelog 风格）。版本号对应 GitHub Release tag（`vX.Y.Z`）。

安全修复若不宜在修复前公开细节，仅写影响面与升级建议；完整披露按 [SECURITY.md](SECURITY.md)。

## [Unreleased]

## [0.1.20] - 2026-08-21

### Fixed

- 节点 agent 重启后，若本机已对齐机群版本，仍会确保本机热更新（ADS）可用，避免 Envoy 失去 CDS/LDS
- Panel systemd：`StartLimit*` 移入 `[Unit]`，兼容 systemd ≥250（如 CentOS Stream 10）

### Changed

- 抽取 `EnsureGatewayADS`，agent / CLI / Panel / 拉取落地共用同一套本机 ADS 启动与快照发布

## [0.1.19] - 2026-08-16

### Breaking

- 节点默认指标改为 **Alloy**（scrape Envoy + 主机指标，`remote_write` 到主控）；不再默认本机 Prometheus / node-exporter
- Compose：`control` = 观测全栈；`node` = Envoy + Alloy（指标）

### Changed

- `ALLOY_CONFIG_FILE`：主控 `config.alloy`（日志），节点 `config.node.alloy`（指标）
- 文档与 `env.example` 按实际组件描述默认栈；安装只选 `control` \| `node`

## [0.1.18] - 2026-08-16

### Breaking

- 本机 Prometheus / node-exporter 改为按 Compose profile 启停（非无条件默认栈）
- 日志采集改为 Grafana Alloy（`packaging/alloy/`）；不再启动 Fluent Bit

### Added

- 本机 Prometheus + node-exporter（主控与节点默认开）
- 主控 Alertmanager：Prometheus 规则可投递 `127.0.0.1:9093`；默认 receiver 仅 UI 可见（webhook/email 见 `packaging/alertmanager/alertmanager.yml`）
- Grafana `gateway-overview` 补强：连接速率、上游健康、限流/溢出/连接失败、UDP 错误

### Changed

- 节点默认：Envoy + Prometheus + node-exporter + agent（remote_write 到主控）；无本机 Grafana/Loki/Alloy
- 主控默认：指标 + Grafana + Loki + Alloy + Alertmanager
- 日志采集统一 Alloy；节点指标仍用本机 Prometheus
- `relaygate apply` 的 `docker compose pull` 显示进度（避免首次拉镜像被误认为卡死）

## [0.1.17] - 2026-08-15

### Added

- 机群页「发布版本」：把当前已保存配置发布为机群新版本（输入「确认」或 Confirm）；按台「同步」仍只拉已发布版本。应用页改为指向机群页，不再把 CLI 当作唯一发布入口
- 节点 Prometheus 经主控 Panel `POST /api/agent/metrics/write` 上报（校验节点令牌后转发到本机 Prometheus remote_write）。`install.sh node` / setup 在已有 `CONTROL_URL` 时写入 `PROMETHEUS_REMOTE_WRITE_URL`

### Changed

- 主控 Prometheus 开启 `--web.enable-remote-write-receiver`（仍只绑 loopback，勿对公网暴露 9090）
- `relaygate render --observability` 把 `AGENT_TOKEN_FILE` 同步为 `DataDir/prometheus/agent.token`（0644），供 Prometheus 容器（nobody）作 remote_write Bearer，避免直接挂 0600 的 secrets

## [0.1.16] - 2026-08-14

### Fixed

- 防火墙 apply：nft 规则备份从硬编码 `/root` 改为 `DataDir/backups`。Panel 在 `ProtectHome=yes` 下即便经 sudo helper 提升，`/root` 仍为只读，原先会报 `read-only file system` 且对外只剩 `exit status 1`
- 特权 helper 失败时把 stderr 并入错误，避免 Panel 只看到 `exit status 1`

## [0.1.15] - 2026-08-14

### Fixed

- 节点名册 `nodes.yaml`：`saveRegistry` 以 0660 落盘并在 root 写入时保留/设置 `relaygate` 组，避免 root CLI（如 fleet sync）覆写后变成 `root:root` 0640 导致 Panel 无法校验节点令牌、心跳 401

## [0.1.14] - 2026-08-14

### Added

- 网卡域：`nic_egress_shape`（tc **出口**整形）与 `nic_ingress_police`（tc **入向** police；params `device`/`rate`）；CLI `security apply-nic --verify` 可同时落地已启用的 `nic_*`；agent AfterPull 顺序内核→网卡（egress+ingress）→防火墙→网关；主控 `PANEL_ENABLED=1` 默认不自动 apply；文档明确不替代高防、手动回滚 root/ingress qdisc
- 机群单节点同步：Panel 节点行「同步」/ `relaygate fleet sync <name>` / `POST /api/ops/fleet/sync`；心跳返回 `pull_now`，仅该节点立即拉取落地（无全局广播）

### Changed

- **破坏性：** 删除 `meta.gateway_name`。节点身份只认 `GATEWAY_NAME` env 与（可选）`gateway.name`；机群发布剥离 `gateway.name` / `public_ip`。升级后请重新发布机群包。
- `tcp-longlived` 场景按低带宽主机（约 3 Mbps）收紧新建/UDP、宽 idle/并发，并默认启用口级出/入向 `3mbit`；其余场景显式保留 `nic_*` 关闭；文档与预览对齐四域落地顺序
- 机群页强调逐台同步；发布仅提升 desired 版本；废除全局一键 sync 产品路径保持不变

## [0.1.13] - 2026-08-13

### Added

- 安全四域（内核 / 防火墙 / 网卡预留 / 网关）；Panel「安全」页与 `relaygate security apply-kernel|kernel-conf`

### Changed

- **破坏性：** `ENABLE_PANEL` → `PANEL_ENABLED`，`ENABLE_GRAFANA` → `GRAFANA_ENABLED`（`install.sh upgrade` 会改写 `.env`）。`APPLY_FIREWALL` 与 `SECURITY_AUTO_APPLY` 分层，勿混用。
- **破坏性：** `security.policies[]` 拆为 `security.access` 与 `security.protections[]`；防护 id 领域前缀、type≡id。见 [security-domains](docs/security-domains.md)。
- **破坏性：** 策略 id/type、preview/status、CLI 按领域名对齐；请改写 `resources.yaml` / profiles；特权 helper：`kernel-harden-apply` → `security apply-kernel`。
- 中性档位名；场景合并默认不覆盖 `security.access`（仅显式写 access 的档位除外）
- 节点 agent 拉取后：`kernel_syn` 关闭则跳过内核域
- 删除未引用的限流/防火墙检查 i18n；Grafana 看板标题领域化
- 删除 `packaging/security/apply-sysctl-harden.sh`（统一 `security apply-kernel`）

## [0.1.12] - 2026-08-04

### Fixed

- `install.sh` upgrade：替换正在运行的 `bin/relaygate` 时先写入旁路再 `mv`，避免 ETXTBSY（Text file busy）

## [0.1.11] - 2026-08-04

### Changed

- 一键安装改为短子命令：`control` / `node` / `upgrade`
- 节点主控地址：`PRIMARY_URL` → `CONTROL_URL`
- 对外角色统一为「主控 / 节点」

### Fixed

- Panel 在非 HTTPS 下复制接入命令等文本时回退剪贴板写入，避免静默失败

### Added

- 登录页支持显示/隐藏密码

## [0.1.10] - 2026-08-04

### Fixed
- `install.sh` 的 `upsert_env_file`：若 `.env` 无尾换行，追加键值前先补换行，避免与上一行粘连
- `make dist` 不再把 `go.mod` 拷进发布包（安装树与源码树分离）

## [0.1.9] - 2026-08-04

### Added
- 主控/节点一键安装与升级，以及 `fleet join` 一句话接入命令
- 机群发布/节点拉取与本机热更新（废除 fleet-sync 产品主路径）
- Panel 支持限流默认值编辑

### Changed
- 危险操作确认词统一为「确认」或 `Confirm`（废除 `HOT_APPLY` / `RELOAD_ENVOY` / `PUBLISH_FLEET` 等按操作区分的指令词）
- Panel 写操作按钮按风险着色：红（破坏/断连）、橙（需留意）、灰（常规）
- 精简运维日志框展示与应用页文案；应用/reload/rollback 标明断连风险
- 对内主控/节点双组件重构（ops→dataplane、doctor→diag）；默认示例密码改为 `relaygate`
- 精简 xDS/机群文档与开源贡献/安全元数据

<!--
发版时：将 Unreleased 条目移入下方新节，例如：

## [0.1.0] - YYYY-MM-DD

### Added
- …
-->
