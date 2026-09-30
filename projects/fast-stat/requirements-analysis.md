# fast-stat 需求细化与理解分析

> **文档性质**：阶段一（需求澄清 + 方案设计）产出物，**不含任何业务代码**
> **输入**：[`requirements.md`](requirements.md)、`AGENTS.md`、`docs/development/*`、[`PROJECT_ALIGNMENT_REPORT.md`](../../PROJECT_ALIGNMENT_REPORT.md)
> **生成时间**：2026-09-30
> **状态**：⏳ **等待需求方确认**（P0 阻塞项未裁决前不进入编码阶段）

---

## 0. 结论摘要（先看这里）

PRD 的方向、目标与工程边界是**清晰的、可实现的**，但有 **7 项必须在写代码前裁决的阻塞问题**、**4 处 PRD 内部矛盾/缺陷**、以及 **1 项与全局规范的硬冲突（依赖白名单）**。

最关键的三个认知：

1. **`ETA`（预计剩余时间）与"百分比进度"在技术上是不可得的** —— 目录遍历前无法预知文件总数，{PRD §二.2} 的该项要求需要改写为"不确定型进度"。这不是实现难度问题，而是信息论问题。
2. **"跳过错误数"与"必须且仅包含三项指标"直接矛盾** —— {PRD §二.4} 与 {PRD §四.2} 互相否定，必须二选一。
3. **总大小的语义未定义（apparent size vs disk usage）** —— 二者在稀疏文件/压缩文件系统/APFS 克隆下可相差数倍，将直接决定统计结果的正确性，**必须先定义再实现**。

---

## 1. 需求逐条细化（把 PRD 转成可执行定义）

### 1.1 项目定位

| 维度 | 细化后的理解 |
|---|---|
| 本质 | 一个**单机、无守护进程、一次性执行**的 CLI 工具（非服务、非 HTTP API） |
| 输入 | 一个本地目录路径 + CLI 参数 |
| 输出 | ① 终端结构化报告；② 可选 JSON 文件；③ 过程进度（终端） |
| 生命周期 | `main` 启动 → 解析参数 → 并发遍历 → 渲染报告 → 退出（含退出码） |
| 依赖形态 | 无数据库、无缓存、无 MQ、无网络调用（**`database.md`/`config.md` 大部分条款不适用**） |

> 这一定位是后续所有规范适用性判断的基准：**它是一个 "CLI 程序"，不是 "Web 服务"**。

### 1.2 核心指标的精确定义（需确认项已标注）

#### 指标 1：扫描次数 (Scan Count)

PRD 原文："系统实际执行的目录节点访问总次数（含递归子目录）"，示例 label 为 `Directories Visited`。

细化后的候选定义（**互斥，需裁决**）：

| 方案 | 定义 | 与示例的匹配度 |
|---|---|---|
| **A（推荐）** | 成功 `ReadDir` 的**目录节点数**（含根目录）。示例 12,450 个目录 / 148,203 个文件 → 平均每目录 11.9 个文件，**数值关系合理** ✅ | 与 label `Directories Visited` 一致 |
| B | 访问的**目录条目(dirent)总数** = 目录数 + 文件数 + 其他条目 | 与 label 不符 ❌ |
| C | 所有 `stat` 系统调用次数（含文件） | 与 label 不符 ❌ |

**待确认**：根目录是否计入？符号链接目录是否计数（但不下潜）？因权限被拒的目录是否计入？是否含重试次数？
**建议**：方案 A；根目录计入；符号链接目录**计数但不下潜**（需与 symlink 策略一致）；被拒目录**不入 Scan Count、入 Skipped Errors**；无重试。

#### 指标 2：文件个数 (File Count)

PRD 原文："目标路径下包含的全部文件总数（不含文件夹本身）"。

未定义且**会显著改变结果**的四个子问题：

| 子问题 | 影响 | 建议默认值 |
|---|---|---|
| 指向文件的**符号链接**是否计入 | 结果差异可能巨大 | **不计入**（单独计入 symlink 计数，但 PRD 明确"仅三项指标"→ 不展示） |
| **硬链接**是否按 inode 去重 | `du` 默认去重；不去重会重复计大小 | **不去重**（去重需维护 inode 集合，与"低内存流式"目标冲突）⚠️ 与 `du` 结果不可比 |
| **特殊文件**（FIFO/socket/device） | 通常应计入"文件"但大小为 0 | 计入（按 `Mode().IsRegular()` 之外另计）→ **需确认是否计为"文件"** |
| **隐藏文件**（`.` 开头） | 无理由排除 | 计入 |

#### 指标 3：路径总大小 (Total Size)

这是**风险最高**的指标。未定义项：

| 未定义项 | 差异幅度 | 说明 |
|---|---|---|
| **apparent size vs disk usage** ⚠️ **必须裁决** | 稀疏文件/APFS 克隆/压缩 FS 下可达数倍 | `apparent` = `st_size` 累加（逻辑大小）；`disk usage` = `st_blocks × 512`（`du` 默认口径）。PRD 示例 `4.82 GB` 未区分 |
| **单位基数** 1024 vs 1000 ⚠️ **必须裁决** | 约 7.4%（GB 量级） | `4.82 GiB` vs `4.82 GB`。示例写作 `GB`，但习惯上多数工具用 1024 |
| 目录自身元数据大小 | 通常可忽略 | 建议**不计入**（与 `du -sh` 语义差异需知晓） |
| 符号链接自身大小 | 小 | 建议**不计入**（与"计数不含 symlink"一致） |
| 无权限文件的大小 | 该文件唯一已知为 0 | 无法获取 → 计 0 并计入 Skipped Errors |
| 大小溢出 | — | ⚠️ 必须用 **`int64`/`uint64`**，禁止 `int`（Windows 32 位 `int` 为 32 位；虽然目标平台均 64 位，仍应显式声明） |

