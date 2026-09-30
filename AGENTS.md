# AGENTS.md

> 项目级 AI Coding Agent 开发规范。
>
> 本文件用于约束 OpenCode、AI Agent、Vibe Coding 以及人工开发行为。
> **本文件中的规则默认均为强制规则，除非明确标记为“建议”。**
>
> 适用范围：本文件所在目录及全部子目录。

---

# 1. 核心原则

## 1.1 第一原则：正确性优先

任何代码修改必须优先保证：

1. 正确性
2. 安全性
3. 可维护性
4. 可测试性
5. 可观测性
6. 性能
7. 简洁性
8. 开发速度

不得为了“快速完成”牺牲前 6 项。

---

## 1.2 AI Agent 不是产品需求的解释者

Agent 不得擅自扩大需求。

如果用户说：

> “增加一个用户查询接口”

不得自行推断并修改：

- 数据库 schema
- 权限模型
- 用户状态机
- API 返回结构
- 缓存策略
- 消息队列
- 部署配置
- 第三方服务
- 其他业务模块

除非当前代码、已有约定或用户明确要求表明这些修改是完成任务所必需的。

### 规则

**需求没有要求 ≠ 可以自行设计。**

当存在多种合理实现时：

- 优先选择当前项目已经使用的方案
- 优先复用已有抽象
- 优先最小变更
- 不主动引入新的架构模式
- 不主动增加依赖
- 不主动重构无关代码

---

# 2. Vibe Coding 总规则

## 2.1 修改代码之前必须先理解上下文

任何非 trivial 修改，都必须先：

1. 查看项目结构
2. 查看 `go.mod`
3. 找到相关 package
4. 阅读目标文件
5. 阅读调用方
6. 阅读被调用方
7. 搜索相关接口、类型、错误、配置
8. 查看相关测试
9. 判断是否存在已有实现

不得：

- 看到一个文件就直接改
- 只根据文件名猜架构
- 只读取函数本身而忽略调用链
- 看到 TODO 就直接实现
- 看到缺失功能就自行设计完整架构

---

## 2.2 先搜索，再创建

新增代码前必须搜索：

- 是否已有同名函数
- 是否已有同类接口
- 是否已有相似 service
- 是否已有 repository
- 是否已有 DTO
- 是否已有错误类型
- 是否已有 middleware
- 是否已有配置项
- 是否已有工具函数
- 是否已有测试
- 是否已有第三方依赖

如果已有类似实现，应优先复用或抽取，而不是复制粘贴。

---

## 2.3 最小修改原则

每次任务只修改完成任务所必需的内容。

禁止因为“顺手”而：

- 格式化整个项目
- 重命名无关变量
- 重构无关函数
- 升级无关依赖
- 修改无关配置
- 删除看起来没用但无法确认无用的代码
- 修改 API 返回结构
- 修改数据库结构
- 修改日志格式
- 修改错误码
- 修改公共接口

如果确实发现无关问题：

1. 不要顺手修复
2. 在最终结果中指出
3. 如确有必要，等待用户确认后单独处理

---

# 3. 修改前的工作流

对于中等及以上复杂度的任务，遵循：

```text
理解需求
  ↓
扫描项目结构
  ↓
确认技术栈与约定
  ↓
搜索相关代码
  ↓
确定影响范围
  ↓
制定最小修改方案
  ↓
实施修改
  ↓
格式化
  ↓
静态检查
  ↓
单元测试
  ↓
集成测试（如果相关）
  ↓
检查 diff
  ↓
总结修改与验证结果
```

---

# 4. 不允许臆测

## 4.1 不得假设不存在的信息

禁止根据常识直接假设：

- 数据库表结构
- API 字段
- 第三方 SDK 行为
- 配置项
- 环境变量
- 用户权限
- 业务规则
- 错误码
- 超时时间
- 重试策略
- 并发模型

必须通过代码、配置、文档、测试或明确需求确认。

---

## 4.2 不确定时的处理

如果无法确认：

### 优先级

1. 搜索代码
2. 搜索测试
3. 搜索配置
4. 搜索项目文档
5. 查看依赖源码/文档
6. 再向用户询问

禁止：

> “我认为应该是……”

然后直接修改核心业务逻辑。

---

# 5. Go 版本与工程基础

## 5.1 遵循项目实际 Go 版本

必须以：

```text
go.mod
```

中的 Go 版本为准。

不得因为 Agent 当前熟悉更新版本而擅自使用项目 Go 版本不存在的语言特性或标准库 API。

---

## 5.2 格式化

所有 Go 文件必须使用：

```bash
gofmt
```

或项目现有等价工具。

禁止手工维护 Go 格式。

优先遵循 Go 官方代码风格。Go 官方也明确建议通过 `gofmt` 统一机械格式问题。citeturn0search0turn0search9

---

## 5.3 Imports

保持 import：

- 无重复
- 无未使用
- 分组符合项目约定
- 不引入不必要依赖

如果项目使用：

```bash
goimports
```

则优先使用项目既有工具。

---

# 6. Go 命名规范

## 6.1 通用原则

名称必须：

- 简洁
- 表意明确
- 与项目现有命名一致

不要：

```go
data
info
obj
item
tmp
foo
bar
manager
helper
util
common
misc
```

除非上下文明确且生命周期非常短。

---

## 6.2 Package

package 名称：

- 使用小写
- 不使用下划线
- 不使用复数
- 不使用 `util`、`common` 等模糊命名，除非项目已经形成明确约定

推荐：

```go
package user
package order
package repository
```

而不是：

```go
package user_utils
package common_utils
package users
```

---

## 6.3 Interface

Interface 应描述行为。

推荐：

