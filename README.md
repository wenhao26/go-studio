# go-studio

> 基于 Go 的后端服务工程。**本仓库当前处于"规范先行"阶段**：开发规范、架构约定与 AI 协作准则已就位，Go 工程骨架（`go.mod` / `cmd/` / `internal/` / `Makefile` 等）尚待初始化。

---

## 1. 项目定位

`go-studio` 是一个采用**整洁架构 / 传统三层架构**的 Go 后端服务工程，遵循"正确性优先"原则：

```text
正确性 > 安全性 > 可维护性 > 可测试性 > 可观测性 > 性能 > 简洁性 > 开发速度
```

本项目同时是 **AI Coding / Vibe Coding 的受约束工程**：AI Agent 与人工开发共享同一套强制规范，目标是以**最小、可审查、可测试、可回滚**的代码变更安全地实现明确需求，而非"生成更多代码"。

---

## 2. 当前状态（如实说明）

| 项目 | 状态 |
|---|---|
| 开发规范体系（`AGENTS.md` + `docs/`） | ✅ 已建立 |
| AI 协作对齐报告 | ✅ 已建立（`PROJECT_ALIGNMENT_REPORT.md`） |
| `go.mod` / Go 版本 | ❌ 未初始化 |
| 工程骨架（`cmd/` `internal/` `pkg/`） | ❌ 未初始化 |
| `Makefile` / `docker-compose.yml` | ❌ 未初始化 |
| `.env.example` / `.golangci.yml` | ❌ 未初始化 |
| 数据库 Migration | ❌ 未初始化 |

> 本文档中的"快速开始"命令为规范定义的目标形态，在工程骨架落地前**不可直接执行**。

---

## 3. 目录结构

```text
go-studio/
├── AGENTS.md                      # 【最高准则】项目级 AI Coding / 开发规范
├── PROJECT_ALIGNMENT_REPORT.md    # 规范对齐与 Vibe Coding 前置认知报告
├── README.md                      # 本文件
├── docs/
│   ├── README.md                  # 规范导航地图（索引 + 使用方式）
│   └── development/               # 专项工程规范
│       ├── go.md                  # Go 语言与编码规范
│       ├── architecture.md        # 架构分层与依赖注入
│       ├── domain.md              # 领域模型、状态机、ID、金额
│       ├── api.md                 # API 路由、统一响应、错误码、幂等
│       ├── database.md            # MySQL Schema、索引、GORM、Redis
│       ├── config.md              # 强类型配置与多环境矩阵
│       ├── observability.md       # Zap 结构化日志、TraceID、Metrics
│       ├── security.md            # 认证授权、注入防御、脱敏、限流
│       ├── testing.md             # 覆盖率、表格驱动、Mock、Race
│       ├── dependencies.md        # 技术栈白名单 / 黑名单
│       ├── commenting.md          # Godoc 注释、TODO/FIXME、Swag
│       └── environment.md         # Docker Compose、Makefile、Seed
└── projects/                      # 预留目录
```

> 目标工程结构（落地后）遵循 `golang-standards/project-layout`：`cmd/api`、`cmd/worker`、`internal/{config,handler,service,repository,model,pkg}`、`api/`、`migrations/`、`scripts/`。

---

## 4. 规范体系（开发前必读）

