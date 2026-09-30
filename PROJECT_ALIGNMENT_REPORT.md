# 项目规范对齐与协作确认报告

> 生成时间：2026-09-30
> 阅读范围：`AGENTS.md`、`docs/README.md`、`docs/development/` 下全部 Markdown 文件
> 说明：本报告仅为规范对齐，不包含任何业务代码。

---

## 阅读结果与现状说明

| 文件 | 状态 |
|---|---|
| `AGENTS.md`（根目录，最高准则） | 已完整阅读 |
| `docs/README.md`（规范导航地图） | 已完整阅读 |
| `docs/development/go.md` | 已完整阅读 |
| `docs/development/architecture.md` | 已完整阅读 |
| `docs/development/domain.md` | 已完整阅读 |
| `docs/development/api.md` | 已完整阅读 |
| `docs/development/database.md` | 已完整阅读 |
| `docs/development/config.md` | 已完整阅读 |
| `docs/development/observability.md` | 已完整阅读 |
| `docs/development/security.md` | 已完整阅读 |
| `docs/development/testing.md` | 已完整阅读 |
| `docs/development/dependencies.md` | 已完整阅读 |
| `docs/development/commenting.md` | 已完整阅读 |
| `docs/development/environment.md` | 已完整阅读 |
| `docs/development/git.md` | **文件不存在**（`docs/README.md` 索引中已登记但实际缺失） |

补充观察（仅告知，不自行处理）：

- 仓库当前**尚未初始化**：`go.mod`、`Makefile`、`docker-compose.yml`、`cmd/`、`internal/`、`migrations/`、`.env.example`、`.golangci.yml` 均不存在。
- 因此规范中引用的 `go 1.22/1.23` 版本、白名单依赖、GORM/Redis/MySQL 具体选型在代码落地前仍需以实际 `go.mod` 为准，我不会凭规范示例臆断。
- `docs/README.md` 索引的 `git.md`（Conventional Commits、PR 自检清单、禁止破坏性指令）缺失。在补齐前，Git 相关行为我将以 `AGENTS.md` 第 39 章（不擅自提交、不修改用户已有改动、禁止破坏性命令）为准。

---

## 1. 核心红线与禁忌

以下 5 条为后续开发中**绝对不可触碰**的底线，任何情况下不因"任务赶进度"而妥协。

### 红线一：最小修改原则与"不扩大需求"（对应 `AGENTS.md` §1.2 / §2.3 / §57 / §85）

- 需求没有要求 ≠ 可以自行设计。不得擅自推断并修改 DB schema、权限模型、状态机、API 返回结构、缓存策略、MQ、部署配置或第三方服务。
- 严禁"顺手"行为：格式化全项目、重命名无关变量、重构无关函数、升级无关依赖、删除看不懂的代码、修改错误码/日志格式/公共接口。
- 发现无关问题只**报告**，不**修复**，等待你单独确认。
- 需求与实现冲突时按 `AGENTS.md` §101 优先级裁决：你的明确要求 > 项目已有实际行为 > 安全与数据一致性 > 当前架构约定 > 本文件规则 > Go 官方惯例 > 我的个人偏好。**我的偏好永远不能凌驾于项目约定。**

### 红线二：禁止伪实现、假数据与伪造验证结果（对应 `AGENTS.md` §37 / §79 / §80 / §81 / §83）

- 禁止用 `return nil` / `return nil, nil` / `TODO` / `panic("not implemented")` / `mock response` 冒充完成。
- 禁止在生产代码中植入 fake user、mock order、hardcoded response、random ID、dummy token、fake success。测试数据只能存在于 `_test.go` 或 fixture 中。
- **绝不谎报测试通过**。必须严格区分"已执行并通过 / 未执行 / 无法执行（原因是什么）"。
- 不得为了让测试通过而放宽校验、删除错误处理、特判测试数据或用 `time.Sleep` 掩盖并发问题。
- 完不成时必须显式输出"已完成 / 未完成 / 原因 / 需要什么"，不伪装完成。

### 红线三：安全防御不可绕过（对应 `AGENTS.md` §43–§46 / `security.md`）

- **零信任输入**：HTTP Header、Query、Path、Body、Cookie、Webhook、MQ、环境变量一律不可信，必须在边界做 validation / normalization / authorization / escaping / size limit。
- **注入防御不可妥协**：SQL 必须参数化（禁止 `fmt.Sprintf` 拼 SQL、禁止 `SELECT *`）；命令执行禁止 `exec.Command("sh", "-c", input)`；外部 URL 必须防 SSRF（拦截 `127.0.0.1`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`169.254.169.254`）；文件路径必须防穿越并校验 Magic Number。
- **Secret 零泄漏**：源码中禁止出现明文密码、DB 凭据、AK/SK、JWT Key；一律走环境变量 / 配置中心；Git 提交须有 `gitleaks`/`trufflehog` 拦截。日志禁止输出 password、token、API key、Cookie、Authorization、完整身份证/银行卡；手机号、身份证、银行卡必须脱敏。
- **鉴权与授权分离**：所有个体数据接口（如 `/api/v1/orders/{id}`）必须做显式属主校验 BOLA/IDOR 防御（如 `order.UserID != currentUser.ID → 403`），默认拒绝，权限检查贴近业务边界，后端不依赖前端隐藏按钮。
- **JWT 硬约束**：仅允许 RS256/ES256 非对称算法，严禁 `none` 算法与弱 HMAC 密钥；Payload 最小化；Access Token 有效期 < 2 小时并配合 Redis 存 Refresh Token 与主动撤销。
- **内部错误不外泄**：禁止把 `sql: no rows in result set` 之类原始错误返回给用户，必须映射为项目定义的业务错误码。

### 红线四：依赖变更与架构边界（对应 `dependencies.md` / `AGENTS.md` §31 / §69 / §89）