```go
type UserRepository interface {
    FindByID(ctx context.Context, id int64) (*User, error)
}
```

避免：

```go
type IUserRepository interface {}
```

Go 不需要 `I` 前缀。

---

## 6.4 缩写

遵循 Go 常见缩写：

```text
ID
URL
HTTP
JSON
API
UUID
DB
SQL
RPC
TCP
IP
```

例如：

```go
userID
requestURL
httpClient
apiServer
db
```

不要：

```go
userId
requestUrl
HttpClient
```

---

# 7. 函数设计

## 7.1 单一职责

函数应该完成一个清晰任务。

避免：

```go
CreateUserAndSendEmailAndWriteAuditAndUpdateCache(...)
```

如果业务确实需要多个步骤，应由 service/application 层进行流程编排，而不是把所有职责塞进一个函数。

---

## 7.2 控制函数复杂度

避免：

- 多层嵌套
- 巨型 switch
- 巨型 if/else
- 超长函数
- 隐式状态变化

优先：

```go
if err := validate(req); err != nil {
    return err
}

user, err := repo.Create(ctx, req)
if err != nil {
    return err
}

return publishUserCreated(ctx, user)
```

而不是大量嵌套。

---

## 7.3 Early Return

优先使用 guard clause：

```go
if err != nil {
    return err
}

if user == nil {
    return ErrUserNotFound
}
```

避免不必要的深层嵌套。

---

# 8. Context 规范

## 8.1 context.Context 必须作为第一个参数

推荐：

```go
func (s *Service) GetUser(ctx context.Context, id int64) (*User, error)
```

不要：

```go
func (s *Service) GetUser(id int64, ctx context.Context)
```

---

## 8.2 禁止 context 存业务数据

不得滥用：

```go
context.WithValue(...)
```

传递正常业务参数。

Context 主要用于：

- cancellation
- deadline
- request-scoped metadata

业务参数应该使用明确的参数或 struct。

---

## 8.3 不要随意创建 Background Context

业务调用链中禁止随意：

```go
context.Background()
context.TODO()
```

如果已经存在 `ctx`，必须继续传递。

---

# 9. Error 处理

## 9.1 禁止吞错

禁止：

```go
_ = doSomething()
```

除非明确证明错误可以安全忽略，并通过注释说明原因。

---

## 9.2 错误必须被处理

推荐：

```go
result, err := repo.Find(ctx, id)
if err != nil {
    return nil, fmt.Errorf("find user %d: %w", id, err)
}
```

---

## 9.3 使用 `%w` 保留错误链

需要向上层传播错误时：

```go
fmt.Errorf("create user: %w", err)
```

不要：

```go
fmt.Errorf("create user: %v", err)
```

如果需要 `errors.Is` / `errors.As`，必须保留 error chain。

---

## 9.4 错误信息必须包含上下文

坏：

```go
return errors.New("failed")
```

好：

```go
return fmt.Errorf("load user profile: %w", err)
```

---

## 9.5 不要通过字符串判断错误

禁止：

```go
if err.Error() == "not found" {
}
```

应该使用：

```go
errors.Is(err, ErrNotFound)
```

或：

```go
errors.As(err, &target)
```

---

## 9.6 Sentinel Error

项目如果已经定义：

```go
var (
    ErrNotFound = ...
    ErrInvalid  = ...
)
```

必须复用。

不要重复创建同语义错误。

---

# 10. Panic 规范

业务代码默认禁止：

```go
panic(...)
```

尤其禁止：

- HTTP handler 中 panic
- Service 中 panic
- Repository 中 panic
- 用户输入导致 panic

只有在以下情况才允许：

- 程序启动阶段发现不可恢复配置错误
- 明确的程序员 invariant violation
- 项目已有统一 panic 机制

即使允许，也应遵循项目既有处理方式。

---

# 11. Nil 处理

必须明确判断可能为 nil 的对象。

尤其关注：

- pointer
- interface
- map
- slice
- channel
- error
- optional dependency

不要依赖“这里正常情况下不会 nil”作为安全假设。

如果确实存在 invariant，应通过结构设计保证，而不是到处隐式假设。

---

# 12. Slice / Map

## 12.1 Slice

除非有明确需求，不要过早进行复杂预分配。

合理：

```go
items := make([]Item, 0, len(input))
```

不合理：

```go
items := make([]Item, 0, 1000000)
```

---

## 12.2 Map

写入前必须确保 map 已初始化。

例如：

```go
m := make(map[string]Value)
```

如果使用 nil map，必须明确知道它只读。

---

# 13. Struct 设计

## 13.1 不要滥用大 Struct

避免一个 struct 同时承担：

- DB Model
- API Request
- API Response
- Domain Entity
- Message
- Config

除非项目明确采用这种设计。

---

## 13.2 DTO 与 Domain Model

如果项目存在分层：

```text
handler
  ↓
service
  ↓
domain
  ↓
repository
```

不要让数据库模型直接泄漏到 API 层。

例如避免：

```go
return userDBModel
```

作为 HTTP response。

应该根据项目架构转换：

```text
DB Model
    ↓
Domain Model
    ↓
Response DTO
```

---

# 14. Pointer 使用

使用 pointer 前必须考虑：

- 是否需要表示 nil
- 是否需要避免复制
- struct 是否很大
- 是否需要修改对象
- 是否会造成生命周期复杂化

不要为了“看起来高级”把所有参数都改成 pointer。

---

# 15. Receiver 规范

对于 method：

```go
func (s *Service) Create(...)
```

与：

```go
func (s Service) Create(...)
```

的选择必须基于：

- 是否修改 receiver
- receiver 大小
- 是否项目统一使用 pointer receiver
- 是否需要保持 method set 一致

