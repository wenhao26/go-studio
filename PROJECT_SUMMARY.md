# 项目汇总（go-studio）

> 本文件汇总仓库内各子项目的现状、功能、关键路径、构建产物与验证要点，便于快速了解每个项目定位。后续新增项目时需同步更新本文件。

---

## 1. 项目概览

| 项目 | 类型 | 状态 | 主要用途 | 关键目录/文件 |
|---|---|---|---|---|
| [file-pilot](projects/file-pilot) | Wails 桌面应用（Go + Vue3 + Vite） | 已交付（v1.0 基础版，当前 `main` 已合并 `feat/v1.1` 的日志修复+落库优化） | 本地文件扫描、统计、去重预处理、整理归类前的清点工具。对大目录进行批量文件梳理、生成扫描结果、支持按规则分类等。 | `projects/file-pilot/app/`、`projects/file-pilot/frontend/`、`projects/file-pilot/internal/`、`projects/file-pilot/build/bin/file-pilot.exe` |
| [file-stats](projects/file-stats) | Go 命令行工具 | 已交付（功能可用） | 快速统计指定目录的文件数量、总大小、扩展名分布、最大文件等摘要信息。常用于盘点、评估清理范围。 | `projects/file-stats/cmd/file-stats/`、`projects/file-stats/internal/`、`projects/file-stats/README.md` |
| [fast-stat](projects/fast-stat) | Go 命令行工具 | 已交付（功能可用） | 基于 `filepath.WalkDir` 的高速目录统计工具，支持并发、过滤、排序、格式化输出（表格/JSON）。 | `projects/fast-stat/cmd/fast-stat/`、`projects/fast-stat/internal/`、`projects/fast-stat/README.md` |
| 根工程（规范基座） | 规范文档库 | 规范就绪，尚未落地 Go 工程骨架 | `AGENTS.md`、`docs/` 为仓库全局开发规范与 AI 协作准则的唯一权威。当前未初始化 `go.mod`、`cmd/`、`internal/` 等工程骨架。 | `AGENTS.md`、`docs/development/*.md`、`PROJECT_ALIGNMENT_REPORT.md` |

> 提示：`projects/` 下的 3 个项目是独立的 Go 模块（各自拥有 `go.mod`）。根目录不是单一聚合模块，而是**规范基座+多个独立子项目**共存的仓库结构。

---

## 2. file-pilot（桌面应用）

### 2.1 功能定位
本地文件盘点与预处理工具。核心能力：
- **目录扫描**：多 worker 并发遍历目录树，支持隐藏/系统文件开关、跳过大文件等过滤策略。
- **实时日志与进度**：前端日志面板**钉顶**显示新日志，滚动时仅在真实用户输入（向下滚轮、触摸拖动、按住指针拖动条）下暂停跟随；非用户位移会自动校正回最底（钉顶），已暂停时提供「已暂停跟随 · 回到最新」按钮恢复。
- **分类与整理预览**：基于内置规则对文件进行初步分类（按扩展名等），展示分类统计、可跳转结果页。
- **本地存储**：扫描结果写入本地 SQLite（`file-pilot.db`，WAL 模式）。采用**批量多行 VALUES** 落库（每条 SQL 插入 N 行）以提升落库吞吐，减少事务次数。
- **任务驱动 UI**：前后端通过 Wails 事件（`task:progress`、`task:log`、`task:status` 等）同步任务状态。

### 2.2 关键文件
| 路径 | 说明 |
|---|---|
| `projects/file-pilot/app/app.go` | Wails App 绑定、生命周期、任务协调入口 |
| `projects/file-pilot/internal/scanner/scanner.go` | 目录遍历、worker 池、跳过策略、文件计数/大小累计 |
| `projects/file-pilot/internal/task/throttle.go` | 200ms 节流：进度+日志批量下发，限制每窗口最大明细行数并在溢出时输出「已丢弃 N 条明细」 |
| `projects/file-pilot/internal/storage/files.go` | 文件记录落库：**每语句插入 10 行多行 VALUES**、**每批 500 行提交事务**（当前实现） |
| `projects/file-pilot/internal/storage/scans.go` | 扫描元数据（任务/扫描记录）创建、完成状态更新 |
| `projects/file-pilot/frontend/src/views/ScanView.vue` | 扫描中视图：日志面板滚动钉顶逻辑、暂停触发判定、自动校正、恢复跟随 |
| `projects/file-pilot/frontend/src/views/OrganizeView.vue` | 整理预览页日志面板同款逻辑（与 ScanView 一致） |
| `projects/file-pilot/frontend/src/lib/task.ts` | 前端任务事件订阅、日志缓冲（原地 push + splice，`LOG_LIMIT=500`）、进度状态同步 |

### 2.3 构建产物
- `projects/file-pilot/build/bin/file-pilot.exe`（Windows x64，Wails 打包）
- 最新构建（`main` 合并 `feat/v1.1` 后重新打包）：大小 16,193,536 B，修改时间 2026-10-19 09:43:26，SHA-256 `2817C67D6FEB589394D54A5958F9627EB5DF7921C14F53836161A1A1714FC574`

