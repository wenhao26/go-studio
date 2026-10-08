# fast-stat 业务流程梳理

> 面向「第一次读懂这个项目」的读者。只描述代码做什么与为什么这么做，不复述实现细节。
>
> 需求权威来源见 `requirements.md`；参数与统计口径的对外契约见 `README.md`。

---

## 一、这个程序是做什么的

**一句话**：给一个目录，并发遍历它的整棵子树，统计出 4 个指标（目录数、文件数、总大小、跳过错误数），渲染成终端表格或 JSON 输出。

```text
faststat D:\code --format json --verbose
```

四项指标与口径：

| 指标 | 口径 |
|---|---|
| 扫描次数 | 成功完整读取的目录节点数，含根目录；读取失败的目录不计入 |
| 文件个数 | 普通文件 + 特殊文件（FIFO/socket/设备），不含目录与符号链接 |
| 路径总大小 | 文件逻辑大小（`st_size`）之和，非磁盘占用；硬链接不去重 |
| 跳过错误数 | 因权限等原因读取失败的路径数 |

---

## 二、分层结构（依赖方向自上而下单向）

```text
cmd/faststat/main.go          组合根：装配依赖、信号处理、os.Exit
        │
        ▼
internal/cli                  Transport 层：解析参数、驱动流程、映射退出码
        │
        ├──► internal/scan    领域层：并发遍历 + 统计聚合（核心）
        │         │
        │         ▼
        │     internal/fsx    基础设施：文件系统抽象（可注入假实现做测试）
        │
        ├──► internal/report  表现层：纯函数渲染 Text / JSON
        │
        └──► internal/version 构建期注入的版本号
```

关键约束：`cli` 不碰文件系统，`scan` 不碰终端，`report` 不读文件系统。三者只通过 `scan.Snapshot` 这一个结构体交换数据。

| 包 | 职责 | 明确不做 |
|---|---|---|
| `cmd/faststat` | 依赖组装、信号处理、进程退出 | 不含统计与渲染逻辑 |
| `internal/cli` | 参数解析校验、进度与报告的输出时机、退出码映射 | 不遍历文件系统、不做聚合 |
| `internal/scan` | 目录遍历编排、并发控制、指标聚合、错误收集 | 不渲染、不格式化 |
| `internal/report` | 把结果渲染为文本或 JSON | 不读文件系统、不感知并发 |
| `internal/fsx` | 文件系统抽象 + 原子写文件工具 | 不含业务判断 |
| `internal/version` | 暴露构建期注入的版本号 | — |

---

## 三、端到端主流程

### 阶段 1：进程启动 — `cmd/faststat/main.go`

```text
main()
 └─ os.Exit(run())                ← 单独抽 run() 是为了让 defer 能执行
     ├─ newLogger()               zap → stderr（报告走 stdout，日志不能污染管道）
     ├─ signal.NotifyContext(...)  Ctrl-C / SIGTERM 会取消 ctx
     ├─ isTerminal(os.Stdout)     探测是否终端（决定要不要画进度条）
     ├─ terminalWidth()           读 COLUMNS，失败回退 120
     └─ app.Run(ctx, os.Args[1:])
```

### 阶段 2：参数解析 — `cli.Parse()`

```text
Parse(args)
 ├─ --help            → 返回 flag.ErrHelp → 打印 Usage → 退出码 0
 ├─ 参数非法          → stderr + Usage → 退出码 2
 ├─ --version         → 打印版本 → 退出码 0
 ├─ --path 优先于位置参数；都缺省 → "."
 ├─ filepath.Clean 规范化路径（输入不可信）
 ├─ --format 只收 text | json
 ├─ --threads 校验 [0, 1024]，非法立即报错（不静默钳制）
 └─ --plain + --format json 组合报错（JSON 本身没排版）
```

参数优先级与校验刻意放在 CLI 边界而非遍历层：用户传了明显错误的数值时应当立即知道，而不是拿到一份「参数被悄悄改过」的报告。