同一个类型原则上保持一致。

---

# 16. Interface 设计

## 16.1 Interface 应该小

推荐：

```go
type UserReader interface {
    GetUser(ctx context.Context, id int64) (*User, error)
}
```

避免：

```go
type UserService interface {
    Create(...)
    Update(...)
    Delete(...)
    Get(...)
    List(...)
    Export(...)
    Import(...)
    ...
}
```

除非项目确实需要。

---

## 16.2 不要为了 mock 而到处创建 interface

Interface 应由使用方根据需要定义。

不要为了“测试方便”给每个 struct 创建一个一模一样的 interface。

---

# 17. Goroutine

## 17.1 禁止无责任 Goroutine

禁止：

```go
go func() {
    doSomething()
}()
```

除非明确知道：

- 谁负责等待
- 谁负责取消
- 谁负责错误处理
- 谁负责生命周期
- goroutine 什么时候退出

---

## 17.2 Goroutine 必须可控

优先：

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()

g, ctx := errgroup.WithContext(ctx)
```

如果项目已经使用 errgroup，优先复用。

---

## 17.3 防止 Goroutine Leak

必须关注：

- channel 无人消费
- channel 无人关闭
- 无限循环
- ticker 未 Stop
- context 未取消
- worker 无退出条件

---

# 18. Channel

使用 channel 前必须回答：

1. 谁创建？
2. 谁写？
3. 谁读？
4. 谁关闭？
5. 什么时候关闭？
6. 是否需要 buffer？
7. 阻塞时怎么办？
8. context cancel 后怎么办？

不明确时不要新增 channel。

---

# 19. Mutex / 并发安全

使用：

```go
sync.Mutex
sync.RWMutex
sync.Once
atomic
```

前必须确认共享状态的生命周期。

不要为了“保险”给所有代码加锁。

锁必须：

- 尽可能小
- 生命周期清晰
- 不跨越外部 IO
- 避免嵌套锁
- 避免锁顺序不一致

---

# 20. HTTP Handler

Handler 主要负责：

1. 接收请求
2. 参数解析
3. 基础校验
4. 调用业务层
5. 转换响应
6. 设置 HTTP status
7. 处理错误

Handler 不应该承载复杂业务逻辑。

避免：

```text
Handler
 ├── SQL
 ├── 业务规则
 ├── 外部 HTTP
 ├── MQ
 ├── Cache
 └── 复杂事务
```

---

# 21. HTTP API

## 21.1 API 修改必须谨慎

修改以下内容属于高风险变更：

- URL
- HTTP Method
- Request JSON
- Response JSON
- status code
- error code
- header
- authentication
- pagination
- sorting
- filtering

如果不是需求明确要求，不得修改。

---

## 21.2 API Response

保持项目现有格式。

例如项目已有：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

新增接口必须保持一致。

不要因为个人偏好改成：

```json
{
  "success": true,
  "result": {}
}
```

---

# 22. 数据库

## 22.1 SQL 必须参数化

禁止拼接用户输入：

```go
query := "SELECT * FROM users WHERE name = '" + name + "'"
```

必须使用参数：

```go
query := "SELECT * FROM users WHERE name = ?"
```

具体 placeholder 以数据库驱动为准。

---

## 22.2 不要默认 SELECT *

除非项目已有明确约定，否则优先明确字段。

避免：

```sql
SELECT *
FROM users
```

推荐：

```sql
SELECT id, name, email
FROM users
```

原因：

- 减少不必要数据
- 降低 schema 变更影响
- 明确依赖字段

---

## 22.3 Transaction

涉及多个必须保持一致性的写操作时，必须判断是否需要事务。

事务边界应该：

- 清晰
- 尽量短
- 不包含无关外部 IO
- 正确 rollback
- 正确 commit

不要在 transaction 内调用慢的第三方 HTTP API，除非业务设计明确要求。

---

## 22.4 Migration

数据库 schema 修改必须：

1. 添加 migration
2. 遵循现有 migration 工具
3. 不直接修改已执行 migration
4. 考虑旧版本兼容
5. 考虑回滚
6. 考虑线上已有数据

禁止直接修改生产数据库结构而不留下 migration。

---

# 23. Repository

Repository 负责数据访问，不负责业务决策。

合理：

```text
Repository
- 查询
- 插入
- 更新
- 删除
- transaction 辅助
```

不合理：

```text
Repository
- 判断用户是否可以升级
- 计算订单价格
- 发送邮件
- 调用支付服务
```

业务逻辑应放在 service/domain/application 层。

---

# 24. Service

Service 负责业务流程与业务规则。

典型结构：

```text
Handler
   ↓
Service
   ↓
