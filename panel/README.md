# RelayGate Panel（Go）

RelayGate 配置运维 + Envoy/Prometheus 状态摘要（主机 `gateway-01`）。详细曲线与告警仍使用 Grafana。

## 本地开发

```bash
export PANEL_ADMIN_PASSWORD='dev-password'
export PANEL_ROOT="$(pwd)"   # 仓库根
go run ./cmd/gateway-panel
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
./bin/gateway-render                 # 渲染 Envoy + nft
./bin/gateway-render --check-only
./bin/gateway-render enable server-01
./bin/gateway-render disable server-03
./bin/gateway-render enable --all-production
```
