# 项目开发规范体系 (Project Development Standards)

> 本目录包含了项目在 AI Coding / Vibe Coding 以及人工协作开发下的全套生产级约束规范。  
> **核心原则**：所有规范默认均为**强制执行规则**，优先保证代码的正确性、安全性、可维护性与可测试性。

---

# 1. 规范文档目录地图 (Documentation Index)

下表列出了各专项规范的适用场景与核心职责。开发或让 AI Agent 修改代码时，请严格对齐对应的规范。

| 规范文档 | 相对路径 | 职责与约束范围 | 核心关注点 / 关键章节 |
|---|---|---|---|
| **AGENTS.md** | `../../AGENTS.md` | **全局 AI Agent 行为准则（根目录）** | 第一原则、最小修改原则、禁止伪实现与假测试、修改工作流 |
| **go.md** | `development/go.md` | **Go 语言与编码规范** | 包命名、Guard Clause、Error 包装 (`%w`)、Goroutine 安全 |
| **architecture.md** | `development/architecture.md` | **系统架构与分层规范** | 整洁架构分层边界 (Handler/Service/Repo)、目录结构、依赖注入 |
| **domain.md** | `development/domain.md` | **领域模型与状态机规范** | 通用词汇表 (Glossary)、订单/业务状态机矩阵、分布式唯一 ID |
| **api.md** | `development/api.md` | **API 接口与 RESTful 规范** | URL 路由格式、统一 JSON 响应结构、错误码、幂等性格式 |
| **database.md** | `development/database.md` | **数据库与存储设计规范** | MySQL 表设计模板、索引规范、防 GORM N+1 查询、Redis 规范 |
| **config.md** | `development/config.md` | **配置与环境变量规范** | 强类型 Config Struct 启动校验、`.env.example` 模板、多环境矩阵 |
| **observability.md** | `development/observability.md` | **可观测性与日志/Tracing** | Zap 结构化日志字段、OpenTelemetry TraceID 传递、日志脱敏 |
| **security.md** | `development/security.md` | **信息安全与防御性编码** | JWT 签名策略、防越权 (BOLA)、SQL/命令/SSRF 注入防御 |
| **testing.md** | `development/testing.md` | **自动化测试规范** | 单元测试覆盖率 (70%+)、表格驱动测试模板、Mock 策略 |
| **dependencies.md** | `development/dependencies.md` | **依赖管理与 Stack 白名单** | 批准的技术栈白名单、禁用的黑名单依赖、引入评估流程 |
| **commenting.md** | `development/commenting.md` | **代码注释与 Godoc 规范** | 包注释、导出标识符注释、TODO/FIXME 标记、Swag 注释 |
| **git.md** | `development/git.md` | **Git 提交与 PR 协作流** | Conventional Commits 格式、PR 提交自检清单、禁止破坏性指令 |
| **environment.md** | `development/environment.md` | **本地 Dev 环境与 Task 规范** | Docker Compose 一键启动、Makefile 标准指令集、Seed 测试数据 |

---

# 2. 如何在 AI Coding / OpenCode 中使用本规范

1. **上下文加载与定位**：`AGENTS.md` 位于项目**根目录**，在进行任何 AI 编码前作为全局指令读取。在发起具体模块功能开发时（如“实现用户订单退款功能”），提示 Agent：
   > “请严格遵循根目录下的 `AGENTS.md`，并参考 `docs/development/domain.md` 中的状态机和 `docs/development/api.md` 中的接口规范实现该功能。”
2. **提交前自检**：在宣称任务完成或提交 PR 之前，Agent 必须对照 `git.md` 中的自检清单（Checklist）逐项验证。

---

# 3. 规范维护与更新

- 本规范体系由技术团队统一维护。
- 若需修改架构设计或引入新依赖包，必须先更新 `architecture.md` 或 `dependencies.md`，经团队 Code Review 通过后再执行代码变更。