Repository / Client
```

Service 不应该直接处理：

- HTTP request parsing
- HTTP response writing
- SQL 字符串细节
- JSON 编解码

除非项目已有明确架构。

---

# 25. 外部 HTTP Client

调用第三方服务必须考虑：

- timeout
- context cancellation
- response status
- response body
- retry
- retry 是否幂等
- connection reuse
- logging
- tracing
- 错误映射

禁止：

```go
http.Get(url)
```

在核心业务代码中随意使用。

优先使用项目已有 HTTP client 封装。

---

# 26. Timeout

任何可能阻塞的外部操作都必须考虑 timeout：

- HTTP
- DB
- RPC
- MQ
- 文件 IO
- 第三方 SDK

不要无限等待。

优先使用上游传入的 context deadline。

---

# 27. Retry

Retry 不是默认行为。

新增 retry 前必须确认：

1. 操作是否幂等
2. 什么错误可以 retry
3. 最大次数
4. backoff
5. jitter
6. 总超时时间
7. 是否可能造成重复写入

禁止无条件：

```go
for i := 0; i < 100; i++ {
    ...
}
```

---

# 28. 日志

## 28.1 日志必须有上下文

坏：

```text
failed
```

好：

```text
failed to create order
```

更好：

```text
failed to create order: order_id=123
```

具体字段格式遵循项目现有 logger。

---

## 28.2 禁止敏感信息进入日志

禁止记录：

- password
- access token
- refresh token
- API key
- secret
- session token
- 完整身份证号
- 完整银行卡号
- 私钥
- Cookie
- Authorization header

必要时进行脱敏。

---

## 28.3 不要过度日志

不要在：

- 高频循环
- 大批量查询
- 每个普通成功请求

中无脑输出 info log。

日志必须有运营价值。

---

# 29. 配置

新增配置必须考虑：

- 默认值
- 环境变量
- 配置文件
- 启动校验
- 类型
- 单位
- 安全性

配置命名必须与现有项目一致。

不要为了一个小功能创建大量配置。

---

# 30. Secret 管理

绝对禁止把以下内容写入 Git：

```text
API key
password
private key
JWT secret
database password
cloud credential
access token
```

不要：

```go
const APIKey = "sk-xxxxx"
```

应该使用项目已有 secret/config 机制。

---

# 31. 第三方依赖

## 31.1 默认不新增依赖

如果标准库或现有依赖可以解决问题：

**优先不新增依赖。**

新增依赖必须有明确理由。

---

## 31.2 新依赖评估

引入依赖前检查：

- 是否维护
- license
- 安全记录
- API 稳定性
- 依赖树
- 是否与当前 Go 版本兼容
- 是否项目已有类似依赖

不要因为“少写十行代码”引入一个巨大依赖。

---

# 32. 测试

## 32.1 修改代码必须考虑测试

任何行为修改都应该判断：

> 是否需要新增或修改测试？

默认答案是：**需要。**

例外：

- 纯格式修改
- 注释修改
- 明确不影响行为的机械修改

---

## 32.2 单元测试

优先覆盖：

- 正常路径
- 空输入
- nil
- 边界值
- 非法输入
- 依赖错误
- 超时
- 并发相关行为
- 权限
- 重复调用
- 数据不存在

---

## 32.3 Table-driven Test

对于大量输入场景，优先：

```go
tests := []struct {
    name    string
    input   Input
    want    Output
    wantErr bool
}{
    {
        name: "valid",
    },
    {
        name: "empty",
    },
    {
        name: "invalid",
    },
}
```

---

## 32.4 测试名称

测试名称必须表达行为：

```go
func TestCreateUser_DuplicateEmail(t *testing.T)
```

而不是：

```go
func TestCreateUser2(t *testing.T)
```

---

## 32.5 不要测试实现细节

优先测试：

```text
输入 → 行为 → 输出
```

而不是：

```text
调用了几个内部函数
内部变量是什么
具体实现顺序是什么
```

除非这些实现本身就是必须保证的 contract。

---

# 33. Mock

Mock 只用于隔离真正需要隔离的依赖。

不要：

- 为每个 struct 自动生成大量 mock
- mock 所有东西
- mock 简单纯函数
- 为了测试覆盖率制造无意义 mock

优先测试真实业务行为。

---

# 34. Race Detector

涉及并发修改时，应运行：

```bash
go test -race ./...
```

如果项目规模过大，可以针对相关 package：

```bash
go test -race ./path/to/package
```

发现 race 后不得通过简单增加 sleep 掩盖问题。

---

# 35. 静态检查

优先遵循项目现有工具。

常见检查：

```bash
go vet ./...
```

如果项目使用：

```bash
staticcheck ./...
```

则必须遵循项目配置。

不要为了通过 lint 而机械修改代码导致可读性下降。

---

# 36. Build / Test 最低验证要求

代码修改后，根据影响范围执行：

### 小范围修改

至少：

```bash
gofmt
go test ./相关/package
```

### 中等修改

至少：

```bash
gofmt
go test ./...
go vet ./...
```

### 涉及并发

增加：

```bash
go test -race ./...
```

### 涉及 API / DB / 外部依赖

还应执行项目对应的：

```text
integration test
e2e test
migration test
contract test
```

如果无法执行，必须明确说明原因。

---

# 37. 不允许伪造验证结果

绝对禁止：

> “测试通过”

如果实际上没有执行测试。

必须区分：

```text
已执行并通过
```

与：

```text
未执行
```

以及：

```text
无法执行，因为环境缺少 XXX
```

---

# 38. Diff 检查

完成修改后必须检查：

```bash
git diff
git status
```

确认：

- 没有意外修改
- 没有 debug code
- 没有临时文件
- 没有日志污染
- 没有 secrets
- 没有生成无关文件
- 没有大量格式化噪音
- 没有误删代码

---

# 39. Git 规范

## 39.1 不要擅自提交

除非用户明确要求：

> commit

否则 Agent 不应该自行创建 Git commit。

---

## 39.2 不要修改用户已有改动

如果工作区存在：

```text
modified
untracked
staged
```

文件，必须先识别这些修改是否属于当前任务。

不得：

```text
reset
checkout
restore
clean
```

来清理用户改动。

---

## 39.3 不要强制覆盖

禁止未经确认使用：

```bash
git reset --hard
git clean -fd
git checkout -- .
git restore .
```

这些操作可能导致用户工作丢失。

---

# 40. Generated Code

如果文件是自动生成的：

```text
*.gen.go
swagger generated files
protobuf generated files
sqlc generated files
mock generated files
```

原则上不要手工修改。

应该修改源文件，然后重新运行生成命令。

如果项目已有明确 generated marker，必须遵循。

---

# 41. 注释规范

## 41.1 注释解释 Why

坏：

```go
// Increment i
i++
```

好：

```go
// Retry once because the upstream service occasionally returns
// a transient connection reset.
```

---

## 41.2 不要写无意义注释

代码：

```go
user.Name = req.Name
```

不要：

```go
// Set user name
user.Name = req.Name
```

---

## 41.3 导出的 API

导出的：

- package
- type
- function
- method
- const
- var

如果项目要求 godoc 风格，应提供完整注释。

---

# 42. TODO / FIXME

新增 TODO 必须说明：

- 为什么
- 后续需要什么
- 如果可能，关联 issue

不要：

```go
// TODO: fix this
```

推荐：

```go
// TODO(#123): Replace the temporary polling mechanism with event-driven updates.
```

---

# 43. 安全规范

必须特别关注：

## 输入

- SQL Injection
- Command Injection
- SSRF
- Path Traversal
- XSS
- Deserialization
- Header Injection

## 身份

- Authentication
- Authorization
- Session
- Token
- Permission

## 数据

- Secret
- PII
- Encryption
- Logging
- Data exposure

## 网络

- TLS
- Timeout
- Redirect
- SSRF
- DNS rebinding

---

# 44. 用户输入必须视为不可信

任何来自以下位置的数据：

```text
HTTP request
query parameter
path parameter
header
cookie
file upload
message queue
database
third-party API
environment
```

都不能默认可信。

必须在适当边界进行：

- validation
- normalization
- authorization
- escaping
- size limit

---

# 45. 文件操作

文件路径不能直接信任用户输入。

特别注意：

```text
../
absolute path
symlink
特殊字符
超长路径
```

需要限制目录范围时必须进行安全校验。

---

# 46. Shell / Command 执行

禁止把用户输入直接拼接到 shell：

```go
exec.Command("sh", "-c", userInput)
```

如果必须执行外部命令：

- 参数必须结构化
- 不使用 shell 拼接
- 限制 command
- 限制参数
- 设置 timeout
- 检查 exit code

---

# 47. JSON

JSON 字段必须遵循项目已有命名规范。

不要无理由修改：

```go
json:"user_id"
```

为：

```go
json:"userId"
```

API contract 是兼容性边界。

---

# 48. 时间处理

时间必须明确：

- 时区
- UTC / local
- 精度
- 序列化格式

跨服务、数据库、API 通信优先遵循项目既有 UTC 策略。

不要在业务代码中随意：

```go
time.Now().In(time.Local)
```

---

# 49. 金额处理

金额禁止使用浮点数表示核心财务数据：

```go
float64
```

优先根据项目设计使用：

```text
整数最小货币单位
decimal
专用 money type
```

例如：

```text
1000 = 10.00 元
```

具体方案必须遵循项目现有实现。

---

# 50. ID

不要假设 ID 一定是：

```text
int
int64
UUID
string
```

必须查看项目已有定义。

ID 相关代码必须遵循现有类型。

---

# 51. 性能

性能优化必须建立在实际问题上。

不要为了“可能更快”而：

- 提前引入 cache
- 使用 goroutine
- 使用 channel
- 使用 sync.Pool
- 自定义 memory pool
- 改写简单算法
- 引入复杂并发模型

优先：

```text
正确 → 可读 → 可测 → 再优化
```

---

# 52. 数据库性能

关注：

- N+1 query
- missing index
- large scan
- unnecessary columns
- transaction duration
- connection pool
- pagination
- lock contention

但不要在没有证据时盲目添加 index。

---

# 53. Cache

新增 cache 必须回答：

1. cache key 是什么？
2. TTL 是什么？
3. 谁负责失效？
4. 数据不一致怎么办？
5. cache miss 怎么处理？
6. cache failure 怎么处理？
7. 是否允许 stale data？
8. 是否存在 cache stampede？

Cache 不应该成为默认解决方案。

---

# 54. 分层原则

如果项目采用分层架构，应保持依赖方向。

例如：

```text
Transport / Handler
        ↓
