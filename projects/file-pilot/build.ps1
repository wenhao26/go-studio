# FilePilot 一键打包脚本（Windows PowerShell）
#
# 产物：build/bin/file-pilot.exe（单文件、无 CGO、内嵌 WebView2Loader，可直接分发）
#
# 用法（在 projects/file-pilot/ 目录下执行）：
#   .\build.ps1              # 完整流程：Go 测试 → 前端构建 → wails build → 产物校验
#   .\build.ps1 -SkipTests   # 跳过 Go 测试（仅重新打包时用）
#
# 前置：go / node / npm / wails（v2.16.0）在 PATH；WebView2 Runtime 已安装。
# 说明：任一步失败即中止并给出提示；脚本不改动 go.mod/go.sum，不提交 Git。

[CmdletBinding()]
param(
  [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

function Step([string]$msg) {
  Write-Host ""
  Write-Host "=== $msg ===" -ForegroundColor Cyan
}

function Require-Cmd([string]$name) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    throw "未找到命令 '$name'，请先确认其在 PATH 中。"
  }
}

Step "前置检查"
foreach ($c in 'go', 'node', 'npm', 'wails') {
  Require-Cmd $c
}

if (-not $SkipTests) {
  Step "Go 单元测试"
  go test ./...
  if ($LASTEXITCODE -ne 0) {
    throw 'Go 测试未通过，中止打包。'
  }
}

Step "前端类型检查 + 构建"
Push-Location frontend
try {
  npm run build
  if ($LASTEXITCODE -ne 0) {
    throw '前端构建失败，中止打包。'
  }
}
finally {
  Pop-Location
}

Step "Wails 生产构建"
wails build
if ($LASTEXITCODE -ne 0) {
  throw 'wails build 失败，中止打包。'
}

Step "校验产物"
$exe = 'build/bin/file-pilot.exe'
if (-not (Test-Path $exe)) {
  throw "未找到产物 $exe"
}
$size = (Get-Item $exe).Length
Write-Host ""
Write-Host "[OK] 打包完成" -ForegroundColor Green
Write-Host "  产物：$(Resolve-Path $exe)"
Write-Host "  大小：$size 字节"
