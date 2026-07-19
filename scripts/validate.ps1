$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
  $candidates = @(
    "C:\Program Files\Go\bin\go.exe",
    "$env:LOCALAPPDATA\Programs\Go\bin\go.exe"
  )
  foreach ($c in $candidates) {
    if (Test-Path $c) { $env:Path = "$(Split-Path $c);$env:Path"; break }
  }
}

Write-Host "==> go mod tidy / build"
go mod tidy
bash scripts/build.sh 2>$null
if (-not (Test-Path "bin/gateway-render.exe") -and -not (Test-Path "bin/gateway-render")) {
  $env:CGO_ENABLED = "0"
  New-Item -ItemType Directory -Force -Path bin | Out-Null
  go build -o bin/gateway-render.exe ./cmd/gateway-render
  go build -o bin/gateway-panel.exe ./cmd/gateway-panel
}

$render = if (Test-Path "bin/gateway-render.exe") { ".\bin\gateway-render.exe" } else { ".\bin\gateway-render" }
Write-Host "==> render/check"
& $render --check-only
if ($LASTEXITCODE -ne 0) { throw "check-only failed" }
& $render
if ($LASTEXITCODE -ne 0) { throw "render failed" }

if (-not (Test-Path "gateway/generated/envoy.yaml")) { throw "missing envoy.yaml" }
if (-not (Test-Path "deploy/firewall/generated/game-ports.nft")) { throw "missing game-ports.nft" }
Write-Host "OK: local validation passed"