- **标准库优先**，默认不新增依赖；不为少写十行代码引入巨型 SDK。
- 仅可使用 `dependencies.md` 白名单：gin/chi、gorm 或 sqlx、`redis/go-redis/v9`、viper、zap、`golang.org/x/sync`、testify + `go.uber.org/mock`、`encoding/json` 或 sonic。
- **黑名单严禁引入**：`github.com/pkg/errors`（统一用 `fmt.Errorf("%w")` + `errors.Is/As`）、反射式隐式注入框架（`uber-go/dig`）、全局万能包（`lancet`、自建 `util/common/helpers/misc`）。
- 禁止擅自 `go get` / `go mod tidy` 引发级联升级；依赖变更必须锁定具体 Tag 或 Commit SHA（禁止依赖 `master`），变更后强制执行 `go mod tidy` + `go test ./...` + 检查 `git diff go.mod go.sum`。
- 禁止擅自引入新架构模式：CQRS、event sourcing、DDD 全套、DI 框架、新 ORM、新 MQ、monolith 拆微服务。**架构复杂度本身就是成本。**
- 禁止 `gofmt ./...` 造成全项目噪音 diff；只格式化本次改动的文件。

### 红线五：数据与生产环境安全（对应 `database.md` / `AGENTS.md` §22 / §39 / §47 / §49 / §90）

- **Schema 变更视为高风险**：`DROP`、`TRUNCATE`、`ALTER COLUMN`、改字段类型、改唯一约束/索引/外键，必须先与你确认；必须走 Migration（`golang-migrate`，文件名 `YYYYMMDDHHMMSS_description.up.sql` / `.down.sql`），不直接改已执行 migration，生产禁止破坏性 DDL（须"加新列 → 双写 → 迁移数据 → 废弃旧列"）。
- **金额禁止浮点**：一律 `int64` 最小货币单位（分），仅在 Response DTO 格式化展示；DB 用 `BIGINT` 或 `DECIMAL(16,4)`，禁止 `FLOAT/DOUBLE`。
- **API contract 是兼容性边界**：URL、Method、Request/Response JSON、status code、error code、header、分页/排序/过滤规则，未明确要求不得改动；统一响应体 `{code, message, data, trace_id}`，5 位数字业务错误码命名空间（`1xxxx` 通用 / `2xxxx` 用户 / `3xxxx` 订单支付）。
- **JSON 字段命名不得随意变更**（`user_id` 不许改成 `userId`）。
- **Git 红线**：不擅自 commit；不执行 `git reset --hard` / `git clean -fd` / `git checkout -- .` / `git restore .`；不修改或清理你已有的 modified / untracked / staged 改动。
- 完成后必须 `git diff` / `git status` 自检：无 debug code、无临时文件（`foo.go`/`tmp.go`/`*.bak`/`debug.log`）、无 secret、无误删代码。

---

## 2. 架构分层与依赖规则

### 2.1 统一分层模型（`architecture.md` §1–§3）

```text
[ Transport / Handler ]        HTTP / gRPC / MQ Consumer
            ↓
[ Application / Service ]      业务流程编排、领域逻辑
            ↓
[ Domain / Model ]             核心业务实体、值对象
            ↓
[ Infrastructure / Repository ] MySQL / Redis / 外部 API
```

依赖方向严格**由外向内单向**；底层不得反向 import 上层。**禁止跨层调用**：Handler 严禁绕过 Service 直接调 Repository 或写 SQL。

标准目录结构（遵循 `golang-standards/project-layout`）：

```text
cmd/api/          cmd/worker/        # main 入口
internal/config/  internal/handler/  internal/service/
internal/repository/  internal/model/  internal/pkg/   # 仅内部使用
pkg/              api/               migrations/  scripts/
```

### 2.2 各层职责边界与禁止行为

| 层 | 职责 | 严禁 |
|---|---|---|
| **Handler** | 绑定解析 Body/Query/Path；调用 Validator 校验入参；提取 Context 中 Auth 元数据；调用 Service；将业务结果转为统一 HTTP Response；设置 status | 写 SQL / Redis 命令；处理复杂事务与回滚；承载业务规则、外部 HTTP、MQ、缓存 |
| **Service** | 组合多个 Repository 完成复杂业务；处理状态机、计费、权限判定；控制 Transaction 边界 | 引用 `gin.Context` / `http.Request` 等 HTTP 结构；返回 HTTP Status Code；处理 SQL 字符串细节、JSON 编解码、Request/Response 解析 |
| **Repository** | CRUD；屏蔽 SQL 驱动细节、Redis Key 拼装细节、第三方 HTTP SDK 细节；事务辅助 | 任何业务判断（如"余额是否足够"应提供查询 API 由 Service 判定）；反向依赖 Handler/Service |

`context.Context` 恒为**第一个参数**、变量名 `ctx`；禁止把 `ctx` 存进结构体；`context.WithValue` 仅用于 request-scoped 元数据（`trace_id`、`client_ip`、`current_user_id`），Key 必须是私有自定义类型（如 `type ctxKeyTraceID struct{}`），严禁用于传业务隐式参数；派生 `WithTimeout/WithCancel` 必须 `defer cancel()`。

### 2.3 数据传递与 DTO 规范

- **单一 Struct 不得兼职** DB Model / API Request / API Response / Domain Entity / Message / Config。
- **转换链路单向**：`DB Model → Domain Model → Response DTO`，数据库模型禁止直接作为 HTTP Response 返回。
- **Request DTO** 与 **Response DTO** 分离；Request 承担 `go-playground/validator` 校验标签（如 `binding:"required,email,max=100"`）；Response 承担脱敏（如手机号 `138****1234`）与金额格式化。
- **命名对齐 `domain.md` 通用词汇表**：User/Account(`user_id`)、Order(`order_no`)、Touchpoint(`touch_id`)、Lead(`lead_id`)、Balance(`balance`)、Campaign(`campaign_id`)；禁止 `usr`/`member`/`trade`/`bill` 等错误命名；DB 字段、JSON Tag、变量名三者必须一致。
- **ID 与单号**：对外业务单号带前缀 —— 订单 `ord_`+Snowflake、支付流水 `pay_`+Snowflake、触点 `tch_`+UUID v7；ID 类型以项目已有定义为准，不臆断。
- **数据库约定**：表名小写下划线复数（`order_items`）；`utf8mb4` + `InnoDB`；基础字段 `id / created_at / updated_at / deleted_at(软删除)`；唯一索引 `uk_*`、普通索引 `idx_*`，组合索引遵循最左前缀，单表索引 ≤ 5 个；GORM 关联查询必须 `Preload`/`Joins` 预加载，禁止循环内查询（N+1）；事务范围最小化，事务内严禁外部 HTTP / RPC / 耗时计算。
- **Redis**：Key 命名 `系统名:模块名:业务实体:ID`（如 `mall:user:profile:10086`）；**所有 Key 必须设 TTL**；过期加随机抖动防雪崩；热点 Key 考虑互斥重建/分布式锁。缓存不是默认解决方案，新增前必须先回答 key/TTL/失效/不一致/miss/失败/stale/stampede 八个问题。

