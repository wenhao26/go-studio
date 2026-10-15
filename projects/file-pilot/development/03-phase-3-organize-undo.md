# Phase 3 · 整理与撤销（Organize & Undo）

> 目标：交付**安全**的整理闭环 —— 二次确认、同卷移动、冲突改名不覆盖、逐条操作记录、整理报告、撤销上一次整理。
> 前置：[`02-phase-2-classification.md`](02-phase-2-classification.md) DoD 达成；P0-2（覆盖防护）已裁决并验证。
> 依赖文档：[`contracts.md`](contracts.md)、[`test-and-acceptance.md`](test-and-acceptance.md)、[`../requirements-analysis.md`](../requirements-analysis.md) §2.2/§2.3。
> **不含实现代码**。

---

## 1. 范围

| 包含 | 不包含 |
|---|---|
| FR-9 二次确认 + 目标目录预检 + 同卷移动 | 复制式移动（不做，改名失败即失败） |
| 同名冲突改名（**绝不覆盖**） | 批量撤销 / 任意历史撤销（不做，R-9） |
| 逐条操作记录（成功/失败/原因） | 删除 / 清理空目录（不做） |
| FR-10 整理报告（含已移动数据量、失败明细） | 自动整理（Phase 4） |
| FR-11 撤销上一次整理（含部分完成） | |

---

## 2. 前置条件

- P0-2 已裁决：**不依赖 `os.Rename` 失败防覆盖**（Windows 会覆盖，已核实）。
- 目标目录名常量已定（P1-3）。

---

## 3. 任务分解

### T3.1 整理计划构建

- 读取该 `scan_id` 下 `excluded=0` 的文件，按 `category` 分组。
- 目标：`<扫描根>/FilePilot整理/<类别>/<文件名>`。
- **路径安全**：`filepath.Clean` 后校验 `dst` 前缀 = 目标根（SEC-3），否则拒绝该条。
- 目标目录 `MkdirAll` 预检；失败 → 任务整体失败、**零移动**（`ErrTargetDirCreate`）。

### T3.2 同卷移动 + 冲突改名（安全核心）

- 冲突算法（analysis §2.2）：
  1. 候选名 `原名`；`Lstat` 存在则 `base + " (n)" + ext`，取第一个不存在者（n 上限 10000，超出该条 `failed`）。
  2. **紧邻改名前再校验一次**目标不存在（缩小 TOCTOU）。
  3. 执行同卷改名（方案见 P0-2：`os.Rename` + 前置检查，或 `syscall.MoveFile`）。
- **绝不覆盖**：由存在性检查 + 紧邻复检保证；不使用「rename 会失败」的假设。
- 单文件失败（占用/权限/跨卷意外/超长）→ 记录 `failed`，**继续**下一条。
- **顺序执行**（无额外并发）。
- 每条立即写 `operations`（含 `size_bytes`，供「已移动数据量」聚合）。

### T3.3 整理任务与进度

- 任务类型 `organize`；状态机复用 Phase 1。
- 进度**确定型**：`已完成 x / y`、百分比、当前文件、当前目标类别、已耗时。
- 可取消：已完成移动保留，任务 `canceled`，报告标注部分完成。

### T3.4 整理报告（FR-10）

- `GetOrganizeReport(taskID)`：成功数/失败数（含明细）/耗时/**已移动数据量**/状态。
- 「已移动数据量」= 成功 move 的 `size_bytes` 之和。

### T3.5 撤销上一次整理（FR-11，analysis §2.3）

- 定位「最近一次可撤销整理任务」（P1-2 规则）。
- 已 `undone` → `ErrAlreadyUndone`；无可撤销 → `ErrNothingToUndo`。
- 读取 `task_id=? AND action='move' AND status='success'`，逐条反向：`dst` 存在且 `src` 空缺 → 移回；否则 `failed` 继续。
- 每条尝试写 `operations(action='undo')`。
- 结束后标记原整理任务 `undone=1`。
- 撤销任务建模：`organize` 类型 + `parent_task_id` 指向被撤销任务（P1-1）。
- 进度确定型；可取消。

### T3.6 Wails 绑定与前端 P4

- 绑定：`StartOrganize`（**仅前端二次确认后调用**）、`GetOrganizeReport`、`UndoLastOrganize`。
- 前端 P4 整理页：二次确认弹窗（数量 + 目标目录）、确定型进度、日志、报告、失败明细、撤销按钮（二次确认）。
- 交互硬规则：取消 = 零文件系统写操作。

---

## 4. 交付物

`internal/organizer/{organizer.go,conflict.go,undo.go}` + 测试；`internal/storage`（operations、tasks.undone）；`internal/app`（3 个绑定）；前端 P4。

---

## 5. Definition of Done

```text
✓ t.TempDir 集成测试：正常移动 / 同名冲突 / 占用失败 / 取消 / 撤销 全路径
✓ 「零删除、零覆盖」断言：目标已存在时生成新名而非覆盖；目录内既有文件逐一比对未变
✓ 目标目录创建失败 → 零移动
✓ 单文件失败不中断整体任务
✓ 撤销：成功回移、原路径占用跳过、重复撤销报错
✓ 已移动数据量 = 成功项 size 之和（对账一致）
✓ gofmt / go vet / go test / go test -race（organizer 顺序执行，如引入并发则强制）
✓ git diff 自检干净；如实报告
```

---

## 6. 风险与待裁决

- **覆盖防护**：方案②（`syscall.MoveFile`）需自处理长路径；方案① 依赖前置检查（TOCTOU 极低但存在）。最终以 Phase 0 V1 结论为准。
- `undone` 标记与撤销任务的 `parent_task_id` 建模（P1-1）需先定。
- 撤销对「文件被外部修改」不校验内容，仅回移（P2-7，需确认可接受）。
- 长路径 / junction 目标仍需 Phase 0 V4/V5 结论。

