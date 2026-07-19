#!/usr/bin/env bash
# 构建本机或 Linux 目标二进制到 bin/
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
mkdir -p bin

GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
GOEXE="${GOEXE:-}"
if [[ "$GOOS" == "windows" && -z "$GOEXE" ]]; then
  GOEXE=".exe"
fi

echo "==> build relaygate (${GOOS}/${GOARCH})"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -o "bin/relaygate${GOEXE}" ./cmd/relaygate

echo "OK: bin/relaygate${GOEXE}"
echo "Linux 服务器交叉编译示例: GOOS=linux GOARCH=amd64 bash scripts/build.sh"
