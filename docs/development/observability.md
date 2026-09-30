# 可观测性与日志、Tracing 规范 (Observability Standard)

> **版本**：v1.0.0  
> **适用范围**：结构化日志输出、全链路 TraceID 传递、Prometheus Metrics 埋点及告警。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [go.md](go.md), [security.md](security.md)

---

# 1. 结构化日志规范 (Structured Logging)

## 1.1 Logger 选用与格式
- 强制使用强类型高效率结构化 Logger，推荐 **`go.uber.org/zap`**。
- 格式要求：
  - 本地开发环境 (`development`)：Text / Console 格式。
  - 生产环境 (`production`)：JSON 格式。

## 1.2 核心必带字段 (Contextual Key-Value Pairs)
所有日志输出必须包含统一的基础字段：
- `ts`: ISO8601 高精度时间戳。
- `level`: 日志级别 (`DEBUG`, `INFO`, `WARN`, `ERROR`)。
- `trace_id`: 全链路追踪 ID。
- `caller`: 日志调用的代码行号（如 `service/user.go:42`）。

```go
// 正确示例：结构化日志
logger.Info("user login success",
    zap.String("trace_id", traceID),
    zap.Int64("user_id", userID),
    zap.String("client_ip", clientIP),
)

// 错误示例：拼装字符串
logger.Infof("user %d login success from ip %s", userID, clientIP)
```

## 1.3 日志级别适用准则
- **DEBUG**：开发环境调测使用，上线后必须关闭。
- **INFO**：重要系统生命周期事件（服务启动、优雅关闭）、关键业务成功状态。**禁止在高频循环体内打印 INFO Log**。
- **WARN**：非致命异常，系统可自动恢复（如第三方 API 超时重试、缓存 Miss 触发回源）。
- **ERROR**：业务处理失败、系统内部错误，必须人工介入排查并触发告警。

---

# 2. 全链路追踪规范 (Distributed Tracing)

1. **Context 级级传递**：Tracing 信息（`TraceID`, `SpanID`）必须通过 `context.Context` 在 Client、Service、Repository 以及外部 HTTP/gRPC 调用间显式传递。
2. **OpenTelemetry 规范**：使用 `go.opentelemetry.io/otel` 创建 Span：

```go
func (s *OrderService) GetOrder(ctx context.Context, orderID int64) (*model.Order, error) {
    tr := otel.Tracer("order-service")
    ctx, span := tr.Start(ctx, "OrderService.GetOrder",
        trace.WithAttributes(attribute.Int64("order_id", orderID)),
    )
    defer span.End()

    order, err := s.repo.GetByID(ctx, orderID)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return nil, err
    }
    return order, nil
}
```

---

# 3. 指标监控规范 (Metrics Standard)

指标命名统一使用 **Prometheus 蛇形命名法（Snake_case）**，并附带明确单位：

1. **Counter（累加器）**：如 `http_requests_total{method="POST", status="200"}`
2. **Histogram（直方图）**：用于统计耗时分布，如 `http_request_duration_seconds_bucket{le="0.1"}`

---

# 4. 敏感数据脱敏拦截 (Data Masking)

日志在写入 Output 前必须过滤脱敏字段，**绝对禁止将以下数据打入日志文件**：
- 明文密码 (`password`)
- 完整信用卡号 / 银行卡号 (`credit_card_no`)
- JWT AccessToken / RefreshToken
- 手机号（脱敏格式：`138****1234`）
