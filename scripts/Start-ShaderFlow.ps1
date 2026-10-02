[CmdletBinding()]
param(
  [string]$Config = "server.example.json",
  [switch]$SkipPreflight,
  [switch]$Build
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$configPath = if ([System.IO.Path]::IsPathRooted($Config)) { $Config } else { Join-Path $root $Config }
if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) { throw "Config not found: $configPath" }
Push-Location $root
try {
  if (-not $SkipPreflight) {
    & go run ./cmd/shaderflow -config $configPath -preflight
    if ($LASTEXITCODE -ne 0) { throw "ShaderFlow preflight failed with exit code $LASTEXITCODE" }
  }
  if ($Build) {
    $output = Join-Path $root "shaderflow.exe"
    & go build -o $output ./cmd/shaderflow
    if ($LASTEXITCODE -ne 0) { throw "ShaderFlow build failed with exit code $LASTEXITCODE" }
    & $output -config $configPath
  } else {
    & go run ./cmd/shaderflow -config $configPath
  }
  if ($LASTEXITCODE -ne 0) { throw "ShaderFlow exited with code $LASTEXITCODE" }
} finally { Pop-Location }
