# core/ — 单二进制内模块

| 包 | 职责 |
|----|------|
| `agent` | 节点 agent：名册/发布版本/心跳/拉取；CLI `agent`、`fleet publish|join|leave|status` |
| `dataplane` | 本机转发与运维：validate/apply/reload/hot/drain/firewall/upgrade 等 |
| `diag` | 本机诊断：CLI `diag`（原 doctor） |
| `panel` | 主控 HTTP UI/API |
| `host` | 宿主安装（Panel systemd 等） |
| `xds` | 本机 ADS（非产品主命令；由 agent/panel 内嵌） |
| `config` / `resources` / `render` / `setup` / `profile` / `status` | 配置与渲染支撑 |
| `cli` / `cmd/relaygate` | 命令入口 |

不做双仓库；主控与节点共用此树，靠 `.env` 与启用进程区分角色。
