# Phase 2 · 智能分类（Classification）

> 目标：把 Phase 1 的「分类占位」替换为完整规则引擎，并交付分类预览、详情分页、逐文件调整。
> 前置：[`01-phase-1-scan.md`](01-phase-1-scan.md) DoD 达成；[`../requirements-analysis.md`](../requirements-analysis.md) §2.1 口径已裁决（P0-4）。
> 依赖文档：[`contracts.md`](contracts.md)、[`test-and-acceptance.md`](test-and-acceptance.md)。
> **不含实现代码**。

---

## 1. 范围

| 包含 | 不包含 |
|---|---|
| FR-6 分类引擎（扩展名/关键词/路径） | 规则 UI 编辑 / 配置文件（不做） |
| §2.1 选类与置信度算法（口径 B） | 新增类别（音频/压缩包等，不做） |
| 分类日志（reason 一致） | 文件内容解析（不做） |
| FR-7 分类汇总 + 详情分页 | 排序切换 / 批量操作（不做） |
| FR-8 逐文件调整（改类别 / 排除 / 恢复） | 规则重跑（不做） |

---

## 2. 前置条件

- 分类算法口径（口径 A/B）已由需求方裁决；若采纳口径 B，`requirements.md` §5.2 已同步修订。
- Phase 1 已能稳定产出 `files` 记录与 `reason` 字段（占位）。

---

## 3. 任务分解

### T2.1 规则表（代码内常量，单点定义）

- 扩展名表（图片/视频/文档；财务/工作/学习**无扩展名映射**）。
- 关键词表：财务 `发票 账单 invoice 工资 银行 支付 报销 流水 税`；工作 `合同 项目 方案 报告 会议 简历 kpi okr`；学习 `课程 教程 笔记 电子书 习题 论文`。
- 路径表：`发票 报销`→财务；`合同 项目文档`→工作；`课程 教程 电子书`→学习。
- 规则表集中定义于 `classifier` 单点；后续微调属参数变更，须同步修订 `requirements.md`。

### T2.2 分类核心（`internal/classifier`，纯函数）

- 归一化：文件名（不含扩展名）与路径统一小写后子串匹配。
- 计分：扩展名 +60（唯一类别）；关键词每命中 +35、同类封顶 +70；路径每条 +20、同类封顶 +20。
- 选类（口径 B）：在命中类别中按优先级 `财务 > 工作 > 学习 > 视频 > 图片 > 文档 > 其他` 取最高；零命中 → `其他`。
- 置信度：跨类别命中项总分，封顶 100；零命中 0。
- 原因（确定性）：决定该类别的命中项中分值最高者；并列按「扩展名 > 关键词 > 路径」再按关键词表声明顺序。
- **纯函数**：不读文件系统、不感知并发；输入 FileMeta，输出（category, confidence, reason）。
- **测试**：表格驱动，覆盖 §2.1 全部校验用例 + 边界（多关键词、路径+关键词、零命中、封顶、大小写、中文、无扩展名）。

### T2.3 接入扫描流水线

- Phase 1 的占位分类被完整替换；确保 `files.source='rule'`、`reason` 一致。
- 分类日志复用**同一 reason 字符串**（FR-4 与 FR-7 一致，§5.6）。

### T2.4 分类汇总（FR-7）

- `GetCategorySummary(scanID)` → `{category,count}`，固定 7 类顺序（图片/视频/文档/财务/工作/学习/其他）。
- 聚合查询 `GROUP BY category`，走 `(scan_id, category, id)` 索引；不做内存全量扫描。

### T2.5 分类详情与分页（FR-7）

- `ListFiles(scanID, category, page, pageSize)`：默认 100/页；`ORDER BY id` 稳定分页；`category=""` = 全部（P2-9）。
- 每行字段：文件名/路径/大小/目标类别/置信度/原因。
- 禁止全量渲染 DOM。

### T2.6 逐文件调整（FR-8）

