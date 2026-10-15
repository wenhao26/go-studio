# file-pilot 跨阶段契约（Contracts）

> **文档性质**：跨阶段共用的**接口/数据/事件/错误契约**，是实现与测试的唯一对齐基准。**不含实现代码**。
> **权威来源**：[`../requirements.md`](../requirements.md)（需求）+ [`../requirements-analysis.md`](../requirements-analysis.md)（实施展开）。
> **说明**：本文若与 `requirements.md` 冲突，以 `requirements.md` 为准，并先修订文档再改实现。

---

## 1. 命名与序列化约定

| 项 | 约定 |
|---|---|
| Go 包名 | 小写单词，无下划线：`app`、`task`、`scanner`、`classifier`、`organizer`、`storage`、`fsx` |
| 跨 Wails 边界字段 | **`snake_case`**（D-12），不套用 `api.md` 的 `{code,message,data,trace_id}` |
| 缩略词 | 遵循 `go.md` §2.2：`ID`、`URL`、`DB` 等全大写或全小写 |
| 类别标识 | 中文字符串常量：`图片/视频/文档/财务/工作/学习/其他`（即目录名） |
| 时间 | 统一 **UTC 毫秒整数**（P1-7 建议），展示层再格式化 |
| 大小 | 一律 `int64`，单位字节；展示层格式化（§2.5） |
| 路径 | 存绝对路径；跨边界传递原样字符串 |

---

## 2. Go 内部分层与依赖方向

```text
app        （Wails 绑定 = Transport/Handler + 事件桥接）
  ↓
task        （任务管理器：状态机、单活动约束、取消、节流器）
  ↓
scanner / classifier / organizer   （Service 层）
  ↓
storage     （Repository：SQLite）
  ↓
fsx         （Infrastructure：文件系统抽象 + 平台属性）
```

约束（对齐 `architecture.md`）：

- 依赖**单向**、禁止反向与跨层。
- 接口由**消费方定义**，构造函数显式注入（禁止全局变量）。
- `scanner/classifier/organizer` 不 import `app`；`storage` 不 import 上层。
- 文件系统操作经 `fsx` 小接口抽象，便于注入故障测试。

---

## 3. 核心领域类型（字段级）

> 以下为类型语义描述（非代码）。字段名即 Wails 边界 JSON 名。

### 3.1 ScanConfig（扫描配置）

| 字段 | 类型 | 默认 | 约束 |
|---|---|---|---|
| `workers` | int | `clamp(NumCPU,1,64)` | `[1,64]`，非法报错 |
| `scan_hidden` | bool | `false` | |
| `scan_system` | bool | `false` | |
| `skip_large` | bool | `false` | `>1GiB` 跳过 |

### 3.2 Category（类别）

两个顺序**不同，实现时不可混用**：

- **枚举 / 展示 / 目录顺序**（§5.1）：`图片 > 视频 > 文档 > 财务 > 工作 > 学习 > 其他`
- **选类优先级（口径 B，§5.2）**：`财务 > 工作 > 学习 > 视频 > 图片 > 文档 > 其他`（语义类优先于媒体类）——需在 classifier 中显式定义 `categoryPriority` 常量，**禁止**用枚举声明顺序代替。

### 3.3 TaskStatus

| 值 | 含义 | 终态 |
|---|---|---|
| `pending` | 等待 | 否 |
| `running` | 执行中 | 否 |
| `completed` | 完成 | 是 |
| `failed` | 失败 | 是 |
| `canceled` | 取消 | 是 |
| `paused` | **预留，v1 不可达** | — |

### 3.4 TaskType

`scan`（扫描，含分类流水线） / `organize`（整理，含撤销）。

### 3.5 FileRow（详情列表一行）

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int64 | |
| `name` | string | |
| `path` | string | |
| `size_bytes` | int64 | |
| `category` | string | |
| `confidence` | int \| null | null 表示用户手动 |
| `source` | `rule` \| `user` | |
| `reason` | string | 展示短语 |
| `excluded` | bool | |

