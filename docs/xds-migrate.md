# xDS 迁移步骤（一次性维护窗口）

> 关联：[hot-update-xds.md](hot-update-xds.md) · [fleet-scale-control-plane.md](fleet-scale-control-plane.md)

## 前提

- Panel systemd 常驻（Primary 或需本机 ADS 的节点）
- `GATEWAY_NAME` 与 Envoy `--service-node` 一致
- 双活：先 `relaygate drain fail`，确认 NLB unhealthy
- **新安装默认 `XDS_ENABLED=1`**；仅当 bootstrap 仍是全量 static 时需本页一次性 `--hard` 迁移

## 步骤

1. 确认 `.env` 中 `XDS_ENABLED=1`（新模板默认已开；可选 `XDS_PORT=18000`）
2. `relaygate validate`（生成 bootstrap `envoy.yaml`）
3. **一次** Hard reload 使 Envoy 加载 bootstrap（旧环境从未迁移时必做）：
   ```bash
   relaygate reload --hard
   ```
4. 启动/确认 Panel（内嵌 ADS）或 Secondary 上 `relaygate xds serve`
5. `relaygate diag` — 应看到 xDS 端口监听 + Envoy ready
6. 此后日常变更：`relaygate reload` 或 Panel Apply（Hot 路径，无 drain）

## 回滚 / 未迁移 fallback

- `.env` 设 `XDS_ENABLED=0` — 始终 HardReload（与升级前行为一致）
- 或单次 `relaygate reload --hard`（强制 drain+restart，不依赖 xDS）
- 全量 static 恢复：`relaygate render` + `reload --hard`

## 手工验收（长连接）

```bash
pid1=$(docker inspect -f '{{.State.Pid}}' ${GATEWAY_NAME}-envoy)
# 建立长连接后改无关上游 → reload（Hot）
pid2=$(docker inspect -f '{{.State.Pid}}' ${GATEWAY_NAME}-envoy)
test "$pid1" = "$pid2"
curl -sS 127.0.0.1:9901/ready
```

无 Docker 集成环境：运行 `go test ./core/render/... ./core/xds/... ./core/dataplane/...` 覆盖快照与分流逻辑。
