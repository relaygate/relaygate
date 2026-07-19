$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
  $candidates = @(
    "C:\Program Files\Go\bin\go.exe",
    "$env:LOCALAPPDATA\Programs\Go\bin\go.exe",
    "$env:LOCALAPPDATA\go-sdk\go\bin\go.exe"
  )
  foreach ($c in $candidates) {
    if (Test-Path $c) { $env:Path = "$(Split-Path $c);$env:Path"; break }
  }
}

if (-not $env:GATEWAY_NAME) { $env:GATEWAY_NAME = "gateway-01" }
if (-not $env:ENVOY_ADMIN_PORT) { $env:ENVOY_ADMIN_PORT = "9901" }

Write-Host "==> render observability (gateway=$($env:GATEWAY_NAME))"
$tpl = "deploy/prometheus/prometheus.yml.tpl"
$out = "deploy/prometheus/prometheus.yml"
if (-not (Test-Path $tpl)) { throw "missing $tpl" }
$content = Get-Content -Raw $tpl
$content = $content.Replace('${GATEWAY_NAME}', $env:GATEWAY_NAME)
$content = $content.Replace('${ENVOY_ADMIN_PORT}', $env:ENVOY_ADMIN_PORT)
$content = $content.Replace('${PROMETHEUS_REMOTE_WRITE_URL}', "")
Set-Content -Path $out -Value $content -NoNewline

Write-Host "==> go mod tidy / build"
go mod tidy
$env:CGO_ENABLED = "0"
New-Item -ItemType Directory -Force -Path bin | Out-Null
go build -o bin/relaygate.exe ./cmd/relaygate
if ($LASTEXITCODE -ne 0) { throw "relaygate build failed" }

$relaygate = ".\bin\relaygate.exe"
Write-Host "==> render/check"
& $relaygate render --check-only
if ($LASTEXITCODE -ne 0) { throw "check-only failed" }
& $relaygate render
if ($LASTEXITCODE -ne 0) { throw "render failed" }

if (-not (Test-Path "gateway/generated/envoy.yaml")) { throw "missing envoy.yaml" }
if (-not (Test-Path "deploy/firewall/generated/game-ports.nft")) { throw "missing game-ports.nft" }
if (-not (Select-String -Path $out -Pattern "gateway: $($env:GATEWAY_NAME)" -Quiet)) {
  throw "prometheus.yml missing gateway label"
}
Write-Host "OK: local validation passed"
