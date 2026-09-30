# 代码注释与文档编码规范 (Go Commenting & Documentation Standard)

> **版本**：v1.0.0  
> **适用范围**：所有 Go 后端服务、微服务、SDK 及公共基础库开发。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [go.md](go.md), [api.md](api.md)

---

# 1. 核心原则 (Core Principles)

1. **代码即文档 (Code as Documentation)**：好的变量名和清爽的函数设计优先于冗余注释。注释的作用是解释 **Why（为什么这么做）**、**Trade-offs（权衡与限制）** 和 **Caveats（注意事项）**，而不是重复代码逻辑（How/What）。
2. **Godoc 原生兼容**：所有导出（Exported）的标识符（Package、Struct、Interface、Func、Const、Var）必须包含符合 Godoc 规范的注释，保证自动化文档生成工具（如 `pkgsite`、`swag`）能正确解析。
3. **注释与代码同更新**：修改函数逻辑或参数时，**必须同步更新注释**。过期、误导性的注释比没有注释危害更大。
4. **英文/中文规范**：项目内部推荐统一使用简练的中文或英文。但在一个文件内部必须保持语言的一致性。

---

# 2. 包级注释 (Package Comments)

每个 Go 包必须在 package 声明上方包含包级注释。若一个包包含多个文件，只需在其中一个文件（通常是 `doc.go` 或包的核心入口文件如 `user.go`）中编写完整的包注释。

## 2.1 规范格式
- 必须以 `// Package <packagename>` 开头。
- 简述该包的核心职责、使用场景及核心依赖。

```go
// Package auth 提供了基于 JWT 与 Redis 的统一身份认证与 Token 状态管理功能。
// 
// 本包支持 RS256 非对称加密签发，并内置了针对黑名单撤销（Token Revocation）的
// 缓存校验逻辑。通常被 Transport/Handler 层作为 Middleware 调用。
package auth
```

---

# 3. 导出标识符注释 (Exported Identifiers)

所有首字母大写的**导出类型、接口、函数、结构体、常量、变量**必须编写注释。

## 3.1 导出的函数与方法 (Functions & Methods)
- **必须以函数/方法名开头**（Godoc 引擎会自动截取该句作为摘要）。
- 必须包含：**入参特殊要求**、**返回值含义**、**可能返回的 Sentinel Error** 以及 **并发安全性说明**。

```go
// CreateOrder 创建一笔新的用户预支付订单。
//
// 参数：
//   - ctx: 上下文，必须携带 TraceID，支持超时取消。
//   - req: 创建订单入参，req.Amount 必须大于 0。
//
// 返回值：
//   - *OrderResp: 创建成功的订单详情。
//   - error: 当库存不足时返回 ErrInsufficientStock；当触发表限流时返回 ErrRateLimited。
//
// 注意：该方法内部具备幂等保障（依赖 req.IdempotencyKey），并发安全。
func (s *OrderService) CreateOrder(ctx context.Context, req *CreateOrderReq) (*OrderResp, error) {
    // ...
}
```

## 3.2 导出的结构体与字段 (Structs & Fields)
- 结构体注释必须以结构体名开头。
- 结构体内**每个导出的字段**必须在同行或上方说明其业务含义及约束规则。

```go
// UserProfile 表示系统用户的核心画像与基础属性。
type UserProfile struct {
    // UserID 用户全局唯一 ID ( Snowflake 算法生成 )
    UserID int64 `json:"user_id"`

    // Username 用户登录名，长度 4-32 位，不可包含特殊字符
    Username string `json:"username"`

    // Status 用户状态: 1-正常, 2-冻结, 3-已注销
    Status int8 `json:"status"`

    // Mobile 脱敏后的手机号（如 138****5678）
    Mobile string `json:"mobile"`
}
```

## 3.3 导出的接口 (Interfaces)
- 接口注释说明其抽象职责。
- 接口中的**每个方法**也必须编写单独注释。

```go
// UserRepository 定义了用户领域模型持久化与检索的标准接口。
type UserRepository interface {
    // GetByID 根据用户 ID 检索用户信息，若不存在则返回 ErrUserNotFound。
    GetByID(ctx context.Context, id int64) (*model.User, error)

    // UpdateStatus 批量更新用户状态，返回受影响的行数。
    UpdateStatus(ctx context.Context, ids []int64, status int8) (int64, error)
}
```

## 3.4 导出的常量与变量 (Constants & Variables)
- 常量组/变量组可以统一注释，但具有特殊含义的枚举值必须逐行注释。