Application / Service
        ↓
Domain
        ↓
Repository / Infrastructure
```

低层不要反向依赖高层。

例如 Repository 不应该 import HTTP Handler。

---

# 55. Circular Dependency

不得通过奇怪技巧绕过 Go package import cycle。

如果出现循环依赖：

1. 分析职责
2. 找到真正依赖方向
3. 抽象接口
4. 拆分 package
5. 或调整模块边界

不要通过复制代码解决。

---

# 56. Utility / Helper 约束

禁止建立万能：

```text
utils
common
helpers
misc
```

然后不断往里面塞代码。

如果函数属于具体领域，应放到具体 package：

```text
order
user
payment
auth
```

而不是：

```text
utils
```

---

# 57. 重构规则

重构与功能开发分开。

如果当前任务是：

> 修复 bug

不要顺便：

> 重构整个模块。

如果确实需要重构才能完成：

1. 说明原因
2. 控制范围
3. 保持行为兼容
4. 增加测试
5. 分阶段进行

---

# 58. API / Schema 兼容性

修改公共接口必须考虑：

```text
Backward Compatibility
```

尤其是：

- JSON field
- DB column
- protobuf
- RPC
- event
- message
- public Go API

删除字段通常比新增字段风险更高。

---

# 59. Event / MQ

发送消息必须考虑：

- message schema
- version
- idempotency
- duplicate delivery
- ordering
- retry
- dead letter
- consumer compatibility

消费者必须能够处理合理的重复消息。

---

# 60. 幂等性

以下操作必须主动考虑幂等：

- payment
- order creation
- message consumption
- webhook
- retry
- job execution
- external API write

不要因为代码执行一次正常，就认为重复执行也安全。

---

# 61. 定时任务 / Worker

必须明确：

- 启动方式
- 并发度
- shutdown
- retry
- timeout
- duplicate execution
- leader election（如果需要）
- metrics
- logging

进程退出时必须尽可能优雅停止。

---

# 62. Graceful Shutdown

服务如果存在：

- HTTP server
- worker
- consumer
- background goroutine

必须考虑：

```text
signal
  ↓