### 2.4 依赖注入要求（`architecture.md` §4）

1. **构造函数显式注入**，所有 Service / Repository 通过构造函数接收其依赖的 Interface；**禁止全局变量**持有依赖。
2. **接口由消费方定义、保持极小**：单方法接口以 `er` 结尾（`Reader`/`Writer`/`Validator`/`Storer`），多方法接口以业务名词命名（`UserRepository`/`OrderService`）；**禁止 `I` 前缀**；禁止为 mock 方便而给每个 struct 造一个镜像接口。
3. **DI 框架**：小型项目用纯手动组装（推荐）；大型项目用 `google/wire`（编译期静态生成）；**禁止基于运行时反射且隐式魔改的 DI 框架**（含 `uber-go/dig`）。
4. **Functional Options**：可选参数多的结构体用 `Option func(*T)` + `WithXxx()` 模式，禁止传一长串 `nil` / 零值。
5. **循环依赖解耦顺序**：先下沉接口/公共模型到 `model` 或独立接口包 → 再考虑事件驱动异步解耦 → 最后重新评估并拆分包。**禁止靠复制代码或奇怪技巧绕过 import cycle。**

---

## 3. 代码质量与工程规范

### 3.0 编码风格与代码格式化（唯一权威：Go 官方 `gofmt`）

> **对齐结论：此前未对齐。** 经全仓库检索，规范体系仅在 `go.md` §1.2 规定 `gofmt -s -w` 与 import 三段分组，**未定义任何 IDE 代码风格**，也未约束 GoLand 的格式化与命名行为。存在"IDE 默认风格覆盖官方风格"的风险。本节即为补齐后的**唯一有效约定**，与其他章节冲突时**以本节为准**。

#### 3.0.1 权威基准与优先级

```text
Go 官方 gofmt（golang.org/x/tools/cmd/goimports 之前的权威 formatter）
    ↓ 一致性基准
GoLand Code Style: 必须显式选择 "gofmt"（禁用默认 "GoLand" 风格）
    ↓ 一致性基准
其他一切工具（gofumpt / gci / goimports / 手工排版）
```

- **唯一格式化权威 = Go 官方 `gofmt`**（`-s` 开启 simplify）。缩进、对齐、大括号、逗号、注释位置、复合字面量排布**全部以 `gofmt` 输出为准**。
- **`gofmt` 与 IDE 冲突时，一律以 `gofmt` 输出为准**（重新格式化后提交）。IDE 不得成为风格源头。
- 允许的工具及职责边界（各司其职，不得越界）：

| 工具 | 唯一职责 | 不得承担 |
|---|---|---|
| `gofmt -s -w` | 全部机械格式化（缩进/对齐/括号/行尾） | 不管 import 分组 |
| `goimports` | import 增删 + 按 `go.md` §1.2 三段分组 | 不做其他排版决策 |
| `gofumpt` | **本项目默认不启用**；如启用须经你确认，且不得与 gofmt 产生冲突 diff | — |
| `gci` | **本项目默认不启用**（import 分组由 goimports 承担） | — |
| `golangci-lint` | 只做静态检查（`gofmt`/`gofumpt` 检查项仅作门禁） | 不做修复 |

- **gofmt 不做的事，不得手工代劳**（避免"人工另类风格"）：
  - **不做**行宽折行 —— 官方 `gofmt` **不折长行**，长表达式/长函数签名保持单行；禁止手写换行"美化"。
  - **不排序** import 分组内部成员（仅按路径字典序），分组的语义顺序由 `goimports` 的三段式决定。
  - **不删除/不新增**空行语义之外的排版决策（空行位置以 `gofmt` 输出为准）。

#### 3.0.2 GoLand IDE 强制配置（Code Style 需人工设置，默认值不合规）

> GoLand 的默认 Code Style 并**不等价于** `gofmt`（存在额外重排规则）。**必须逐台机器按下列设置校准**，否则会出现"IDE 保存后与 CI 的 `gofmt` 结果不一致"。

| 设置路径 | 必须值 | 违反后果 |
|---|---|---|
| `Settings → Editor → Code Style → Go → Code style` | **`gofmt`**（禁用 `GoLand` 默认风格） | IDE 自动重排与官方风格冲突，产生噪音 diff |
| `Settings → Editor → Code Style → Go → Import layout` | 与 `go.md` §1.2 **三段分组**：`std | blank | others`（本项目内包并入第三段，段间空一行） | import 分组风格漂移 |
| `Settings → Editor → Code Style → Go → Tabs and Indents` | **Use tab character = on**、**Indent = 4**（用于 tab 显示宽度）；**实际缩进字符必须是 Tab** | 空格缩进 → 全文件 diff |
| `Settings → Go → GoFmt integration` | 开启 **Run 'gofmt' on save**（或 Format on save 使用 Go 格式化器） | 依赖人工 Reformat，易漏 |
| `Settings → Editor → Code Style → Go → Naming conventions` | **保持默认、禁止启用任何自动重命名规则** | GoLand 自动改写标识符，破坏 domain 词汇表 |
| `Settings → Editor → Code Style → Go → Wrapping and Braces` | **保持 gofmt 默认**，不得自定义 brace style / 空格 / 引号 | Allman 大括号、`= 号对齐` 等另类风格 |
| `Settings → Editor → Code Style → Go → Blank Lines` | **保持 gofmt 默认**（上限 2） | 手工大段空行 |

- **必须禁用的高危 IDE 行为**：
  - `Reformat Code`（`Ctrl+Alt+L`）**仅允许作用于本次改动文件**；对既有文件全局 Reformat 视为 §84 禁止的"无关格式化噪音"。
  - 禁止启用 `Save with cleanup` / `Optimize imports on save` 之外的自定义模板替换。
  - 禁止多人使用**不同 Code Style**（`Settings → Editor → Code Style → Go → Enable per-directory code style` 时，必须共享同一份 `Project.xml`）。