### 2.4 分支与合并说明
- `feat/v1.1`：日志视图修复（`6370229`）+ 落库批量写入优化（`2885fd8`）
- `main`：已于 2026-10-19 合并 `feat/v1.1`（`8291f1e merge: feat/v1.1 (...)`）并推送至 `origin/main`

---

## 3. file-stats（CLI 工具）

### 3.1 功能
- 递归统计目录：文件总数、总大小（字节）、按扩展名聚合计数/大小、最大文件等摘要信息。
- 轻量、直接，适合快速盘点与评估清理范围。

### 3.2 关键路径
- `projects/file-stats/cmd/file-stats/main.go`
- `projects/file-stats/internal/stats/`
- `projects/file-stats/go.mod`

### 3.3 用法（简要）
```bash
cd projects/file-stats
go build ./cmd/file-stats
# Windows
.\file-stats.exe <目录路径>
# Linux/macOS
./file-stats <目录路径>
```

---

## 4. fast-stat（CLI 工具）

### 4.1 功能
- 基于 `filepath.WalkDir` 的高速目录统计，支持并发遍历。
- 支持过滤（扩展名、最小/最大大小、隐藏文件等）、排序（按大小/数量等）、表格或 JSON 输出。
- 模块化结构：`walker`、`collector`、`formatter` 等。

### 4.2 关键路径
- `projects/fast-stat/cmd/fast-stat/main.go`
- `projects/fast-stat/internal/walker/`、`internal/collector/`、`internal/formatter/`
- `projects/fast-stat/go.mod`

### 4.3 用法（简要）
```bash
cd projects/fast-stat
go build ./cmd/fast-stat
./fast-stat --help  # 查看可用参数
```

---

## 5. 根目录规范基座

根目录聚焦**工程规范与 AI 协作**，当前未承载业务代码（暂无 `go.mod`）。

### 5.1 核心文档
| 文档 | 说明 |
|---|---|
| [`AGENTS.md`](AGENTS.md) | 项目级 AI Coding 开发规范（强制规则为主，明确标注"建议"）。涵盖：第一原则（正确性优先）、最小修改、先搜索再创建、错误处理、并发安全、测试要求、Git 禁止破坏性操作等。 |
| [`docs/development/*.md`](docs/development/) | 专项规范：`go.md`、`architecture.md`、`domain.md`、`api.md`、`database.md`、`config.md`、`observability.md`、`security.md`、`testing.md`、`dependencies.md`、`commenting.md`、`environment.md` |
| [`docs/README.md`](docs/README.md) | 规范导航地图（索引与使用方式） |
| [`PROJECT_ALIGNMENT_REPORT.md`](PROJECT_ALIGNMENT_REPORT.md) | 规范对齐与 Vibe Coding 前置认知报告（含待裁决项 C1–C10） |

### 5.2 设计原则（节选自 `AGENTS.md`）
- **正确性优先**：不得牺牲正确性、安全性、可维护性、可测试性、可观测性等前六项。
- **先理解上下文，再改代码**：读结构、`go.mod`、调用链、相关测试再动手。
- **最小修改**：只改完成任务必需的内容；禁止顺手格式化全项目、无关重构。
- **不臆测**：不凭常识假设表结构/API/配置，必要时先搜索再向用户询问。
- **禁止伪实现**：不写 `return nil,nil`、`panic("not implemented")`、假数据、假测试结果。
- **可验证闭环**：改动后按影响范围执行 `gofmt`、`go vet`、相关测试；完成前检查 `git diff` 并如实说明验证范围与风险。

---

## 6. 维护说明

- **新增项目**：在 `projects/<name>/` 下创建独立 Go 模块（自带 `go.mod`），并在本文件 `项目概览`、对应章节补充：功能定位、关键路径、构建方式、用法要点。
- **文档优先**：根目录规范基座的文档是全仓库共用准则。修改规范需同步考虑对 `projects/*` 的影响，且不得与现有强制规则相悖。
- **file-pilot 发布物**：如需替换 Release v1.0 的 `file-pilot.exe`，应基于 `main` 合并后的最新构建（当前 SHA-256 如上）并在发布前校验哈希。
- **临时文件**：开发时产生的临时脚本、探针、bench 副本等**不得**提交到仓库。
- **分支策略**：功能性较大的试验性改动优先用 `feat/*` 分支；小而确定的改动也可单独摘出合并。合并时优先用最小 diff。
- **本文件职责**：`README.md` 偏向「仓库整体规范与状态」，`PROJECT_SUMMARY.md` 偏向「各子项目清单与速览」。

---

## 7. 快速索引

| 目标 | 路径 |
|---|---|
| 根规范总览 | [`README.md`](README.md) |
| AI 开发规范（最高准则） | [`AGENTS.md`](AGENTS.md) |
| 规范导航地图 | [`docs/README.md`](docs/README.md) |
| 项目汇总（本文件） | [`PROJECT_SUMMARY.md`](PROJECT_SUMMARY.md) |
| file-pilot 源码 | [`projects/file-pilot/`](projects/file-pilot) |
| file-pilot 最新 exe | [`projects/file-pilot/build/bin/file-pilot.exe`](projects/file-pilot/build/bin/file-pilot.exe) |
| file-stats 源码 | [`projects/file-stats/`](projects/file-stats) |
| fast-stat 源码 | [`projects/fast-stat/`](projects/fast-stat) |