**单位转换规则的细化**（PRD §二.4）：

- 阈值比较应在**字节**上做，而非在已换算值上做（避免 `1023.9 KB` 被显示成 `1.00 KB` 的边界抖动）。
- `Bytes` 档位是否加千分位？建议 `1,024 Bytes`。
- 需明确 `Bytes` 是否加 "Bytes" 后缀、MB/KB 是否保留两位小数（PRD 仅对 GB/MB 举例两位，KB/Bytes 未说明）→ **建议统一两位小数（Bytes 除外，整数）**。

#### 指标 4（PRD 未列入但被要求展示）：跳过错误数 (Skipped Errors)

🔴 **PRD 内部矛盾（详见 §3.1）**：§二.4 说"必须且**仅**包含"三项，§四.2 又要求在报告中展示"跳过错误数"，而 §三 的排版示例**没有这一行**。

### 1.3 性能与并发（PRD 无量化指标）

PRD §二.1 宣称"极致性能"，但**没有任何可验证的数字**。按 `AGENTS.md` §37（不得伪造验证）与 §94（任务完成定义），"高性能"必须转成可验收指标，否则无法判定完成。

**建议补充的验收标准（需你确认或修订数值）**：

| 指标 | 建议目标 | 验证方式 |
|---|---|---|
| 吞吐 | 100 万文件 / 50 GB 数据集，NVMe SSD 上 ≤ 60s | `go test -bench` + 手工基准，记录到文档 |
| 峰值内存 | ≤ 256 MB，且**不随文件数线性增长** | 扫描 10 万 vs 100 万文件对比 RSS |
| CPU 利用率 | 多核并行（不低于单核版本的 N 倍吞吐，N≥4） | 基准对比 |
| 启动开销 | 小目录（<1000 文件）总耗时 ≤ 200ms | 基准 |

**并发模型的关键技术约束（设计层面，必须提前确定）**：

- 🔴 **`errgroup.SetLimit` 递归自死锁陷阱**：若"持有一个 worker slot 的 goroutine 再去 `g.Go()` 创建子任务"，当 slot 耗尽时 `Go()` 会阻塞，而持有 slot 的 goroutine 正等待它 → **必然死锁**。必须改用「固定 worker 池 + 工作队列 channel」或「`semaphore.Acquire` 在派发前完成」。
- 无界 `go func()` per directory 在百万目录下会**耗尽内存/线程**。按 `AGENTS.md` §17，goroutine 必须有界且可控。
- 机械硬盘（HDD）上高并发会因磁头寻道**降低**吞吐 → 是否允许配置并发度（`--threads`）？PRD 未提及。
- 计数器聚合：热路径用 `atomic.Int64`（Go 1.19+）避免锁竞争，配合 `go test -race` 验证。

### 1.4 进度展示（PRD 存在技术不可行项）

PRD §二.2 要求展示：速率 ✅、已耗时 ✅、**ETA** 🔴、**当前目录路径** ✅。

🔴 **ETA 不可得**：ETA = 剩余工作量 / 速率。但"剩余工作量"（总文件数/总字节数）**在遍历完成前无法获知**——这正是 `du` 无法显示百分比的原因。若强行实现只能用"上一级目录的历史平均"等启发式，结果会严重误导。

**建议方案（需裁决）**：改为**不确定型进度**（indeterminate progress）：

```text
⠹ Scanning...  148,203 files | 4.82 GB | 11,940 files/s | 412.6 MB/s | 12.4s elapsed
  └─ current: /var/log/journal/8f2c...
```

即：**去掉百分比与 ETA，改为"已扫描量 + 瞬时速率 + 已耗时 + 当前路径"**。若你坚持要 ETA，请指定其计算口径（例如"仅在 `--max-depth` 已知且已扫描完第一层后估算"），我会按你的口径实现并在 UI 上标注为"估算"。

**其他未定义项**：

- 进度渲染形态：**单行覆盖（`\r`）** 还是 **全屏 TUI（alt-screen，`bubbletea`）**？二者依赖与终端要求差异巨大。
- **非 TTY 环境**（重定向、CI、`| grep`）必须降级：不渲染进度/颜色/emoji，否则会污染输出。
- **stdout/stderr 分流**：🔴 若走 `--format json`，进度**必须**输出到 `stderr`，否则 JSON 不可解析。PRD 未定义该分流契约。
- 刷新频率未定义（建议 100–200ms，避免高频重绘吃 CPU）。
- 文件名可能含**控制字符/ANSI 转义序列** → 🔴 直接打印会造成**终端注入**（可伪造输出、篡改标题栏）。必须净化（见 §4.3 安全）。

### 1.5 终端排版（PRD 示例经验证含缺陷）

我用 Unicode East Asian Width 精确测算了 PRD §三 示例的显示宽度，结论如下（**证据见下表**）：

