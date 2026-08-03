# 旧节点：一次性硬重启

新安装默认已开启热更新。仅当磁盘 `envoy.yaml` 仍是全量 `static_resources`（无 `dynamic_resources`）时，在维护窗口执行一次硬重启。行为说明见 [hot-update-xds.md](hot-update-xds.md)。

## 步骤

1. 双活：先 `relaygate drain fail`，确认入口 unhealthy
2. `relaygate validate`
3. **一次**硬重启：`relaygate reload --hard`
4. 确认 Panel（主控）或 `agent run`（节点）在跑
5. `relaygate diag` — 应看到热更新就绪 + Envoy ready
6. 此后日常：`relaygate reload` 或 Panel 本机应用（热更新）

逃生（一般不需要）：`XDS_ENABLED=0` 强制始终硬重启。

## 手工验收（长连接）

```bash
pid1=$(docker inspect -f '{{.State.Pid}}' ${GATEWAY_NAME}-envoy)
# 建立长连接后改无关上游 → reload（热更新）
pid2=$(docker inspect -f '{{.State.Pid}}' ${GATEWAY_NAME}-envoy)
test "$pid1" = "$pid2"
curl -sS 127.0.0.1:9901/ready
```
