# 贡献指南

感谢关注 RelayGate。本项目是基于 Envoy 的通用 L4 网关（开源，MIT）。产品用语见根目录 [README](README.md)「术语」与仓库规则；请使用**主控 / 节点、上游 / 下游 / 入口 / 转发**等中性表述。

## 快速开始（开发）

```bash
# 依赖：Go 1.22+、Node（构建 Panel）、Docker（校验 Envoy / Compose，按 CI）
cp packaging/shared/env.example .env   # 勿提交 .env
make build                             # 含 ui + 二进制 → bin/relaygate
make test
make validate                          # 写入 .runtime/，不入库
```

运行态默认在 `.runtime/`（可用 `RELAYGATE_DATA_DIR`）。模板在 `packaging/**`；密钥与本地 runtime **不要**提交。

## 贡献流程

1. 先搜现有 [Issues](https://github.com/relaygate/relaygate/issues)；安全问题见 [SECURITY.md](SECURITY.md)。
2. Fork + 基于最新 **`main`** 开短生命周期分支；**小步 PR**（单一意图）。集成以 `main` 为准（CI 亦可能校验 `develop`）。
3. 本地：`gofmt`、`go vet ./...`、`go test ./...`；改 UI 时 `make ui` 并说明验证方式。最低依赖：Go（见根目录 `go.mod`）、构建 Panel 需 Node、Compose/Envoy 校验需 Docker（与 CI 一致）。
4. PR 描述：动机、行为变化、如何测试；破坏性变更单独标明，并更新 [CHANGELOG.md](CHANGELOG.md)。
5. 等待 CI（`.github/workflows/ci.yml`）通过后再请求合并。

## DCO（Developer Certificate of Origin）

本项目不强制 CLA。每个提交请附带：

```text
Signed-off-by: Your Name <you@example.com>
```

可用 `git commit -s`。表示你有权按项目许可证提交该变更（[DCO 1.1](https://developercertificate.org/)）。

## 行为准则

参与本仓库即表示遵守 [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)。

## 文档

| 受众 | 位置 |
|------|------|
| 安装与日常运维 | [README.md](README.md)、`packaging/` |
| 设计与深度方案 | `docs/`（不打进 release tar） |

用户可见文案（CLI / Panel）不要依赖仓库内 `docs/` 路径；深度说明放 README 或 `docs/` 供源码读者查阅。
