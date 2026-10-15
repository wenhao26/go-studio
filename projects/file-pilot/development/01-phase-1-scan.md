# Phase 1 · 扫描（File Scan）

> 目标：打通「选目录 → 配参数 → 并发扫描 → 实时进度/日志 → 统计 → 落库 → 任务状态机」全链路。
> 前置：[`00-phase-0-prerequisites.md`](00-phase-0-prerequisites.md) 全部 DoD 达成。
> 依赖文档：[`contracts.md`](contracts.md)、[`test-and-acceptance.md`](test-and-acceptance.md)。
> **不含实现代码**。

---

## 1. 范围

| 包含 | 不包含 |
|---|---|
| FR-1 目录选择、FR-2 扫描配置 | 分类规则引擎（Phase 2） |
| FR-3 并发递归扫描（跳过链接/黑名单/大文件） | 分类汇总/详情/调整（Phase 2） |
| FR-4 扫描进度（不确定型）+ 三类日志 | 整理与撤销（Phase 3） |
| FR-5 扫描统计 | 历史报告页（Phase 4 或不实现） |
| FR-12 任务状态机 + 单活动约束 + 异常恢复 | 规则 UI 编辑（不做） |
| FR-13 `scans`/`files`/`tasks`/`settings` + Migration | |
| 扫描流水线中**调用**分类（分类实现由 Phase 2 补齐；本阶段可先用「扩展名占位规则」跑通数据流）⚠️ | |

> ⚠️ 关于「分类占位」：为避免 Phase 1 阻塞在规则细节，本阶段可先落一个**最小扩展名→类别**占位实现以打通 `files.category` 数据流；**完整规则引擎在 Phase 2 完成**。占位实现不得伪装为最终实现，须在文档中标注（`AGENTS.md` §79）。

---

## 2. 前置条件

- Phase 0 完成；依赖白名单已含 wails/sqlite（必要时含平台 API 依赖）。
- Wails 骨架可 `dev` 运行；前端可调用至少一个绑定方法并收到事件。

---

## 3. 任务分解

### T1.1 任务管理器与状态机（`internal/task`）

- 状态集合 5 态 + 预留 `paused`；**合法性表**：`pending→running`、`running→completed|failed|canceled`、`pending→canceled`；其余拒绝并返回错误。
- 单活动约束：`mu` + `running` 句柄；`StartScan` 冲突返回 `ErrTaskRunning`。
- 取消：`CancelTask(taskID)` → `ctx` 取消；worker 协作退出。
- 异常恢复：启动扫描/初始化时把遗留 `running` 置 `failed(应用中断)`。
- 状态转换与 DB 落库集中一处（避免状态分裂）。
- **测试**：状态机全量合法/非法转换（表格驱动）。

### T1.2 扫描配置（FR-2）

- `ScanConfig` 强类型 + 边界校验（`workers∈[1,64]`，非法报错不钳制）。
- 持久化/回填：`settings.scan_config`。
- 最近目录：`recent_dirs`（≤5、去重、最近在前）。

### T1.3 目录选择（FR-1）

- `ChooseDirectory` 绑定；取消 → `ErrDirectoryPickCanceled`。
- 前端未选目录拦截（不调后端）。
- 「预计文件」恒为「未知」。

### T1.4 递归扫描核心（`internal/scanner`）

- 复用 fast-stat 的**固定 worker 池 + Cond 队列**设计（不用缓冲 channel；`push` 后 `done`）。
- 计数用 `atomic.Int64`；错误明细 `mutex` 最多 20 条。
- 跳过策略：符号链接/junction（不下潜、不计主计数、计入跳过）、黑名单目录（`$Recycle.Bin`、`System Volume Information`）、`<根>/FilePilot整理`、按开关跳过隐藏/系统/大文件。
- 采集字段：path / name / ext（小写无点）/ size（`int64`）/ mtime / ctime（best-effort）/ 隐藏 / 系统。
- 文件发现后**立即调用分类**（Phase 1 占位），写入内存缓冲。
- **落库**：`files` 每 500 条一个事务；扫描行状态与统计在结束时更新。
- **取消**：停止派发、worker 退出、保留已扫描部分。
- **文件系统抽象** `fsx`：便于注入权限拒绝/占用/链接等故障（对齐 fast-stat `fsx` 思路）。
- **测试**：`t.TempDir` 真实目录树集成测试 + 故障注入；`-race`。

