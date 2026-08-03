# Changelog

本文件记录 RelayGate **用户可见**的重要变更（Keep a Changelog 风格）。版本号对应 GitHub Release tag（`vX.Y.Z`）。

安全修复若不宜在修复前公开细节，仅写影响面与升级建议；完整披露按 [SECURITY.md](SECURITY.md)。

## [Unreleased]

### Added

### Changed

### Fixed

### Security

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