cancel context
  ↓
stop accepting new work
  ↓
finish active work
  ↓
close resources
  ↓
exit
```

---

# 63. 资源释放

使用以下资源时必须确认释放：

- file
- response body
- DB rows
- transaction
- ticker
- timer
- network connection
- lock

例如：

```go
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
```

---

# 64. defer

`defer` 适合：

- close
- unlock
- rollback
- cleanup

但在高频性能敏感循环中使用前要考虑成本。

不要为了追求“代码漂亮”在复杂生命周期中滥用 defer。

---

# 65. API Error Mapping

内部错误不应直接暴露给用户。

例如不要直接返回：

```text
sql: no rows in result set
```

应该映射成项目定义的业务错误。

内部日志可以保留详细上下文。

---

# 66. Authorization

Authentication 与 Authorization 必须分开理解：

```text
Authentication = 你是谁
Authorization = 你能做什么
```

不能因为：

```text
authenticated
```

就认为：

```text
authorized
```

所有敏感操作必须检查权限。

---

# 67. 权限检查位置

权限必须尽可能靠近业务边界。

不要只依赖前端隐藏按钮。

后端必须再次验证。

---

# 68. 默认拒绝

涉及安全权限时，优先：

```text
deny by default
```

而不是：

```text
allow by default
```

---

# 69. 不修改架构边界

除非需求明确要求，否则不要：

- 从 monolith 拆 microservice
- 引入 CQRS
- 引入 event sourcing
- 引入 DDD 全套结构
- 引入 repository abstraction
- 引入 dependency injection framework
- 引入新的 ORM
- 引入新的 message broker

架构复杂度本身也是成本。

---

# 70. 不要过度工程化

不要为了：

```text
未来可能扩展
可能有百万用户
可能需要多租户
可能切换数据库
可能增加第三方
```

提前创建复杂抽象。

遵循：

```text
当前真实需求 > 假想未来需求
```

---

# 71. 代码重复

不要为了消除两段相似代码立即抽象。

先判断：

- 逻辑是否真正相同
- 是否变化方向一致
- 抽象后是否更容易理解

错误抽象比少量重复更危险。

---

# 72. Magic Number / Magic String

业务含义明确的常量应该命名。

坏：

```go
if retryCount > 3 {
}
```

好：

```go
const maxRetryCount = 3
```

但不要把所有数字都抽成常量。

---

# 73. Feature Flag

如果项目使用 feature flag：

- 默认值必须明确
- flag 生命周期必须明确
- 不得留下永久 flag
- 删除 feature 后应清理 flag

---

# 74. 环境差异

不要在代码中硬编码：

```text
development
staging
production
```

除非项目已有明确设计。

环境差异优先通过配置注入。

---

# 75. 本地开发

修改后优先使用项目已有：

```text
Makefile
Taskfile
scripts/
Justfile
README
```

中定义的命令。

不要自己发明一套执行方式。

---

# 76. 命令执行原则

Agent 执行 shell 命令前必须判断：

### 安全命令

通常可直接执行：

```bash
go test
go vet
gofmt
git diff
git status
go list
```

### 高风险命令

必须谨慎：

```bash
rm
git reset
git clean
git checkout
git restore
docker system prune
DROP DATABASE
DROP TABLE
TRUNCATE
```

涉及破坏性操作时必须先确认。

---

# 77. 不修改生成物

如果修改：

```text
generated.go
*.pb.go
mock_*.go
swagger.json
openapi generated files
```

必须确认生成流程。

优先修改 source，然后 regenerate。

---

# 78. 不制造临时垃圾

完成任务后不得留下：

```text
/tmp project files
debug.log
test-output.txt
foo.go
tmp.go
backup.go
*.bak
```

除非它们本身是任务产物。

---

# 79. AI 专用：禁止伪实现

禁止为了让代码“看起来完成”而写：

```go
return nil
```

```go
return nil, nil
```

```go
TODO
```

```go
panic("not implemented")
```

```go
mock response
```

除非需求明确要求 stub / placeholder。

---

# 80. AI 专用：禁止假数据

禁止在生产代码中加入：

```text
fake user
mock order
hardcoded response
random ID
dummy token
fake success
```

如果需要 mock：

- 放在测试
- 使用测试 fixture
- 使用项目 mock framework

---

# 81. AI 专用：禁止“为了通过测试”修改业务

不得为了让测试通过：

- 放宽校验
- 删除错误处理
- 修改生产逻辑
- hardcode 测试输入
- 特判测试数据
- 添加无意义 sleep

正确流程：

```text
测试失败
↓
分析原因
↓
判断实现错误 / 测试错误
↓
修复真实问题
```

---

# 82. AI 专用：不要迎合错误需求

如果用户要求：

> “把这个错误吞掉，让测试通过”

Agent 应检查：

- 这个错误是否真的应该被忽略
- 是否影响数据一致性
- 是否影响安全
- 是否违反现有 contract

如果会造成明显风险，应指出风险，而不是机械执行。

---

# 83. AI 专用：不要凭空增加 TODO

如果无法完成需求，不得伪装完成。

应明确：

```text
已完成：
...

未完成：
...

原因：
...