### T1.5 进度与节流（FR-4，§2.4）

- 扫描进度**不确定型**：scanned / rate / current_dir / elapsed / skipped；**无总数/百分比/ETA**。
- 节流器：`task:progress` ≥200ms；`task:log` 200ms 窗口批量、≤20 条、超采样丢弃 + `dropped` 披露。
- 三类日志：scan / classify / system；`system` 不采样。
- 节流 goroutine 生命周期：随任务创建，结束 `Stop` ticker + `Wait`（防泄漏）。

### T1.6 扫描统计（FR-5）

- 文件数/目录数/总大小/耗时/平均速度/最大并发/跳过错误数。
- 单位与耗时格式化（§2.5）；跳过明细 ≤20 条入日志。
- 取消 → `canceled` + 「已取消 · 部分结果」；部分结果**不进入预览/整理**（Phase 2/3 入口校验）。

### T1.7 持久化与 Migration（FR-13）

- 建表：`scans`/`files`/`tasks`/`operations`/`settings`/`schema_version`（列级见 analysis §3.1）。
- 索引：`(scan_id, category, id)` 等。
- Migration：`schema_version` + 嵌入式步进（不修改已应用版本）。
- 保留：启动清理 >20 次扫描级联数据（显式删除）。
- SQLite 运行参数（WAL/busy_timeout/foreign_keys）按裁决。
- **测试**：临时 DB 批量写、分页查询、Migration 幂等、保留清理。

### T1.8 Wails 绑定与事件桥接（Phase 1 子集）

- 实现：`ChooseDirectory`、`GetSettings`、`SaveSettings`、`StartScan`、`CancelTask`、`GetActiveTask`、`GetScanSummary`、`ListFiles`（Phase 1 可先支持按扫描读列表）。
- 事件：`task:status`、`task:progress`、`task:log`。
- 错误 → 前端文案映射（contracts §4）。
- 未映射错误与关键失败路径写入 **zap 诊断日志文件**（NFR-3，裁决 D-A8；`internal/diag`）。

### T1.9 前端页面 P1/P2

- P1 首页：目录选择、扫描配置、最近目录、开始扫描。
- P2 扫描中：不确定型进度、当前目录/速率/耗时/跳过、三类日志流、取消按钮。
- 空态/失败态文案；任务进行中离开页面策略。
- 前端无自动化测试（R-8），按 [`test-and-acceptance.md`](test-and-acceptance.md) 手动清单验收。

---

## 4. 交付物

`internal/task/*`、`internal/scanner/*`、`internal/fsx/*`、`internal/storage/*`（scans/files/tasks/settings/migrate）、`internal/app/*`（Phase 1 绑定子集）、`internal/diag/*`（NFR-3 诊断日志，裁决 D-A8）、`frontend`（P1/P2）；对应 `*_test.go`。

---

## 5. Definition of Done

```text
✓ gofmt 仅改动文件；go vet ./... 通过
✓ go test ./... 通过；go test -race ./...（scanner/task 强制）
✓ 核心模块覆盖率 ≥70%
✓ 10 万文件扫描 ≤60s（记录环境与数据集）
✓ 取消 / 权限拒绝 / 异常恢复 / 单活动冲突用例通过
✓ 进度契约：无总数/百分比/ETA；事件率受 200ms 节流约束
✓ 无 panic / 无吞错 / 无无责任 goroutine / 无 time.Sleep
✓ git diff 自检干净；如实报告验证范围与风险
```

---

## 6. 风险与待裁决

- 平台属性（隐藏/系统/ctime）采集路径取决于 Phase 0 V2/V3 结论。
- 长路径与 junction 行为需在真实 Windows 环境验证（Phase 0 V4/V5）。
- 「分类占位实现」必须在 Phase 2 被完整替换，禁止遗留伪实现。

### 需求方裁决记录（2026-10-15，Phase 1 编码前）

