# Go 代码与架构规范 (Go Development Standard)

> **版本**：v1.0.0  
> **适用范围**：所有 Go 后端服务、微服务、命令行工具及核心库开发。  
> **关联规范**：[AGENTS.md](../../AGENTS.md)

---

# 1. 语言版本与工程基础 (Language Version & Base)

## 1.1 Go 版本治理
1. **版本对齐**：必须以根目录 `go.mod` 声明的 Go 版本为最高标准（如 `go 1.22` 或 `go 1.23`）。禁止在低版本 Go 环境中使用尚未稳定或尚未支持的特性（如低版本中使用 Generic type alias 或特定标准库改动）。
2. **依赖管理**：
   - 依赖变更必须通过 `go mod tidy` 维护，禁止直接编辑 `go.mod` / `go.sum`。
   - 禁止在未经过团队 Code Review 的情况下升级主版本（Major Version）依赖。

## 1.2 自动格式化与 Imports 规范
1. **统一格式化**：所有 Go 源码在提交前必须通过 `gofmt -s -w` 格式化，或使用 `goimports` 进行自动整理。
2. **Import 分组结构**：Import 块必须严格按以下顺序进行 3 段分组，组与组之间空一行：
   ```go
   import (
       // 1. 标准库
       "context"
       "fmt"
       "time"

       // 2. 第三方依赖库
       "github.com/gin-gonic/gin"
       "go.uber.org/zap"

       // 3. 本项目内部包 (Internal Package)
       "yourproject/internal/model"
       "yourproject/internal/service"
   )
   ```
3. **禁止匿名 Import**：除数据库驱动（如 `_ "github.com/go-sql-driver/mysql"`）或 Migration 插件外，禁止使用空导入 `_`。

---

# 2. 命名规范 (Naming Conventions)

## 2.1 包名 (Package Names)
- 包名必须纯小写，单个单词，禁止使用下划线（`_`）、中划线（`-`）或驼峰命名（如禁止 `user_service` 或 `userService`，应使用 `user`）。
- 禁止使用通用无语义的包名：`util`, `common`, `helpers`, `misc`。如果涉及字符串处理，建立 `stringutil`；涉及时间处理，建立 `timeutil`。

## 2.2 专有名词与缩写 (Acronyms)
 Go 社区惯例，专有名词（Acronyms）全大写或全小写，保持全一致：
 - **正确**：`userID`, `appAPI`, `httpURL`, `xmlHTTPRequest`, `dbConn`, `jsonString`
 - **错误**：`userId`, `appApi`, `httpUrl`, `xmlHttpRequest`, `dbConnection`, `jsonStr`

## 2.3 接口命名 (Interfaces)
- 单方法接口名称以 `er` 结尾（如 `Reader`, `Writer`, `Validator`, `Storer`）。
- 多方法接口以具体业务名词命名（如 `UserRepository`, `OrderService`）。
- **禁止添加 `I` 前缀**：不要使用 `IUserService`。

---

# 3. 函数设计与控制流 (Functions & Control Flow)

## 3.1 Guard Clauses 与 Early Return
嵌套层级禁止超过 **3 层**。遇到异常或边界条件时，必须立即返回（Early Return），主逻辑保持在最左侧靠齐。

```go
// 错误示例：深层嵌套
func ProcessOrder(ctx context.Context, order *Order) error {
    if order != nil {
        if order.Status == StatusPending {
            err := validateOrder(order)
            if err == nil {
                return saveOrder(ctx, order)
            } else {
                return err
            }
        } else {
            return ErrInvalidStatus
        }
    } else {
        return ErrNilOrder
    }
}

// 正确示例：Guard Clauses
func ProcessOrder(ctx context.Context, order *Order) error {
    if order == nil {
        return ErrNilOrder
    }
    if order.Status != StatusPending {
        return ErrInvalidStatus
    }
    if err := validateOrder(order); err != nil {
        return fmt.Errorf("validate order: %w", err)
    }
    return saveOrder(ctx, order)
}
```

## 3.2 构造函数与 Options 模式
对于配置项复杂、可选参数较多的结构体，必须使用 **Functional Options** 模式，禁止传一长串 `nil` 或零值参数。