| 文档 | 职责 | 核心关注点 |
|---|---|---|
| [`AGENTS.md`](AGENTS.md) | **全局最高准则** | 第一原则、最小修改、禁止伪实现与假测试、标准工作流、20 条底线 |
| [`docs/README.md`](docs/README.md) | 规范导航地图 | 各专项规范索引与使用方式 |
| [`docs/development/go.md`](docs/development/go.md) | Go 编码规范 | 包命名、Guard Clause、`%w` 错误链、Goroutine 安全、golangci-lint |
| [`docs/development/architecture.md`](docs/development/architecture.md) | 架构分层 | Handler/Service/Repository 边界、依赖注入、循环依赖解耦 |
| [`docs/development/domain.md`](docs/development/domain.md) | 领域模型 | 通用词汇表、订单状态机矩阵、Snowflake/UUID v7、金额精度 |
| [`docs/development/api.md`](docs/development/api.md) | API 规范 | RESTful 路由、统一响应体、5 位错误码、分页、幂等 `X-Idempotency-Key` |
| [`docs/development/database.md`](docs/development/database.md) | 存储规范 | 表设计模板、索引规范、禁 `SELECT *`、防 N+1、Redis TTL、Migration |
| [`docs/development/config.md`](docs/development/config.md) | 配置规范 | 强类型 Config + 启动校验、`.env.example`、多环境矩阵 |
| [`docs/development/observability.md`](docs/development/observability.md) | 可观测性 | Zap 结构化字段、OTel TraceID、Prometheus 指标、日志脱敏 |
| [`docs/development/security.md`](docs/development/security.md) | 安全规范 | JWT 非对称签名、BOLA/IDOR、SQL/命令/SSRF 防御、限流、安全头 |
| [`docs/development/testing.md`](docs/development/testing.md) | 测试规范 | Service ≥70% / pkg ≥85%、表格驱动、Mock 策略、`-race` |
| [`docs/development/dependencies.md`](docs/development/dependencies.md) | 依赖管理 | 批准白名单、禁用黑名单、引入评估与 `go mod tidy` 流程 |
| [`docs/development/commenting.md`](docs/development/commenting.md) | 注释规范 | 包注释、导出标识符 Godoc、TODO/FIXME 标记、Swag 注释 |
| [`docs/development/environment.md`](docs/development/environment.md) | 本地环境 | Docker Compose 一键启动、Makefile 标准指令、Seed 数据 |

---

## 5. 架构分层约定

```text
[ Transport / Handler ]        解析请求、参数校验、组装响应
            ↓
[ Application / Service ]      业务流程编排、状态机、事务边界、权限判定
            ↓
[ Domain / Model ]             业务实体、值对象、DTO
            ↓
[ Infrastructure / Repository ] MySQL / Redis / 外部 API
```

- 依赖方向严格**由外向内单向**，禁止反向依赖与**跨层调用**（Handler 不得直连 Repository 或写 SQL）。
- 禁止使用全局变量持有依赖，一律**构造函数显式注入** Interface；接口由**消费方定义**且保持极小。
- 禁止反射式隐式注入（如 `uber-go/dig`）；大型项目使用编译期静态生成的 `google/wire`。

---

## 6. AI 协作工作流（强制）

任何非 trivial 修改必须走**双阶段工作流**：

**阶段一 · 分析与方案（不写业务代码）**

1. 复述并确认需求边界（明确"不做什么"）
2. 阅读现状：结构 → `go.mod` → 目标文件 → 调用方/被调用方 → 相关测试
3. 输出**改动文件清单**，标注是否触及高风险面（API contract / DB schema / 依赖 / 公共接口 / 生成物）
4. 输出**架构设计方案**（分层落点、接口签名、DTO 链路、状态机、事务与幂等、日志埋点、测试矩阵）
5. 输出**验证计划**与**待确认问题**
6. **等待人工确认**

**阶段二 · 实施与验证（确认后执行）**

1. 最小变更实现 + 同步补测试
2. 门禁顺序：`gofmt -l .`（断言为空）→ `gofmt -s -w`（仅改动文件）→ 相关包测试 → `go test ./...` → `go vet ./...` → 并发时 `go test -race ./...`
3. `git diff` 自检：无 debug code、无临时文件、无 secret、无格式噪音、无误删
4. 如实报告：**修改内容 / 实际执行的验证命令 / 遗留风险**（严禁谎报测试通过）

> 三个强制停止条件：**信息不足**（不臆测）、**不可逆或高影响操作**（先确认）、**方案未获确认**（不写码）。

详见 [`PROJECT_ALIGNMENT_REPORT.md`](PROJECT_ALIGNMENT_REPORT.md) §4、§5。

---

## 7. 编码风格与质量门禁