需要：
...
```

---

# 84. AI 专用：禁止无关格式化

不得因为运行：

```bash
gofmt ./...
```

导致大量无关文件发生 diff。

优先只格式化修改过的文件。

---

# 85. AI 专用：保持 Diff 可审查

理想 diff：

```text
功能代码
+ 测试
+ 必要配置
```

不理想 diff：

```text
功能代码
+ 整个项目重排
+ 大量 rename
+ dependency upgrade
+ unrelated refactor
```

Diff 必须让人能够快速 review。

---

# 86. AI 专用：修改前后对照

对于复杂任务，Agent 应能回答：

```text
修改前：
A → B → C

修改后：
A → B → D → C
```

不能只说：

> “已优化。”

必须能解释行为变化。

---

# 87. AI 专用：发现已有模式必须复用

如果项目已经存在：

```text
Service
Repository
Handler
Middleware
Error
Logger
Config
Test fixture
Mock
Client
```

优先复用。

不要因为 Agent 更喜欢另一种设计而建立第二套体系。

---

# 88. AI 专用：不要混用架构风格

例如项目已经：

```text
handler → service → repository
```

不要新增一个模块使用：

```text
controller → usecase → gateway → adapter → port
```

除非有明确迁移计划。

一个项目中保持一致性通常比局部“理论最佳实践”更重要。

---

# 89. AI 专用：不要自动升级依赖

禁止因为：

```text
go get
go mod tidy
```

而自动升级大量依赖。

如果任务不需要：

- 不升级
- 不替换
- 不迁移

如果 `go mod tidy` 导致依赖变化，应检查 diff，并确认这些变化确实是必要的。

---

# 90. AI 专用：数据库变更必须特别谨慎

任何以下操作均视为高风险：

```text
DROP
TRUNCATE
ALTER COLUMN
删除字段
修改字段类型
修改唯一约束
修改索引
修改外键
```

必须考虑已有数据与线上兼容性。

---

# 91. AI 专用：先保护现有行为

修 bug 时：

```text
先写/确认回归测试
↓
修改实现
↓
测试
```

如果无法先写测试，至少先确认现有行为。

---

# 92. AI 专用：不要删除看不懂的代码

看到：

```go
// strange code
```

不要直接删除。

可能存在：

- workaround
- compatibility
- race prevention
- legacy data
- vendor behavior
- production incident fix

先搜索：

- 调用方
- commit history
- issue
- comment
- tests

如果无法确认，保留。

---

# 93. AI 专用：不要擅自修改公共行为

公共行为包括：

```text
API
DB schema
event
message
error code
CLI
config
environment variable
file format
```

任何变化都需要明确理由。

---

# 94. AI 专用：任务完成定义

任务只有满足以下条件才算完成：

- 需求已实现
- 编译通过
- 相关测试通过
- 新行为有测试（适用时）
- 格式正确
- 静态检查通过（适用时）
- 没有明显安全问题
- diff 已检查
- 没有无关修改
- 没有临时文件
- 最终结果明确说明验证范围

---

# 95. 推荐验证矩阵

| 修改类型 | gofmt | unit test | go vet | race | integration |
|---|---:|---:|---:|---:|---:|
| 注释 | ✓ | - | - | - | - |
| 单函数逻辑 | ✓ | ✓ | ✓ | 按需 | - |
| Service | ✓ | ✓ | ✓ | 按需 | 按需 |
| Repository | ✓ | ✓ | ✓ | 按需 | ✓ |
| API | ✓ | ✓ | ✓ | 按需 | ✓ |
| 并发 | ✓ | ✓ | ✓ | ✓ | 按需 |
| DB schema | ✓ | ✓ | ✓ | 按需 | ✓ |
| MQ | ✓ | ✓ | ✓ | 按需 | ✓ |
| 配置 | ✓ | ✓ | ✓ | - | 按需 |
| 安全逻辑 | ✓ | ✓ | ✓ | 按需 | ✓ |

---

# 96. Code Review 自检清单

提交修改前逐项检查。

## 需求

- [ ] 我是否准确理解需求？
- [ ] 是否做了需求之外的修改？
- [ ] 是否引入了不必要的设计？

## 架构

- [ ] 是否遵循当前项目架构？
- [ ] 是否复用了已有抽象？
- [ ] 是否产生新的重复实现？
- [ ] 是否产生循环依赖？

## Go

- [ ] 是否 gofmt？
- [ ] error 是否正确处理？
- [ ] context 是否正确传递？
- [ ] 是否存在 panic？
- [ ] 是否存在 goroutine leak？
- [ ] 是否存在 race？
- [ ] 是否存在资源泄漏？

## API

- [ ] 是否破坏兼容性？
- [ ] status code 是否正确？
- [ ] error response 是否符合现有格式？
- [ ] 是否正确处理非法输入？

## DB

- [ ] SQL 是否参数化？
- [ ] 是否需要 transaction？
- [ ] 是否产生 N+1？
- [ ] migration 是否正确？
- [ ] 是否考虑旧数据？

## 安全

- [ ] 是否泄露 secret？
- [ ] 是否存在 injection？
- [ ] 是否缺少 authorization？
- [ ] 是否把敏感数据写入日志？

## 测试

- [ ] 是否添加回归测试？
- [ ] 是否测试边界情况？
- [ ] 是否测试错误路径？
- [ ] 是否运行相关测试？

## Diff

- [ ] git diff 是否干净？
- [ ] 是否存在无关修改？
- [ ] 是否存在 debug code？
- [ ] 是否存在临时文件？

---

# 97. 标准开发行为

当收到任务：

```text
实现 XXX
```

默认执行：

```text
1. 阅读项目结构
2. 阅读 go.mod
3. 找到相关模块
4. 搜索已有实现
5. 阅读调用链
6. 阅读相关测试
7. 确认现有模式
8. 选择最小修改方案
9. 实现
10. 添加/修改测试
11. gofmt
12. 执行相关测试
13. 根据影响范围执行更大范围测试
14. go vet / lint（如果项目使用）
15. git diff
16. 总结
```

---

# 98. 标准 Debug 行为

遇到 bug：

```text
复现
 ↓
