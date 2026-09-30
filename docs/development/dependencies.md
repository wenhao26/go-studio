# 第三方依赖管理与 Stack 规范 (Dependencies Standard)

> **版本**：v1.0.0  
> **适用范围**： Go 依赖包引入准则、核心技术栈白名单、禁用的黑名单包。  
> **关联规范**：[AGENTS.md](../../AGENTS.md), [go.md](go.md)

---

# 1. 依赖引入基本原则 (Core Principles)

1. **标准库优先**：如果 Go 标准库（如 `net/http`, `encoding/json`, `crypto`）能够高效安全地解决问题，**绝对不额外引入第三方包**。
2. **极简原则 (Minimal Dependency)**：不为了少写十行代码而引入含有大量不必要传递依赖（Transitive Dependencies）的巨型 SDK。
3. **安全与维护度评估**：引入新依赖前，必须确认该 Github 项目：
   - Open Issues 得到积极维护。
   - 具有宽松的开源协议（Apache 2.0 / MIT / BSD）。
   - 没有严重的已知 CVE 漏洞。

---

# 2. 项目批准的技术栈白名单 (Approved Stack)

AI Agent 与开发者在编写新模块时，**仅允许使用以下白名单依赖包**：

| 领域分类 | 批准的依赖包 (Approved Package) | 替代与弃用说明 |
|---|---|---|
| **HTTP Web 框架** | `github.com/gin-gonic/gin` 或 `github.com/go-chi/chi` | 禁止引入全家桶框架如 Beego |
| **ORM / 数据库驱动** | `gorm.io/gorm` 或 `github.com/jmoiron/sqlx` | 原生 SQL 操作优先推荐 sqlx |
| **Redis 客户端** | `github.com/redis/go-redis/v9` | 禁用旧版本 `redigo` |
| **配置管理** | `github.com/spf13/viper` | - |
| **日志组件** | `go.uber.org/zap` | 禁用 `log/syslog`, 禁用 `logrus` |
| **并发工具** | `golang.org/x/sync` (`errgroup`) | 推荐官方扩展包 |
| **单元测试与 Mock** | `github.com/stretchr/testify`, `go.uber.org/mock` | - |
| **JSON 序列化** | `encoding/json` 或 `github.com/bytedance/sonic` | 高吞吐场景可使用 sonic |
| **CLI / 终端工具链** | `github.com/mattn/go-runewidth`, `github.com/mattn/go-isatty` | 仅限 `projects/fast-stat`（见 §2.1），且仅用于标准库无法覆盖的终端宽度计算与 TTY 判定；CLI 框架优先使用标准库 `flag` |

---

## 2.1 项目级依赖批准记录 (Per-project Approvals)

`projects/fast-stat` 所需的终端能力**无法用标准库实现**，经需求方裁决（决策 P0-1，路线 R4）批准以下最小组合：

| 依赖 | 用途 | 不可替代性说明 |
|---|---|---|
| `github.com/mattn/go-runewidth` | 终端显示宽度计算（东亚宽字符与 emoji 按 2 列） | Go 标准库不提供任何「终端显示宽度」能力；用字节数或码点数计算会让含中文/emoji 的报告必然错位 |
| `github.com/mattn/go-isatty` | 判断 stdout 是否连接到终端 | 标准库无等价能力；判错会把进度行写进重定向文件，污染可被管道解析的输出 |
| `go.uber.org/zap` | 诊断日志（输出到 stderr） | 已在通用白名单内，遵循 `observability.md` 的强制要求 |

明确**未批准**引入（避免传递依赖膨胀，见 §1 极简原则）：

- `github.com/charmbracelet/bubbletea` / `lipgloss` / `bubbles`（全屏 TUI）
- `github.com/schollz/progressbar`、`cheggaaa/pb`、`vbauerster/mpb`（进度条）
- `github.com/spf13/cobra`、`urfave/cli`（CLI 框架）
- `golang.org/x/term`（终端宽度探测：改用 `COLUMNS` 环境变量 + 默认值回退）

> 变更流程：本节内容如需调整，必须先修改本文件并经 Code Review，再修改代码（见 `docs/README.md` §3）。

---

# 3. 禁用的包与黑名单 (Blacklisted Packages)

以下依赖存在严重性能隐患、内存泄露风险或架构不合规，**严禁引入**：

1. **`github.com/pkg/errors`**：项目已停止维护，请统一使用 Go 1.13+ 标准库 `fmt.Errorf("%w", err)` 与 `errors.Is/As`。
2. **基于反射的隐式注入框架**：如 `uber-go/dig`，推荐使用静态编译期生成代码的 `google/wire`。
3. **全局 Common/Util 万能包**：如 `github.com/duke-git/lancet` 等大包，必须按需提取或使用细分小包。

---

# 4. 依赖变更操作流程 (Dependency Change Workflow)

1. 执行 `go get <package>@version` 时，必须锁定具体 Tag 或 Commit SHA，禁止直接依赖 `master`。
2. 执行完依赖变更后，必须强制运行：
   ```bash
   go mod tidy
   go test ./...
   ```
3. 检查 `git diff go.mod go.sum`，确保没有发生无关的级联升级。
