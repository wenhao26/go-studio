# FilePilot 项目专用命令入口（仅作用于 projects/file-pilot/）。
# Windows PowerShell 等价命令见 README.md「常用命令」；本仓库根目录无 Makefile。

WAILS ?= wails

.PHONY: dev build test test-race fmt vet

dev: ## 前后端联调（热重载）
	$(WAILS) dev

build: ## 生产构建：前端 npm run build + go build（产物 build/bin/）
	$(WAILS) build

test: ## 单元测试
	go test ./...

# -linkmode=external：本机 Go 1.25.4 + MinGW gcc 7.3.0 下，race 测试二进制走内部链接会
# 在进程启动时以 0xC0000139 (STATUS_ENTRYPOINT_NOT_FOUND) 静默退出（纯标准库包同样复现）；
# 改用外部链接后全部通过。结论与复现步骤见 development/00-phase-0-prerequisites.md V8。
# 前置：PATH 中需有可用的 gcc（本机 D:\Program Files\mingw64\bin）。
test-race: ## 并发检测（涉及 goroutine 的改动必须执行）
	go test -race -ldflags=-linkmode=external ./...

fmt: ## 只格式化本项目 Go 文件（避免触发根目录无关 diff）
	gofmt -l -w .

vet: ## 静态检查
	go vet ./...
