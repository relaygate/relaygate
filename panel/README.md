# RelayGate Panel（Go）

RelayGate 配置运维 + Envoy/Prometheus 状态摘要。详细曲线与告警仍使用 Grafana。

双活场景建议仅在 **primary**（如 `gateway-01`）启用 Panel；`PANEL_ROLE` 是 `.env` 运维约定，进程不读取——standby 请设 `ENABLE_PANEL=0`（见 [`docs/HA.md`](../docs/HA.md)）。生产默认以宿主二进制 + systemd（`relaygate-panel`）运行，不在 Compose 中。

Servers 页支持增删：`POST /api/servers`（自动生成 production 规则模板）、`DELETE /api/servers/{name}`（级联删除关联规则）；改完需到 Apply 渲染并热重载。JSON API 仍保留；页面交互走 htmx fragment（`/hx/...`）。

## 前端栈

- **服务端渲染**：Go `html/template`（`web/templates/`）
- **htmx + Alpine.js**：本地 vendor，见 `web/static/vendor/`（禁止生产依赖外链 CDN）
- **Tailwind CSS v4**：已生成的 `web/static/app.css`（由 `web/static/input.css` 构建）；运行 Panel **不需要** Node
- 重建 CSS（可选）：下载 [Tailwind standalone CLI](https://github.com/tailwindlabs/tailwindcss/releases)，执行  
  `./tailwindcss -i web/static/input.css -o web/static/app.css --minify`

## 本地开发

```bash
export PANEL_ADMIN_PASSWORD='dev-password'
export PANEL_ROOT="$(pwd)"   # 仓库根
go run ./cmd/relaygate panel
# 浏览器 http://127.0.0.1:9000
```

## 构建

```bash
bash scripts/build.sh
# 或交叉编译到 Linux:
GOOS=linux GOARCH=amd64 bash scripts/build.sh
```

## 生产部署（systemd）

```bash
sudo bash scripts/install_panel_service.sh
sudo systemctl status relaygate-panel
sudo journalctl -u relaygate-panel -f
```

数据面（Envoy/Prometheus/Grafana）仍用 Compose；勿再 `docker compose ... panel`。

## SSH 隧道

```bash
ssh -p 30455 -L 9000:127.0.0.1:9000 root@107.149.191.37
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
