# Changelog

本文件记录 RelayGate **用户可见**的重要变更（Keep a Changelog 风格）。版本号对应 GitHub Release tag（`vX.Y.Z`）。

安全修复若不宜在修复前公开细节，仅写影响面与升级建议；完整披露按 [SECURITY.md](SECURITY.md)。

## [Unreleased]

## [0.1.14] - 2026-08-14

### Added

- 网卡域：`nic_egress_shape`（tc **出口**整形）与 `nic_ingress_police`（tc **入向** police；params `device`/`rate`）；CLI `security apply-nic --verify` 可同时落地已启用的 `nic_*`；agent AfterPull 顺序内核→网卡（egress+ingress）→防火墙→网关；主控 `PANEL_ENABLED=1` 默认不自动 apply；文档明确不替代高防、手动回滚 root/ingress qdisc
- 机群单节点同步：Panel 节点行「同步」/ `relaygate fleet sync <name>` / `POST /api/ops/fleet/sync`；心跳返回 `pull_now`，仅该节点立即拉取落地（无全局广播）

### Changed

- **破坏性：** 删除 `meta.gateway_name`（无双读）。节点身份只认 `GATEWAY_NAME` env 与（可选）`gateway.name`；机群发布继续剥离 `gateway.name` / `public_ip`。升级后请重新发布机群包；节点本机用安装 env 承接身份。旧 YAML 中的 `meta.gateway_name` 被忽略。
- `tcp-longlived` 场景按低带宽主机（约 3 Mbps）收紧新建/UDP、宽 idle/并发，并默认启用口级出/入向 `3mbit`；其余场景显式保留 `nic_*` 关闭；文档与预览对齐四域落地顺序
- 机群页强调逐台同步；发布仅提升 desired 版本；废除全局一键 sync 产品路径保持不变

## [0.1.13] - 2026-08-13

### Added

- 安全四域（内核 / 防火墙 / 网卡预留 / 网关）；Panel「安全」页与 `relaygate security apply-kernel|kernel-conf`

### Changed

- **破坏性：** 环境变量重命名（无双读）：`ENABLE_PANEL` → `PANEL_ENABLED`，`ENABLE_GRAFANA` → `GRAFANA_ENABLED`。升级请用 `install.sh upgrade`（会改写 `.env`），或手动改键后重启相关服务。`APPLY_FIREWALL`（安装/CLI 一次性）与 `SECURITY_AUTO_APPLY`（节点拉取后自动应用主机侧）分层保留，勿混用。
- **破坏性：** `security.policies[]` 拆为 `security.access`（来源 ACL）与 `security.protections[]`（限速/加固）；防护 id 对齐领域前缀（`gateway_conn_limit` / `firewall_udp_limit`），type≡id；**无**旧键/旧 id 兼容。详见 [security-domains](docs/security-domains.md)。
- **破坏性：** 策略 id/type、preview/status JSON、CLI 子命令按领域名对齐；**无**旧 id（如 `sysctl_syn` / `nft_new_conn_limit` / `envoy_new_conn_limit` / `allowlist` / `conn_limit` / `udp_limit`）与旧 status 键兼容。请改写 `resources.yaml` / profiles；特权 helper 动作为 `kernel-harden-apply`（调用 `security apply-kernel`）。
- 中性档位配置替换游戏化 profile 名；场景合并默认不覆盖 `security.access`（仅显式写 access 的档位如 `strict-allowlist` / `host-harden-only`）
- 节点 agent 拉取后落地：先判断 kernel_syn，关闭则跳过内核域（不再先调用 apply 再校验），避免误报/多余加载
- 删除 Panel 未引用的限流/防火墙检查 i18n 与 `opsFirewallCheck` 客户端包装；产品变量名对齐防火墙域（`applyingFirewall`）
- 修正 `gateways.env.example` 仍指向已删除 `push-fleet-key.sh` 的说明
- Panel API 解析去掉 PascalCase / 旧字段双读；Grafana 看板标题领域化（网关/上游/下游/网卡）
- 删除薄包装 `packaging/security/apply-sysctl-harden.sh`（统一 `security apply-kernel`）

## [0.1.12] - 2026-08-04

### Fixed

- `install.sh` upgrade：替换正在运行的 `bin/relaygate` 时先写入旁路再 `mv`，避免 ETXTBSY（Text file busy）

## [0.1.11] - 2026-08-04

### Changed

- 一键安装改为短子命令：`control` / `node` / `upgrade`（无旧 `env KEY=… bash -y` 兼容层）
- 节点主控地址环境变量由 `PRIMARY_URL` 更名为 `CONTROL_URL`（无双读兼容）
- 对外角色统一为「主控 / 节点」；安装与 join 命令不再使用 primary 作为角色名

### Fixed

- Panel 在非 HTTPS（如 `http://IP:9000`）下复制接入命令等文本时使用兼容回退，避免剪贴板 API 静默失败

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