#### 3.0.3 官方风格硬性约定（Go Code Review Comments + gofmt 语义）

- **缩进**：一律 **Tab**（`\t`），**禁止空格缩进**；显示宽度 4 不改变实际字符。
- **行尾空格**：禁止。CRLF/LF 以仓库 `.gitattributes` 统一为准，**不得因本地环境产生全文件行尾 diff**。
- **对齐**：仅保留 `gofmt` 自动产生的对齐（连续赋值 `=`、struct 字段与其 tag、连续行尾注释）。**禁止手工对齐"列"**（如为对齐而补空格，而 gofmt 会拆开）。
- **行宽**：官方 `gofmt` 不折行，**不因超长而手工换行**；如需可读性拆分，应通过提前 `return` / 提取局部变量（`go.md` §3.1 Guard Clause），而非折行排版。
- **大括号**：K&R 风格，左大括号与语句同行，**禁止 Allman / 独立行大括号**。
- **复合字面量**：`gofmt` 会把短 slice/map 字面量压成单行（如 `10: {15, 30}`），这是**正确输出，不得改回多行**。
- **分号**：Go 无分号，**禁止手写行尾分号**。
- **字符串/反引号**：沿用 gofmt 输出；含换行用反引号；**禁止为了"美观"改写引号形态**。
- **import**：严格三段（`go.md` §1.2）——① 标准库 ② 第三方 ③ 本项目内部包；段间空一行；**除数据库驱动/迁移插件外禁止匿名导入 `_`**。
- **命名**：`userID` / `httpURL` / `dbConn`（`go.md` §2.2）；包名小写单词、无下划线；**禁止 `util/common/helpers/misc`**。
- **注释**：解释 Why 而非 What（见 3.4 节 `commenting.md` 要求）。
- **语义化换行**：可读性来自**结构（Guard Clause、Early Return、单一职责）**，**不来自手工排版**。

#### 3.0.4 格式门禁（提交前必须全绿）

```bash
# 1) 格式化门禁：输出必须为空（对全部包做只读检查，不写回）
gofmt -l .
test -z "$(gofmt -l .)"

# 2) 仅对本次改动文件写回格式化（禁止 gofmt ./... 造成全项目 diff）
gofmt -s -w <changed-file.go> ...

# 3) import 分组校验（若已安装 goimports）
goimports -l <changed-file.go>

# 4) 静态检查
go vet ./...
golangci-lint run ./...
```

- **CI 门禁**：Format Check 失败即拒绝合并（`testing.md` §6 序列第一步）。
- **发现格式偏差时的处理流程**：先 `gofmt -l .` 定位文件 → 确认是否为**本次改动引入** → 若是，`gofmt -s -w` 该文件；**若为历史遗留格式问题，只报告不修复**（`AGENTS.md` §2.3 最小修改原则、§84 禁止无关格式化）。
- 任何"我按自己习惯排版更好看"的理由，**不构成偏离 `gofmt` 的依据**。

#### 3.0.5 待补齐的工程配置文件（当前缺失，待你确认后我再创建）

以下文件是"官方风格 + GoLand 一致性"落地所必需的，当前仓库**均不存在**。按 §4.2 边界声明，**我不擅自创建**，请确认后我再执行：

| 建议文件 | 作用 | 关键内容 |
|---|---|---|
| `.editorconfig` | 跨编辑器统一（GoLand 也读取部分项） | `root=true`、`[*]` `end_of_line=lf`、`insert_final_newline=true`、`trim_trailing_whitespace=true`、`indent_style=space`（**非 Go 文件**）；`[*.go]` **`indent_style=tab`**、`indent_size=4`、`charset=utf-8` |
| `.idea/codeStyles/Project.xml` | 让 GoLand 打开即用 `gofmt` 风格，团队共享 | `<codeScheme name="gofmt" .../>` + tab/缩进/import layout 设置 |
| `.idea/codeStyles/codeStyleSettings.xml` | 声明 Project 级 Code Style 为 scheme | 指向 `Project.xml` |
| `.gitattributes` | 统一行尾，防止全文件 CRLF diff | `*.go text eol=lf` |
| `.golangci.yml` | 把 `gofmt` 门禁写进 CI 与 pre-commit | 含 `stylecheck`/`godot`/`godox`/`errcheck` 等（`commenting.md` §7） |
| `Makefile` | 统一 `fmt` / `fmt-check` 指令，AI 与人共用一套 | `fmt-check: gofmt -l .` 断言为空 |

#### 3.0.6 承诺

- 我生成的**任何 Go 代码**，必须与我本地执行 `gofmt -s -l <file>` 的结果**完全一致**（无输出）。
- 我**不产出**任何"个人审美式"排版：手写折行、手工空格对齐、非常规缩进、非 gofmt 的大括号/引号风格。
- 若我给出的代码经 `gofmt` 检查有偏差，视为**我的交付缺陷**，我必须自行修正后再提交，不得让你来兜底。

---

### 3.1 错误处理（`go.md` §5 / `AGENTS.md` §9）

- 跨层传递必须 `fmt.Errorf("动作描述 %w", err)` 追加当前步骤上下文，**禁止 `%v`**（会切断 error chain，`errors.Is/As` 失效）。
- 判等**只能用** `errors.Is` / `errors.As`，**绝对禁止** `err.Error() == "..."` 字符串匹配。
- **Sentinel Error** 在 package 顶层集中定义并复用（`ErrUserNotFound`、`ErrForbidden`、`ErrInsufficientStock`…），禁止重复创建同语义错误。
- **禁止吞错** `_ = doSomething()`，除非能证明可安全忽略并在注释中说明原因（`errcheck` linter 强制）。
- 禁止 `errors.New("failed")` 这类无上下文错误。
- **Panic 严苛限制**：Handler / Service / Cron / 业务代码绝对禁止 `panic`；仅允许在 `main()`/`init()` 启动期遇到不可修复依赖缺失时使用 `panic` 或 `log.Fatalf` 终止。
- 并发编排统一用 `errgroup.WithContext`，`g.Wait()` 后包装错误。