---

### Phase 3 编码裁决记录（2026-10-15，Phase 3 编码前）

Phase 3 编码前确定并采纳以下设计裁决（编号 `D-D1`…`D-D8`，代码注释引用同号）：

| # | 事项 | 裁决 | 依据 / 影响 |
|---|---|---|---|
| D-D1 | 整理目标目录布局 | `<扫描根>/FilePilot整理/<类别>/<文件名>`（`fsx.OrganizeDirName` 常量，P1-3） | 与 FR-9 一致；固定常量便于 SEC-3 前缀校验 |
| D-D2 | 撤销目标选取（P1-2） | 取该 `scan_id` 下**最近一个存在成功 move 的 organize 任务**（`id` 最大）；无 → `ErrNothingToUndo`；该任务 `undone=1` → `ErrAlreadyUndone`；撤销任务（无成功 move）被排除 | 只允许撤销「上一次」整理（R-9）；EXISTS 条件天然排除撤销任务 |
| D-D3 | `StartOrganize` 前置与预检 | 仅接受 `completed` 扫描（否则 `ErrScanIncomplete`）；**同步** `MkdirAll` 预检目标目录，失败 → `ErrTargetDirCreate`（裸哨兵）、**不建任务、零移动** | 「取消 = 零文件系统写」；把可能失败的建目录动作提到任务创建前 |
| D-D4 | 绝不覆盖与冲突改名 | 目标名先经 `Lstat` 存在性检查取第一个空闲变体 `name (n).ext`（n≤10000），检查紧邻 `rename`；上限耗尽 → 该条 `failed`（「同名文件过多」） | SEC-1：Windows `os.Rename` 会覆盖，存在性检查是唯一保障（P0-2） |
| D-D5 | 整理/撤销日志口径 | 仅用 `kind=system`：启动、收尾汇总、失败明细（条数封顶）；成功项**不逐条**记录 | 避免日志污染（AGENTS §28.3）；逐条审计由 `operations` 表承担 |
| D-D6 | 撤销时标记 `undone` | 仅**正常完成**时标记原任务 `undone=1`；取消/失败不标记，允许安全重试 | 取消后原整理仍可再次撤销；`ErrAlreadyUndone` 仅在真正完成后出现 |
| D-D7 | 整理/撤销进度载荷 | 确定型：`done/total/percent/current_file/target_category` **累加**到 `task:progress`（contracts §3.8）；`Throttler` 进度源泛化为 `func() ProgressUpdate` | 整理总数在执行前已知，可展示百分比；与扫描的不确定型共用一个载荷结构、按任务类型取舍 |
| D-D8 | 移动与操作记录的一致性 | 顺序执行；先 `rename` 成功 → 写 `success` 记录；**写记录失败则 best-effort 回滚 `rename` 并中止任务**；写记录用 `context.WithoutCancel`；单条 move 失败直接写 `failed` 并继续 | 避免「已移动但不可撤销」的不一致（AGENTS §22.3）；FR-11 正确性优先于取消即时性 |
| D-D9 | FR-9.1 二次确认的「文件数量 N」口径（**需求方裁决 2026-10-15**） | 取**实际将整理数**（`excluded=0`）；实现为 `ScanSummary` **累加只读字段 `organizable_count`**（`storage.CountOrganizeCandidates`），前端 `N = organizable_count` | 现有 API 无此口径：`file_count`/`GetCategorySummary` 均含已排除；按「即将整理」语义须排除。选累加 `ScanSummary` 字段而非新增绑定：绑定集仍为 14 个、非破坏兼容（AGENTS §58）；已同步 `contracts.md` §3.6 |

> 附 0（需求方裁决）：FR-9.1 的 `N` 采用「实际将整理数（`excluded=0`）」，并据此为 `GetScanSummary` 追加 `organizable_count` 字段（D-D9）。

> 附 1：撤销写记录失败**不**中止撤销（`undoOne` 忽略写返回），以保证 FR-11 对「部分完成」仍有效；持续写库故障会在任务终态落库时暴露为任务失败（AGENTS §9.1 例外，代码已注释说明）。
> 附 2：撤销记录方向为 `src_path`=整理后位置、`dst_path`=原位置（物理方向），报告按任务 `parent_task_id` 是否为 0 选择 `action`（`move`/`undo`）。
> 附 3：`Manager.Finish` 仅对 `Type==TypeScan` 调用 `FinishScan`，防止整理/撤销任务以零值覆盖扫描统计。
> 附 4：Phase 3 **无需新迁移**——`migrations/0001_init.sql` 已含 `tasks.parent_task_id`、`tasks.undone` 与 `operations(size_bytes)`。
> 附 5：`contracts.md` §4 中 `ErrNoActiveOrganize / ErrNothingToUndo` 的并列命名收敛为单一哨兵 `ErrNothingToUndo`（实现只此一个）。

### Phase 3 验收记录（2026-10-15）

| 项 | 结论 |
|---|---|
| Phase 3 验收 | **需求方验收通过**。v1（Phase 1–3）交付完成 |
| D-D9 | **批准**：`GetScanSummary` 保留只读字段 `organizable_count`（`excluded=0`），绑定集仍为 14 个 |
| FR-9.1 位置修正 | 「开始整理」按钮 + 二次确认按 `requirements.md` §九 落在 **P3 结果页**；P4 仅进度/报告/撤销 |
| 实现修正 | 整理/撤销收尾日志改为 `Stop` 之后直接 `EmitLogs`（原 `th.Log` after `Stop` 会静默丢失），已加回归断言（`整理完成` / `撤销完成`） |