| # | 问题 | 裁决 |
|---|---|---|
| D-A1 | `StartScan` 缺根目录参数（契约缺口） | **`StartScan(rootPath, ScanConfig)`**，已同步 `requirements.md` §8.1（R4）与 `contracts.md` §5 |
| D-A2 | SQLite 运行参数 | **全套**：WAL + `busy_timeout=5000` + `foreign_keys=ON` + `MaxOpenConns=1` |
| D-A3 | Phase 1 交付切分 | **分两次**：检查点 A = 后端 T1.1–T1.8（代码审查）；检查点 B = 前端 T1.9（体验验证） |
| D-A4 | 进度 `rate` 计算口径 | **全程平均**：`rate = 已扫描文件数 / 已耗时（秒）`，取整 `files/s`（与 `avg_speed` 同源） |
| D-A5 | 前端导航 | **引入 `vue-router`**（requirements §九 的 4 条路由要求），记入项目 README |
| D-A6 | 隐藏/系统**目录**是否按开关跳过 | **与文件同开关**：false 时不下潜、计入跳过数与日志 |
| D-A7 | FR-3「内存全量保留供预览」与内存 NFR 冲突 | **仅保留 500 条落库批量窗口**，预览/统计读 SQLite（contracts §7） |
| D-A8 | NFR-3 的 zap 诊断日志文件未被任何阶段文档分配 | **Phase 1 检查点 A 一并实现**：`internal/diag`（zap + 自实现 10MB 滚动保留 1 个 `.old`，禁 lumberjack），错误路径同步接入 |

### 检查点 B 设计裁决（2026-10-15，Phase 1 检查点 B）

Phase 1 检查点 B（前端 T1.9，P1/P2）实现中确定并采纳以下 5 项设计裁决：

| # | 事项 | 裁决 | 依据 / 影响 |
|---|---|---|---|
| D-B1 | 任务进行中「离开页面」策略 | **任务后台继续 + 「当前任务」指示器返回**，不做导航拦截 | requirements §九 · 2 给出两选一，取无拦截方案（应用外壳常驻「当前任务」指示器） |
| D-B2 | P2（扫描中）是否展示 FR-5 扫描统计 | **本阶段不展示**，`GetScanSummary` / `ListFiles` 暂不消费；统计展示归入 P3 / Phase 2 | FR-5 统计展示属结果页（P3）职责，P2 聚焦进度与日志 |
| D-B3 | 前端全局状态方案 | **模块级 `reactive` 单例，不引入 Pinia** | `AGENTS.md` §31 默认不新增依赖；单活动任务场景下自证归属的事件载荷足够 |
| D-B4 | 前端路由模式 | **hash 模式** | 内嵌资源无服务端重写，history 模式刷新会 404 |
| D-B5 | 扫描配置持久化时机 | **并发数输入即校验（非法立即提示、不钳制）；有效值在 `@change` 时 `SaveSettings`** | FR-2「立即报错、不静默钳制」；避免每次按键写库 |

> 附（非新增裁决，来源既有规则/裁决）：终态 `task:status.error` 不直接向用户展示、只显示通用文案 + 日志区保留原因（`requirements.md` §8.4、`AGENTS.md` §65，见 D-B3 之外的 §8.4 约束）；导航高亮按 `route.name` 绑定类以规避 Tailwind 类序不确定；`vue-router`（D-A5）已安装并登记于项目 `README.md`。

**补充裁决（FR-12 落地缺口，2026-10-15）**：FR-12 要求「异常恢复后提示用户」，但既有契约仅有 `GetActiveTask`（恢复完成后无活动任务即返回 null），前端无信号可感知「本次启动刚恢复了中断任务」。裁决：**新增只读绑定 `GetRecoveryNotice`**，返回 `{recovered_tasks, recovered_scans}`，无恢复时为 `null`；已同步 `requirements.md` §8.1（R5）与 `contracts.md` §3.10 / §5。前端在应用外壳（`App.vue`）以可关闭横幅提示，原因文案「应用中断」。

### Phase 1 验收记录（2026-10-15）

| 项 | 结论 |
|---|---|
| Phase 1 验收 | **需求方验收通过**（检查点 A 后端 + 检查点 B 前端 P1/P2） |
| FR-12 恢复横幅 | **按现状接受**：横幅显示「恢复了 N 个任务 · M 个扫描」两个计数；「关闭」仅本会话生效（不写库；下次启动仅在再次发生恢复时才提示） |
