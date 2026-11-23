@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

title Lixr Launcher
cls
echo ============================================================
echo                         LIXR LAUNCHER
echo ============================================================
echo Root     : %CD%
echo Frontend : HTML / CSS / JavaScript
echo Backend  : Go
echo Database : SQLite
set "SERVER_PORT=3017"
for /f "tokens=2 delims==" %%P in ('findstr /B /C:"SERVER_PORT=" ".env" 2^>nul') do set "SERVER_PORT=%%P"
set "SERVER_PORT=%SERVER_PORT: =%"

echo URL 127  : http://127.0.0.1:%SERVER_PORT%
echo URL host : http://localhost:%SERVER_PORT%
echo.

if not exist "server\go.mod" (
  echo [ERROR] Missing server\go.mod
  goto :fail
)
if not exist "database\schema.sql" (
  echo [ERROR] Missing database\schema.sql
  goto :fail
)
if not exist "database\seed.sql" (
  echo [ERROR] Missing database\seed.sql
  goto :fail
)
if not exist ".env" (
  echo [ERROR] Missing .env
  goto :fail
)

call :ensure_go
if errorlevel 1 goto :fail

echo [1/5] Go:
go version
if errorlevel 1 goto :fail

pushd "server" >nul

echo.
echo [2/5] Downloading required Go modules...
go mod download
if errorlevel 1 (
  echo [WARN] go mod download failed. Retrying with go mod tidy...
  go mod tidy
  if errorlevel 1 (
    popd >nul
    echo [ERROR] Go dependencies could not be downloaded.
    echo Check internet access and GOPROXY, then run this file again.
    goto :fail
  )
)

echo.
echo [3/5] Repairing and verifying module metadata...
go mod tidy
if errorlevel 1 (
  popd >nul
  echo [ERROR] go mod tidy failed.
  goto :fail
)
go mod verify
if errorlevel 1 (
  popd >nul
  echo [ERROR] Go module verification failed.
  goto :fail
)

echo.
echo [4/5] Waiting for the server before opening the browser...
start "" /b powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -Command "$u='http://127.0.0.1:%SERVER_PORT%/api/health'; for($i=0;$i -lt 90;$i++){ try { $r=Invoke-WebRequest -UseBasicParsing -Uri $u -TimeoutSec 1; if($r.StatusCode -eq 200){ Start-Process 'http://127.0.0.1:%SERVER_PORT%'; exit 0 } } catch {}; Start-Sleep -Milliseconds 500 }" >nul 2>nul

echo.
echo [5/5] Starting Lixr with: go run .
echo Server console is active below. Press Ctrl+C to stop.
echo ------------------------------------------------------------
go run .
set "EXIT_CODE=%ERRORLEVEL%"
popd >nul

if not "%EXIT_CODE%"=="0" (
  echo.
  echo [ERROR] Lixr stopped with exit code %EXIT_CODE%.
  goto :fail
)
exit /b 0

:ensure_go
where go >nul 2>nul
if not errorlevel 1 exit /b 0

echo [SETUP] Go was not found in PATH.
echo [SETUP] Attempting automatic installation with winget...
where winget >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go is missing and winget is unavailable.
  echo Install Go from https://go.dev/dl/ then run run.bat again.
  exit /b 1
)

winget install --id GoLang.Go --exact --accept-package-agreements --accept-source-agreements
if errorlevel 1 (
  echo [ERROR] Automatic Go installation failed.
  exit /b 1
)

if exist "%ProgramFiles%\Go\bin\go.exe" set "PATH=%ProgramFiles%\Go\bin;%PATH%"
where go >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go installed but this terminal cannot find it yet.
  echo Close this window and run run.bat again.
  exit /b 1
)
exit /b 0

:fail
echo.
echo ============================================================
echo [FAILED] Lixr did not start successfully.
echo ============================================================
pause
exit /b 1