### 3.2 结构化日志与 TraceID（`observability.md`）

- 强制 **`go.uber.org/zap`** 强类型结构化日志（禁用 `logrus`、`log/syslog`）；**禁止 `Infof` 拼装字符串**。
- 所有日志**必带基础字段**：`ts`(ISO8601)、`level`、`trace_id`、`caller`(如 `service/user.go:42`)。
- 字段用具名类型表达：
  ```go
  logger.Info("user login success",
      zap.String("trace_id", traceID),
      zap.Int64("user_id", userID),
      zap.String("client_ip", clientIP),
  )
  ```
- **级别准则**：`DEBUG` 仅开发环境、上线关闭；`INFO` 仅关键生命周期与关键业务成功，**禁止在高频循环体内打 INFO**；`WARN` 可自动恢复的非致命异常（第三方超时重试、缓存 miss 回源）；`ERROR` 需人工介入并告警。
- **全链路追踪**：`TraceID`/`SpanID` 通过 `ctx` 在 Handler → Service → Repository → 外部 HTTP/gRPC 显式传递；使用 `go.opentelemetry.io/otel` 建 Span，错误时 `span.RecordError` + `span.SetStatus(codes.Error, ...)`；`defer span.End()`。
- **脱敏拦截**：日志输出前过滤 password、完整卡号、JWT Token、手机号（`138****1234`）、身份证、银行卡、Cookie、Authorization。
- **Metrics**：Prometheus 蛇形命名 + 明确单位（`http_requests_total{method,status}`、`http_request_duration_seconds_bucket`）。
- 日志消息本身必须含上下文（`failed to create order: order_id=123`），不允许仅 `failed`。

### 3.3 状态机合法性转移（`domain.md` §2）

- 所有业务实体状态变更必须走**显式状态机**，禁止随意改状态字段。
- 状态枚举须逐行注释含义；DB 中用 `TINYINT` 并在 SQL COMMENT 写明枚举映射。
- 转移合法性以**转移矩阵**集中定义并由 `ValidateStateTransition(current, target)` 判定，Service 层调用，非法转移返回 Sentinel Error。
- 订单状态机标准枚举：`StatusPending(10) → StatusPaying(15) | StatusCanceled(30)`；`15 → 20(已支付) | 25(失败)`；`20 → 40(已退款)`；终态（`25/30/40`）无出边 → 返回 `false`。
  ```go
  var allowedTransitions = map[int8][]int8{
      10: {15, 30},
      15: {20, 25},
      20: {40},
  }
  ```
- 状态机转移、计费、权限判定归 **Service / Domain** 层，**Repository 不得承担**。
- 涉及状态变更的写操作必须**考虑事务**与**幂等性**（支付、下单、webhook、重试、定时任务、外部写 API）。

### 3.4 单元测试（`testing.md`）

- **同源同包**：`_test.go` 与被测文件同目录同 package。
- **覆盖率硬指标**：Service 层核心业务 **≥ 70%**；工具类 / 基础库（pkg）**≥ 85%**。
- **确定性**：可重复执行，禁止依赖网络连通性与系统绝对时间；测试前后不留垃圾数据（用 `scripts/seed.sql` 或 testfixtures 注入并清理）。依赖时间的代码通过注入 `Clock` 或 `clockwork`，**禁止 `time.Sleep()` 等待**。
- **表格驱动测试为强制**（分支多、输入输出明确的函数）：
  ```go
  tests := []struct {
      name    string
      args    args
      want    int64
      wantErr bool
  }{ ... }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- **覆盖场景**：正常路径、空输入、nil、边界值、非法输入、依赖错误、超时、并发行为、权限、重复调用、数据不存在。
- **命名表达行为**：`TestCreateOrder_DuplicateEmail` 而非 `TestCreateOrder2`；断言库 `testify/assert`（继续校验）与 `require`（失败即停）；自定义 Helper 必须 `t.Helper()`。
- **Mock 范围克制**：`go.uber.org/mock` 或手写 Stub；**单元测试绝对禁止连真实数据库/外部 API**；集成测试可用 `testcontainers-go` 拉起临时 MySQL/Redis。禁止为提升覆盖率制造无意义 mock，禁止测试实现细节而非行为。
- **并发必检**：`go test -v -race -cover ./...`；发现 race 不得用 sleep 掩盖。
- **静态检查**：`gofmt -s -w`（只对改动文件）、`go vet ./...`、`golangci-lint` 必选 `errcheck`/`staticcheck`/`govet`/`ineffassign`/`goconst`，另启用 `stylecheck`（ST1000/ST1003/ST1020–ST1022）、`godot`、`godox`（FIXME/BUG）。
- **CI 序列**：Format Check → `go vet` → `go test -race -coverprofile` → 覆盖率下限检查。
- **注释规范（`commenting.md`）**：每个包需 `// Package xxx` 包注释（建议放 `doc.go`）；所有导出标识符与接口方法必须有 Godoc 注释且**以标识符名开头**、句末标点、说明入参/返回值/Sentinel Error/并发安全；注释解释 **Why / Trade-offs / Caveats**，禁止 `// 把 count 加 1` 式 What 注释；改逻辑必须同步改注释；特殊标记统一 `// MARKER(author/issue): 描述`（`TODO(...)`、`FIXME(...)`、`// Deprecated:`）；Handler 必须写 Swag 注释（`@Summary/@Router/@Success/@Failure/@Param`），**生成物（swagger.json/openapi）不手工改，改源后重新生成**。
- **命名规范**：`userID`、`httpURL`、`dbConn`；包名小写单词无下划线；禁止 `util/common/helpers/misc` 万能包；`interface{}` 之外不留无用抽象；常量替代魔法数字（如 `maxRetryCount`）。
- **并发与资源**：goroutine 必须明确谁等待、谁取消、谁处理错误、何时退出；`defer cancel()` / `defer close()` / `defer resp.Body.Close()`；锁粒度最小、不跨外部 IO、不嵌套、顺序一致；HTTP 客户端必须 timeout + context 传递 + 错误映射 + 连接复用，**禁止裸 `http.Get`**；Retry 非默认，须先确认幂等性、可重试错误、最大次数、backoff、jitter、总超时、重复写风险。
- **配置（`config.md`）**：配置与代码彻底分离；viper 统一加载映射为强类型只读 Struct（`mapstructure` + `validate` 标签）；**业务代码禁止 `os.Getenv`**；启动即校验（Fail-Fast），必填缺失终止启动；根目录提供非敏感 `.env.example`；日志级别随环境（dev=DEBUG / staging=INFO / prod=WARN|ERROR）；环境差异不硬编码。
- **本地开发（`environment.md`）**：优先调用 `Makefile` 预设命令（`fmt`/`vet`/`lint`/`test`/`test-race`/`build`/`dev-up`/`dev-down`/`clean`），**不自行发明脚本**；`docker-compose.yml` 一键起 MySQL/Redis。