```go
// 定义订单核心状态机枚举
const (
    // StatusPending 待支付状态，订单创建后的初始状态
    StatusPending int8 = 10

    // StatusPaid 已支付状态，第三方支付回调确认成功
    StatusPaid int8 = 20

    // StatusCanceled 已取消状态，用户手动取消或超时未支付系统自动取消
    StatusCanceled int8 = 30
)
```

---

# 4. 代码块与单行注释 (Inline & Block Comments)

## 4.1 何时编写单行注释
- **复杂算法与业务公式**：解释数学公式推导或状态机转换逻辑。
- **Workaround / 避坑代码**：解释为了规避第三方库 Bug 或历史兼容性问题而写的特殊代码。
- **防踩坑提示**：警告后续维护者不要随意重构此段代码。

```go
// 正确示例：解释 Why
// 规避老版本 MySQL Driver 在高并发下对 time.Time 序列化时的时区丢失 Bug
// 参见 Issue: #1024
formattedTime := t.In(time.UTC).Format(time.RFC3339)

// 错误示例：解释了毫无意义的 What
// 把 count 加 1
count++
```

## 4.2 Guard Clause / 错误处理中的注释
普通的 `if err != nil` 校验**不需要编写注释**，除非错误处理包含特定的降级逻辑（Fallback）。

```go
user, err := repo.GetUser(ctx, id)
if err != nil {
    // 当主库 Redis 缓存击穿且 DB 超时时，触发本地 LRU 缓存降级，保障高可用
    if errors.Is(err, context.DeadlineExceeded) {
        return s.localCache.GetUser(id), nil
    }
    return nil, fmt.Errorf("get user %d: %w", id, err)
}
```

---

# 5. 特殊标记注释 (Special Marker Comments)

对于未完成、待优化或存在隐患的代码，必须使用标准的特有标记，格式为 `// MARKER(author/issue): 详细描述`。

## 5.1 TODO
用于标记尚未实现的功能或有待后续补全的非阻塞逻辑。

```go
// TODO(alex): 接入全链路 OpenTelemetry Trace 埋点，关联上下文 Issue #45
```

## 5.2 FIXME
用于标记**已知有 Bug 或性能缺陷**，但暂未修复、需尽快解决的代码。

```go
// FIXME(jack): 此处的切片未做预分配，在大流量下会导致频繁 GC 内存分配，需重构成 make([]T, 0, len)
```

## 5.3 DEPRECATED
用于标记废弃的 API 或类型，提示调用方迁移方案。

```go
// Deprecated: Use FetchUserProfileV2 instead. 本接口将于 v2.5.0 版本下线。
func FetchUserProfile(ctx context.Context, id int64) (*OldProfile, error) {
    // ...
}
```

---

# 6. OpenAPI / Swagger 注释规范

所有 Transport/Handler 层的 HTTP 接口入口，必须遵循 `Swag` 语法编写可自动化生成 Swagger UI 的规范注释：

```go
// CreateOrderHandler 创建订单 HTTP 接口
//
// @Summary      创建用户预支付订单
// @Description  校验用户余额与库存，生成唯一订单号并锁定库存
// @Tags         订单模块
// @Accept       json
// @Produce      json
// @Param        X-Idempotency-Key header string true "防重提交幂等 Key"
// @Param        request body model.CreateOrderReq true "创建订单请求参数"
// @Success      200 {object} api.Response{data=model.OrderResp} "成功返回订单信息"
// @Failure      400 {object} api.Response "参数校验错误 (Code: 10001)"
// @Failure      401 {object} api.Response "未授权/Token过期"
// @Failure      500 {object} api.Response "服务器内部错误"
// @Router       /api/v1/orders [post]
func (h *OrderHandler) CreateOrderHandler(c *gin.Context) {
    // ...
}
```

---

# 7. 自动化 Lint 检查 (Automated Linting)

为了在 CI/CD 流程中强制执行上述规范，项目中必须集成 `golangci-lint` 并开启以下检查器（Linters）：

1. **`godot`**：检查导出声明的注释是否以句号（`.` 或 `。`）或正常标点结尾。
2. **`godox`**：检测代码中过期的 `TODO` / `FIXME` 标识。
3. **`stylecheck`**：强制实施 Go 官方 Style Guide（包括导出的标识符必须有注释）。

### `.golangci.yml` 推荐配置切片：
```yaml
linters:
  enable:
    - stylecheck
    - godot
    - godox
linters-settings:
  stylecheck:
    checks: ["ST1000", "ST1003", "ST1020", "ST1021", "ST1022"]
  godox:
    keywords:
      - FIXME
      - BUG
```