- `SetFilePlan(fileID, category, excluded)`：即时落库。
  - 改类别：`category` + `source='user'` + `confidence=NULL` + `reason='用户手动指定'`。
  - 排除：`excluded=1`；恢复：`excluded=0`。
- 校验：类别 ∈ 7 类（否则 `ErrCategoryInvalid`）；文件属于该扫描（否则 `ErrFileNotInScan`）。
- 立即影响后续整理计划（Phase 3 读取最新 `files` 状态）。

### T2.7 前端页面 P3（分类部分）

- 汇总视图 → 点击类别 → 详情分页列表 → 逐文件改类别/排除/恢复。
- 置信度列：规则显示 `NN%`；用户手动显示 `手动`。
- 空态：无分类结果文案。

---

## 4. 交付物

`internal/classifier/{classifier.go,rules.go,reason.go}` + 测试；`internal/storage`（files 分页/计数/更新计划）；`internal/app`（`GetCategorySummary`、`ListFiles`、`SetFilePlan`）；前端 P3 分类部分。

---

## 5. Definition of Done

```text
✓ §2.1 校验用例作为表格驱动测试全部通过（含口径定义边界）
✓ 分类原因在「分类日志」与「详情列表」完全一致
✓ 汇总计数与详情分页结果一致（交叉校验）
✓ SetFilePlan 即时落库且整理计划随之更新（Phase 3 集成验证）
✓ gofmt / go vet / go test / go test -race（如涉及并发）通过
✓ 覆盖率：classifier 作为核心逻辑 ≥70%（纯函数建议更高）
✓ git diff 自检干净；如实报告
```

---

## 6. 风险与待裁决

- 关键词/路径表为初始值（R-7），可能需需求方补充。
- 「原因」确定规则（并列时）按 §2.1 定义；若需求方另有偏好需先改文档。
- 前端人工交互（P3 结果页：汇总→详情→改类/排除）需真实图形窗口验证，本环境无法驱动 Wails 窗口。

### Phase 2 编码裁决记录（2026-10-15，Phase 2 编码前）

Phase 2 编码前确定并采纳以下 4 项设计裁决：

| # | 事项 | 裁决 | 依据 / 影响 |
|---|---|---|---|
| D-C1 | 结果页缺 scanID 通道（终态后 `GetActiveTask` 返回 null，`task:status` 原载荷不含 scanID） | **`task:status` 载荷新增 `scan_id`**（累加、非破坏）；已同步 `requirements.md` §8.2 与 `contracts.md` §6 | 结果页（P3）需 scanID 查询汇总/详情；从终态事件直接取得，刷新场景亦覆盖，优于前端经 `GetActiveTask` 间接捕获 |
| D-C2 | `SetFilePlan(fileID, category, excluded)` 签名不含 scanID，无法校验「文件属于该扫描」 | **仅校验类别 ∈ 7 类（`ErrCategoryInvalid`）与 fileID 存在（`ErrFileNotInScan`）**；归属由结果页 UI 保证；已同步 `contracts.md` §5 | 单用户本地应用信任模型；不改公共签名（AGENTS.md §2.3 最小变更）。`ErrFileNotInScan` 落地语义为「目标文件不存在」 |
| D-C3 | §5.5 路径规则匹配基准（「完整路径」含文件名？） | **对文件所在目录（不含文件名）小写子串匹配**（`parentPath`，同时识别 `/` 与 `\`） | §5.2 校验示例 `合同.pdf → 工作 95%` 要求文件名不产生路径命中，否则会经关键词与路径双重命中得 100；§5.5 规则名为「所在路径」 |
| D-C4 | `GetCategorySummary` 是否返回 0 计数类别 | **返回全部 7 类（按 `classifier.Categories` 顺序，含 0 计数）** | 前端汇总视图布局稳定；与 §8.1「固定 7 类顺序」一致 |

> 附（非新增裁决）：排除/恢复（category 不变）只改 `excluded`，保留原 `source`/`confidence`/`reason`；仅改类别时才写入 `source=user`/`confidence=NULL`/`reason=用户手动指定`（排除一个规则命中文件不应将其改写为「用户手动指定」）。
