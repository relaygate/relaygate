# RelayGate Panel（Go）

RelayGate 配置运维 + Envoy/Prometheus 状态摘要。详细曲线与告警仍使用 Grafana。

双活场景建议仅在 **primary**（如 `gateway-01`）启用 Panel；`PANEL_ROLE` 是 `.env` 运维约定，进程不读取——standby 请去掉 Compose `with-panel`（见 [`docs/HA.md`](../docs/HA.md)）。

Servers 页支持增删：`POST /api/servers`（自动生成 production 规则模板）、`DELETE /api/servers/{name}`（级联删除关联规则）；改完需到 Apply 渲染并热重载。

## 本地开发

```bash
export PANEL_ADMIN_PASSWORD='dev-password'
export PANEL_ROOT="$(pwd)"   # 仓库根
go run ./cmd/relaygate panel
# 浏览器 http://127.0.0.1:8080
```

## 构建

```bash
bash scripts/build.sh
# 或交叉编译到 Linux:
GOOS=linux GOARCH=amd64 bash scripts/build.sh
```

## Compose

```bash
docker compose -f deploy/compose.yaml --env-file .env up -d --build panel
```

## SSH 隧道

```bash
ssh -p 30455 -L 8080:127.0.0.1:8080 root@107.149.191.37
```

## CLI（同仓库）

```bash
./bin/relaygate render
./bin/relaygate render --check-only
./bin/relaygate server enable server-01
./bin/relaygate server disable server-03
./bin/relaygate server enable --all-production
./bin/relaygate version
```