### 3.5 完成定义（DoD）

需求已实现 → 编译通过 → 相关测试通过 → 新行为有测试 → 格式正确 → 静态检查通过 → 无明显安全问题 → diff 已检查 → 无无关修改与临时文件 → 明确说明验证范围与未验证风险。任一未满足即**不得宣称完成**。

---

## 4. 后续协作契约与工作流

### 4.1 双阶段工作流承诺

我确认：当你提出具体开发需求后，我将**严格采用两阶段工作流，绝不跳过第一阶段直接产出业务代码。**

**阶段一：分析与方案（不写业务代码）**

1. 复述并确认需求边界 —— 明确"要改什么"与"不改什么"，超出部分单独列出待你决策。
2. 扫描上下文：项目结构 → `go.mod`（Go 版本 / 现有依赖）→ 相关 package → 目标文件 → **调用方与被调用方** → 相关接口 / 类型 / 错误 / 配置 / 测试 → 确认是否已有同类实现。
3. 输出**改动文件清单**（新增 / 修改，逐个说明职责与所属层），标注是否触及 DB schema、API contract、公共 Go 接口、依赖、生成物等**高风险面**。
4. 输出**架构设计方案**：分层落点（Handler / Service / Repository / Model）、接口定义（方法签名、参数、返回值、Sentinel Error）、数据流与 DTO 转换链路、状态机转移合法性、事务边界、幂等与并发考量、配置项、日志与 Trace 埋点、测试用例矩阵与覆盖率目标。
5. 输出**验证计划**：将执行的 gofmt / go test / go vet / go test -race / integration 及对应的 AGENTS.md §95 验证矩阵判定。
6. 列出**待确认问题**：任何无法从代码、测试、配置、文档中确认的信息（表结构、字段、错误码、超时、重试策略、权限规则等）一律**提问，绝不臆测**。
7. **等待你明确确认或提出修订**。

**阶段二：实施与验证（确认后才执行）**

1. 按确认方案实施最小变更。
2. 同步补充/更新单元测试（表格驱动优先，覆盖边界与错误路径）。
3. 依次执行：`gofmt -l .`（断言为空）→ `gofmt -s -w`（仅改动文件）→ 相关包测试 → `go test ./...` → `go vet ./...` → 涉及并发时 `go test -race ./...` → 涉及 API/DB/外部依赖时执行对应 integration / migration 验证。
4. `git diff` / `git status` 自检：无 debug code、无临时文件、无 secret、无格式化噪音、无误删。
5. 按 `AGENTS.md` §100 输出最终说明：**修改内容 / 实际执行的验证命令（区分已通过·未执行·无法执行及原因）/ 遗留风险**。

### 4.2 边界声明

- 我**不会**自行 commit，除非你明确要求 `commit`。
- 我**不会**使用破坏性 Git 命令，**不会**清理或覆盖你已有的工作区改动。
- 我**不会**在方案未获确认前写入业务代码；分析阶段的产出仅为文档与清单。
- 我交付的所有 Go 代码均以 **Go 官方 `gofmt` 为唯一风格权威**，并已与 GoLand `gofmt` Code Style 校准；我不会产出任何个人审美式排版（详见 3.0 节）。若你或其他 IDE 的 `gofmt` 结果与我不同，**以 CLI `gofmt` 输出为准**。
- 我需要 IDE 配置文件（`.editorconfig` / `.idea/codeStyles/*` / `.gitattributes` / `.golangci.yml` / `Makefile`）时，将先给方案、经你确认后再创建。
- 遇到规范冲突，按 `AGENTS.md` §101 优先级判定；若冲突涉及你的既有业务决策或未记录在案的隐含知识，我会**停下来提问**而不是自行决定。
- 若 `docs/development/git.md` 需要补齐，或规范与实际代码现状冲突（如 `go.mod` 缺失导致白名单选型待定），我会先提出建议并等你确认，不擅自创建。
- 后续若引入新依赖、修改 API contract、变更 DB schema、调整状态机或跨模块重构，我将**先出专项方案并获得你确认**再动手。

---

## 5. Vibe Coding 前置内核（深度理解与交叉复核）

> 本节不是新增规范，而是把上述 13 份文档**压缩成可执行的判断结构**，并给出我已识别的规范内部冲突清单。作用：让后续每一次开发都走同一条确定性路径，而不是临场发挥。

### 5.1 规范的内在逻辑：四层约束模型

所有规范可归约到 4 层，每层只回答一个问题：

| 层 | 回答的问题 | 主要来源 | 违规后果 |
|---|---|---|---|
| **L1 意图层** | **能不能做 / 要不要做** | 红线一~五、`AGENTS.md` §1.2 §2.3 §101 | 越界、扩大需求、破坏安全与数据 |
| **L2 结构层** | **放在哪一层 / 什么形态** | `architecture.md`、§2.2–§2.4 | 跨层调用、循环依赖、DB Model 泄漏到 API |
| **L3 契约层** | **叫什么 / 长什么样 / 传什么** | `domain.md` §1–§4、`api.md` §2–§4、`database.md`、`config.md` | 命名漂移、契约破坏、状态机错乱、金额精度事故 |
| **L4 门禁层** | **凭什么算完成** | §3.0 gofmt、`go.md` §7、`testing.md`、`environment.md` | 噪音 diff、伪完成、不可审查 |

**关键认知（本项目最易被 AI 犯的两类错）**：

