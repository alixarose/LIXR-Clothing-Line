@echo off
setlocal EnableExtensions
cd /d "%~dp0"

title Lixr Source Packager
cls
echo ============================================================
echo                    LIXR SOURCE PACKAGER
echo ============================================================
echo.

where powershell.exe >nul 2>nul
if errorlevel 1 (
  echo [ERROR] PowerShell is required.
  goto :fail
)
where robocopy.exe >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Robocopy is required.
  goto :fail
)

for /f %%i in ('powershell.exe -NoProfile -Command "Get-Date -Format yyyyMMdd-HHmmss"') do set "STAMP=%%i"
if not defined STAMP set "STAMP=source"

set "OUT=%CD%\lixr-source-%STAMP%.zip"
set "STAGE=%TEMP%\lixr_zip_%RANDOM%_%RANDOM%"
set "DEST=%STAGE%\lixr"

if exist "%STAGE%" rmdir /s /q "%STAGE%" >nul 2>nul
mkdir "%DEST%" >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Could not create staging folder.
  goto :fail
)

echo [1/4] Staging clean source...
for %%D in (client server database) do (
  if not exist "%%D\" (
    echo [ERROR] Missing %%D folder.
    goto :stage_fail
  )
  robocopy "%%D" "%DEST%\%%D" /E /R:1 /W:1 /NFL /NDL /NJH /NJS /NC /NS /NP /XD node_modules dist build release releases logs .git .vite .cache tmp temp /XF *.db-wal *.db-shm *.log *.tmp *.zip *.exe *.dll >nul
  if errorlevel 8 (
    echo [ERROR] Failed while staging %%D.
    goto :stage_fail
  )
)

for %%F in (.env run.bat zip.bat) do (
  if not exist "%%F" (
    echo [ERROR] Missing %%F.
    goto :stage_fail
  )
  copy /y "%%F" "%DEST%\%%F" >nul
)

echo [2/4] Removing transient SQLite files...
del /s /q "%DEST%\*.db-wal" "%DEST%\*.db-shm" >nul 2>nul

echo [3/4] Creating ZIP...
if exist "%OUT%" del /q "%OUT%" >nul 2>nul
set "LIXR_STAGE=%STAGE%"
set "LIXR_OUT=%OUT%"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; Compress-Archive -LiteralPath (Join-Path $env:LIXR_STAGE 'lixr') -DestinationPath $env:LIXR_OUT -CompressionLevel Optimal -Force"
if errorlevel 1 (
  echo [ERROR] ZIP creation failed.
  goto :stage_fail
)

echo [4/4] Cleaning staging folder...
rmdir /s /q "%STAGE%" >nul 2>nul

echo.
echo [DONE] %OUT%
echo Contains:
echo   lixr\client\
echo   lixr\server\
echo   lixr\database\
echo   lixr\.env
echo   lixr\run.bat
echo   lixr\zip.bat
echo.
echo information.json is intentionally not included.
pause
exit /b 0

:stage_fail
if exist "%STAGE%" rmdir /s /q "%STAGE%" >nul 2>nul
if defined OUT if exist "%OUT%" del /q "%OUT%" >nul 2>nul

:fail
echo.
echo [FAILED] Source package was not created.
pause
exit /b 1