| 行 | 字符数 | 显示宽度 | 内容头部 |
|---|---|---|---|
| 61 | 58 | 58 | `╔═══...═══╗` |
| 62 | 57 | 58 | `║ ... 🚀 FASTSTAT SCAN REPORT ... ║` |
| 63 | 58 | 58 | `╠═══...═══╣` |
| 64 | 58 | 58 | `║ Target Path ...` |
| 65 | 58 | 58 | `║ Status ...` |
| **66** | 58 | 58 | **`╠═══...═══╝`** ⚠️ |
| 67 | 53 | 58 | `║ 📂 扫描次数 ...` |
| 68 | 53 | 58 | `║ 📄 文件个数 ...` |
| 69 | 52 | 58 | `║ 📦 路径总大小 ...` |
| 70 | 58 | 58 | `╚═══...═══╝` |

**发现 1（缺陷）**：第 66 行首字符是 `╠`（U+2560，左三叉连接符），尾字符是 `╝`（U+255D，右下角）——**这不是合法的边框组合**。它既想表达"分隔线继续"又想表达"区块结束"，语义自相矛盾。应为 `╠═══╣`（继续）或 `╚═══╝`（结束）。

**发现 2（实现约束）**：**所有行的显示宽度恰好 = 58**，而字符数在含 emoji/CJK 的行是 52–53。这证明：
> 对齐必须基于 **display width**（东亚宽字符 + emoji 计 2 列），**不能用 `len()` 或字符数**。

→ 这直接产生一个依赖需求：需要 `go-runewidth` 等价能力来裁剪/补齐。**待裁决**：是否接受引入该依赖，或允许"仅 ASCII 内容严格对齐、含中文行不保证"的降级方案。

**发现 3（跨终端风险）**：emoji 宽度在不同终端/字体下可能是 1 或 2 列（`🚀` U+1F680 的 EAW 为 Wide，但部分 Windows 终端渲染为 1 列）→ **含 emoji 的边框在部分终端必然错位**。可选：提供 `--no-emoji`/`--plain` 降级，或在 emoji 后不使用边框对齐。

**发现 4（措辞歧义）**：示例中 `Target Path` / `Status` / `Duration` 属于**报告元信息**，不属于 §二.4 的"统计指标"。PRD 的"必须且仅包含三项"应理解为"**统计指标**仅三项"，而非"报告仅三行"。**需确认此理解**，否则与示例冲突。

---

## 2. CLI 契约细化（PRD 只有 3 个示例，缺口很大）

### 2.1 已给出的调用形式

```bash
faststat .                                          # 位置参数
faststat --path /var/log --verbose                  # 命名参数
faststat --path /data --format json --output result.json
```

### 2.2 必须补齐的契约（均未定义）

| 项 | 缺口 | 建议 |
|---|---|---|
| **二进制名** | PRD 正文用 `faststat`，项目名是 `fast-stat` | `faststat`（无连字符，与示例一致）→ **需确认** |
| **位置参数 vs `--path`** | 二者都指定路径，是否等价？同时出现时谁优先？ | `--path` 优先于位置参数；二者均缺省时 = `.` |
| **无参数行为** | 未定义 | 等同 `faststat .`（PRD 示例 1 的语义） |
| **`--help` / `-h`** | 未定义 | 必须提供（标准约定） |
| **`--version`** | 未定义 | 必须提供；版本号经 `-ldflags -X` 注入 |
| **`--format` 取值** | 仅出现 `json` | `text`(默认) / `json`；非法值 → 用法错误退出码 |
| **`--verbose` 语义** | 完全未定义 | 建议：输出① 错误明细（前 N 条）② 扫描参数的生效值 ③ 计时分解 |
| **`--output`** | 未定义写行为 | 仅在 `--format json` 时有意义？还是文本也可写文件？建议`--output` 通用；写入采用「临时文件 + 原子 rename」避免半截文件 |
| **`--threads`** | 未定义 | 建议提供，默认 `min(NumCPU, 16)`；**需裁决**是否列入 v1 |
| **退出码** | 未定义 | 建议：`0` 成功（含"有跳过错误但扫描完成"）；`1` 致命错误（根路径不存在）；`2` 用法错误 |
| **`--max-depth` / `--exclude` / `--follow-symlinks` / `--no-progress` / `--no-color`** | 未定义 | **按 §70 不过度工程化，v1 默认不实现**，除非你要求 |

### 2.3 JSON 输出 schema（完全未定义）

🔴 **需裁决**：是否套用 `api.md` 的统一响应体 `{"code","message","data","trace_id"}`？

- **若套用**：CLI 工具输出会包含无意义的 `code`/`trace_id`，不利于其它程序对接，但与 `api.md` 一致。
- **若不套用**（我建议）：输出"纯数据"结构，仍沿用 `api.md` 的**字段命名风格**（`snake_case`）。

建议的 schema 草案（**待确认，不是最终契约**）：

```json
{
  "target_path": "/data",
  "status": "completed",
  "duration_seconds": 1.24,
  "statistics": {
    "directories_visited": 12450,
    "total_files": 148203,
    "total_size_bytes": 5175123456,
    "total_size_human": "4.82 GB"
  },
  "errors": {
    "skipped_count": 3,
    "items": []
  },
  "tool": { "name": "faststat", "version": "1.0.0" }
}
```

