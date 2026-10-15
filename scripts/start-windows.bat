@echo off
setlocal EnableExtensions
cd /d "%~dp0" || exit /b 1

if not exist "ZzzMessage.exe" (
  echo [ERROR] ZzzMessage.exe not found in this directory.
  pause
  exit /b 1
)
if not exist "web\index.html" (
  echo [ERROR] web\index.html not found. Extract the whole Windows ZIP.
  pause
  exit /b 1
)
if not exist ".env" (
  if exist ".env.example" copy /Y ".env.example" ".env" >nul
  echo [ERROR] Configure INVITE_CODE and SESSION_SECRET in .env first.
  if exist ".env" notepad ".env"
  pause
  exit /b 1
)

set "WEB_DIR=%CD%\web"
if /I "%~1"=="lan" (
  set "LISTEN_ADDR=0.0.0.0:58463"
  echo [ZzzMessage] LAN mode: binding to all network interfaces, port 58463.
  echo [ZzzMessage] IMPORTANT: Add http://YOUR_LAN_IP:58463 to ALLOWED_ORIGINS in .env.
  echo [ZzzMessage] Use a Private-network, LocalSubnet-only Windows Firewall rule.
) else (
  set "LISTEN_ADDR=127.0.0.1:58463"
  echo [ZzzMessage] Local mode: http://127.0.0.1:58463
)
echo [ZzzMessage] Static web directory: %WEB_DIR%
echo [ZzzMessage] Press Ctrl+C to stop the server.
echo.
"ZzzMessage.exe"
set "EXIT_CODE=%ERRORLEVEL%"
echo.
echo [ZzzMessage] Server exited with code %EXIT_CODE%.
pause
exit /b %EXIT_CODE%
