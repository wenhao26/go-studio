# 自动化测试规范 (Testing Standard)

> **版本**：v1.0.0  
> **适用范围**：单元测试、集成测试、基准测试及 Mock 规范。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [go.md](go.md)

---

# 1. 测试核心原则与指标 (Testing Core Principles)

1. **测试与代码同源**：测试文件必须以 `_test.go` 结尾，与被测试文件放置在同一个 package 目录下。
2. **覆盖率指标**：
   - Service 层业务核心代码单元测试覆盖率必须达到 **70% 以上**。
   - 工具类 / 基础库（Pkg）测试覆盖率必须达到 **85% 以上**。
3. **确定性与独立性**：测试用例必须可重复执行，禁止依赖外部不可控环境（如网络连通性、当前系统绝对时间）。每个 Test Case 执行前后不得在数据库留存垃圾数据。

---

# 2. 表格驱动测试 (Table-Driven Tests)

针对逻辑分支多、输入输出明确的函数，**必须**使用 Go 官方推荐的表格驱动测试结构：

```go
func TestCalculateDiscount(t *testing.T) {
    type args struct {
        userType string
        amount   int64
    }
    tests := []struct {
        name    string
        args    args
        want    int64
        wantErr bool
    }{
        {
            name:    "VIP user 20% off",
            args:    args{userType: "VIP", amount: 10000},
            want:    8000,
            wantErr: false,
        },
        {
            name:    "Invalid amount",
            args:    args{userType: "NORMAL", amount: -100},
            want:    0,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CalculateDiscount(tt.args.userType, tt.args.amount)
            if (err != nil) != tt.wantErr {
                t.Fatalf("CalculateDiscount() error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("CalculateDiscount() got = %v, want %v", got, tt.want)
            }
        })
    }
}
```

---

# 3. 断言与 Helper 库 (Assertions & Helpers)

1. **推荐断言库**：可选择使用 `github.com/stretchr/testify/assert` 和 `github.com/stretchr/testify/require`。
   - `require`: 失败时立即中止当前 Test（`t.FailNow()`），用于前置条件校验（如创建测试数据失败）。
   - `assert`: 失败时记录错误并继续执行后续校验。
2. **Test Helper 函数**：自定义辅助函数必须标记 `t.Helper()`，以便测试失败时精准定位到调用方行号：

```go
func createTestUser(t *testing.T, db *gorm.DB) *model.User {
    t.Helper()
    user := &model.User{Name: "test_user"}
    if err := db.Create(user).Error; err != nil {
        t.Fatalf("failed to create test user: %v", err)
    }
    return user
}
```

---

# 4. Mock 与 依赖隔离 (Mocking Guidelines)

1. **Mock 框架**：推荐使用 Go 官方维护的 `go.uber.org/mock` (gomock) 或基于接口手动编写 Stub。
2. **隔离范围**：
   - **单元测试**：绝对禁止真实连接生产/开发数据库及外部 API，必须 Mock `Repository` 和 `External Client` 接口。
   - **集成测试**：可使用 `testcontainers-go` 在本地 Docker 中拉起真实的 Temporary MySQL/Redis 容器进行数据集成测试。

---

# 5. 并发安全与 Race 检查 (Race Detection)

1. **强制命令**：在 CI 阶段执行单元测试，必须添加 `-race` 参数：
   ```bash
   go test -v -race -cover ./...
   ```
2. **时间依赖测试**：若代码依赖 `time.Now()`，必须通过接口注入 `Clock` 实例或使用时间 Mock 库（如 `jonboulle/clockwork`），禁止在测试中使用 `time.Sleep()` 强行等待，这会导致测试变慢且极其不稳定。

---

# 6. CI 自动化集成 (Continuous Integration)

GitHub Actions 或 GitLab CI 执行序列：

```yaml
steps:
  - name: Run Format Check
    run: test -z $(gofmt -l .)
  - name: Run Vet
    run: go vet ./...
  - name: Run Unit Tests with Race Detector
    run: go test -race -coverprofile=coverage.out ./...
  - name: Check Coverage Floor
    run: go tool cover -func=coverage.out
```
