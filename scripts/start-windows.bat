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
set "ZZZ_PORT=58463"
if not "%~2"=="" set "ZZZ_PORT=%~2"
if /I "%~1"=="lan" (
  set "LISTEN_ADDR=0.0.0.0:%ZZZ_PORT%"
  echo [ZzzMessage] LAN mode: binding to all interfaces, port %ZZZ_PORT%.
  echo [ZzzMessage] LAN origins allowed by LAN_ORIGIN_CIDRS in .env.
  echo [ZzzMessage] Use a Private-network, LocalSubnet-only Windows Firewall rule.
) else (
  set "LISTEN_ADDR=127.0.0.1:%ZZZ_PORT%"
  echo [ZzzMessage] Local mode: http://127.0.0.1:%ZZZ_PORT%
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
