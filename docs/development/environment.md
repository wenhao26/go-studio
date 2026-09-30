# 本地开发与 Makefile 规范 (Environment & Task Standard)

> **版本**：v1.0.0  
> **适用范围**：Docker 本地环境拉起、快捷构建命令（Makefile）及 Seed 测试数据加载。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [go.md](go.md), [testing.md](testing.md)

---

# 1. 本地 Docker 依赖编排 (docker-compose)

项目根目录必须提供 `docker-compose.yml`，用于在一键拉起本地开发所需的底座服务（MySQL, Redis 等），确保团队和 AI Agent 在隔离且一致的环境下验证代码。

```yaml
version: '3.8'

services:
  mysql:
    image: mysql:8.0
    container_name: local_mysql
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: my_database
    ports:
      - "3306:3306"
    volumes:
      - ./migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 5s
      timeout: 3s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: local_redis
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5
```

---

# 2. 标准 Makefile 命令规范 (Task Execution)

项目必须提供顶层 `Makefile`，将构建、格式化、测试、静态检查命令标准化。**AI Agent 必须优先调用 Makefile 中的预设命令，不得自行发明奇怪的脚本**。

```makefile
.PHONY: all build fmt vet lint test test-race migrate-up clean help

# 默认动作：格式化、检查并编译
all: fmt vet test build

## build: 编译二进制文件
build:
	@echo "Building API server..."
	@go build -ldflags="-s -w" -o bin/api ./cmd/api

## fmt: 格式化 Go 代码
fmt:
	@gofmt -s -w .

## vet: 运行 Go 基础静态检查
vet:
	@go vet ./...

## lint: 运行 golangci-lint
lint:
	@golangci-lint run ./...

## test: 执行单元测试
test:
	@go test -v ./...

## test-race: 执行竞态测试与覆盖率统计
test-race:
	@go test -v -race -coverprofile=coverage.out ./...

## dev-up: 启动本地 Docker 开发容器
dev-up:
	@docker-compose up -d

## dev-down: 停止本地 Docker 容器
dev-down:
	@docker-compose down

## clean: 清理编译产物与临时文件
clean:
	@rm -rf bin/ coverage.out
```

---

# 3. 单元测试与 Fixtures 数据初始化

1. **测试数据隔离**：本地开发和自动化集成测试不得直接写死依赖现有数据库的旧数据。
2. **Seed 脚本规范**：测试用的初始化数据应通过 `scripts/seed.sql` 或 Go 语言中的 `testfixtures` 库在测试启动阶段动态注入，测试结束时清理。
