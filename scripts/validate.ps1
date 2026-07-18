# Windows local validation: render assets and check outputs
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

$pyCandidates = @(
    "$env:LOCALAPPDATA\Programs\Python\Python312-arm64\python.exe",
    "$env:LOCALAPPDATA\Programs\Python\Python312\python.exe",
    "python"
)
$py = $null
foreach ($c in $pyCandidates) {
    if ($c -eq "python") { $py = $c; break }
    if (Test-Path $c) { $py = $c; break }
}
if (-not $py) { throw "Python not found" }

Write-Host "==> deps"
& $py -m pip install -r requirements.txt -q

Write-Host "==> render/check"
& $py scripts/render_config.py --check-only
if ($LASTEXITCODE -ne 0) { throw "check-only failed" }
& $py scripts/render_config.py
if ($LASTEXITCODE -ne 0) { throw "render failed" }

if (-not (Test-Path "envoy/generated/envoy.yaml")) { throw "missing envoy.yaml" }
if (-not (Test-Path "firewall/generated/game-ports.nft")) { throw "missing game-ports.nft" }

Write-Host "OK: local validation passed. On server run: bash scripts/validate.sh"