### 3.6 ScanSummary（FR-5）

`file_count`、`dir_count`、`total_size_bytes`、`skipped_count`、`duration_ms`、`avg_speed`、`max_workers`、`status`、`organizable_count`（Phase 3 / D-D9：参与整理的文件数 = `excluded=0`，供 FR-9.1 二次确认展示「即将整理: 文件数量 N」；`file_count - organizable_count` 即被排除的文件数）。

### 3.7 OrganizeReport（FR-10）

`task_id`、`status`、`success_count`、`failed_count`、`duration_ms`、`moved_size_bytes`、`failures[]`（`path`、`reason`）。

### 3.8 进度载荷（按任务类型取舍）

扫描（不确定型）：`scanned`、`rate`、`current_dir`、`elapsed_ms`、`skipped`。
整理/撤销（确定型）：`done`、`total`、`percent`、`current_file`、`target_category`、`elapsed_ms`。

### 3.9 日志条目

`{ ts, kind: scan|classify|system, text }`；`kind=system` 不采样。

### 3.10 RecoveryNotice（FR-12 启动恢复提示）

`recovered_tasks`、`recovered_scans`：本次启动把遗留的 `pending`/`running` 任务与 `running` 扫描行修正为 `failed` 的数量（原因 `应用中断`）。**无恢复时 `GetRecoveryNotice` 返回 `null`**，前端据此不展示提示。

---

## 4. 哨兵错误（Sentinel Errors）

| 哨兵 | Go 侧错误消息（前端匹配键） | 触发 | 前端文案 |
|---|---|---|---|
| `ErrTaskRunning` | `task already running` | 已有 running 任务 | 当前有任务进行中 |
| `ErrInvalidWorkers` | `workers must be between 1 and 64` | 并发数非法 | 并发数需在 1–64 之间 |
| `ErrDirNotFound` | `directory not found` | 目录不存在 | 所选目录不可用 |
| `ErrDirNotDirectory` | `path is not a directory` | 路径非目录 | 所选目录不可用 |
| `ErrConfigInvalid` | `invalid scan config` | 配置非法 | 请检查扫描配置 |
| `ErrDirectoryPickCanceled` | `directory pick canceled` | 用户取消目录选择 | 目录选择失败 / 用户取消操作 |
| `ErrCategoryInvalid` | `invalid category`（Phase 2） | 类别非法 | 类别不合法 |
| `ErrFileNotInScan` | `file not in scan`（Phase 2） | 文件不属于该扫描 | （通用文案 + 诊断日志） |
| `ErrScanIncomplete` | `scan incomplete`（Phase 3） | 部分结果不可整理 | 扫描未完成，请重新扫描 |
| `ErrTargetDirCreate` | `create target dir failed`（Phase 3） | 目标目录创建失败 | 无法创建整理目录 |
| `ErrNothingToUndo` | `nothing to undo`（Phase 3） | 无可撤销整理 | 没有可撤销的整理 |
| `ErrAlreadyUndone` | `already undone`（Phase 3） | 已撤销 | 该整理已撤销 |

约定：错误一律 `fmt.Errorf("...: %w", err)` 包装，判别用 `errors.Is/As`，业务代码禁止 `panic`（`AGENTS.md` §9.5/§65）。

**Wails 边界错误形态**：绑定方法返回给前端的错误，凡命中上表哨兵的一律返回**裸哨兵**（其 `message` 恰为匹配键），供前端按 `error.message` 精确匹配后展示中文文案；内部包装过的完整错误链只写诊断日志，不上边界。未命中匹配键的错误（含 Phase 1 之外的一切错误）前端展示通用文案并记诊断日志。

---

## 5. Wails 绑定方法契约（v1 全集，15 个）