### 阶段 3：驱动扫描 — `cli.scanAndReport()`

```text
scanAndReport(ctx, opts)
 ├─ workers := opts.Threads; 为 0 → DefaultWorkers() = min(NumCPU, 16)
 ├─ scanner := scan.New(fsys)
 ├─ stopProgress := startProgress()         ← 见阶段 4
 ├─ snapshot, err := scanner.Scan(ctx, ...) ← 见阶段 5
 ├─ stopProgress()                           ← 必须先停，再写报告，避免交叉写坏终端
 ├─ 判断 err：
 │    ├─ 是 context.Canceled → status = Canceled，但仍渲染部分结果
 │    └─ 其他致命错误        → stderr 打印 → 退出码 1
 ├─ 组装 report.Input → render() → report.Text 或 report.JSON
 ├─ writeReport()：--output → 原子写文件；否则 → stdout
 └─ 退出码：canceled → 1，否则 0
```

取消与致命错误必须区分：取消时统计结果是部分结果但仍可展示，而根路径不可用时没有任何可信结果可报。

### 阶段 4：进度渲染（并行，stderr）— `cli/progress.go`

**只在 `stdout 是终端 && format == text` 时启用**，否则 `startProgress` 返回空函数。

```text
startProgress()
 ├─ go func() {
 │    每 150ms tick：
 │      读 scanner.Progress()   ← 无锁读原子计数器
 │      rates() 算瞬时速率（两次采样差分，异常值归 0）
 │      line() 组装：spinner | 目录 | 文件 | 大小 | files/s | B/s | 耗时 | 当前目录
 │      Truncate() 到终端宽度
 │      "\r" + 新行 + 空格补位（清掉上一帧残留）
 │   }()
 └─ 返回的 stop()：close(stop) → 等 goroutine 退出 → finish() 用空格整行擦除
```

> **刻意不显示百分比和 ETA** —— 遍历结束前根本不知道总工作量，任何百分比都是编造出来的数字。

### 阶段 5：并发扫描（核心）— `internal/scan/scanner.go`

```text
Scan(ctx, opts)
 ├─ 前置校验：fsys/ctx 为 nil → 报错
 ├─ Lstat(root)：失败 → 致命错误；不是目录 → ErrNotDirectory
 ├─ 派生 runCtx（确保 goroutine 必定退出，且不把取消信号泄漏回调用方）
 ├─ 新建 runState（4 个原子计数器 + 当前目录指针）存进 s.run
 ├─ 收敛 workers 到 [1, 1024]
 ├─ q.push(root)                                  ← 根目录入栈
 ├─ 启动 watcher goroutine：ctx.Done → q.close()   ← 取消时唤醒所有阻塞的 worker
 ├─ 启动 N 个 worker → wg.Wait()
 ├─ cancel() + 等 watcher 退出（不留 goroutine 泄漏）
 └─ 汇总成 Snapshot；若 ctx 被取消 → 包装 context 错误返回
```

**worker 循环：**

```text
worker(ctx, st, q):
  for {
    if ctx.Err() != nil { return }     ← 每轮开头检查取消
    dir, ok := q.pop()                 ← 栈空且无在途任务时阻塞
    if !ok { return }                  ← 遍历结束
    processDir(ctx, st, dir, q)
  }
```

**单目录处理 `processDir` —— 整个项目的业务心脏：**

```text
processDir(dir)
 ├─ defer q.done()                     ← 保证任何返回路径都归还在途计数
 ├─ st.current.Store(dir)              ← 供进度条显示「当前目录」
 ├─ entries := ReadDir(dir)
 │    ├─ 成功 → dirs++                 ← 只有「完整成功读取」才算一次扫描
 │    └─ 失败 → record(dir, err)       ← 计入 SkippedErrors，不计入 dirs
 ├─ 遍历每个条目：
 │    ├─ 是符号链接   → 直接跳过，不计入任何指标（防环、防越界）
 │    ├─ 是目录       → 收集到 children
 │    └─ 其他（文件/FIFO/socket/设备） → countEntry()
 ├─ q.push(children...)                ← ★ 先回填子目录
 └─ （defer 执行） q.done()             ← ★ 再归还计数 —— 顺序不能颠倒
```

