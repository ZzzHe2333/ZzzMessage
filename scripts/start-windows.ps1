$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot
if (-not (Test-Path -LiteralPath ".env")) {
  Copy-Item ".env.example" ".env"
  Write-Host "请先在 .env 中设置 INVITE_CODE (至少12位) 和 SESSION_SECRET (至少32位)，再运行。" -ForegroundColor Yellow
  exit 1
}
if (-not $env:WEB_DIR) { $env:WEB_DIR = Join-Path $PSScriptRoot "web" }
if (-not $env:LISTEN_ADDR) { $env:LISTEN_ADDR = "127.0.0.1:58463" }
& (Join-Path $PSScriptRoot "ZzzMessage.exe")
exit $LASTEXITCODE
