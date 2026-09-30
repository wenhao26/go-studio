# 系统架构与分层规范 (Architecture Standard)

> **版本**：v1.0.0  
> **适用范围**：后端业务系统架构设计、包划分及依赖解耦。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [go.md](go.md)

---

# 1. 架构核心原则 (Core Architecture Principles)

1. **单向依赖原则**：依赖方向必须由外向内（高层业务逻辑不得依赖底层具体实现，必须通过 Interface 隔离）。
2. **整洁架构 (Clean Architecture) / 传统三层架构**：
   ```text
   [ Transport Layer / Handler ]  (HTTP / gRPC / MQ Consumer)
                  ↓
   [ Application / Service Layer ] (业务流程编排、领域逻辑)
                  ↓
   [ Domain / Model Layer ]        (核心业务实体、值对象)
                  ↓
   [ Infrastructure / Repository ] (MySQL, Redis, External API)
   ```
3. **禁止跨层调用**：Handler 层严禁绕过 Service 层直接调用 Repository 或执行 SQL 操作。

---

# 2. 目录结构规范 (Standard Project Layout)

遵循 Go 社区推荐的 `golang-standards/project-layout` 规范：

```text
├── cmd/                    # 应用程序入口目录 (main.go 所在地)
│   ├── api/                # HTTP API 服务入口
│   └── worker/             # 后台任务/MQ 消费者入口
├── internal/               # 私有应用程序代码 (防止被外部项目 import)
│   ├── config/             # 配置结构体与加载逻辑
│   ├── handler/            # 传输层：解析请求、校验参数、响应 HTTP/gRPC
│   ├── service/            # 业务逻辑层：组合 Repository 执行核心业务
│   ├── repository/         # 数据访问层：MySQL/Redis 增删改查
│   ├── model/              # 数据实体与 DTO 定义
│   └── pkg/                # 项目内仅供内部使用的工具包
├── pkg/                    # 可被外部项目复用的公共库 (极力慎用)
├── api/                    # OpenAPI / Swagger 规范文件、Protobuf 定义文件
├── migrations/             # 数据库 Migration SQL 文件
├── scripts/                # 构建、部署、自动化脚本
├── go.mod
├── go.sum
└── AGENTS.md
```

---

# 3. 各层职责边界与禁止行为 (Layer Boundaries & Anti-Patterns)

## 3.1 Transport / Handler 层
- **职责**：
  1. 绑定并解析 HTTP Body / Query / Path 参数。
  2. 调用参数校验器（Validator）检查入参合法性。
  3. 提取 Context 中的 Auth 元数据。
  4. 调用 Service 层函数并处理返回结果。
  5. 将领域模型/业务结果转换为统一的 HTTP Response 结构体。
- **禁止**：
  - 禁止在 Handler 中编写 SQL 或 Redis 命令。
  - 禁止在 Handler 中处理复杂的事务或事务回滚。

## 3.2 Service / Business 层
- **职责**：
  1. 组合多个 Repository 完成复杂业务。
  2. 处理核心状态机、计费逻辑、权限判定。
  3. 控制数据库事务（Transaction）边界。
- **禁止**：
  - 禁止引用 `gin.Context`、`http.Request` 或任何 HTTP 相关的结构体。
  - 禁止返回 HTTP Status Code 给上层。

## 3.3 Infrastructure / Repository 层
- **职责**：
  1. 实现数据存储的增删改查（CRUD）。
  2. 屏蔽 SQL 驱动细节、Redis Key 拼装细节及外部第三方 HTTP SDK 调用细节。
- **禁止**：
  - 禁止编写业务状态判断逻辑（如“用户余额是否足够”不应在 Repo 中判断，而应由 Repo 提供查询余额 API，由 Service 判定）。

---

# 4. 依赖注入 (Dependency Injection)

1. **构造函数显式注入**：所有 Service 和 Repository 必须通过构造函数显式传递其依赖的 Interface，禁止使用全局变量。
2. **推荐模式**：

```go
// 1. 定义依赖接口 (在消费方定义)
type UserStorer interface {
    GetByID(ctx context.Context, id int64) (*model.User, error)
}

// 2. 服务结构体持有接口
type UserService struct {
    repo UserStorer
}

// 3. 构造函数注入
func NewUserService(repo UserStorer) *UserService {
    return &UserService{repo: repo}
}
```

3. **DI 框架使用**：小型项目推荐纯手动 Wire 组装；大型项目建议使用 Google `wire`（编译期静态生成代码），禁止使用基于运行时反射且隐式魔改的 DI 框架。

---

# 5. 循环依赖解耦策略 (Decoupling Cyclic Dependencies)

若出现 `package A` 引用 `package B`，而 `package B` 又需引用 `package A` 的情况：

1. **下沉接口/公共模型**：提取共同依赖的类型至 `model` 包或独立的接口定义包中。
2. **事件驱动解耦**：通过内部 Event Bus 或 MQ 机制将强同步依赖转为异步事件通知。
3. **架构审查**：循环依赖 90% 代表包职责不单一，必须重新评估并拆分包结构。
