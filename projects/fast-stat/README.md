# fast-stat

> 高性能本地目录多维统计与实时进度可视化 CLI 工具。
>
> 需求来源：[`requirements.md`](requirements.md) · 需求细化：[`requirements-analysis.md`](requirements-analysis.md)
> 上线状态：**v1.0.0（已实现，待验收）**

---

## 1. 这是什么

一次执行、无守护进程的本地目录统计工具：并发遍历目录树，实时展示进度，输出带边框的终端报告或机器可读的 JSON。

```bash
faststat .                                          # 统计当前目录
faststat --path /var/log --verbose                  # 指定路径并输出诊断信息
faststat --path /data --format json --output result.json
```

**它不是** Web 服务、数据库客户端或文件内容分析器：不联网、不写数据库、不读取文件内容。

---

## 2. 构建与运行

模块定义在仓库根目录（多项目共享 `go.mod`），因此构建入口指向本项目：

```bash
# 方式一：Makefile（推荐，含版本注入）
cd projects/fast-stat
make build          # 产物：bin/faststat
make run            # 直接运行，统计当前目录

# 方式二：go 命令（在仓库根目录执行）
go run ./projects/fast-stat/cmd/faststat --path .
go build -o bin/faststat ./projects/fast-stat/cmd/faststat
```

> Windows 提示：Makefile 使用 POSIX shell 语法，请在 **Git Bash** 中执行 `make`。

### 完整参数示例（Windows，统计 `D:\www`）

```powershell
cd F:\AI-Agent-Project\go-studio; go run .\projects\fast-stat\cmd\faststat --path D:\www --threads 12 --verbose --format json --output D:\www-faststat.json
```

* 改成文本报告：把 `--format json --output D:\www-faststat.json` 换成 `--output D:\www-faststat.txt`。
* 并发按磁盘类型调整：机械硬盘用 `--threads 2`，NVMe 用 `--threads 16`（默认 `min(NumCPU, 16)`）。
* 去掉 `--output` 则结果直接打印到终端，并显示实时进度。
* 不要用 `>` 或 `|` 保存输出：本机控制台为 GBK/CP936，会把边框与 emoji 变成 `???` 且中文乱码，一律使用 `--output`。

### 验证命令

```bash
cd projects/fast-stat
make fmt-check      # gofmt 只读校验，输出必须为空
make vet            # go vet
make test           # 单元测试
make test-race      # 竞态检测 + 覆盖率（见 §9 环境要求）
make cover          # 覆盖率明细
```

---

## 3. 命令行契约

| 参数 | 说明 |
|---|---|
| `<路径>` | 位置参数，要统计的目录；缺省为当前目录 `.` |
| `--path <dir>` | 命名参数指定目录，**优先于位置参数** |
| `--format <fmt>` | `text`（默认）或 `json`；其他值视为用法错误 |
| `--output <file>` | 把报告写入文件；缺省写 stdout。采用「临时文件 + rename」原子写入 |
| `--threads <n>` | 并发遍历的 worker 数量；`0`（默认）表示自动：`min(NumCPU, 16)`，上限 1024。机械硬盘上建议降到 2 以减少寻道 |
| `--verbose` | 输出诊断区：生效参数与错误明细；同时把诊断日志级别提升到 INFO |
| `--no-emoji` | 用纯文本标签替换 emoji 图标（终端 emoji 宽度异常或字体缺失时使用） |
| `--plain` | 省略报告边框，仅输出文本行（便于写入日志）；**不可与 `--format json` 同用** |
| `--version` | 打印版本信息后退出 |
| `-h`, `--help` | 显示用法 |

### 退出码

| 码 | 含义 |
|---|---|
| `0` | 扫描完成（**可能包含被跳过的错误**，错误数在报告中单独展示） |
| `1` | 运行失败：根路径不存在/不是目录、报告写入失败、被用户取消（Ctrl-C） |
| `2` | 命令行用法错误 |

### 输入输出分工

| 通道 | 内容 |
|---|---|
| `stdout` | **只**输出最终报告（text 或 json），可直接被管道或程序解析 |
| `stderr` | 进度渲染、结构化诊断日志（zap）、用法错误提示 |

进度只在 stdout 连接到终端且 `--format text` 时启用；`--format json`、管道与 CI 环境下一律禁用，保证 JSON 可被直接解析。

---

## 4. 统计口径（重要）

同类工具最容易产生争议的地方在这里，因此全部显式声明：

