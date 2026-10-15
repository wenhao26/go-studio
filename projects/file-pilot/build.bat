@echo off
REM ============================================================
REM  FilePilot one-click build script (Windows cmd)
REM  Output: build/bin/file-pilot.exe (single file, no CGO)
REM  Usage:
REM    build.bat              Full: Go test -> frontend -> wails build -> verify
REM    build.bat -SkipTests   Skip Go tests
REM  Prereq: go / node / npm / wails (v2.16.0) in PATH
REM ============================================================
setlocal
cd /d "%~dp0"

echo === Prerequisites ===
call :require go
call :require node
call :require npm
call :require wails

if /i not "%1"=="-SkipTests" if /i not "%1"=="/SkipTests" (
  echo === Go tests ===
  go test ./...
  if errorlevel 1 (
    echo [ERROR] Go tests failed, abort.
    exit /b 1
  )
)

echo === Frontend build ===
pushd frontend
call npm run build
if errorlevel 1 (
  popd
  echo [ERROR] Frontend build failed, abort.
  exit /b 1
)
popd

echo === Wails build ===
call wails build
if errorlevel 1 (
  echo [ERROR] wails build failed, abort.
  exit /b 1
)

echo === Verify output ===
set EXE=build\bin\file-pilot.exe
if not exist "%EXE%" (
  echo [ERROR] Output not found: %EXE%
  exit /b 1
)
for %%A in ("%EXE%") do echo   Size: %%~zA bytes
echo.
echo [OK] Build complete
echo   Output: %CD%\%EXE%
exit /b 0

:require
where %~1 >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Command '%~1' not found in PATH.
  exit /b 1
)
exit /b 0
