# fast-stat 项目任务入口。
#
# 说明：
#   - 本模块的 go.mod 位于仓库根目录（多项目共享），因此这里的包路径都相对本目录；
#     在 projects/fast-stat 下执行时，Go 会自动向上定位模块根。
#   - fmt 目标只作用于本项目源码，不触碰仓库中的其他项目
#     （AGENTS.md §84：禁止无关格式化造成大规模 diff）。
#   - 本 Makefile 使用 POSIX shell 语法；Windows 上请在 Git Bash 中执行 make。

BINARY     := faststat
BIN_DIR    := bin
CMD_PKG    := ./cmd/faststat
SRC_DIRS   := ./cmd ./internal

VERSION    ?= 1.0.0
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)

MODULE     := github.com/wenhao26/go-studio/projects/fast-stat
LDFLAGS    := -s -w \
	-X '$(MODULE)/internal/version.Version=$(VERSION)' \
	-X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/version.Date=$(BUILD_DATE)'

.PHONY: all build fmt fmt-check vet lint test test-race cover bench clean run help

# 默认动作：格式校验、静态检查、测试、编译
all: fmt-check vet test build

## build: 编译二进制（注入版本信息）
build:
	@go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

## fmt: 格式化本项目源码
fmt:
	@gofmt -s -w $(SRC_DIRS)

## fmt-check: 只读校验格式，输出非空即失败（CI 门禁）
fmt-check:
	@out="$$(gofmt -l $(SRC_DIRS))"; \
	if [ -n "$$out" ]; then \
		echo "以下文件未通过 gofmt:"; echo "$$out"; exit 1; \
	fi
	@echo "gofmt: ok"

## vet: 运行 go vet
vet:
	@go vet $(SRC_DIRS)/... 

## lint: 运行 golangci-lint（需本地已安装）
lint:
	@golangci-lint run $(SRC_DIRS)/...

## test: 单元测试
test:
	@go test $(SRC_DIRS)/...

## test-race: 竞态检测并输出覆盖率
## 注意：Windows 上 race 检测需要 MinGW-w64 GCC 8+；GCC 7 及更早版本会因
## 运行时缺少符号而报 STATUS_ENTRYPOINT_NOT_FOUND。
test-race:
	@go test -race -coverprofile=coverage.out $(SRC_DIRS)/...

## cover: 查看覆盖率报告（需先执行 test-race）
cover: test-race
	@go tool cover -func=coverage.out

## bench: 运行基准测试
bench:
	@go test -run '^$$' -bench . -benchmem $(SRC_DIRS)/...

## run: 本地运行，统计当前目录
run:
	@go run $(CMD_PKG) --path .

## clean: 清理构建产物与覆盖率文件
clean:
	@rm -rf $(BIN_DIR) coverage.out

## help: 显示可用命令
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