| 指标 | 口径 |
|---|---|
| **扫描次数** | 成功**完整**读取的目录节点数，**含根目录**。读取失败的目录不计入，而是计入「跳过错误数」 |
| **文件个数** | 普通文件与特殊文件（FIFO / socket / 设备）的数量。不含目录本身；**不跟随、也不计入符号链接与 Windows 目录联接（junction）**；不对硬链接去重 |
| **路径总大小** | 文件**逻辑大小**（`st_size`）之和，即 `os.FileInfo.Size()` 的口径 |
| **跳过错误数** | 因无权限等原因读取失败的路径数；明细最多保留 20 条（`--verbose` 可见） |

### 与 `du -sh` 的差异（预期行为，非缺陷）

| 差异点 | 本工具 | `du -sh` 默认 |
|---|---|---|
| 大小口径 | 逻辑大小 `st_size` | 磁盘占用 `st_blocks × 512` |
| 符号链接 | 不跟随、不计数 | 不跟随 |
| Windows 目录联接（junction） | 不跟随、不计数 | 不跟随 |
| 硬链接 | **不去重**，重复计入 | 按 inode 去重 |
| 单位基数 | 1024 | 1024 |

选择逻辑大小的理由：跨平台语义一致（Windows 不暴露块数），且与 Go 标准库的 `FileInfo` 对应；磁盘占用口径需要平台特定代码。硬链接不去重是为了避免维护 inode 集合而与「低内存流式」目标冲突。

### 单位换算

按 **1024** 基数换算，但沿用 `KB / MB / GB` 字样；阈值判断在字节上完成。

```
0            -> "0 Bytes"
1023         -> "1,023 Bytes"
1024         -> "1.00 KB"
1048575      -> "1024.00 KB"   ← 按 PRD 的档位定义，不足 1MB 一律按 KB 展示
1048576      -> "1.00 MB"
5175123456   -> "4.82 GB"
```

---

## 5. JSON 输出契约

```json
{
  "target_path": "/data",
  "status": "completed",
  "duration_seconds": 1.24,
  "statistics": {
    "directories_visited": 12450,
    "total_files": 148203,
    "total_size_bytes": 5175123456,
    "total_size_human": "4.82 GB",
    "skipped_errors": 3
  },
  "tool": {
    "name": "faststat",
    "version": "1.0.0"
  }
}
```

- `status` 取值：`completed` | `canceled`。
- `total_size_bytes` 是无精度损失的**整数字节**；`total_size_human` 仅供展示，程序对接请使用前者。
- `--verbose` 时追加 `error_details: [{path, error}]`；默认不输出，使 JSON 体积与目录树的损坏程度无关。
- 本报告**不套用** HTTP 统一响应体 `{code, message, data, trace_id}`：本地一次性进程没有这些语义。

---

## 6. 进度展示

```text
- 10 dirs | 1,200 files | 1.00 MB | 1,200 files/s | 1.00 MB/s | 1.00s | /current/dir
```

**为什么不显示百分比与 ETA**：目录遍历结束前无法获知总文件数与总字节数，任何「剩余时间」都只能是被编造出来的数字。本工具只展示可观测的量：已扫描目录/文件/大小、瞬时速率、已耗时、当前目录。

进度行刷新间隔 150ms，用 `\r` 覆盖并在结束时清行。

---

## 7. 架构

```text
cmd/faststat          组合根：信号处理、依赖组装、进程退出码
      ↓
internal/cli          Transport：参数解析与校验、进度渲染、报告输出、退出码映射
      ↓
internal/scan         Service：遍历编排、并发控制、指标聚合、错误收集
      ↓
internal/fsx          Infrastructure：文件系统抽象与原子写入
      ↓
internal/report       表现层：纯函数渲染（text / json）、单位换算、宽度对齐、净化
```

设计要点：

- **可测试性驱动抽象**：遍历层依赖 `fsx.FileSystem` 接口而非直接调用 `os`，否则权限拒绝、读取中途失败等错误分支无法被单元测试覆盖。
- **并发模型**：固定数量 worker 反复从共享 LIFO 目录栈取任务；出队与回填由 `workQueue` 串行化。**不使用 channel 传递任务**——worker 既是生产者又是消费者时，channel 阻塞发送存在「全部 worker 同时阻塞、无人接收」的死锁路径，共享栈从结构上消除了它。LIFO 同时让深度优先访问获得更好的磁盘局部性。
- **无责任 goroutine 为零**：worker 数量有硬上限；取消时唤醒所有等待者；进度 goroutine 由 `ticker.Stop()` 与退出信号双重收敛。
- **内存与目录规模解耦**：待处理路径只保留「前沿」而非全树；错误明细限流 20 条。

