# file-pilot 开发阶段文档（Development Docs）

> **目录性质**：`file-pilot` 项目**分阶段开发**的实施文档集合，统一定位于 `projects/file-pilot/development/`。
> **上游文档**：[`../requirements.md`](../requirements.md)（需求权威）、[`../requirements-analysis.md`](../requirements-analysis.md)（需求分析 / 待裁决项）、[`../../AGENTS.md`](../../AGENTS.md)、[`../../docs/`](../../docs/)。
> **不含代码**：本目录全部为「分析 + 计划 + 验收」文档；实现代码将在需求方确认后、按阶段文档逐阶段产出。

---

## 1. 阶段路线图

```text
Phase 0  前置准备（🔴 阻塞）
   └─ 依赖白名单裁决 / 独立 go.mod / Wails 骨架 / 关键技术验证
        │
        ▼
Phase 1  扫描（文件发现 + 进度 + 统计 + 持久化 + 任务状态机）
        │
        ▼
Phase 2  智能分类（规则引擎 + 预览 + 逐文件调整）
        │
        ▼
Phase 3  整理与撤销（安全移动闭环 + 报告 + 撤销）
        │
        ▼
Phase 4  增强（非 v1：自动整理 / 历史 / 学习 / 重复检测）
```

阶段依赖：Phase 1 → 2 → 3 顺序不可跳；Phase 0 是全部前置。Phase 4 与前三者无强依赖（均不在 v1）。

---

## 2. 文档索引

| 文档 | 内容 | 主要读者 |
|---|---|---|
| [`00-phase-0-prerequisites.md`](00-phase-0-prerequisites.md) | 依赖白名单、模块初始化、技术验证清单、阻塞项裁决 | 所有开发者 / Code Review |
| [`01-phase-1-scan.md`](01-phase-1-scan.md) | 目录选择、扫描配置、并发扫描、进度、统计、SQLite、任务状态机 | 后端 + 前端 |
| [`02-phase-2-classification.md`](02-phase-2-classification.md) | 分类规则引擎、分类日志、预览汇总/详情、逐文件调整 | 后端 + 前端 |
| [`03-phase-3-organize-undo.md`](03-phase-3-organize-undo.md) | 二次确认、同卷移动、冲突改名、操作记录、报告、撤销 | 后端 + 前端 |
| [`04-phase-4-enhancement-backlog.md`](04-phase-4-enhancement-backlog.md) | 非 v1 增强方向（仅登记，不实现） | 规划 |
| [`contracts.md`](contracts.md) | 跨阶段契约（类型 / Wails / 事件 / 错误 / 存储接口） | 所有开发者 |
| [`test-and-acceptance.md`](test-and-acceptance.md) | 测试矩阵、覆盖率门禁、性能基准、手动 UI 验收清单 | 所有开发者 |

---

## 3. 使用方式

1. **进入某阶段前**：先读该阶段文档的「前置条件」，确认依赖已完成。
2. **实现时**：严格按「任务分解」逐项推进；任何跨模块字段/方法以 [`contracts.md`](contracts.md) 为准。
3. **完成时**：对照该阶段「Definition of Done」+ [`test-and-acceptance.md`](test-and-acceptance.md) 逐项验证，并**如实报告**（已执行 / 未执行 / 风险）。
4. **需求有疑问时**：回到 [`../requirements-analysis.md`](../requirements-analysis.md) §7 待裁决清单；不臆测、不扩权。

---

## 4. 当前状态与阻塞

> **交付状态（2026-10-15）**：**v1（Phase 1–3）已交付完成并通过需求方验收**；Phase 4 为 v1 之外的增强 backlog，**不实现**。