1. **跳过 L1 直冲 L3** —— 这是最危险的错误形态。表现为"顺手"加表字段、自创 JSON tag、自定义错误码、自造状态值、新增缓存层。**结果看似完整，实则破坏了他人依赖的契约。**
2. **跳过 L4 宣称完成** —— 表现为"代码写完了"但 `gofmt` 未跑、测试没跑、diff 没看、覆盖率不达标、风险未说明。

因此我的固定动作序列是：**先 L1 定边界 → 再读现状（L0）→ L2 定落点 → L3 定契约 → 实现 → L4 全绿 → 报告**。任何一层缺证据，就停在那一层提问，不向下推进。

### 5.2 贯穿主线与三个"停止条件"

```text
需求理解
  → 边界界定（明确"不做什么"）
  → 读现状（结构 → go.mod → 调用方/被调用方 → 相关测试）
  → 先搜索后创建（是否已有同类实现）
  → 分层落点（Handler/Service/Repo/Model 各承担什么）
  → 契约固化（命名 · 状态转移 · 错误码 · DTO · schema · 配置 · 埋点）
  → 最小实现
  → 门禁全绿（fmt → test → vet → race → 覆盖率 → diff）
  → 如实报告（已验证 / 未验证 / 风险）
```

**三个强制停止条件**（触发即停下提问，绝不自行推进）：

- **STOP-1 信息不足**：无法从代码/测试/配置/文档/依赖源码确认的信息（表字段、错误码、超时、重试、权限规则、ID 类型、Go 版本）→ 提问，禁止"我认为应该是…"然后改核心逻辑。
- **STOP-2 不可逆或高影响**：破坏性 DDL、API contract 变更、依赖新增/升级、公共 Go 接口改动、生成物手改 → 先出专项方案待确认。
- **STOP-3 方案未确认**：第一阶段未获你明确确认 → 不写任何业务代码。

### 5.3 决策速查表（遇到场景 → 查依据 → 动作）

| 场景 | 判定依据 | 我的动作 |
|---|---|---|
| 要新增接口 | 先搜是否已有同层 service/repo | 复用优先；新增独立 DTO，**不改统一 Response 结构** |
| 要改状态 | `domain.md` 转移矩阵 | 非法转移直接拒绝并返回 Sentinel Error，不加"临时"后门 |
| 想加缓存 | §53 八问 | 八问答不全 → **不加** |
| 想加依赖 | `dependencies.md` 白名单 + C1 缺口 | 白名单外 → 先出评估方案待确认，锁 Tag/SHA |
| 想改表 | `database.md` + §22.4 | 走 migration（up/down），生产禁破坏性 DDL |
| 涉及金额 | `domain.md` §4 | `int64` 分，仅 DTO 层格式化，禁 float |
| 外部调用 | §25 §26 §27 | timeout + ctx 传递 + 错误映射 + 连接复用；retry 非默认 |
| 写日志 | `observability.md` §1 | zap 结构化 + `trace_id` + caller；禁 `Infof`；高频循环禁 INFO |
| 报错 | §9 / `go.md` §5.1 | `%w` + 上下文 + Sentinel Error；禁 `%v`、禁字符串比对 |
| 测试不通过 | §98 / §81 | 先判"实现错 or 测试错"，**禁止改业务迁就测试**、禁止 sleep |
| 覆盖率不达标 | `testing.md` §1 | 补测试用例，**不删测试、不改断言迁就** |
| 格式不达标 | §3.0 | `gofmt -s -w` 改动文件；历史遗留问题只报告 |
| 需求本身有风险 | §82 | **指出风险并说明影响**，不机械照做；若仍坚持则执行但记录风险 |
| 想重构/优化 | §57 §51 | 与功能分离；先建测试保护再小步改 |
| 发现无关问题 | §2.3 | **只报告，不修复**，等单独确认 |

### 5.4 交叉复核结果：规范内部冲突与缺口（10 项，**需你裁决，我不擅自处理**）

> 以下均为**规范之间**的冲突或覆盖不全，不是我要改代码，而是必须先由项目所有者定调，否则 vibe coding 会把矛盾带进实现。

