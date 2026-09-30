# API 接口设计与 RESTful 规范 (API Standard)

> **版本**：v1.0.0  
> **适用范围**：面向 Web、App 及第三方 Open API 的 HTTP/gRPC 接口设计。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [go.md](go.md)

---

# 1. URL 路由与 HTTP 方法规范 (Routing & HTTP Methods)

## 1.1 RESTful 资源化路径
- URL 中必须使用小写、中划线（Kebab-case）分隔的复数名词表示资源，**禁止在路径中出现动词**。
  - **正确**：`GET /api/v1/users`, `POST /api/v1/orders/123/cancel-requests`
  - **错误**：`GET /api/v1/getUser`, `POST /api/v1/createOrder`

## 1.2 HTTP Standard Methods 映射
| 动作 | HTTP Method | 路径示例 | 含义 |
|---|---|---|---|
| 获取列表 | `GET` | `/api/v1/orders` | 翻页获取订单列表 |
| 获取单条 | `GET` | `/api/v1/orders/{id}` | 获取指定 ID 订单详情 |
| 创建资源 | `POST` | `/api/v1/orders` | 创建新订单 |
| 替换资源 | `PUT` | `/api/v1/users/{id}` | 全量更新用户信息 |
| 局部更新 | `PATCH` | `/api/v1/users/{id}` | 增量更新指定字段 |
| 删除资源 | `DELETE` | `/api/v1/orders/{id}` | 删除指定订单 |

---

# 2. 统一响应结构 (Standard Response Format)

所有 HTTP Response 必须统一返回 `JSON` 格式，主结构体定义如下：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1024,
    "username": "alex",
    "created_at": "2026-09-30T10:00:00Z"
  },
  "trace_id": "c3b8a910-3882-41f2-897e-128a1a9eef11"
}
```

## Go Response DTO 定义
```go
type Response struct {
    Code    int         `json:"code"`               // 业务自定义错误码，0 代表成功
    Message string      `json:"message"`            // 人类可读的提示信息
    Data    interface{} `json:"data,omitempty"`     // 成功时的业务数据
    TraceID string      `json:"trace_id"`           // 全链路追踪 ID
}
```

---

# 3. HTTP 状态码与业务错误码 (Status Code & Business Error Code)

1. **HTTP Status Code 规则**：
   - `200 OK`: 请求成功。
   - `400 Bad Request`: 参数校验失败或 JSON 格式错误。
   - `401 Unauthorized`: 令牌缺失、过期或无效。
   - `403 Forbidden`: 已认证但无权限访问该资源。
   - `404 Not Found`: 路由或实体不存在。
   - `429 Too Many Requests`: 触发表限流。
   - `500 Internal Server Error`: 未捕获的服务端内部错误（**必须掩盖敏感堆栈，对外通用提示**）。

2. **业务 Code 命名空间设计**（5 位数字）：
   - `0`: 成功
   - `1xxxx`: 通用错误（如 `10001` 参数校验失败，`10002` 系统繁忙）
   - `2xxxx`: 用户模块错误（如 `20001` 用户未找到，`20002` 密码错误）
   - `3xxxx`: 订单与支付模块错误（如 `30001` 余额不足）

---

# 4. 分页与过滤规范 (Pagination & Filtering)

1. **分页请求参数**：统一使用 Query Parameter 方式传递：
   - `page`: 当前页码（从 1 开始，默认 1）
   - `page_size`: 每页数量（默认 20，单页上限最大值禁止超过 100）
2. **分页响应数据结构**：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [ ... ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total_count": 150,
      "total_page": 8
    }
  },
  "trace_id": "c3b8a910-3882-41f2-897e-128a1a9eef11"
}
```

---

# 5. 接口幂等性设计 (Idempotency)

对于非幂等写操作（如创建支付单、扣减库存），API 层必须支持幂等请求头 `X-Idempotency-Key`。
1. **客户端生成**：客户端在发起创建请求前生成 UUID v4，放在 HTTP Header `X-Idempotency-Key` 中。
2. **服务端逻辑**：服务端利用 Redis 执行 `SET key token NX EX 60` 锁定该请求 key。若已存在，直接返回上一笔处理成功的结果或提示“请求正在处理中”。

---

# 6. OpenAPI / Swagger 文档契约 (API Contracts)

1. **代码即文档**：使用 Swag (`github.com/swaggo/swag`) 在 Handler 上编写符合 OpenAPI 规范的注释：

```go
// CreateOrder godoc
// @Summary      创建用户订单
// @Description  根据商品列表生成预支付订单
// @Tags         orders
// @Accept       json
// @Produce      json
// @Param        X-Idempotency-Key header string true "幂等性Key"
// @Param        request body model.CreateOrderReq true "创建订单入参"
// @Success      200 {object} Response{data=model.OrderResp}
// @Failure      400 {object} Response
// @Router       /api/v1/orders [post]
func (h *OrderHandler) CreateOrder(c *gin.Context) { ... }
```