> 注意：`total_size_bytes` 用**字节整数**，`total_size_human` 仅作展示 —— 避免程序对接方解析人类可读字符串（遵循 `domain.md` §4 的思路：精度用整数，展示才格式化）。

---

## 3. PRD 内部矛盾与缺陷（需裁决，共 4 项）

### 3.1 🔴 "仅三项指标" vs "展示跳过错误数"

| 位置 | 原文 |
|---|---|
| §二.4 | 统计结果必须且**仅**包含以下核心硬性指标：（三项） |
| §四.2 | 必须在日志中记录并在最终报告的**"跳过错误数"**中展示 |
| §三 示例 | **没有**"跳过错误数"这一行 |

**三个要求互相冲突**。必须裁决（三选一）：

- **A（推荐）**：报告展示 **4 个区块**：3 个统计指标 + 1 个"跳过错误数"（错误数不是"统计指标"而是"运行诊断"）。同时修订 §二.4 措辞为"统计指标仅包含以下三项"。
- B：严格三项，错误数只写日志（`--verbose` 才可见）。
- C：错误数作为第 4 个统计指标，修订 §二.4。

### 3.2 🔴 排版示例的边框残缺（§三 第 66 行）

`╠═══...═══╝` 为非法组合（详见 §1.5 发现 1）。

**需裁决**：是**严格复刻**该示例（含缺陷），还是**修正为合法边框**（我建议修正）？严格复刻会产生视觉上的断裂边框。

### 3.3 🟡 "扫描次数" 与 "Directories Visited" 术语不一致

PRD 正文用"扫描次数 (Scan Count)"，示例 label 用 "Directories Visited"。若指标定义为"目录数"，则"扫描次数"这个名字会误导（用户可能理解为"遍历条目数"）。

**建议**：报告中使用 `Directories Visited` 并保留中文"扫描目录数"，或保留"扫描次数"但明确定义。**需确认**。

### 3.4 🟡 `Duration` 未列入指标却出现在报告

`Status : Completed (Duration: 1.24s)` —— 耗时是重要指标（也是"高性能"宣称的证据），应明确其**是否属于承诺输出**及**精度**（建议 2 位小数，秒）。

---

## 4. 与全局规范的适用性分析

### 4.1 适用性矩阵

| 规范 | 适用性 | 说明 / 需裁决点 |
|---|---|---|
| `AGENTS.md` 全部 | ✅ 完全适用 | 尤其：最小修改、`%w`、goroutine 可控、不伪造验证、diff 自检 |
| `go.md` | ✅ 适用 | 命名/Guard Clause/错误链/并发/`go vet`/golangci-lint |
| `architecture.md` §1 | ⚠️ 部分适用 | 分层思想适用，但 **Handler/Repository 的命名与目录布局需映射**（见 §5.1） |
| `architecture.md` §4 依赖注入 | ✅ 适用 | 构造函数注入 + 消费方定义接口（**遍历器依赖 FS 抽象**，见 §5.4） |
| `domain.md` §2 状态机 | ❌ 不适用 | 本工具无业务状态机（`Completed` 不是状态机） |
| `domain.md` §3 分布式 ID | ❌ 不适用 | 无 ID 生成需求 |
| `domain.md` §4 金额 | ⚠️ 部分适用 | 金额规则不适用，但**"整数精度 + 展示层格式化"的思路适用**于字节数 |
| `api.md` 路由/HTTP/幂等 | ❌ 不适用 | 无 HTTP |
| `api.md` 统一响应体 | ⚠️ **需裁决** | JSON 导出是否套用 `{code,message,data,trace_id}`（见 §2.3） |
| `database.md` | ❌ 不适用 | 无 DB / Redis |
| `config.md` | ⚠️ **需裁决** | 是否引入配置文件？若只用 CLI flag，则 viper/启动校验条款**豁免**；`--output` 等参数是否需校验（建议：是） |
| `observability.md` §1 日志 | ⚠️ **需裁决** | CLI 是否强制 zap？建议：诊断日志走 `stderr`，遵循白名单用 zap；**人机报告不用 zap** |
| `observability.md` §2/§3 | ❌ 不适用 | 无 OTel/Prometheus 需求（本地一次性进程） |
| `observability.md` §4 脱敏 | ⚠️ **需裁决** | 输出的是**本地路径**，含用户名等 PII。建议**不脱敏**（脱敏会使工具失去可用性）；但**错误日志**中不应记录文件内容 |
| `security.md` §1/§4/§5 | ❌ 不适用 | 无 JWT/限流/HTTP 安全头 |
| `security.md` §2 注入防御 | ✅ **适用** | ① 路径不可信输入 ② **终端转义序列注入**（见 §4.3）③ 符号链接逃逸 |
| `security.md` §3.1 Secret | ✅ 适用 | 无硬编码凭据；版本号用 ldflags 注入 |
| `testing.md` | ✅ 适用 | 覆盖率 ≥70%、表格驱动、`-race`、**FS 抽象导致的可测性设计**（见 §5.4） |
| `dependencies.md` | 🔴 **硬冲突** | 见 §4.2 |
| `commenting.md` | ✅ 适用 | 包注释 + 导出标识符 Godoc；**Swag 注释不适用**（无 HTTP Handler） |
| `environment.md` | ⚠️ 部分适用 | `Makefile` 适用（但需修正 `fmt` 目标，见 C2）；`docker-compose`（MySQL/Redis）❌ 不适用；`seed.sql` ❌ 不适用 |
| `git.md` | ⚠️ 缺失 | 索引存在但文件不存在（报告 C8） |

