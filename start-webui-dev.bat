@echo off
setlocal
cd /d "%~dp0"
where node >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Install Node.js 22.22.2 or a newer supported LTS and add it to PATH.
    exit /b 2
)
node "%~dp0scripts\webui-start.mjs" dev %*
exit /b %errorlevel%