```go
type Server struct {
    host    string
    port    int
    timeout time.Duration
}

type Option func(*Server)

func WithTimeout(timeout time.Duration) Option {
    return func(s *Server) {
        s.timeout = timeout
    }
}

func NewServer(host string, port int, opts ...Option) *Server {
    srv := &Server{
        host:    host,
        port:    port,
        timeout: 30 * time.Second, // 默认值
    }
    for _, opt := range opts {
        opt(srv)
    }
    return srv
}
```

---

# 4. Context 规范 (Context Standards)

1. **绝对首位**：`context.Context` 必须作为函数的第一个参数，变量名统一为 `ctx`。
2. **显式传递**：禁止将 `ctx` 存储在结构体成员变量中（除 `http.Request` 内置的 `ctx` 外）。
3. **不得污染 Context**：`context.WithValue` 仅能用于传递 Request-scoped 的元数据（如 `trace_id`, `client_ip`, `current_user_id`），**严禁用于传递业务隐式参数**。传递 Value 的 Key 必须是私有自定义类型：
   ```go
   type ctxKeyTraceID struct{}
   
   func WithTraceID(ctx context.Context, traceID string) context.Context {
       return context.WithValue(ctx, ctxKeyTraceID{}, traceID)
   }
   ```
4. **生命周期控制**：派生出 `WithTimeout` 或 `WithCancel` 时，必须使用 `defer cancel()` 防止 Context 泄露。

---

# 5. 异常与 Error 处理 (Error Handling & Panic)

## 5.1 错误包装 (Error Wrapping)
- 在跨层级传递 error 时，必须通过 `%w` 向上包装并追加当前步骤上下文描述：
  ```go
  if err != nil {
      return fmt.Errorf("query user by id %d: %w", userID, err)
  }
  ```
- 判等必须使用 `errors.Is` 或 `errors.As`，绝对禁止通过 `err.Error() == "..."` 匹配字符串。

## 5.2 Sentinel Error 与 自定义 Error 类型
- 业务常见可预见的错误必须在 package 顶层定义 Sentinel Error：
  ```go
  var (
      ErrUserNotFound = errors.New("user not found")
      ErrForbidden    = errors.New("permission denied")
  )
  ```

## 5.3 Panic 的严苛限制
- **业务代码禁止 panic**：HTTP Handler、RPC Service、Cron Job 中绝对禁止抛出 `panic`。
- **允许场景**：服务启动初始化阶段（如 `main()` 或 `init()`）遇到不可修复的依赖缺失（如数据库连接失败、必填环境变量丢失），可以使用 `log.Fatalf` 或 `panic` 终止程序。

---

# 6. 并发与 Goroutine 安全 (Concurrency & Safety)

## 6.1 Goroutine 生命周期控制
- 启动 Goroutine 时，必须明确知道它何时退出、如何退出。禁止启动无人管辖的“孤儿 Goroutine”。
- 多任务并发推荐使用 `golang.org/x/sync/errgroup` 并绑定 `ctx` 进行超时与取消联动：

```go
g, ctx := errgroup.WithContext(parentCtx)
g.Go(func() error {
    return fetchUserData(ctx, id)
})
g.Go(func() error {
    return fetchUserOrders(ctx, id)
})
if err := g.Wait(); err != nil {
    return fmt.Errorf("parallel fetch failed: %w", err)
}
```

## 6.2 Data Race 防范
- 在多线程读写的数据结构上，必须使用 `sync.RWMutex` 或 `atomic` 包保护。
- 自动化检查：在 CI 流程和本地单元测试中，必须添加 `-race` 标志运行：`go test -race ./...`。

---

# 7. 静态检查与验证 (Static Check)

1. **go vet**：任何提交必须通过 `go vet ./...`。
2. **golangci-lint 必选 linter 列表**：
   - `errcheck`（检查未处理的 error）
   - `staticcheck`（Go 官方高级静态分析）
   - `govet`（基础语法与潜在风险分析）
   - `ineffassign`（检查无效赋值）
   - `goconst`（重复常量提取）