### 4.2 🔴 硬冲突：依赖白名单（最高优先级阻塞项）

PRD §四.3 明确要求"依赖包选择需遵循依赖白名单"，但 `dependencies.md` 白名单中**没有任何 TUI、进度条、框线/颜色、CLI 框架库**。本项目所需库全部落在白名单之外：

| 用途 | 候选库 | 是否在白名单 |
|---|---|---|
| 全屏 TUI | `charmbracelet/bubbletea` (+`lipgloss`,`bubbles`) | ❌ |
| 进度条 | `schollz/progressbar` / `cheggaaa/pb` / `vbauerster/mpb` | ❌ |
| 框线/样式 | `charmbracelet/lipgloss` / `fatih/color` / `jedib0t/go-pretty` | ❌ |
| 表格 | `olekukonko/tablewriter` | ❌ |
| **双宽字符宽度**（§1.5 发现 2 必需） | `mattn/go-runewidth` | ❌ |
| TTY 检测（非 TTY 降级必需） | `mattn/go-isatty` / `golang.org/x/term` | ❌ |
| CLI 框架 | `spf13/cobra` / `urfave/cli` | ❌ |
| 并发 | `golang.org/x/sync/errgroup` | ✅ **唯一在内** |
| 日志 | `go.uber.org/zap` | ✅ 在内 |

**这是一个「必须实现 vs 禁止引入」的死锁**，必须在写码前裁决。四个可选路线（**需你选一个**）：

| 路线 | 内容 | 代价 |
|---|---|---|
| **R1 扩充白名单（推荐）** | 在 `dependencies.md` 增加"CLI/TUI"分类，批准 2–4 个具体库 | 需你/团队 Code Review 通过（`docs/README.md` §3 要求先更新规范再改代码）|
| **R2 纯标准库实现** | `flag` + 手写 ANSI/`\r` 单行进度 + 手写边框 + 自实现 runewidth（East Asian Width 表需自维护，工作量大且易错） | 零新依赖，但**双宽对齐**与 TTY 检测需自研，风险高 |
| **R3 分阶段** | v1.0 用 R2（纯标准库、纯文本输出）先交付正确性；v1.1 再引入 TUI 库 | 交付慢一步，但风险最低，且符合 §70「当前真实需求 > 假想未来」|
| **R4 最小依赖组合** | 仅引入 `mattn/go-runewidth` + `mattn/go-isatty`（两个极小、无传递依赖的库）+ 标准库 `flag`，进度用 `\r` 单行 | 折中：不做全屏 TUI，但解决对齐与降级两个硬需求 |

> **我的建议：R4 + 后续按需升到 R1**。理由：`runewidth`/`isatty` 是**技术必需品**（无法用标准库替代，且体积极小）；而 `bubbletea` 引入大量传递依赖，与 `dependencies.md` §1「极简原则」张力最大，建议等到真正需要全屏交互时再评估。
> **无论选哪条，都必须先更新 `dependencies.md`**，否则违反 `docs/README.md` §3 的变更流程。

### 4.3 ✅ 适用但 PRD 未提及的安全要点

1. **终端转义序列注入（Terminal Escape Injection）**
   文件名可包含 `ESC`、`\n`、`\r`、`BEL` 等控制字符。若直接打印：
   - 可**伪造报告内容**（注入 `\r` 覆盖已输出行，让"总大小 4.82 GB"变成攻击者想要的值）；
   - 可篡改终端标题、写剪贴板（OSC 52）。
   → **必须净化**：输出前剥离/转义 C0/C1 控制字符（`\n`/`\t` 也需按场景处理，因为路径会破坏单行布局）。

2. **符号链接逃逸与成环**
   `--path` 下的 symlink 可能指向目录外，甚至形成循环（`a → b → a`）造成**无限遍历**。
   → 必须定义策略（默认**不跟随**，或跟随 + 基于 `dev+ino` 的环检测）。**需裁决**（P1）。

3. **路径输入不可信**（`security.md` §2.1 / §45）
   `--path ../../..`、超长路径、`\\?\` 前缀（Windows >260 字符）、`--output` 指向 `/etc/passwd` 等。
   → 建议：本地工具**允许任意路径**（否则失去用途），但必须：① `filepath.Clean` ② 明确不遵循 shell 展开 ③ 对不存在的根路径给出明确错误与退出码。

4. **Windows 特有**：junction / reparse point / 长路径（`\\?\`）行为与 Linux 不同，需在 `--verbose` 中可见。

---

## 5. 建议的架构设计（阶段一方案，待确认后实施）

### 5.1 🔴 第一决策：模块与目录布局

仓库是 `go-studio`（monorepo，含 `projects/`），当前**根目录无 `go.mod`**。三种布局（**需裁决**）：

| 方案 | 结构 | 优点 | 缺点 |
|---|---|---|---|
| **M1（推荐）独立 module** | `projects/fast-stat/go.mod` + `cmd/faststat/` + `internal/...` | 依赖隔离、独立版本与 CI、可单独 `go install`；根目录无需 go.mod | 根目录 `go build ./...` 不可用，需 `cd` 进入 |
| M2 根 module 单模块 | 根 `go.mod`，`projects/fast-stat/internal/...` | 统一 `go build ./...` | 未来所有 projects 共享依赖与版本，互相污染 |
| M3 go.work 多 module | 根 `go.work` + M1 结构 | 兼顾 | 引入 `go.work`（且需决定是否入库），当前仅一个项目 = 过度设计（§70）|

> **我推荐 M1**：`projects/` 语义就是"可独立的项目"。根目录 `AGENTS.md` 规范照常适用于其下所有目录。

### 5.2 分层设计（对齐 `architecture.md` 的分层思想）

```text
cmd/faststat/main.go            组合根：flag 解析入口、ctx+signal、依赖组装、退出码
        ↓