| 编号 | 严重度 | 问题 | 冲突双方 / 缺口 | 待你裁决 |
|---|---|---|---|---|
| **C1** | **高** | **依赖白名单覆盖不全**：`dependencies.md` §2 白名单未包含其他规范**强制要求**的库 —— `go-playground/validator/v10`（`security.md` §2.1、`config.md` §2 启动校验）、`swaggo/swag`（`api.md` §6、`commenting.md` §6）、`go.opentelemetry.io/otel` + Prometheus client（`observability.md` §2/§3）、**JWT 库完全未指定**（`security.md` §1.1 只讲策略不讲实现）、Snowflake/UUID v7 生成器（`domain.md` §3）、`clockwork`（`testing.md` §5）、`testcontainers-go`（`testing.md` §4）、testfixtures（`environment.md` §3） | `dependencies.md` §2 vs 其余 6 份规范 | 扩充白名单？还是以各专项规范要求优先？（严格按现白名单执行将**无法实现**其他规范） |
| **C2** | 中高 | **Makefile `fmt` 与禁止无关格式化冲突**：`environment.md` §2 定义 `fmt: @gofmt -s -w .`（全项目写回），直接违反 `AGENTS.md` §84「禁止无关格式化造成大量 diff」 | `environment.md` §2 vs `AGENTS.md` §84 | 是否改为"`fmt` 接受显式文件参数 + 新增只读 `fmt-check` 门禁"？ |
| **C3** | 中 | **主键策略二义**：`database.md` §1.2 允许 `BIGINT UNSIGNED AUTO_INCREMENT`；`domain.md` §3 要求"统一使用 Snowflake 或 UUID v7"以"不泄漏商业隐私"；`api.md` 示例又出现 `id: 1024` | `database.md` §1.2 vs `domain.md` §3 vs `api.md` §2 | 全部表用雪花/UUIDv7？还是内部表自增 + 仅对外单号带前缀（`ord_`/`pay_`/`tch_`）？ |
| **C4** | 中 | **DTO 目录归属未定义**：要求 Request/Response/DB Model/Domain Entity 分离，但 `architecture.md` 只有 `internal/model/`（"数据实体与 DTO 定义"），未规定子目录或包划分方案；且 `commenting.md` 的 Swag 注释引用 `api.Response`，与 `architecture.md` 中 `api/`（OpenAPI/Proto 文件目录）**命名撞车** | `architecture.md` §2 vs §13.2/本报告 §2.3 vs `commenting.md` §6 | 统一响应体放在哪个包？DTO 是否按 `model/<entity>/{entity,request,response}.go` 划分？ |
| **C5** | 低 | **`internal/pkg/` 与禁止万能包冲突**：`architecture.md` §2 列 `internal/pkg/`（"仅供内部使用的工具包"），而 `AGENTS.md` §56 与依赖黑名单禁止 `util/common/helpers/misc` 万能包 | `architecture.md` §2 vs `AGENTS.md` §56 | 是否明确 `internal/pkg/` 准入规则（仅无领域归属的纯技术件、必须细粒度命名如 `stringutil`/`timeutil`）？ |
| **C6** | 低 | **状态机与 DB 注释不一致**：`domain.md` 定义 6 态（10/15/20/25/30/40），`database.md` §1.1 示例 COMMENT 仅写 "10-待支付, 20-已支付, 30-已取消"（缺 15/25/40）；`domain.md` 示例 `allowedTransitions` 使用**裸数字**而非常量，且 `ValidateStateTransition` 未指明包归属与导出性 | `domain.md` §2 vs `database.md` §1.1 | 落地时以 `domain.md` 转移矩阵为准（DB COMMENT 必须全量枚举）；是否需要我在实现时统一改为常量引用？ |
| **C7** | 低 | **import 三段分组缺工具支持**：`go.md` §1.2 要求 std / 第三方 / 内部包三段，但 `goimports` 默认仅分 std / others，需 `-local` 前缀才能实现第三段 | `go.md` §1.2 vs 工具默认行为 | 是否在 Makefile 固化 `goimports -local <module>` 参数？ |
| **C8** | 中 | **`git.md` 缺失**：`docs/README.md` 索引已登记 Conventional Commits、PR 自检清单、禁止破坏性指令，但文件不存在 | `docs/README.md` 索引 vs 实际文件 | 是否需要补齐该规范？当前暂以 `AGENTS.md` §39 为准 |
| **C9** | 中 | **`AGENTS.md` §105 项目特定规则占位区全为"待补充"**：项目结构、常用命令、分层说明、数据库、API 格式、日志器均未落地，导致大量"以项目实际为准"的判定当前无据可依 | `AGENTS.md` §105 | 是否在工程初始化时一并补齐？ |
| **C10** | 低 | **Go 版本未知影响写法**：`go.mod` 尚不存在 → `api.md` 的 `Data interface{}` 是否改为 `Data any`、能否使用泛型、能否用 `errors.Join` 等均无法确定 | 现状 vs `go.md` §1.1 | 初始化 `go.mod` 时请指定 Go 版本（规范示例提到 1.22/1.23） |

**处理原则**：以上任一项在实现中被触及，我会**先按现行规范执行并向你标注冲突点**，或**停下来提问**，绝不自行"选一边"并默默落地。

### 5.5 开工前置检查清单（每个任务开始前逐条过，不通过不进入实现）

1. `go.mod` 是否存在？Go 版本与现有依赖是什么？（决定白名单与语法特性）
2. 是否已搜索过同层同类实现（service / repository / DTO / 错误 / middleware / 配置 / 工具 / 测试）？
3. 需求边界是否已书面确认？"不做什么"是否明确？
4. 是否触及高风险面（API contract / DB schema / 依赖 / 公共 Go 接口 / 生成物）？
5. 分层落点是否明确？是否会造成跨层调用或反向依赖？
6. 命名是否对齐 `domain.md` 词汇表（变量名 / DB 字段 / JSON tag 三者一致）？
7. 状态变更是否通过转移矩阵校验？是否需要事务与幂等保障？
8. Sentinel Error / 业务错误码是否复用而非新造？是否会被映射到正确 HTTP status？
9. 日志是否结构化并带 `trace_id`？是否有敏感信息风险？是否需要 OTel Span？
10. 测试矩阵是否已列出（正常/空/nil/边界/非法/依赖错误/超时/并发/权限/重复/不存在）？覆盖率目标是否明确？
11. 验证计划是否覆盖 `AGENTS.md` §95 对应行？`-race` 是否需要？
12. 输出代码是否已过 `gofmt -s -l`（无输出）？diff 是否只含必要改动？

### 5.6 我已内化的行为约束（不再重复声明）

1. 先读后改，先搜后建，复用优先，最小变更。
2. 不臆测业务规则；信息不足即停并提问。
3. 不伪造实现、不伪造数据、不伪造测试结果。
4. 不扩大需求、不顺手重构、不自动升级依赖。
5. 不越层、不反向依赖、不用反射式 DI、不建万能包。
6. `%w` 传错、Sentinel Error 复用、禁 panic、禁吞错。
7. 金额用整数分、ID 与单号按项目定义、主键策略先问不猜。
8. zap 结构化 + trace_id + 脱敏；高频路径不打 INFO。
9. 表格驱动测试、覆盖率达标、`-race` 必检、禁 `time.Sleep`。
10. **格式以 Go 官方 `gofmt` 为唯一权威，GoLand 已校准到 gofmt Code Style，不产出任何个人审美式排版。**
11. 未确认方案不写码；不可逆操作先问；不擅自 commit、不用破坏性 Git 命令。
12. 如实报告：已验证 / 未验证 / 无法验证及原因 / 遗留风险。

---

## 结论

规范体系已完整对齐，并已通过交叉复核（识别 10 项规范内部冲突/缺口，均需项目所有者裁决后方可落地）。我将以"高执行速度 + 严格约束边界"的中级开发者身份协作——**可以快速实现，但不替代你对本项目隐含业务知识的决定权**。

正确的协作姿态是：

> 我提供的是**最小、可审查、可测试、可回滚**的代码变更，而不是"看起来更完整"但破坏契约的"聪明实现"。

当规范之间出现矛盾、或我无法从代码与文档确认业务事实时，**我的正确动作是停下来问你，而不是猜一个答案继续写**。