定位
 ↓
确认根因
 ↓
写回归测试
 ↓
修复
 ↓
验证
```

禁止：

```text
猜一个原因
 ↓
改代码
 ↓
测试没过
 ↓
继续乱改
```

---

# 99. 标准重构行为

重构：

```text
确认现有行为
 ↓
建立测试保护
 ↓
小步修改
 ↓
持续测试
 ↓
检查 diff
```

不要一次性：

```text
删除整个模块
 ↓
重新设计
 ↓
最后才发现行为已经改变
```

---

# 100. 最终输出规范

完成任务后，最终说明至少包含：

## 修改内容

简述：

- 修改了什么
- 为什么修改
- 影响哪些模块

## 验证

明确列出实际执行过的命令，例如：

```text
✓ gofmt
✓ go test ./internal/user/...
✓ go test ./...
✓ go vet ./...
```

如果没有执行：

```text
未执行 integration test：本地环境缺少 XXX。
```

## 风险

如果存在未验证风险：

```text
风险：
- 生产环境数据库数据量未在本地验证
- 第三方 API 未进行真实调用测试
```

不要隐藏风险。

---

# 101. 优先级规则

当多个规则发生冲突时，按以下优先级处理：

```text
1. 用户明确要求
2. 项目已有实际行为 / API contract
3. 安全与数据一致性
4. 当前架构约定
5. 本文件规则
6. Go 官方惯例
7. Agent 自身偏好
```

**Agent 自身偏好永远不能凌驾于项目已有约定。**

---

# 102. 最重要的 20 条规则

如果上下文有限，至少严格遵守以下规则：

1. **先读代码，再改代码。**
2. **先搜索已有实现，再新增实现。**
3. **不要猜业务规则。**
4. **不要扩大需求范围。**
5. **保持最小 diff。**
6. **不要无关重构。**
7. **不要擅自升级依赖。**
8. **不要修改用户已有改动。**
9. **不要使用破坏性 Git 命令。**
10. **不要吞 error。**
11. **不要无责任启动 goroutine。**
12. **不要泄露 secret。**
13. **不要拼接用户输入执行 SQL / shell。**
14. **不要修改公共 API 而不考虑兼容性。**
15. **行为修改必须考虑测试。**
16. **修改 Go 文件后必须 gofmt。**
17. **不要伪造测试结果。**
18. **完成后必须检查 git diff。**
19. **不确定时明确说明，不要编造。**
20. **正确、可维护、可验证比“快速生成代码”更重要。**

---

# 103. Agent 行为底线

以下行为禁止：

```text
❌ 编造不存在的 API
❌ 编造不存在的配置
❌ 编造测试结果
❌ 编造数据库字段
❌ 编造业务规则
❌ 偷偷修改无关代码
❌ 偷偷升级依赖
❌ 偷偷修改 schema
❌ 偷偷删除代码
❌ 偷偷修改 Git 历史
❌ 使用 fake implementation 冒充完成
❌ 用 sleep 掩盖并发问题
❌ 用 recover 掩盖真实 bug
❌ 用 panic 代替错误处理
❌ 为了测试通过修改业务语义
❌ 为了 lint 通过牺牲可读性
```

---

# 104. Agent 行为准则

始终遵循：

```text
Understand before changing.
Search before creating.
Reuse before abstracting.
Minimal change before refactoring.
Test before claiming success.
Verify before reporting completion.
Ask before making irreversible decisions.
```

中文：

```text
修改之前先理解。
新增之前先搜索。
抽象之前先复用。
重构之前先确认。
宣称成功之前先测试。
完成之前先验证。
不可逆操作之前先确认。
```

---

# 105. 项目特定规则占位区

以下内容应根据实际项目补充。

## 项目结构

```text
待补充：
- cmd/
- internal/
- pkg/
- api/
- migrations/
- scripts/
```

## 常用命令

```bash
# 安装依赖
待补充

# 启动
待补充

# 测试
go test ./...

# 格式化
gofmt

# 静态检查
go vet ./...

# lint
待补充

# integration test
待补充

# generate
待补充
```

## 架构说明

```text
待补充：
- Handler 层：
- Service 层：
- Repository 层：
- Domain 层：
- Infrastructure 层：
```

## 数据库

```text
待补充：
- 数据库：
- ORM / Driver：
- Migration 工具：
- Transaction 约定：
```

## API

```text
待补充：
- HTTP Framework：
- API Response 格式：
- Error Code：
- Authentication：
- Authorization：
```

## 日志

```text
待补充：
- Logger：
- Log Format：
- Trace ID：
- Sensitive Data Masking：
```

---

# 106. 结论

本项目不是“让 AI 自由写代码”的项目。

AI Agent 应被视为：

```text
高执行速度的初级/中级开发者
+
极强的代码搜索能力
+
极强的代码生成能力
-
对本项目隐含业务知识的天然理解
```

因此：

> **Agent 可以快速实现，但不能替代项目知识。**

任何 Agent 都必须尊重：

```text
已有代码
已有架构
已有 API
已有数据库
已有测试
已有约定
已有用户数据
已有生产行为
```

最终目标不是：

> “生成更多代码”。

而是：

> **以最小、可审查、可测试、可回滚的代码变更，安全地实现明确需求。**