---

## 8. 安全

| 风险 | 处理 |
|---|---|
| **终端转义序列注入** | 文件名与路径属于不可信输入。输出前剥离 C0/C1 控制符、DEL、U+2028/U+2029，阻断 `\r` 覆盖、OSC 52 剪贴板写入、标题篡改等伪造手段 |
| **符号链接逃逸与成环** | v1 一律不跟随符号链接**与 Windows 目录联接（junction）**，既避免遍历越出目标目录，也避免环状链接导致无限遍历。junction 在 Go 中表现为 `ModeIrregular` 而非 `ModeSymlink`，必须额外跟随一次才能识别 |
| **路径注入** | 路径在进入遍历层前经 `filepath.Clean` 规范化；不经过 shell 展开 |
| **凭据泄漏** | 不读取、不记录任何凭据；版本号由构建注入，源码中无密钥 |
| **原子写入** | `--output` 采用同目录临时文件 + rename，失败路径清理临时文件，不会留下半截 JSON |

---

## 9. 已知限制

1. **跨终端字符宽度**：报告排版显式固定「歧义宽度字符按 1 列」策略，与现代终端（Windows Terminal、VS Code 终端、iTerm2 默认配置）一致，并保证输出在任何机器与 CI 上完全一致。若使用把歧义宽度字符渲染为 2 列的传统 CJK 终端，边框会比内容宽——盒子内部仍自洽。可用 `--no-emoji` / `--plain` 进一步降级。
2. **emoji 对齐**：标题与指标图标使用 `U+1F680` 等 Wide 区 emoji（恒为 2 列）。若终端字体渲染 emoji 宽度异常或缺字，请使用 `--no-emoji`。
3. **硬链接不去重**，因此与 `du -sh` 结果不可比（见 §4）。
4. **`Duration` 分两档显示**：≥1 秒保留两位小数秒（`1.24s`），<1 秒保留一位小数毫秒（`12.5ms`）。JSON 的 `duration_seconds` 仍按两位小数秒输出，契约不变。
5. **`duration_seconds` 的精度**：秒级两位小数，亚秒级扫描在 JSON 中可能显示为 `0`。需要更精细的机器可读耗时时请提出需求。
6. **`-race` 的环境要求**：Windows 上竞态检测需要 MinGW-w64 **GCC 8+**。本机为 GCC 7.3.0，执行 `go test -race` 会报 `STATUS_ENTRYPOINT_NOT_FOUND`（连最小程序也失败），属于工具链版本问题，不是代码问题。请在 Linux/macOS/CI 或升级 MinGW 后执行。
7. **终端宽度探测**：为不引入白名单外依赖（`golang.org/x/term`），仅读取 `COLUMNS` 环境变量，取不到时回退 120 列。该值只影响进度行截断长度。
8. **未处理的方向控制符**（如 U+202E）：控制符剥离不覆盖双向文本控制字符。
9. **未实现**：`--exclude`、`--max-depth`、`--follow-symlinks`、Top-N 排行、图表、历史对比、配置文件、颜色主题。按 `AGENTS.md` §70「当前真实需求 > 假想未来需求」暂不实现。
10. **根路径若为符号链接或目录联接（junction），会在校验阶段被拒绝**：退出码 `1`，提示 `root path is not a directory`。原因是根路径校验使用 `Lstat`（不跟随链接），与 v1「不跟随链接」策略一致；但这与 `du` / `find`（会跟随命令行给出的根链接）的行为不同。**变通方法：传入真实路径**。是否允许根路径跟随链接属逻辑变更，待决策（见 §11）。
11. **无效 UTF-8 字节序列会原样透传**（不剥离、不替换）。它不含控制字符，因此不构成终端注入风险，仅会出现显示乱码。在 Windows/NTFS 上文件名始终是合法 UTF-8，该情形实际只在 Unix 上可能出现。

---

## 12. 边界行为验证记录

以下边界均已实测（Windows 10/11 + Go 1.25），用于回答「哪些情况已经被考虑过」：