即使 `ReadDir` 中途失败，已读出的条目也仍然参与统计，避免因为一次 I/O 错误丢掉本可观测的数据。

**`countEntry` —— 单个文件的计数逻辑：**

```text
countEntry(entry)
 ├─ entry.Info() 失败 → files++ 但按 0 字节 + record 错误
 │                      （条目已被确认存在，所以仍算数）
 ├─ ModeIrregular 且 Stat() 跟随一次后是目录
 │    → 这是 Windows 目录联接（junction） → 跳过
 │    （否则会被当成 0 字节文件，导致 Windows 文件数系统性偏大）
 └─ 否则 → files++ 且 size += Size()（仅当 >0）
```

### 阶段 6：渲染报告 — `internal/report`

```text
Input ──┬─ FormatText ─► Text()
        │                ├─ PlainLayout → plainText()（无边框）
        │                └─ 否则 → newBox(56 列) 拼装：
        │                     ╔═══ 标题（居中，可去 emoji）
        │                     ╠═══ 元信息：Target Path / Status (Duration)
        │                     ╠═══ 4 项指标（标签冒号对齐）
        │                     ╠═══ 诊断区（仅 --verbose：Workers + 错误明细）
        │                     ╚═══
        └─ FormatJSON ─► JSON()
                         snake_case，不套 HTTP 响应体
                         同时给 total_size_bytes（精确）与 total_size_human（展示）
                         error_details 仅 --verbose 时输出
```

渲染的两条安全底线：

- `SanitizeText()` 剥离所有控制字符 —— 文件名是不可信输入，否则攻击者能用 ESC 序列清屏、篡改终端标题、用 `\r` 伪造报告数字。
- 对齐基于**显示宽度**（中文/emoji 按 2 列），不是字节数也不是码点数。

---

## 四、核心数据结构：整个项目只有 3 个

| 结构 | 生命周期 | 用途 |
|---|---|---|
| `runState` | 扫描期间，可变 | 4 个 `atomic.Int64`（dirs/files/size/errs）+ 当前目录指针 + mutex 保护的错误明细。**进度条无锁读它** |
| `Snapshot` | 扫描结束，不可变 | 最终结果，`scan` → `report` 的交接物 |
| `report.Input` | 渲染期 | 表现层唯一输入契约，新增格式不用改 `scan` |

另有 `ProgressInfo` 是 `runState` 的只读切片，专供进度条；`ScanError` 携带 `Unwrap`，让 `errors.Is` 能穿透到根因。

并发安全设计：

```text
runState    → 计数器用 atomic，Progress 可在 Scan 期间并发读
              错误明细用 mutex，且只保留前 20 条（内存不随损坏程度增长）
workQueue   → mutex + sync.Cond + stack + inFlight + closed
Scanner     → 同一个 Scanner 不应被并发调用 Scan，但 Progress 可以
```

---

## 五、并发模型：为什么长这样

```text
        ┌─────────────── workQueue（一把锁）───────────────┐
        │  stack []string   ← LIFO 栈（深度优先）           │
        │  inFlight int     ← 已出队、正在处理的任务数        │
        │  closed bool      ← 取消标志                      │
        │  cond *sync.Cond  ← Broadcast 唤醒                │
        └──────────────────────────────────────────────────┘
              ▲ pop / push / done 都在这把锁之后
              │
   ┌──────────┼──────────┬──────────┐
 worker 1  worker 2  worker 3 ... worker N   （固定数量，不随目录数增长）
```

代码里有 4 个「为什么」，是理解本项目的关键：

