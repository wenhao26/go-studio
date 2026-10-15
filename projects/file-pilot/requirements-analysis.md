# file-pilot 需求细化与可落地实施分析

> **文档性质**：阶段一（需求澄清 + 方案设计）产出物，**不含任何业务实现代码**。
> **输入**：[`requirements.md`](requirements.md)（R2）、[`AGENTS.md`](../../AGENTS.md)、[`docs/README.md`](../../docs/README.md)、[`docs/development/*`](../../docs/development/)、[`projects/fast-stat`](../fast-stat)（参考实现与裁决基线）。
> **生成时间**：2026-10-14
> **状态**：✅ 🔴 P0 阻塞项已于 2026-10-15 全部裁决（见 §7 更新说明），Phase 0 工程骨架已落地、待需求方验收；🟡 P1 / P2 项随对应阶段处理。

---

## 0. 阅读指引与结论摘要

### 0.1 本文档定位

`requirements.md` 是**需求权威来源**；本文档是它的**实施级展开**：把每条 FR 翻译成「可编码、可测试、可验收」的定义，并把仍不确定的点集中成 §7 供裁决。

本文档**不复制** `requirements.md` 的全部内容，只做「补充精度、暴露缺口、锁定语义」。

### 0.2 结论摘要（先看这里）

`requirements.md`（R2）方向清晰、边界收敛，**没有需要推倒重来的架构问题**。但存在 **3 项阻塞级问题**与 **大量实现级歧义**必须在编码前裁决或补齐：

| 级别 | 结论 | 位置 |
|---|---|---|
| 🔴 矛盾 | **分类算法描述与校验示例自相矛盾**：`合同.pdf` 在「按类别独立打分、取最高分类别」的写法下应为 `文档 60%`，但示例要求 `工作 95%`。必须重述算法（本文 §2.1 给出两种口径） —— ✅ **2026-10-15 裁决口径 B，`requirements.md` §5.2/§5.6 已按 R3 修订** | §2.1 |
| 🔴 安全 | **`os.Rename` 在 Windows 上会覆盖已存在目标**（已核实 Go 1.25.4 源码：底层 `MoveFileEx(..., MOVEFILE_REPLACE_EXISTING)`）。因此「同名冲突靠 rename 失败来兜底」**不成立**，`绝不覆盖`（FR-9.4 / SEC-1）必须由**前置存在性检查**保证 —— ✅ **裁决方案①（预检 + 复检）** | §2.2、§7 P0-2 |
| 🔴 阻塞 | **依赖白名单未批准** `wailsapp/wails/v2`、`modernc.org/sqlite`（`docs/development/dependencies.md` 无这两个包），且**Windows 隐藏/系统属性与创建时间采集可能还需要平台 API 能力** —— ✅ **已批准并写入 §2.2；V2/V3 验证证明标准库足够，无需 `x/sys/windows`** | §7 P0-1、P0-3 |
| 🟡 缺口 | 撤销任务如何在 `tasks` 表建模、「最近一次整理任务」的精确选取规则、路径规则的匹配基准（绝对 vs 相对）、目标目录名常量等 | §7 P1 |
| 🟢 待确认 | 关键词多命中时「原因」的确定性、`files.path` 大小写敏感、目录计数口径等 | §7 P2 |

### 0.3 三个必须牢记的认知

1. **扫描进度「不确定型」是信息论约束，不是实现偷懒**：目录遍历结束前总文件数不可知，任何百分比/ETA 都是编造（沿用 fast-stat P0-5）。
2. **「移动」= 同卷改名**：不复制、不删除、不改内容；因此**不释放空间**（D-6 已修正「释放空间」缺陷）。撤销 = 反向改名，语义完全对称。
3. **安全边界靠「硬约束 + 兜底校验」双重保证**，而不是靠单一 API 语义：目标目录固定在扫描根内（结构约束）+ `filepath` 前缀校验（SEC-3）+ 存在性检查（防覆盖）。

---

## 1. 需求逐条细化为可落地规格

> 记号约定：`✅` = `requirements.md` 已明确；`⚠️` = 本文补充的落地精度；`❓` = 需裁决（汇总见 §7）。

### 1.0 统一术语（Glossary，供代码标识符与 UI 文案统一）

| 术语 | 代码标识 | 定义 |
|---|---|---|
| 扫描（Scan） | `scan` | 对用户选定根目录的递归遍历，并顺带完成分类；产出 `scans` + `files` |
| 文件条目（File Row） | `files` 表一行 | 一个普通文件的一条索引（含分类结果） |
| 类别（Category） | `category` | 7 类之一：`图片/视频/文档/财务/工作/学习/其他` |
| 整理（Organize） | `organize` | 按分类计划把文件移动到 `<根>/FilePilot整理/<类别>/` |
| 操作记录（Operation） | `operations` 表一行 | 一次 move 或 undo 的原子记录 |
| 任务（Task） | `tasks` 表一行 | 一次 scan 或 organize 的状态机实例 |
| 置信度（Confidence） | `confidence` | 0–100 整数；用户手动指定时为空/`手动` |
| 目标根（Target Root） | 常量 | `<扫描根>/FilePilot整理/`（FR-9） |

### 1.1 产品定位与边界（再次确认）

- 桌面应用（Wails v2），**单进程、完全离线**、无守护、无后台常驻、无网络。
- 只读扫描 + 同卷改名移动；**v1 零删除、零覆盖、零内容改写**。
- 同一时刻**至多一个活动任务**（§6.3）。

### 1.2 FR-1 首页：目录选择