internal/cli/                   Transport 层：参数解析与校验、进度渲染、报告输出、退出码映射
        ↓
internal/scan/                  Service/领域层：遍历编排、并发控制、指标聚合、错误收集
        ↓
internal/fsx/                   Infrastructure 层：os 文件系统实现（FS 抽象接口的定义+实现）
        ↓
（无）                           无数据库、无外部 API
```

| 包 | 职责 | 明确不做 |
|---|---|---|
| `cmd/faststat` | 组装与进程退出；`signal.NotifyContext`；`-ldflags` 版本注入 | 任何业务逻辑 |
| `internal/cli` | `flag` 解析 + 参数校验；调用 `scan`；渲染进度（stderr）/ 报告（stdout）；退出码映射 | 直接调用 `os`/遍历逻辑；不做聚合计算 |
| `internal/scan` | 目录遍历编排、worker 池、`atomic` 计数聚合、错误分类收集、取消响应 | 不做任何终端渲染/格式化；不 import `cli` |
| `internal/fsx` | `os.ReadDir` / `os.Lstat` 的薄封装 + FS 接口定义 | 不含业务判断 |
| `internal/report` | 纯函数：`Snapshot → text/json` | 不读取文件系统、不感知并发 |

> **说明**：`architecture.md` 的 `handler/service/repository` 命名在本项目映射为 `cli/scan/fsx`。这是**同一分层思想在 CLI 场景的正名**，不是新架构模式（不引入 CQRS/DDD/ORM/DI 框架）。**需你确认此映射**。

### 5.3 核心契约示意（**仅接口声明，无实现**）

```go
// Contract sketch only — no implementation in phase 1.

// ==== internal/fsx ====
// FileSystem 抽象本地文件系统的目录读取与元数据查询，便于在单元测试中注入故障场景。
type FileSystem interface {
    // ReadDir 读取目录条目；返回的错误应保留 *PathError 以便 errors.Is 判定。
    ReadDir(dir string) ([]DirEntry, error)
    // Lstat 查询条目元数据，不跟随符号链接。
    Lstat(path string) (FileInfo, error)
}

// ==== internal/scan ====
// Options 定义扫描行为的可调参数（不可变，构造后只读）。
type Options struct {
    Root           string
    FollowSymlinks bool
    // MaxWorkers 为并发遍历的上限；零值使用默认值。
    MaxWorkers int
}

// Snapshot 表示一次扫描结束后的不可变结果快照。
type Snapshot struct {
    DirectoriesVisited int64
    FilesFound         int64
    TotalSizeBytes     int64
    SkippedErrors      int64
    Duration           time.Duration
}

// Scanner 执行目录统计扫描。实现必须并发安全且响应 ctx 取消。
type Scanner interface {
    Scan(ctx context.Context, opts Options) (Snapshot, error)
}