| 阶段 | 状态 | 说明 |
|---|---|---|
| Phase 0 | ✅ **已完成，待需求方审核** | P0-1 ~ P0-4、R-1 ~ R-9 全部裁决；工程骨架 + V1 ~ V8 技术验证完成；待确认项 C-2 / C-3 已关闭，剩 C-1、C-4，详见 [`00-phase-0-prerequisites.md`](00-phase-0-prerequisites.md) §3–§7 |
| Phase 1 | ✅ **已完成（需求方验收通过）** | 检查点 A（后端 T1.1–T1.8）+ 检查点 B（前端 P1/P2）已实现并通过自动化验证；设计裁决见 [`01-phase-1-scan.md`](01-phase-1-scan.md) §6（D-A1 ~ D-A8、D-B1 ~ D-B5）；2026-10-15 需求方验收通过（FR-12 恢复横幅按现状接受） |
| Phase 2 | ✅ **已完成（需求方验收通过）** | 分类规则引擎（`internal/classifier`）+ 分类汇总/详情/逐文件调整（FR-7/FR-8）+ 前端 P3 结果页；编码裁决见 [`02-phase-2-classification.md`](02-phase-2-classification.md) §6（D-C1 ~ D-C4）；2026-10-15 需求方验收通过并授权进入 Phase 3 |
| Phase 3 | ✅ **已完成（需求方验收通过）** | 安全移动闭环（`internal/organizer`）+ 报告（FR-10）+ 撤销（FR-11）+ 前端 P3 开始整理 / P4 进度·报告·撤销；编码裁决见 [`03-phase-3-organize-undo.md`](03-phase-3-organize-undo.md) §6（D-D1 ~ D-D9）；2026-10-15 需求方验收通过，**v1（Phase 1–3）交付完成** |
| Phase 4 | ⏸ 未开始（**非 v1**） | 见 [`04-phase-4-enhancement-backlog.md`](04-phase-4-enhancement-backlog.md)：仅登记（B-1 ~ B-9），**不实现**；开工需明确的新需求文档（该文档 §2） |

**P0 阻塞项关闭情况**：

| # | 阻塞项 | 裁决结果 |
|---|---|---|
| P0-1 | 依赖白名单更新 | ✅ 批准 `wailsapp/wails/v2@v2.16.0` + `modernc.org/sqlite@v1.59.0`，已写入 `docs/development/dependencies.md` §2.2（待 Code Review） |
| P0-2 | 「绝不覆盖」保障方式 | ✅ 方案①：存在性预检（Lstat 找唯一名 + 紧邻改名前复检）+ `os.Rename` |
| P0-3 | 属性与创建时间采集路径 | ✅ 纯标准库 `fi.Sys().(*syscall.Win32FileAttributeData)`，**不引入 `x/sys/windows`** |
| P0-4 | 分类算法口径 | ✅ 口径 B（语义优先选类 + 跨类别总分封顶 100）；`requirements.md` §5.2 / §5.6 已按授权修订（修订号 R3，C-2 关闭） |
| R-1 ~ R-9 | `requirements.md` §十五 复核 | ✅ 全部采纳，§十五 已标记为「需求方已复核」 |

**进入 Phase 1 前待需求方确认的 2 项**：见 [`00-phase-0-prerequisites.md`](00-phase-0-prerequisites.md) §7 —— **C-1** 规范 Code Review（`dependencies.md` §2.2）、**C-4** 体验验证 `build/bin/file-pilot.exe`。（C-2、C-3 已于 2026-10-15 裁决关闭。）

---

## 5. 变更记录

| 日期 | 变更 | 说明 |
|---|---|---|
| 2026-10-14 | 初版 | 建立 Phase 0–4 文档、契约、测试与验收文档 |
| 2026-10-15 | Phase 0 实施 | 关闭 P0-1 ~ P0-4 与 R-1 ~ R-9；创建工程骨架（独立 `go.mod`、Wails v2.16.0、Vue3/TS/Vite/Tailwind、Makefile、README）；回填 V1 ~ V8 验证结论（新增 V8：本机 `-race` 需 `-linkmode=external`） |
| 2026-10-15 | Phase 1 实施 | 检查点 A/B 编码与自动化验证；FR-12 恢复提示（`GetRecoveryNotice`）；检查点 B 设计裁决 D-B1 ~ D-B5 归档 |
| 2026-10-15 | Phase 2 实施 | 分类规则引擎（口径 B）+ FR-7 汇总/详情 + FR-8 逐文件调整 + 前端 P3；`task:status` 新增 `scan_id`（D-C1）；编码裁决 D-C1 ~ D-C4 归档 |
| 2026-10-15 | Phase 2 验收 | 需求方验收通过，授权进入 Phase 3 |
| 2026-10-15 | Phase 3 实施 | 安全移动（冲突绝不覆盖）+ 操作记录（`operations`）+ 整理报告（FR-10）+ 撤销（FR-11）+ 前端 P3 开始整理 / P4 进度·报告·撤销；`tasks.parent_task_id`/`undone`、`StartOrganize`/`GetOrganizeReport`/`UndoLastOrganize` 三个绑定；编码裁决 D-D1 ~ D-D8 归档 |
| 2026-10-15 | Phase 3 验收 / **v1 交付** | 需求方验收通过 Phase 1、Phase 3；批准 D-D9（`GetScanSummary` 追加 `organizable_count`）；FR-12 恢复横幅按现状接受。**v1（Phase 1–3）交付完成**，Phase 4 为 v1 之外 backlog（不实现） |
