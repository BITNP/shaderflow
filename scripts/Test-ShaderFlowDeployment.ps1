[CmdletBinding()]
param(
  [string]$Config = "server.example.json",
  [string]$BaseUrl = "",
  [int]$HealthTimeoutSeconds = 30
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$configPath = if ([System.IO.Path]::IsPathRooted($Config)) { $Config } else { Join-Path $root $Config }
Push-Location $root
try {
  & go run ./cmd/shaderflow -config $configPath -preflight
  if ($LASTEXITCODE -ne 0) { throw "Preflight failed" }
  if ($BaseUrl) {
    $deadline = (Get-Date).AddSeconds($HealthTimeoutSeconds)
    do {
      try {
        $health = Invoke-RestMethod -Uri ($BaseUrl.TrimEnd('/') + '/api/health') -TimeoutSec 5
        if ($health.ok -and $health.storage -eq 'ready') { $health | ConvertTo-Json -Depth 8; return }
      } catch { $lastError = $_ }
      Start-Sleep -Milliseconds 500
    } while ((Get-Date) -lt $deadline)
    throw "Health check did not become ready: $lastError"
  }
} finally { Pop-Location }