// ==== internal/cli ====
// Progress 渲染扫描过程中的进度；非 TTY 时应自动降级为空实现。
type Progress interface {
    Update(s scan.ProgressInfo)
    Finish(s scan.Snapshot)
}
```

**Sentinel Error（复用标准库哨兵，遵循 §9.6 不重复造）**：
`fs.ErrNotExist`（根路径不存在）、`fs.ErrPermission`（跳过并计数）、`context.Canceled`。**不新增同义错误**。

### 5.4 可测性设计（`testing.md` 的硬性影响）

要测试"权限拒绝不崩溃""符号链接成环""超大数据量"等分支，**必须能注入 fs 行为**（真实环境无法稳定构造）。因此：

- 遍历层依赖 `fsx.FileSystem` 接口，而非直接调用 `os`（否则错误分支不可测 → 覆盖率无法达到 70%）。
- 真实实现 `OSFileSystem` 用 `os.ReadDir`/`os.Lstat`；测试用 `fakeFS`（表驱动返回预设条目与错误）。
- **另有集成测试**用 `t.TempDir()` 构造真实目录树，验证端到端正确性。
- 时间依赖（耗时/速率）通过注入 `now func() time.Time` 或 clock 接口，**禁止 `time.Sleep` 等待**（`testing.md` §5）。

### 5.5 并发与取消设计

- **工作队列**：`chan string`（目录路径队列，buffered）→ 固定数量 worker goroutine 消费。
  按 `AGENTS.md` §18 显式回答：① 谁创建=Scanner ② 谁写=发现子目录的 worker ③ 谁读=worker ④ 谁关闭=生产者完成后关闭 ⑤ 何时=所有目录派发完毕 ⑥ 需要 buffer ⑦ 阻塞时阻塞派发不阻塞统计 ⑧ ctx 取消后停止消费并 drain。
- **禁止** `errgroup.SetLimit` 递归自死锁写法（见 §1.3）。
- **计数**：`atomic.Int64` 计数器，热路径零锁。
- **进度**：独立 `time.Ticker`（150ms）goroutine 读取计数快照并渲染；结束时 `ticker.Stop()`（§17.3 防泄漏）。
- **优雅取消**：`signal.NotifyContext(SIGINT/SIGTERM)` → 取消 → 停止派发 → worker 退出 → `WaitGroup.Wait()` → 输出"已取消"摘要 → 退出码非 0。

### 5.6 输出与渲染

- **stdout**：仅最终报告（text 或 json），保证可管道解析。
- **stderr**：进度渲染 + 诊断日志 + 错误明细。
- **非 TTY 检测**：stdout 非 TTY → 关闭颜色/emoji/进度；`--format json` → 强制关闭进度。
- **原子写文件**：`--output` 采用「写临时文件 → `os.Rename`」，避免中断产生半截 JSON。

---

## 6. 实施改动文件清单（阶段二将创建的文件，**待确认后执行**）

> 尚未创建任何文件。以下为实施清单（依赖 §5.1 与 §4.2 裁决结果）。

| # | 文件 | 层 | 说明 |
|---|---|---|---|
| 1 | `projects/fast-stat/go.mod` | — | 独立 module；Go 版本待定（本机 go1.25.4，建议 `go 1.24` 或 `1.25`）|
| 2 | `projects/fast-stat/cmd/faststat/main.go` | 组合根 | 入口、signal、依赖组装、退出码 |
| 3 | `projects/fast-stat/internal/cli/options.go` | Transport | flag 定义与校验、默认值 |
| 4 | `projects/fast-stat/internal/cli/options_test.go` | 测试 | 表格驱动：非法 format、冲突参数、缺省值 |
| 5 | `projects/fast-stat/internal/cli/progress.go` | Transport | 单行进度渲染 + 非 TTY 降级 |
| 6 | `projects/fast-stat/internal/cli/run.go` | Transport | 编排：解析 → 扫描 → 渲染 → 退出码 |
| 7 | `projects/fast-stat/internal/scan/scanner.go` | Service | 遍历编排、worker 池、聚合 |
| 8 | `projects/fast-stat/internal/scan/scanner_test.go` | 测试 | fakeFS：权限拒绝、符号链接环、空目录、大数量 |
| 9 | `projects/fast-stat/internal/scan/errors.go` | Service | 错误分类收集（复用 `fs.ErrPermission` 等）|
| 10 | `projects/fast-stat/internal/fsx/fs.go` | Infra | `FileSystem` 接口 + `OSFileSystem` 实现 |
| 11 | `projects/fast-stat/internal/report/text.go` | 表现 | 边框排版渲染（display width 对齐）|
| 12 | `projects/fast-stat/internal/report/text_test.go` | 测试 | 对齐断言（含 CJK/emoji 宽度）|
| 13 | `projects/fast-stat/internal/report/json.go` | 表现 | JSON schema 序列化 |
| 14 | `projects/fast-stat/internal/report/units.go` | 表现 | 字节 → KB/MB/GB 智能转换 |
| 15 | `projects/fast-stat/internal/report/units_test.go` | 测试 | 表格驱动：边界值（0/1023/1024/1MB/1GB/溢出）|
| 16 | `projects/fast-stat/internal/version/version.go` | — | `-ldflags` 注入的版本变量 |
| 17 | `projects/fast-stat/Makefile` | — | `build/test/test-race/fmt/vet/lint`（**需修正 fmt 目标，见 C2**）|
| 18 | `projects/fast-stat/README.md` | 文档 | 用法、参数表、JSON schema、退出码 |
| 19 | `docs/development/dependencies.md`（**修改**）| 规范 | 🔴 仅在 R1/R4 被批准时；且属"修改公共规范"需先确认 |
| 20 | `projects/fast-stat/go.sum` | — | 由 `go mod tidy` 生成（仅当引入依赖）|

**不改动**：`AGENTS.md`、`docs/development/` 其他文件、根 `README.md`（除后续需要更新项目索引）。

---

## 7. 待确认问题清单（分优先级）

### 🔴 P0 — 阻塞项（未裁决不进入编码）

| # | 问题 | 我的建议 |
|---|---|---|
| P0-1 | **依赖路线**：R1 扩充白名单 / R2 纯标准库 / R3 分阶段 / R4 最小依赖组合（`runewidth`+`isatty`） | **R4**（并同步更新 `dependencies.md`）|
| P0-2 | **总大小语义**：apparent size 还是 disk usage？ | 需你决定；`du` 兼容口径 = disk usage |
| P0-3 | **"跳过错误数"是否入报告**（与"仅三项"矛盾） | **方案 A**：作为独立"运行诊断"区块展示，修订 PRD §二.4 措辞 |
| P0-4 | **单位基数**：1024（KiB/MiB/GiB）还是 1000（KB/MB/GB）？ | **1024**，但显示为 `GB`（沿用示例写法）|
| P0-5 | **ETA/百分比**：接受改为"不确定型进度"（去 ETA）？ | **接受**；若坚持 ETA 请给出计算口径 |
| P0-6 | **模块布局**：M1 独立 module（推荐）/ M2 根 module / M3 go.work | **M1** |
| P0-7 | **JSON 是否套用 `{code,message,data,trace_id}`** | **不套用**，用纯数据 schema（§2.3）|

### 🟡 P1 — 重要（影响实现正确性）

| # | 问题 | 我的建议 |
|---|---|---|
| P1-1 | 符号链接策略：不跟随 / 跟随 / 跟随+环检测 | **默认不跟随**，目录计数但不入大小；提供 flag（若列入 v1）|
| P1-2 | 硬链接去重？ | **不去重**（与 `du` 结果不可比，需在 README 说明）|
| P1-3 | 特殊文件（FIFO/socket/device）是否计为"文件"？ | 计入文件数、大小为 0 |
| P1-4 | 二进制名：`faststat` 还是 `fast-stat`？ | `faststat` |
| P1-5 | 退出码契约（0/1/2）？ | 见 §2.2 建议 |
| P1-6 | 进度形态：单行 `\r` 还是全屏 TUI？ | **单行**（v1），全屏 TUI 留待 v1.1 |
| P1-7 | CLI 是否强制 zap？OTel/Prometheus 是否豁免？ | zap 用于 stderr 诊断；**OTel/Prometheus 明确豁免** |
| P1-8 | 是否引入配置文件（viper）？ | **不引入**，仅 CLI flag（`config.md` 相应条款豁免）|
| P1-9 | `--verbose` 的具体语义 | 错误明细前 N 条 + 生效参数 + 计时分解 |
| P1-10 | 并发度是否可配置（`--threads`）、默认值？ | 提供 `--threads`，默认 `min(NumCPU,16)` |
| P1-11 | 终端转义注入净化方案（剥离 vs 转义） | **剥离 C0/C1 控制字符**，保留可打印字符 |
| P1-12 | 性能验收指标（§1.3 建议值是否采纳） | 采纳或由你给定 |
| P1-13 | Go 版本与 `go.mod` 声明 | 本机 go1.25.4 → 建议 `go 1.24`（保守）|

### 🟢 P2 — 细节（可先按建议实现）

| # | 问题 | 我的建议 |
|---|---|---|
| P2-1 | 示例边框第 66 行的非法组合是否修正 | **修正为合法边框** |
| P2-2 | emoji 宽度导致跨终端错位 | 提供 `--no-emoji`/`--plain` 降级 |
| P2-3 | 「扫描次数」vs「Directories Visited」术语 | 报告用 `Directories Visited` |
| P2-4 | KB/Bytes 档位是否两位小数 | KB 两位小数；Bytes 整数带千分位 |
| P2-5 | `Duration` 精度与是否属承诺输出 | 2 位小数秒；属承诺输出 |
| P2-6 | 根路径不存在/为空目录的行为 | 不存在 → 报错退出码 1；空目录 → 正常报告全 0 |
| P2-7 | `--output` 是否支持 text 格式 | 支持，原子写入 |
| P2-8 | 进度刷新频率 | 150ms |
| P2-9 | 是否为多项目仓库补 `.github/workflows` CI | 建议后续单独任务 |

---

## 8. 我对本任务的自我约束（执行阶段承诺）

1. **不写代码，直到 P0 全部裁决 + 你明确确认方案。**
2. 不擅自扩大需求：v1 **不实现** `--exclude`、Top-N、图表、历史对比、守护模式、配置文件等 PRD 未要求的功能（`§70`）。
3. 不新增依赖，直到 `dependencies.md` 按裁决更新（`docs/README.md` §3 流程）。
4. 所有统计口径（size/symlink/hardlink/特殊文件）一旦裁决，将**写入 README 与 JSON schema 文档**，避免后续歧义。
5. 测试先行保障：错误分支（权限/环/取消）必须有设备可注入的测试覆盖。
6. 完成后如实报告：已执行的验证命令、覆盖率、基准数据、未验证项与风险。

---

## 9. PRD 建议修订项汇总（供你一次性更新 `requirements.md`）

| # | 位置 | 建议修订 |
|---|---|---|
| 1 | §二.2 | 删除/改写 ETA 与百分比要求，改为"不确定型进度：速率 + 已耗时 + 当前路径" |
| 2 | §二.4 | 明确"**统计指标**仅三项"（区分于报告展示字段）；补充 size 语义（apparent/disk usage）、单位基数（1024/1000）、hardlink/symlink/special file 口径 |
| 3 | §二.4.3 | 补充 KB/Bytes 档位的小数位与千分位规则 |
| 4 | §三 | 修正第 66 行非法边框；补充完整 CLI 参数表与退出码；补充 stdout/stderr 分流契约 |
| 5 | §三 | 补充 JSON schema 定义 |
| 6 | §四.2 | 与 §二.4 对齐"跳过错误数"的定位（独立诊断区块 or 第 4 指标）|
| 7 | §四.3 | 补充"依赖白名单需先扩充"的显式前置条件（引用 `dependencies.md` 变更流程）|
| 8 | 新增 §五 | 补充可量化的性能验收指标与验收数据集定义 |
| 9 | 新增 §六 | 补充安全要求：终端转义序列注入净化、符号链接策略、退出码语义 |

---

**下一步**：请对 §7 的 **P0-1 ~ P0-7** 给出裁决（可直接回复"P0-1 选 R4，P0-2 用 disk usage…"）。收到确认后，我将输出**最终实施方案（含精确接口契约、测试矩阵、验证计划）**，待你二次确认再进入编码。
