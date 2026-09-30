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