| 边界 | 结果 |
|---|---|
| 超长路径（10 级、最深 485 字符，超过 `MAX_PATH=260`） | ✅ 正常统计，无跳过错误（Go 自动为超长路径加 `\\?\` 前缀） |
| 根路径不存在 | 退出码 1，不输出报告 |
| 根路径是普通文件 | 退出码 1，提示 not a directory |
| 根路径是指向目录的符号链接 / junction | ⚠️ 退出码 1（见 §9 第 10 条） |
| 空目录 | 正常报告，四项指标全 0 |
| `--path ""` | 等价于未指定，统计当前目录 |
| 以短横线开头的目录名（`-- --weird-dir`） | ✅ 正确当作路径，不被当成参数 |
| 重复指定 `--path` | 取最后一次的值 |
| `--threads` 取 0 / 1 / 1024 | 正常；1025 及以上退出码 2 |
| `--format` 大小写不符（如 `TEXT`） | 退出码 2 |
| 单个文件的元数据读取失败（扫描中被删除等） | 不中断；仍计入文件数，大小按 0，并计入跳过错误数 |
| 目录读取中途失败 | 不中断；已读出的条目仍统计，该目录不计入扫描次数 |
| 目录联接（junction）指向自身形成环 | ✅ 不跟随，不会无限遍历 |
| 终端宽度 0 / 1 / 2 | 不 panic，进度行不超限 |
| 多字节截断边界（中文 / emoji / 日文） | 结果始终是合法 UTF-8，不产生 `U+FFFD` |
| 数值上界（`math.MaxInt64` 个文件 / 字节） | 无溢出、无负值 |
| 输出写入失败（如 `--output` 指向目录） | 退出码 1，不残留临时文件 |
| 不存在的输出目录 | 退出码 1，不创建目录 |

> 上述边界中，除「根路径为链接」一项外，其余行为均已用单元测试锁定（见各包的 `_test.go`）。

---

## 10. 已裁决的需求决策

需求文档存在 4 处内部矛盾/缺口，实施前已由需求方逐项裁决：

| 编号 | 问题 | 裁决 |
|---|---|---|
| P0-1 | 依赖白名单无任何 TUI/宽度库 | **R4 最小依赖组合**：仅 `mattn/go-runewidth` + `mattn/go-isatty`，其余标准库 |
| P0-2 | 总大小口径 | **逻辑大小（`st_size`）** |
| P0-3 | 「跳过错误数」与「仅三项指标」冲突 | **作为第四项统计指标** |
| P0-4 | 单位基数 | **1024**，显示为 KB/MB/GB |
| P0-5 | ETA / 百分比 | **改为不确定型进度**，不展示 ETA 与百分比 |
| P0-6 | 模块布局 | 根目录共享 `go.mod`，代码位于 `projects/fast-stat/` |
| P0-7 | JSON 是否套用 HTTP 响应体 | **不套用**，纯数据 schema |
| P1-4 | 二进制名 | `faststat` |
| P1-7 | 诊断日志 | `zap` 输出到 stderr |
| P2-1 | PRD 示例非法边框 `╠……╝` | **修正为合法组合 `╠……╣`** |

### 依赖白名单变更

`docs/development/dependencies.md` 已按 R4 增补「CLI / 终端工具链」白名单分类与项目级批准记录（含明确未批准的 TUI / 进度条 / CLI 框架清单）。

### 文档同步状态

`requirements.md` 已修订至 R2，与上述裁决完全一致（含 §三 非法边框修正、CLI 参数表、退出码、JSON Schema、安全要求与性能验收草案）。

---

## 11. 待办与后续决策

1. **性能验收指标待确认**：PRD §四.3 已给出建议目标（100 万文件 ≤60s、峰值 RSS ≤256MB 等）但状态为草案，需需求方确认数值后转为正式验收标准，并补 `go test -bench` 基线。在此之前**不做基准断言**（不伪造验证数据）。
2. **`duration_seconds` 的精度**：JSON 中按秒两位小数输出，亚秒级扫描可能显示为 `0`；如需更精细精度请提出需求（会变更 JSON 契约）。
3. **`-race` 环境**：升级 MinGW-w64 至 GCC 8+ 后补跑竞态检测。
4. **`go.sum` 的间接依赖**：`go-runewidth` 引入了 `github.com/clipperhouse/uax29/v2` 作为间接依赖，已记录在 `dependencies.md` 的批准背景中。
5. **根路径为链接时是否跟随**：当前对符号链接 / 目录联接的根路径直接拒绝（§9 第 10 条）。这与 `du` / `find` 的行为不一致。支持它需要改动根路径校验逻辑（`Lstat` → 视目标类型决定），属逻辑变更，**待你确认后再做**。
6. **单目录海量文件的并行瓶颈**：并发单位是「目录」，因此单个目录内数十万文件的部分不享受并行加速（实测单目录 20,000 文件时 `--threads 1` 与 `12` 耗时相同）。突破它需要「目录内条目并行聚合」，属逻辑变更，待决策。