1. **为什么不用 channel** —— 生产者和消费者是同一批 worker。有缓冲 channel 满时 worker 会阻塞在发送上，存在真实可达的死锁路径（全员阻塞在发送、缓冲满、无人接收）。收敛到一把锁后，等待只可能发生在「栈空但有在途任务」这一种情形，结构上消灭了死锁。

2. **为什么用 LIFO 而非 FIFO** —— 深度优先让相邻目录在时间上聚集，对磁盘 IO 更友好；且待处理路径的堆积量只与「分支深度」相关，而不是整棵树的宽度。

3. **为什么 push 之后才能 done** —— 顺序颠倒会出现「栈空且 inFlight 归零」的瞬时状态，其他 worker 会误判遍历结束，**漏扫整棵子树**。

4. **为什么用 Broadcast 不是 Signal** —— `done` 让等待条件成立时往往有多个 worker 需要同时退出，`Signal` 只唤醒一个，其余会永久阻塞在 `Wait` 上。

---

## 六、错误与退出码契约

| 情况 | 行为 | 退出码 |
|---|---|---|
| 正常完成（可能含跳过错误） | 输出报告 | **0** |
| 根路径不存在 / 不是目录 | stderr 报错，**不出报告** | **1** |
| Ctrl-C 取消 | 输出**部分结果**，状态标 Canceled | **1** |
| 报告写入失败 | stderr 报错 | **1** |
| 参数非法 / `--plain --format json` | stderr + Usage | **2** |
| `--help` | stdout + Usage | **0** |

错误分三档，处理方式完全不同：

- **致命**（根不可用）→ 没有任何可信结果，直接失败
- **单目录读取失败**（如权限拒绝）→ 不中断，计入 `SkippedErrors`，明细最多留 20 条
- **取消** → 尽快停，但已统计的部分结果**仍然有效，照常展示**

判别用 `errors.Is(err, context.Canceled)`，不比较错误字符串。

---

## 七、8 个「不看注释会疑惑」的设计决策

| # | 决策 | 理由 |
|---|---|---|
| 1 | 总大小用 `st_size`（逻辑大小） | 跨平台语义一致；磁盘占用需要平台特定代码 |
| 2 | 不跟随符号链接，链接不计入任何指标 | 防环形链接无限遍历 + 防越出目标目录 |
| 3 | Windows junction 特判 | Go 中它是 `ModeIrregular`，不特判会被当成 0 字节文件 |
| 4 | 不显示百分比 / ETA | 遍历结束前不知道总工作量，显示就是编造 |
| 5 | `ReadDir` 不排序 | 标准库 `os.ReadDir` 会做 O(n log n) 排序，百万条目纯浪费 |
| 6 | 显示宽度策略**显式固定** `EastAsianWidth:false` | 否则 locale 变化会让边框宽度漂移，输出不可复现 |
| 7 | `--output` 用临时文件 + rename | 直接写，中断会留半截 JSON —— 比没有文件更糟 |
| 8 | 默认并发度封顶 16 | 上限由 I/O 特征而非 CPU 决定：机械硬盘上并发越高寻道越多，吞吐反而低于单线程 |

---

## 八、建议阅读顺序

```text
1. cmd/faststat/main.go          111 行 · 看清装配与信号
2. internal/cli/run.go           443 行 · 主流程编排（最重要）
3. internal/scan/scanner.go      325 行 · 并发遍历核心
4. internal/scan/queue.go         96 行 · 死锁规避设计（注释密度最高）
5. internal/scan/types.go         97 行 · Snapshot 字段的口径定义
6. internal/report/text.go       239 行 · 报告怎么拼出来的
7. internal/fsx/fs.go            133 行 · 为什么要有文件系统抽象
8. 其余（units / width / sanitize / json / progress）· 纯工具，按需查
```

测试文件可以直接读 —— 表格驱动测试的 `name` 字段就是行为清单，等价于一份可执行的规格说明。
