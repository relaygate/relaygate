# 热更新行为

日常本机应用优先 **热更新**（HotApply）：Envoy PID 不变、不摘流。旧节点若仍是全量静态配置，见 [xds-migrate.md](xds-migrate.md)。

## 何时热更新 / 硬重启

| 路径 | 适用 | 确认词 |
|------|------|--------|
| 热更新 | 上游 / 转发 / 多数 Envoy defaults | `HOT_APPLY` |
| 硬重启 | bootstrap、镜像、`meta.admin_*` 等 | `RELOAD_ENVOY`（`reload --hard`） |

- 热更新：无关长连接通常保留；**改/删的监听口**上连接可能中断
- 硬重启：摘流并重启本机 Envoy，该网关上现有连接断开
- 防火墙（nft）与热更新正交，走「应用防火墙」

## 本机 ADS

Envoy 只连接本机 `127.0.0.1` ADS（CDS + LDS，endpoint 内联）。主控可由 Panel 内嵌；网关节点由 `agent run` 联动。不做远程共享 xDS / Istiod。

逃生（一般不需要）：`XDS_ENABLED=0` 时始终硬重启。

## CLI

```bash
relaygate reload          # 优先热更新
relaygate reload --hard   # 强制硬重启
```