| 维度 | 落地规格 |
|---|---|
| 绑定方法 | `ChooseDirectory() (string, error)`（§8.1） |
| 调用约定 | 前端**必须**先调 `ChooseDirectory` 再点「开始扫描」；未选目录时前端拦截，不调 `StartScan` ✅ |
| 返回值 | 选中绝对路径；用户取消 → 返回错误（前端映射为 `目录选择失败 / 原因: 用户取消操作`）✅ |
| 最近目录 | `settings.recent_dirs`，最多 5 条，**去重 + 最近使用在前**⚠️；点击即回填路径 |
| 「预计文件」 | 恒显示 `未知`，**不预扫描、不估算** ✅ |
| 边界 | 路径含中文/空格/超长（Windows `\\?\`）需可正常回填与展示 ⚠️ |

### 1.3 FR-2 扫描配置

| 配置项 | 字段 | 类型 | 默认值 | 校验（落地） |
|---|---|---|---|---|
| 扫描并发数 | `workers` | int | `clamp(NumCPU, 1, 64)` | 允许 `[1,64]`；非法（<1 或 >64）**立即返回错误，不静默钳制** ✅ |
| 扫描隐藏文件 | `scan_hidden` | bool | `false` | true 时纳入 `.` 前缀文件与 Windows 隐藏属性文件 ✅ |
| 扫描系统文件 | `scan_system` | bool | `false` | true 时纳入 Windows system 属性文件 ✅ |
| 跳过大文件 | `skip_large` | bool | `false` | true 时跳过 `> 1 GiB`（阈值固定，不可配置）✅ |

落地补充（⚠️）：

- **配置载体**：`ScanConfig` 强类型 struct（对齐 `config.md`「禁止散落 `os.Getenv`」精神）；以 JSON 存 `settings.scan_config`。
- **校验位置**：`StartScan` 边界校验（对齐 fast-stat「参数校验放在 CLI 边界」的做法），非法值**不创建任务**。
- **持久化时机**：`SaveSettings` 显式保存；下次启动 `GetSettings` 回填 ✅。
- **内置黑名单**：`$Recycle.Bin`、`System Volume Information` **始终跳过**，与开关无关 ✅。
- **隐藏/系统目录**：开关为 false 时，隐藏/系统**目录**与文件同样跳过（不下潜、计入跳过数）——需求只写了「文件」，此处由需求方 2026-10-15 裁决补齐口径，避免 `.git` 等目录遍历撑爆结果 ✅。
- ❓ ❽「系统文件」是否同时指**系统目录**（如 `Windows/`）？`requirements.md` 只写「文件」——按字面仅过滤带 system 属性的文件。见 §7 P1-4。

### 1.4 FR-3 文件扫描

**遍历语义（落地）**：

| 项 | 规格 |
|---|---|
| 范围 | 所选根目录递归全量（多层子目录）✅ |
| 采集字段 | 路径、文件名、扩展名、大小（`st_size` 逻辑大小）、修改时间、创建时间（best-effort）、是否隐藏/系统 ✅ |
| 排除 | ① `<根>/FilePilot整理/` 始终排除（FR-3/SEC-4）；② 黑名单目录；③ 符号链接/junction 不下潜、不纳入、计入跳过 ✅ |
| 流水线 | 文件一经发现**立即**分类（§2.1），结果随文件一起批量落库；**「分类任务」并入扫描任务**（D-5）✅ |
| 落库 | 内存全量缓冲供预览 + **每 500 条一个事务**批量写 `files` ✅ |
| 取消 | `ctx` 取消后停止派发、worker 退出、已扫描部分落库 ✅ |

落地补充（⚠️）：

- **大小类型**：一律 `int64`，禁止 `int`（`AGENTS.md` §95 / fast-stat 同类裁决）。
- **扩展名归一**：存储为**小写、不带点**（如 `pdf`）；无扩展名存空串。
- **路径归一**：存**绝对路径**，展示时再决定是否截断。
- **隐藏/系统属性采集**：Windows 需读文件属性（`GetFileAttributes`）。❓ 具体技术路径待定（标准库 `syscall` vs `golang.org/x/sys`），影响依赖白名单——见 §7 P0-3。
- **创建时间**：Windows 取 `FILE_BASIC_INFO.CreationTime`（best-effort，失败置 NULL）；非 Windows 回退 `mtime`（R-2）❓待复核。
- **Windows junction/reparse point**：Go 中可能表现为 `ModeIrregular` 或 symlink，需显式识别并跳过（防越界/防环，参照 fast-stat `countEntry` 的 junction 特判思路）⚠️。
- **长路径**：目标平台 Windows 需考虑 `>260` 字符（Go 对多数 os API 自动 `\\?\` 前缀化，但需在 Phase 1 验证）⚠️。
- **目录计数口径**：`dir_count` **仅统计成功完整读取的目录**（对齐 fast-stat「只有完整成功 ReadDir 才算一次」）❓P2-3。
- ❓ ❾ 扫描结果**是否包含空目录**？按 `files` 语义推断**不含**（只索引文件）——需确认。
- **内存口径（✅ 需求方 2026-10-15 裁决）**：FR-3「内存全量保留供预览」与 §1.17 内存 NFR、contracts §7「预览走 DB 聚合/分页」冲突，采纳**仅保留 500 条落库批量窗口**，预览/统计一律读 SQLite，峰值内存与文件数无关。

### 1.5 FR-4 实时进度与日志

**进度模型（必须分型）**：

| 阶段 | 模型 | 展示字段 |
|---|---|---|
| 扫描 | **不确定型** | 已扫描文件数、速率 `files/s`、当前目录、已耗时、跳过数。**禁止总数/百分比/ETA** ✅ |
| 整理 | **确定型** | `已完成 x / y`、百分比、当前文件、当前目标类别、已耗时 ✅ |

**三类日志**：

| 类型 | `kind` | 内容 | 采样 |
|---|---|---|---|
| 扫描日志 | `scan` | `扫描: <name> · <size>` | ✅ 需采样（不逐文件推送） |
| 分类日志 | `classify` | `分析: <name> · <原因> · 建议分类: <c> · 可信度: NN%` | ✅ |
| 系统日志 | `system` | 任务启动/完成/取消、丢弃计数披露 | 不丢弃 ⚠️ |

落地补充（⚠️）：事件节流与采样算法见 §2.4；前端环形缓冲上限 500 行 ✅。

### 1.6 FR-5 扫描统计

| 分组 | 指标 | 字段 | 口径 |
|---|---|---|---|
| 文件统计 | 文件数量 | `file_count` | 普通文件数（不含目录、不含链接）✅ |
| | 目录数量 | `dir_count` | 成功读取的目录数 ❓P2-3 |
| | 总大小 | `total_size_bytes` | 逻辑大小之和（`st_size`），`int64` ✅ |
| 性能统计 | 扫描耗时 | `duration_ms` | 见 §2.5 精度 ✅ |
| | 平均速度 | `avg_speed` | `file_count / 耗时秒`，展示 `files/s` ⚠️ |
| | 最大并发数 | `max_workers` | 本次实际生效的 worker 数 ✅ |
| | 跳过错误数 | `skipped_count` | 权限失败 + 链接 + 开关跳过（隐藏/系统/大文件）+ 黑名单目录 ✅ |

- **跳过明细**：最多保留 20 条，展示于日志区 ✅。
- **单位口径**：基数 1024，显示 `KB/MB/GB` 两位小数，`Bytes` 整数带千分位（D-9）✅。
- **耗时精度**：≥1s 两位小数秒；<1s 一位小数毫秒 ✅。
- **取消扫描**：`canceled` + 标注「已取消 · 部分结果」；**部分结果可看统计，但不进入预览与整理**（D-13）✅。

### 1.7 FR-6 / §五 分类引擎

算法与边界见 **§2.1**（🔴 含矛盾裁决）。规则表（扩展名/关键词/路径）以 `requirements.md` §5.3–§5.5 为**唯一来源**，实现时为代码内常量，集中单点定义 ✅。

### 1.8 FR-7 分类预览与详情

| 项 | 落地规格 |
|---|---|
| 汇总 | 按类别计数 `{category, count}`；7 类**固定顺序**：图片、视频、文档、财务、工作、学习、其他 ✅ |
| 计数来源 | 聚合查询（`GROUP BY category`），不做内存全量扫描 ⚠️ |
| 详情 | 分页 `page`（从 1 开始）、`page_size` 默认 100 ✅；排序需**稳定**（建议 `ORDER BY id`，即插入序）⚠️ |
| 行字段 | 文件名、路径、大小、目标类别、置信度、原因 ✅ |
| ❓ ❺ | 详情是否支持「全部类别」视图（不筛选类别）？建议支持 `category = ""` 表示全部 ⚠️ |
| ❓ ❻ | 是否支持排序切换（名称/大小）？`requirements.md` 未要求 → v1 **不做**（§70） |

### 1.9 FR-8 分类调整

| 操作 | 落库 | 展示 |
|---|---|---|
| 重新指派类别 | `category = ?`, `source = 'user'`, `confidence = NULL`, `reason = '用户手动指定'` | 置信度列显示 `手动` ✅ |
| 排除出本次整理 | `excluded = 1`（保留可恢复） | 标记「已排除」，可撤销该标记 ✅ |
| 恢复排除 | `excluded = 0` ⚠️（requirements 明确「可恢复」） | — |

- 立即落库、立即影响整理计划 ✅。
- `SetFilePlan` 入参需校验 `category ∈ 7 类`、`fileID` 属于目标 `scan`，否则返回校验错误 ⚠️。

### 1.10 FR-9 执行整理（安全关键）

**前置**：前端二次确认弹窗（数量 + 目标目录），取消 = **零文件系统写操作** ✅。

**执行步骤（落地）**：

1. **预检**：`MkdirAll(<根>/FilePilot整理/<类别>/)`；任一创建失败 → 任务整体失败，**零移动** ✅。
2. 仅处理 `excluded=0` 的文件；按类别移动到对应子目录 ✅。
3. 移动 = **同卷改名**；目标固定在扫描根内 → 保证同卷（否则 `rename` 报跨卷错误）✅。
4. **同名冲突**：追加 `名 (n).扩展名`，**绝不覆盖**——但**不能依赖 `os.Rename` 失败**（见 §2.2 与 §0.2 安全发现）🔴。
5. **顺序执行**；单文件失败（占用/权限）→ 记录失败、**继续**，不中断 ✅。
6. 每次移动**立即**逐条写 `operations`（成功/失败/原因/时间）✅。
7. **可取消**：已完成移动保留，任务 `canceled`，报告标注部分完成；撤销对部分完成任务同样有效（仅回移成功项）✅。

**待补精度**：

- 目标目录名常量 `FilePilot整理`：❓固定常量？可配置？含中文的编码/长路径风险？见 §7 P1-3。
- 若 `<根>/FilePilot整理` 已存在（上次整理遗留）：允许（冲突改名机制处理）✅（推断）。
- 跨卷兜底（理论不可达，因目标在根内）：若遇 `EXDEV` → 该条 `failed`，不复制 ✅（推断，需确认「不自动降级为复制」）。

### 1.11 FR-10 整理报告

展示：成功数、失败数（含失败明细入口）、耗时、**已移动数据量**、任务状态（`完成 / 已取消·部分完成`）✅。「已移动数据量」= 成功 move 的源文件 `size_bytes` 之和 ⚠️。

### 1.12 FR-11 撤销上一次整理

精确算法见 **§2.3**。待补精度：

- ❓「最近一次整理任务」的精确选取（见 §7 P1-2）。
- ❓ 撤销任务在 `tasks` 表如何建模（`type=undo` 还是 `type=organize` + `parent_task_id`）——`requirements.md` §6.1 把撤销归入 `organize`，但 `undone` 标记的是**原**整理任务；本文建议新增 `tasks.parent_task_id`，见 §7 P1-1。
- 撤销的进度为确定型（x/y）✅。

### 1.13 FR-12 任务管理

**状态机（v1 激活 5 态）**：`pending → running → {completed | failed | canceled}`；`pending → canceled`（未启动即取消/关闭）。`paused` 预留不可达（D-4）✅。

落地补充（⚠️）：

- **非法转换必须拒绝并返回错误**（不静默）✅；建议状态机表驱动（对齐 `domain.md` §2 思路）。
- **单活动任务**：由**任务管理器**（内存态）持有「当前 running 任务」引用 + 互斥；`StartScan/StartOrganize/UndoLastOrganize` 入口统一执行此检查 ✅。
- **异常恢复**：启动时把 DB 中遗留 `running` 置 `failed`（原因 `应用中断`）✅。
- **进度模型随类型**：`scan` 不确定型、`organize`（含 undo）确定型 ✅。
- `failed`/`canceled` 必须携带原因字段 ✅。

### 1.14 FR-13 数据持久化

见 **§3**（列级落地）。

### 1.15 §八 Wails 契约

落地补充（⚠️）：

- `snake_case` 字段命名，**不套用** `api.md` 的 `{code,message,data,trace_id}`（D-12）✅。
- 绑定方法全集 12 个，事件 3 个 ✅；**分页返回结构**建议 `{items, page, page_size, total_count, total_page}`（借鉴 `api.md` §4 的字段风格，但非 HTTP 包装）⚠️。
- 哨兵错误清单见 §7 前置与 §3 附录；前端维护「哨兵 → 友好文案」映射 ✅。
- ❓ 事件不持久化：前端重连/切页后靠 `GetActiveTask` + `GetScanSummary` + `ListFiles` 对齐状态，不依赖事件重放 ⚠️。

### 1.16 §九 UI 与交互

页面 P1–P4 + 交互硬规则 4 条 ✅。落地补充：空态文案、失败明细去留、任务进行中离开页面策略（二次确认或后台继续 + 指示器）✅。

### 1.17 §十/§十一 NFR 与安全可验证化

| 项 | 落地可验证定义 |
|---|---|
| 扫描吞吐 | SSD 10 万文件 ≤ 60s（默认并发）；用固定数据集人工基准 + 记录环境 ⚠️ |
| 整理吞吐 | 10 万文件移动 ≤ 60s（顺序改名）⚠️ |
| 内存 | 峰值不随文件数线性暴涨（缓冲批量落库 + 日志环形缓冲）；观察 RSS ⚠️ |
| UI 不卡顿 | 事件节流（§2.4）保证事件率上限；手动验收 ⚠️ |
| 稳定性 | 无 goroutine 泄漏；异常退出恢复；优雅退出 ≤3s ✅ |
| SEC-3 | 移动前 `filepath.Clean` 校验 `dst` 前缀 = 目标根 ✅ |
| SEC-5 | 写日志/事件前剥离控制字符 ✅ |

---

## 2. 核心算法与语义精确定义

### 2.1 分类评分与选类（🔴 需裁决）

**现行描述的问题**：`requirements.md` §5.2 写「类别得分 = 扩展名分 + 关键词分 + 路径分」且「得分最高者为推荐类别」，但同时给出 `合同.pdf → 工作 95%`。**二者不自洽**：

- 按字面（每类别独立打分、取最高）：`文档 = 60`、`工作 = 35` → 结果应为 **`文档 60%`**。
- 示例要求 **`工作 95%`**（= 60 + 35）。

即：示例把「跨类别总分」当作置信度，并让**语义信号（关键词/路径）优先于扩展名**决定类别。

**两种候选口径（需选一）**：

| 口径 | 类别选择 | 置信度 | 与 4 个示例是否一致 |
|---|---|---|---|
| **A 字面版** | 每类别独立求和，取分值最高者；平局按优先级 | 胜出类别的得分 | ❌ `合同.pdf` 会变 `文档 60%`，示例需改 |
| **B 语义优先版（推荐）** | 在「至少命中一条规则的类别」中，按**固定优先级**选类：`财务 > 工作 > 学习 > 视频 > 图片 > 文档 > 其他` | **所有命中项分数之和**，封顶 100 | ✅ 4 个示例全部吻合 |

**推荐口径 B 的精确算法**（可复算）：

1. 计算命中项：扩展名（+60，唯一类别）、文件名关键词（每个 +35，同一类别封顶 +70）、路径子串（每条 +20，同一类别封顶 +20）。匹配前统一小写；关键词对**不含扩展名的文件名**做子串匹配；路径对**完整路径**做子串匹配。
2. 令 `matched = 至少命中一条规则的类别集合`。
3. 若 `matched` 为空 → 类别 = `其他`，置信度 = `0`，原因 = `未命中任何规则`。
4. 否则，类别 = `matched` 中**优先级最高**者（优先级就是上面 7 类顺序，语义类天然优先于媒体类）。
5. 置信度 = `min(100, 所有命中项分数之和)`（跨类别求和，与选类无关）。
6. 原因 = **决定该类别的命中项**中分值最高者；若并列，按「扩展名 > 关键词 > 路径」再按**关键词表声明顺序**取第一个（确定性）⚠️。

**校验用例（口径 B，全部可复算）**：

| 文件 | 命中项 | matched | 选类 | 置信度 | 原因 |
|---|---|---|---|---|---|
| `合同.pdf` | pdf→文档 60；合同→工作 35 | {文档, 工作} | **工作** | 95 | 命中关键词: 合同 |
| `发票2025.pdf` | pdf→文档 60；发票→财务 35 | {文档, 财务} | **财务** | 95 | 命中关键词: 发票 |
| `photo.jpg` | jpg→图片 60 | {图片} | **图片** | 60 | 扩展名 .jpg → 图片 |
| `README` | 无 | {} | **其他** | 0 | 未命中任何规则 |
| `项目方案.docx` | docx→文档 60；项目 35；方案 35 | {文档, 工作} | **工作** | 100（130 截顶） | 命中关键词: 项目 |
| `D:/合同/a.pdf` | pdf→文档 60；路径"合同"→工作 20 | {文档, 工作} | **工作** | 80 | 路径命中: 合同 |

> ✅ **已执行**：需求方 2026-10-15 裁决采纳口径 B，`requirements.md` §5.2 已改写为「按优先级选类、置信度为跨类别匹配总分」，§5.6 原因规则同步修订（修订号 R3）。

### 2.2 同名冲突命名与「绝不覆盖」保障（🔴 安全）

**已核实事实**（Go 1.25.4 源码）：`os.Rename` 在 Windows 底层为 `MoveFileEx(from, to, MOVEFILE_REPLACE_EXISTING)` → **会覆盖已存在的目标文件**。因此：

> 「目标已存在时 `os.Rename` 会失败，所以不会覆盖」这一假设**在 Windows 上错误**。

**落地算法（推荐）**：

1. 生成候选名：`原名`；若 `Lstat(目标)` 存在 → `base + " (" + n + ")" + ext`，`n = 1,2,3...`，取**第一个不存在**者。
2. **紧邻 rename 前再校验一次**目标不存在（缩小 TOCTOU 窗口；目标目录由本应用独占管理，外部篡改概率极低）。
3. 执行**同卷改名**。可选加固：改用 stdlib `syscall.MoveFile`（`MoveFileW`，**不带替换标志**，目标存在即失败），代价是**不做长路径 `\\?\` 归一**（需自行处理超长路径）。二者取舍见 §7 P0-2。
4. 若 `n` 超过上限（建议 10000）仍冲突 → 该条 `failed`，原因 `无法生成唯一文件名`，继续下一条。

**注意**：`MOVEFILE_REPLACE_EXISTING` 不能替换目录，只能替换文件，所以「覆盖」风险仅存在于**文件目标**。

### 2.3 撤销反向算法

前置：撤销任务可取消；进度确定型。

1. 定位「最近一次可撤销整理任务」`T`（选取规则见 §7 P1-2）。
2. 若 `T.undone = 1` → 返回 `ErrAlreadyUndone`（前端文案 `该整理已撤销`）。
3. 读取 `operations WHERE task_id = T AND action = 'move' AND status = 'success'`。
4. 对每条逐条反向：
   - `src = op.src_path`（原位置），`dst = op.dst_path`（整理后位置）。
   - 若 `dst` 不存在 → 该条 `failed`（原因 `目标不存在`），继续。
   - 若 `src` 已被占用 → 该条 `failed`（原因 `原路径已被占用`），继续。
   - 否则同卷改名 `dst → src`，成功则该条 `undo` 记录 `status = success`。
5. 每条尝试**立即**写 `operations`（`action = 'undo'`，成功/失败/原因）⚠️。
6. 结束后令 `T.undone = 1`。
7. 撤销任务本身作为一条 `tasks` 行；❓其与 `T` 的关联方式见 §7 P1-1。
8. **不校验文件内容是否被外部修改**：撤销只做位置回移（❓是否可接受，见 §7 P2）。

> 撤销的 `operations` 记录**是否也需要**纳入「最近一次整理」判定？不需要——`undo` 记录只用于审计，不作为再次撤销的数据源。

### 2.4 事件节流与日志采样

| 事件 | 节流/批量 | 上限 |
|---|---|---|
| `task:progress` | 后端统一节流，间隔 **≥ 200ms**；禁止逐文件发送 ✅ | 每任务 ≤ ~5 次/秒 |
| `task:log` | **200ms 窗口**批量聚合为数组 ✅ | 单窗口 ≤ 20 条，超出**采样丢弃**并累计 `dropped` ⚠️ |
| `task:status` | 每次**合法转换**立即发（低频，不节流）✅ | — |

落地补充（⚠️）：

- **采样策略需确定性**：窗口内 ≤20 条→全发；>20 条→保留**前 20 条**，其余丢弃并累加 `dropped`；在**下一个系统日志**中追加 `已丢弃 N 条明细`。**完成/失败/取消等关键系统日志永不丢弃**。
- 前端环形缓冲 500 行；分类原因**全量存库**（UI 丢弃不丢数据）✅。
- ❓ 节流器 goroutine 生命周期：随任务创建/结束，退出前需 `Stop` ticker 并 `Wait`（§4.5）。

### 2.5 大小 / 耗时 / 速率格式化

| 项 | 规则 |
|---|---|
| 大小 | 基数 1024；`B` 整数 + 千分位；`KB/MB/GB` 两位小数；阈值比较在**字节**上做（避免 `1023.9KB → 1.00KB` 抖动）⚠️ |
| 耗时 | ≥1s → 两位小数秒；<1s → 一位小数毫秒 ✅ |
| 速率 | `files/s` 取整或一位小数（❓建议取整）⚠️ |

---

## 3. 数据模型落地细节

> 驱动：`modernc.org/sqlite`（纯 Go，D-2）。库位置：Wails 应用数据目录（`%APPDATA%/FilePilot/file-pilot.db`）。所有 SQL 参数化、禁 `SELECT *` ✅。字段类型遵循 SQLite 动态类型，但**逻辑上**按下表约束。

### 3.1 表结构（列级）

**`scans`**（一次扫描一行）

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | INTEGER | PK AUTOINCREMENT | 扫描 ID |
| `root_path` | TEXT | NOT NULL | 绝对路径 |
| `status` | TEXT | NOT NULL | 与任务状态一致（`completed`/`canceled`/`failed`） |
| `started_at` / `finished_at` | TEXT | ISO8601 或 INTEGER 毫秒 | ❓时间存储格式统一（建议 UTC 毫秒整数） |
| `duration_ms` | INTEGER | | 时长 |
| `file_count` / `dir_count` / `total_size_bytes` / `skipped_count` / `max_workers` | INTEGER | | 统计口径见 §1.6 |
| `avg_speed` | REAL | | `files/s` |

**`files`**（每文件一行，10 万级/次）

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | INTEGER | PK AUTOINCREMENT | 插入序，供稳定分页 |
| `scan_id` | INTEGER | NOT NULL, INDEX | 关联 `scans.id` |
| `path` | TEXT | NOT NULL | 绝对路径 |
| `name` | TEXT | NOT NULL | 文件名 |
| `ext` | TEXT | | 小写、无点 |
| `size_bytes` | INTEGER | NOT NULL | `st_size` |
| `mtime` / `ctime` | INTEGER/TEXT | `ctime` NULL | ctime best-effort |
| `category` | TEXT | NOT NULL | 7 类之一 |
| `confidence` | INTEGER | NULL | 0–100；用户手动为 NULL |
| `source` | TEXT | NOT NULL | `rule` / `user` |
| `reason` | TEXT | NOT NULL | 展示短语 |
| `excluded` | INTEGER | NOT NULL DEFAULT 0 | 0/1 |

**`tasks`**

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | INTEGER | PK AUTOINCREMENT | |
| `type` | TEXT | NOT NULL | `scan` / `organize`（❓undo 建模见 P1-1） |
| `status` | TEXT | NOT NULL | 5 态 |
| `scan_id` | INTEGER | NULL | 关联扫描 |
| `parent_task_id` | INTEGER | NULL | ❓新增：撤销任务指向被撤销的整理任务 |
| `started_at` / `finished_at` | TEXT | | |
| `error` | TEXT | NULL | 失败/取消原因 |
| `undone` | INTEGER | NOT NULL DEFAULT 0 | 原整理任务是否已撤销 |

**`operations`**

| 列 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | INTEGER | PK AUTOINCREMENT | |
| `task_id` | INTEGER | NOT NULL, INDEX | 关联任务 |
| `action` | TEXT | NOT NULL | `move` / `undo` |
| `src_path` / `dst_path` | TEXT | NOT NULL | 反向所需 |
| `status` | TEXT | NOT NULL | `success` / `failed` |
| `error` | TEXT | NULL | 失败原因 |
| `size_bytes` | INTEGER | NULL | ❓新增：用于「已移动数据量」聚合（否则需回查 `files`） |
| `created_at` | TEXT | NOT NULL | 记录时间 |

**`settings`**：`key` TEXT PK, `value` TEXT（JSON）。键：`scan_config`、`recent_dirs`。

**`schema_version`**：`version` INTEGER, `applied_at` TEXT。

### 3.2 索引与查询模式

| 查询 | 索引 |
|---|---|
| 分类计数 `GROUP BY category WHERE scan_id=?` | `idx_files_scan_category_id (scan_id, category, id)` ✅ |
| 分页 `WHERE scan_id=? AND category=? ORDER BY id LIMIT ? OFFSET ?` | 同上（最左前缀命中） |
| 任务查询 | `idx_tasks_scan (scan_id)`、状态查询可走 `tasks.status`（❓是否需要独立索引，量小可不建） |
| 操作查询 `WHERE task_id=? AND action='move' AND status='success'` | `idx_operations_task (task_id, action, status)` ⚠️ |

原则：单表索引尽量 ≤5；避免 `SELECT *`；分页用 `ORDER BY id`（稳定）。

### 3.3 事务、批量与保留

- `files` 每 **500 条**一个事务；`operations` **逐条同步写**（撤销依赖，不可延迟）✅。
- **保留策略**：仅保留最近 **20** 次扫描及级联的 `files`/`tasks`/`operations`；启动时清理更旧记录（R-4）✅。
- ❓级联删除需显式实现（SQLite 需 `PRAGMA foreign_keys=ON` 且定义 `ON DELETE CASCADE`，或显式多表删除）。建议**显式删除**以避免 PRAGMA 依赖 ⚠️。

### 3.4 Migration

- 版本化 schema：`schema_version` + **嵌入式 SQL 步进**（`go:embed`），不修改已应用版本（`AGENTS.md` §22.4）✅。
- ❓是否采用 `golang-migrate/migrate`（`database.md` §5 推荐，但**不在白名单**且引入重依赖）——建议**自实现最小版本化步进器**，避免新增依赖 ⚠️（见 §7 P1-6）。

### 3.5 SQLite 运行参数

落地建议（⚠️，需确认）：开启 WAL（单进程读写仍受益）、`busy_timeout`、`PRAGMA foreign_keys`、单连接串行写（避免 `database is locked`）。均为连接字符串/PRAGMA 层配置，不引入依赖。

> ✅ **已裁决（需求方 2026-10-15，Phase 1）**：采纳**全套**——WAL + `busy_timeout=5000` + `foreign_keys=ON` + `MaxOpenConns=1`（单连接串行）。

---

## 4. 并发、任务与生命周期

### 4.1 扫描并发模型

沿用 **fast-stat 已验证的队列设计**（`internal/scan/queue.go` 思路），明确回答 `AGENTS.md` §18 的 8 问：

- 工作队列 = `mutex + sync.Cond + stack + inFlight + closed`（**不用有缓冲 channel**，规避生产/消费同体导致的死锁）。
- 固定 `workers` 个 goroutine；LIFO 深度优先；`push` 后 `done` 顺序不可颠倒（否则漏扫子树）。
- 取消：watcher goroutine 监听 `ctx.Done` → `q.close()` 唤醒全部 worker。
- 计数：`atomic.Int64` 热路径无锁；错误明细 `mutex` 最多 20 条。
- **禁止** `errgroup.SetLimit` 递归派发自死锁写法（`AGENTS.md` §17）✅。

### 4.2 单活动任务

- 任务管理器（内存）持 `mu sync.Mutex` + `running *TaskHandle`；入口先判 `running != nil` → `ErrTaskRunning`。
- 状态转换与 DB 持久化需**同一处**完成（避免状态机分裂）；建议状态机包在 `task` 内，DB 由 `storage` 落。
- 任务结束（任意终态）必须清空 `running` 并派发 `task:status`。

### 4.3 取消与恢复

- 协作式取消：`CancelTask(taskID)` → 找到 handle → `cancel()`；worker 每轮检查 `ctx.Err()`。
- 启动恢复：遗留 `running` → `failed(应用中断)`。
- 取消后仍保留已有结果（部分统计 / 部分移动）✅。

### 4.4 优雅退出

窗口关闭 → 取消活动任务 → 等待收尾（**≤3s 超时强退**）→ 关闭 DB；不留 `running` 谎言 ✅。需注意 Wails 生命周期钩子（`OnShutdown`）。

### 4.5 事件推送 goroutine

- 进度/日志节流器随任务创建，任务结束 `Stop()` ticker 并等待 goroutine 退出（防泄漏，§17.3）⚠️。
- 事件发送失败（前端未就绪）需有策略：丢弃或将 `dropped` 计入系统日志 ⚠️。

---

## 5. 规范适用性矩阵（`docs/development/*`）

| 规范 | 适用性 | 说明 / 处理 |
|---|---|---|
| `AGENTS.md` | ✅ 全适用 | 最小修改、`%w`、goroutine 可控、不伪造验证、diff 自检 |
| `go.md` | ✅ 适用 | 命名/Guard Clause/错误链/并发/`go vet`；`golangci-lint` 视环境 |
| `architecture.md` | ✅ 适用（需映射） | Handler/Service/Repo 映射为 `app / task·scanner·classifier·organizer / storage`；构造函数注入 + 消费方定义接口 |
| `domain.md` §2 状态机 | ✅ 适用 | 任务状态机矩阵化（5 态 + 预留 `paused`） |
| `domain.md` §3 ID | ❌ 不适用 | 本地自增 `INTEGER`，无分布式 ID |
| `domain.md` §4 金额 | ❌ 不适用 | 无金额；但「整数精度 + 展示层格式化」思路用于字节数 |
| `api.md` 全部 | ❌ 不适用 | 无 HTTP；仅**借鉴**分页字段命名风格（§4） |
| `database.md` | ⚠️ 部分适用 | MySQL/utf8mb4/InnoDB/GORM N+1/Redis 条款 ❌；参数化、禁 `SELECT *`、索引、事务边界、Migration 版本化 ✅ |
| `config.md` | ⚠️ 部分适用 | viper/`.env`/多环境 ❌；强类型配置 + 启动校验 ✅（收敛于 `ScanConfig`） |
| `observability.md` §1 日志 | ⚠️ 部分适用 | zap ✅（诊断日志文件）；`trace_id`/OTel/Prometheus ❌（单进程本地） |
| `observability.md` §4 脱敏 | ✅ 精神适用 | 不记录文件内容/凭据；路径含用户名的 PII 但为功能必需 |
| `security.md` §1/§4/§5 | ❌ 不适用 | 无 JWT/限流/HTTP 头 |
| `security.md` §2 注入 | ✅ 适用 | SQL 参数化、路径不可信（`filepath.Clean` + 前缀校验）、无 shell 调用 |
| `testing.md` | ✅ 全适用 | 覆盖率 ≥70%、表格驱动、`-race`、时间依赖注入（禁 `time.Sleep`） |
| `dependencies.md` | 🔴 **硬冲突** | `wails/v2`、`modernc.org/sqlite` 未批准，必须先更新白名单（§7 P0-1） |
| `commenting.md` | ✅ 适用 | 包注释 + 导出注释；Swag 注释 ❌ |
| `environment.md` | ⚠️ 部分适用 | Makefile ✅（但**本项目需项目级 Makefile**，`fmt` 只格式化本项目改动文件）；docker-compose/seed ❌ |
| `git.md` | ⚠️ 缺失 | 索引存在但仓库内无该文件（沿用 fast-stat 报告的现状） |

---

## 6. 与 fast-stat 的复用与差异

| 维度 | fast-stat | file-pilot | 可复用性 |
|---|---|---|---|
| 形态 | CLI 一次性进程 | Wails 桌面应用（长驻窗口） | 并发/队列/单位/净化思路可复用 |
| 模块 | 独立 module | 独立 module（D-1） | 结构对齐 |
| 遍历 | 固定 worker 池 + Cond 队列 | **同款**（分类嵌入流水线） | ✅ 直接借鉴 |
| 进度 | 不确定型（单行） | 不确定型（扫描）/ 确定型（整理） | ✅ 概念复用 |
| 大小/耗时 | `st_size` + 1024 + 精度规则 | **同口径**（D-9） | ✅ 直接复用 |
| 链接/环 | 不跟随 + junction 特判 | **同口径**（D-11） | ✅ 直接复用 |
| 净化 | 剥离控制字符 | **同口径**（SEC-5） | ✅ 直接复用 |
| 持久化 | 无 | SQLite | 新增（fast-stat 无参考） |
| 状态机 | 无 | 任务状态机 | 新增 |
| 测试注入 | `fsx` 抽象 | 需 FS 抽象 + Clock 注入 | ✅ 思路复用 |

**重要差异**：file-pilot 是**长驻应用 + 有状态持久层**，因此必须额外处理：任务并发互斥、异常恢复、DB 迁移与保留、事件节流生命周期。

---

## 7. 待裁决问题清单（P0 / P1 / P2）

> **裁决状态更新（2026-10-15，Phase 0 实施时）**：
> - 🔴 **P0-1 ~ P0-4 已全部裁决**：P0-1 批准两项依赖并已写入 `dependencies.md` §2.2；P0-2 采纳方案①（预检 + `os.Rename` + 紧邻复检）；P0-3 实测**纯标准库可取**，不引入 `x/sys/windows`；P0-4 采纳口径 B，**并已按需求方授权修订 `requirements.md` §5.2 / §5.6（修订号 R3）**。裁决详情与验证证据见 [`development/00-phase-0-prerequisites.md`](development/00-phase-0-prerequisites.md)。
> - 🟡 **P1-8 / P1-9 已关闭**：Wails 锁定 `v2.16.0`（CLI 同版本）、npm 依赖已记录到 `projects/file-pilot/README.md`。
> - 其余 P1/P2 项按原建议进入对应阶段处理；待需求方确认项 **C-1（规范 Code Review）与 C-4（体验验证）尚未关闭**（C-2、C-3 已于 2026-10-15 关闭），见 Phase 0 文档 §7。
>
> **裁决状态更新（2026-10-15，Phase 1 检查点 A 实施时）**：
> - ✅ **P2-8（SQLite 运行参数）已裁决**：采纳 §3.5 全套（WAL + busy_timeout + foreign_keys + 单连接串行）。
> - ✅ **新增裁决 7 项**：`StartScan(rootPath, ScanConfig)`（修订号 R4）、隐藏/系统目录同开关不下潜、扫描内存仅保留批量窗口、Phase 1 分两次交付（先后端后前端）、进度 `rate` 取全程平均、前端引入 `vue-router`、SQLite 全套运行参数。记录见 [`development/01-phase-1-scan.md`](development/01-phase-1-scan.md) §6。

### 🔴 P0 — 阻塞项（✅ 2026-10-15 已全部裁决）

| # | 问题 | 建议 |
|---|---|---|
| P0-1 | **依赖白名单**：`github.com/wailsapp/wails/v2` 与 `modernc.org/sqlite` 不在 `dependencies.md`；需先更新白名单并经 Code Review | 更新 `dependencies.md` 新增 file-pilot 项目级批准记录（含传递依赖），再 `go get` |
| P0-2 | **「绝不覆盖」保障方式**（`os.Rename` 在 Windows 会覆盖，已核实） | 方案①：存在性前置检查 + `os.Rename`（长路径友好，TOCTOU 极低）；方案②：`syscall.MoveFile`（不替换，但需自处理长路径）。**建议①+紧邻复检** |
| P0-3 | **Windows 隐藏/系统属性与创建时间采集的技术路径**：标准库 `syscall` 是否够用？若不够则需 `golang.org/x/sys/windows`（**会扩大依赖白名单**） | Phase 0 先做最小验证；优先只用标准库 `syscall` |
| P0-4 | **分类算法口径**（§2.1 矛盾） | 采纳**口径 B（语义优先 + 跨类别总分）**，并同步修订 `requirements.md` §5.2 措辞 |

### 🟡 P1 — 重要（影响实现正确性）

| # | 问题 | 建议 |
|---|---|---|
| P1-1 | 撤销任务在 `tasks` 的建模（`type=undo` vs `organize` + `parent_task_id`） | 新增 `tasks.parent_task_id`，撤销用 `organize` 类型（贴合 §6.1） |
| P1-2 | 「最近一次整理任务」精确选取（仅 `completed`？含 `canceled`？如何处理已 `undone` / 零移动） | 最近一个 `undone=0` 且存在 `success` move 的 `organize` 任务；无则 `ErrNothingToUndo` |
| P1-3 | 目标目录名 `FilePilot整理`：固定常量？可配置？中文编码/长路径风险 | v1 固定常量（`const targetDirName = "FilePilot整理"`），写入 §13/常量单点 |
| P1-4 | 「系统文件」是否包含系统目录（如 `Windows/`） | 按字面仅按属性过滤文件；系统目录交给黑名单 |
| P1-5 | 路径规则匹配基准（绝对路径 vs 相对扫描根） | 建议**绝对路径**（含扫描根之外的上层目录名也可命中）——需确认是否符合直觉 |
| P1-6 | Migration 工具（自实现步进器 vs `golang-migrate`） | **自实现最小版本化步进器**（避免新增依赖） |
| P1-7 | 时间存储格式（TEXT ISO8601 vs INTEGER 毫秒） | 统一 **UTC 毫秒 INTEGER**（排序/计算简单，展示再格式化） |
| P1-8 | Wails v2 具体版本锁定 + wails CLI 前置环境 | 锁定具体 tag；README 写明 WebView2 与 wails CLI 前置 |
| P1-9 | 前端构建产物与 Go 侧打包（Wails embed）流程；npm 依赖不受白名单约束 | 记录 npm 依赖到项目 README |
| P1-10 | 「已移动数据量」数据来源（operations 加 `size_bytes` vs 回查 files） | operations 增列 `size_bytes` |

### 🟢 P2 — 细节（可先按建议实现）

| # | 问题 | 建议 |
|---|---|---|
| P2-1 | 关键词多命中时「原因」的确定性 | 见 §2.1 步骤 6 |
| P2-2 | 速率显示精度 | 取整 `files/s` |
| P2-3 | 目录计数口径 | 仅成功完整读取的目录 |
| P2-4 | 扫描是否含空目录 | 不含（只索引文件） |
| P2-5 | `files.path` 大小写敏感（Windows 不敏感、Go 字符串敏感） | 存原始大小写；冲突检测用 `Lstat`（由 OS 判定） |
| P2-6 | 事件不持久化时的状态对齐 | 靠查询接口对齐，不重放事件 |
| P2-7 | 撤销是否校验内容被外部修改 | 仅位置回移（不校验内容） |
| P2-8 | SQLite 运行参数（WAL/busy_timeout/foreign_keys） | 见 §3.5 |
| P2-9 | 详情是否支持「全部类别」视图 | 支持 `category=""` |
| P2-10 | `settings` JSON 结构版本兼容 | 加 `version` 字段或容错解析 |

> 另有 `requirements.md` §十五 的 **R-1 ~ R-9** 仍待复核（音频/压缩包兜底、创建时间、并发动态调整、保留 20 次、1GiB 阈值、500 批量、关键词表、前端测试门禁、撤销范围）。本文与之一致，不重复。

---

## 8. 实施文件清单（预告，供阶段文档展开）

> 尚未创建任何代码文件。以下为 Phase 1–3 的**预期文件清单**，最终以各阶段文档为准。

| 层 | 预期文件 |
|---|---|
| 组合根 | `main.go`、`app.go`、`wails.json`、`go.mod`/`go.sum`、`Makefile`、`README.md` |
| Transport（Wails 绑定） | `internal/app/{app.go,bindings.go,events.go,errors_map.go,lifecycle.go}` |
| 任务层 | `internal/task/{manager.go,state.go,throttle.go}` |
| 扫描层 | `internal/scanner/{scanner.go,queue.go,types.go,skip.go}` |
| 分类层 | `internal/classifier/{classifier.go,rules.go,reason.go}` |
| 整理层 | `internal/organizer/{organizer.go,conflict.go,undo.go}` |
| 存储层 | `internal/storage/{db.go,migrate.go,scans.go,files.go,tasks.go,operations.go,settings.go}` |
| 基础设施 | `internal/fsx/{fs.go,stats_windows.go,stats_other.go}`（平台属性采集，文件名待定） |
| 前端 | `frontend/src/{views,components,stores,api}`（Vue3 + TS + Vite + Tailwind） |
| 测试 | 各包 `*_test.go`（表格驱动 + `t.TempDir` 集成） |

**明确不改动**：`AGENTS.md`、`docs/development/` 其他文件、其他 projects（除非 P0-1 裁决后**单独**更新 `dependencies.md`）。

---

## 9. 开发阶段文档索引

为支持「分阶段开发」，已在本目录下建立 `development/`（自定义目录名），首批文档：

| 文档 | 内容 |
|---|---|
| [`development/README.md`](development/README.md) | 阶段路线图、依赖关系、使用方式、状态跟踪 |
| [`development/00-phase-0-prerequisites.md`](development/00-phase-0-prerequisites.md) | Phase 0 前置：依赖白名单、模块骨架、技术验证 |
| [`development/01-phase-1-scan.md`](development/01-phase-1-scan.md) | Phase 1 扫描 |
| [`development/02-phase-2-classification.md`](development/02-phase-2-classification.md) | Phase 2 分类 |
| [`development/03-phase-3-organize-undo.md`](development/03-phase-3-organize-undo.md) | Phase 3 整理与撤销 |
| [`development/04-phase-4-enhancement-backlog.md`](development/04-phase-4-enhancement-backlog.md) | Phase 4 增强（非 v1） |
| [`development/contracts.md`](development/contracts.md) | 跨阶段契约（Go/Wails/事件/数据/错误） |
| [`development/test-and-acceptance.md`](development/test-and-acceptance.md) | 测试矩阵与验收清单 |

---

## 10. 我对本任务的自我约束（执行阶段承诺）

1. **不写任何代码**，直到 §7 P0 全部裁决 + 需求方明确确认方案。
2. 不擅自扩大需求：v1 不实现暂停、批量撤销、自动整理、重复检测、规则 UI 编辑、网络能力（§2.2 / §70）。
3. 不新增依赖，直到 `dependencies.md` 按裁决更新（`docs/README.md` §3 流程）。
4. 不修改 `requirements.md` 与任何已确认文档，除非需求方明确要求（本文发现的矛盾将**先请示**再改）。
5. 所有统计/进度/撤销口径一旦裁决，将写入项目 README 与契约文档，避免歧义。
6. 完成后如实报告：已执行的验证、未验证项与风险，绝不伪造。
