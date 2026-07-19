#!/usr/bin/env bash
# 构建本机或 Linux 目标二进制到 bin/
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
mkdir -p bin

GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

echo "==> build gateway-render (${GOOS}/${GOARCH})"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -o "bin/gateway-render${GOEXE:-}" ./cmd/gateway-render

echo "==> build gateway-panel (${GOOS}/${GOARCH})"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -o "bin/gateway-panel${GOEXE:-}" ./cmd/gateway-panel

echo "OK: bin/gateway-render bin/gateway-panel"
echo "Linux 服务器交叉编译示例: GOOS=linux GOARCH=amd64 bash scripts/build.sh"