| 方法 | 入参 | 返回 | 关键约束 |
|---|---|---|---|
| `ChooseDirectory` | — | `string, error` | 取消返回 `ErrDirectoryPickCanceled` |
| `GetSettings` | — | `ScanConfig + recent_dirs, error` | |
| `SaveSettings` | `ScanConfig` | `error` | |
| `RemoveRecentDir` | `dir` | `error` | 前端不可写 `recent_dirs`（§3.1）；此绑定是唯一移除入口，幂等：不存在时返回 nil |
| `StartScan` | `rootPath, ScanConfig` | `int64(taskID), error` | 校验配置/目录；单活动约束（R4：根目录显式传参） |
| `CancelTask` | `taskID` | `error` | 协作式取消 |
| `GetActiveTask` | — | `TaskStatus \| null, error` | 恢复 UI |
| `GetRecoveryNotice` | — | `RecoveryNotice \| null, error` | FR-12 异常恢复提示；无恢复返回 null |
| `GetScanSummary` | `scanID` | `ScanSummary, error` | |
| `GetCategorySummary` | `scanID` | `[]{category,count}, error` | 固定 7 类顺序 |
| `ListFiles` | `scanID, category, page, pageSize` | `{items,page,page_size,total_count,total_page}, error` | 默认 100/页；`category=""` = 全部（P2-9） |
| `SetFilePlan` | `fileID, category, excluded` | `error` | 校验类别 ∈ 7 类（`ErrCategoryInvalid`）与 fileID 存在（`ErrFileNotInScan`）；归属由 UI 保证（D-C2） |
| `StartOrganize` | `scanID` | `int64(taskID), error` | **前端二次确认后**调用；含预检 |
| `GetOrganizeReport` | `taskID` | `OrganizeReport, error` | |
| `UndoLastOrganize` | `scanID` | `int64(taskID), error` | 已撤销/无可撤销返回哨兵错误 |

> 注：`GetSettings`/`SaveSettings` 合并计 2 个方法，合计 15 个绑定入口（`requirements.md` §8.1 表述为「全集」）。以 `requirements.md` 为准。

---

## 6. 事件契约（Go → JS，3 个）

| 事件 | 载荷 | 触发 | 节流 |
|---|---|---|---|
| `task:status` | `{task_id, type, status, scan_id, error}` | 每次合法转换 | 不节流 |
| `task:progress` | `{task_id, ...进度字段, elapsed_ms}` | 周期 | ≥200ms |
| `task:log` | `{task_id, entries[]}` | 周期 | 200ms 窗口，≤20 条，超采样丢弃 + `dropped` 披露 |

约定：前端**环形缓冲 500 行**；分类原因**全量存库**；事件不持久化，UI 恢复靠查询接口。

---

## 7. 存储层接口（消费方定义，语义）

| 接口 | 方法（语义） | 主要消费方 |
|---|---|---|
| ScanStore | 建扫描行 / 更新扫描终态与统计 / 读扫描统计 | task/scanner |
| FileStore | 批量插入 files / 按类别计数 / 分页查询 / 更新单文件计划（category/excluded/source/confidence/reason） | scanner/app |
| TaskStore | 建任务 / 更新状态 / 查活动任务 / 启动恢复（running→failed） / 标记 undone / 设置 parent_task_id / 查最近可撤销的整理（P1-2） | task/app |
| OperationStore | 追加一条 move/undo 记录 / 按任务查成功 move / 按任务聚合成功·失败·已移动数据量 / 列失败明细 | organizer/app |
| SettingsStore | 读/写 scan_config 与 recent_dirs | app |

约束：全部参数化；禁 `SELECT *`；files 每 500 条一事务；operations 逐条写；启动清理 >20 次扫描级联数据。

---

## 8. 数据模型

列级 schema 见 [`../requirements-analysis.md`](../requirements-analysis.md) §3.1；索引见 §3.2；事务/保留见 §3.3；migration 见 §3.4。

---

## 9. 契约变更规则

1. 任何契约字段/语义变更，先改 `requirements.md` 或本文，再改代码。
2. 破坏性变更需在 [`README.md`](README.md) 的「变更记录」登记。
3. 前端文案映射表与 §4 哨兵错误**必须同步**。