- **唯一格式化权威 = Go 官方 `gofmt`（`-s`）**；GoLand 必须校准为 `gofmt` Code Style（禁用默认 `GoLand` 风格），缩进一律 **Tab**。
- `gofmt` 不折长行、不做 import 语义排序 —— **禁止任何个人审美式排版**。
- import 严格三段：标准库 / 第三方 / 本项目内部包，段间空行（除驱动与迁移插件外禁止匿名导入）。
- 错误链用 `%w` + `errors.Is/As`，禁止字符串比对错误；`Sentinel Error` 集中定义复用；业务代码禁止 `panic`。
- 日志强制 `go.uber.org/zap` 结构化输出，必带 `trace_id`；禁止 `Infof` 拼接字符串；敏感字段必须脱敏。
- 测试：表格驱动、覆盖边界与错误路径，**禁止 `time.Sleep` 等待**，禁止为通过测试而修改业务语义。

门禁命令（工程骨架落地后生效）：

```bash
make fmt        # gofmt -s -w（仅改动文件）
make vet        # go vet ./...
make lint       # golangci-lint run ./...
make test       # go test -v ./...
make test-race  # go test -v -race -coverprofile=coverage.out ./...
```

---

## 8. 技术栈约定

| 领域 | 批准使用 | 禁止 |
|---|---|---|
| Web 框架 | `gin` 或 `chi` | Beego 等全家桶 |
| ORM / 驱动 | `gorm` 或 `sqlx` | — |
| Redis | `redis/go-redis/v9` | `redigo` |
| 配置 | `spf13/viper` | — |
| 日志 | `go.uber.org/zap` | `logrus`、`log/syslog` |
| 错误处理 | 标准库 `fmt.Errorf("%w")` + `errors.Is/As` | `github.com/pkg/errors` |
| 并发 | `golang.org/x/sync/errgroup` | 无责任 goroutine |
| 测试 | `stretchr/testify`、`go.uber.org/mock` | 真实连生产库 |
| DI | 手动组装 / `google/wire` | 反射式注入（`uber-go/dig`） |

> ⚠️ 白名单存在**待裁决缺口**（`validator` / `swag` / `otel` / JWT 库等被其他规范强制要求但不在白名单内），详见对齐报告 §5.4 的 C1–C10。

---

## 9. 已知待决策事项

以下为规范体系交叉复核发现的内部冲突/缺口，**需项目所有者裁决后方可落地**（完整清单见 [`PROJECT_ALIGNMENT_REPORT.md`](PROJECT_ALIGNMENT_REPORT.md) §5.4）：

- **C1** 依赖白名单未覆盖其他规范强制要求的库（`validator`、`swaggo/swag`、`otel`、JWT、Snowflake/UUIDv7、`clockwork`、`testcontainers-go`）
- **C2** `environment.md` 的 `fmt: gofmt -s -w .` 与"禁止无关格式化"冲突
- **C3** 主键策略二义：自增主键 vs 统一 Snowflake/UUID v7
- **C4** DTO 目录归属未定义；`api.Response` 与 `api/` 目录命名撞车
- **C5** `internal/pkg/` 与"禁止万能包"的边界
- **C6** 状态机六态 vs `database.md` 示例 COMMENT 仅三态
- **C8** `docs/development/git.md` 索引存在但文件缺失
- **C9** `AGENTS.md` §105 项目特定规则占位区待补充
- **C10** Go 版本未定（影响 `any` / 泛型 / `errors.Join` 等写法）

---

## 10. 提交规范

采用 [Conventional Commits](https://www.conventionalcommits.org/)：

```text
<type>(<scope>): <subject>

<body>

<footer>
```

- **type**：`feat` / `fix` / `docs` / `refactor` / `test` / `chore` / `perf` / `build` / `ci`
- **subject**：祈使句、简洁、说明"做了什么"，不加句号
- **body**：说明"为什么"、影响范围、必要权衡
- 破坏性变更在 footer 标注 `BREAKING CHANGE:`

提交前必须确认：`gofmt` 无输出、`go vet` 通过、相关测试通过、`git diff` 无无关改动与临时文件、无 secret 泄漏。

---

## 11. 许可

待补充（尚未确定 License）。
