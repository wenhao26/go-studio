# Phase 0 · 前置准备（🔴 阻塞阶段）

> 目标：在写任何业务代码前，关闭全部 P0 阻塞项、完成工程骨架与关键技术验证。
> 上游：[`README.md`](README.md)、[`../requirements-analysis.md`](../requirements-analysis.md)、[`contracts.md`](contracts.md)。
> **不含实现代码**：本阶段产出为「裁决记录 + 工程骨架文件 + 验证结论」。骨架文件（`go.mod`/`wails.json`/`Makefile`/README）在需求方确认后创建。

---

## 1. 目标与前置

- **前置**：无（本阶段是全部前置）。
- **完成定义**：P0-1 ~ P0-4 全部裁决并落地；工程骨架可 `build`；技术验证项均有结论。
- **不做**：任何扫描/分类/整理业务逻辑；任何前端业务页面。

---

## 2. 任务分解

### T0.1 依赖白名单裁决（🔴 P0-1）✅ 已裁决并落地

**背景**：`docs/development/dependencies.md` 白名单**不含**桌面框架与 SQLite 驱动。按 `docs/README.md` §3，必须先更新规范并经 Code Review，再改代码。

**裁决（2026-10-15，需求方授权）**：

| 依赖 | 版本锁定 | 用途 | 不可替代性 | 状态 |
|---|---|---|---|---|
| `github.com/wailsapp/wails/v2` | `v2.16.0`（tag，非 master） | 桌面窗口 / 绑定 / 事件 / 原生目录对话框 | 桌面形态由它决定，标准库无法替代 | ✅ 已批准 |
| `modernc.org/sqlite` | `v1.59.0` | 纯 Go SQLite 驱动（无 CGO） | Windows 无 gcc 构建；D-2 裁决 | ✅ 已批准 |
| `golang.org/x/sys/windows` | — | Windows 隐藏/系统属性、创建时间采集 | **V2/V3 验证后确认不需要**（纯标准库可取） | ❌ 不引入 |

**版本锁定点**：`modernc.org/sqlite` v1.60.x 的 `go.mod` 要求 `go >= 1.26.0`，与根 `go.mod` 的 `go 1.25`（本机 `GOTOOLCHAIN=local`）冲突，故锁定**最高兼容版 v1.59.0**（其 `go` 指令为 1.25.0）。

**已落地**：`dependencies.md` 新增 `§2.2 file-pilot 项目级依赖批准记录`，列明依赖、用途、不可替代性、验证结论与**明确未批准**清单。

> ⚠️ 本步骤已按需求方授权修改公共规范 `docs/development/dependencies.md`，请在 Code Review 时重点复核该节。

### T0.2 模块与工程骨架 ✅ 已创建

**裁决**：项目使用**独立 `go.mod`**（D-1，Wails 工具链要求项目本地 `go.mod`/`wails.json`）。

**已创建文件**：

| 文件 | 内容 / 状态 |
|---|---|
| `projects/file-pilot/go.mod` | `module github.com/wenhao26/go-studio/projects/file-pilot`（需求方裁决）；`go 1.25.0`（对齐根 `go.mod` 的 `go 1.25`；`go get wails` 将其规范为 `1.25.0`，属同一 minor） |
| `projects/file-pilot/go.sum` | 依赖校验和 |
| `projects/file-pilot/wails.json` | Wails v2 项目描述：`name/outputfilename = file-pilot`，前端 `npm install` / `npm run build` / `npm run dev`，`frontend:dev:serverUrl = auto` |
| `projects/file-pilot/main.go` | 组合根**最小骨架**：`//go:embed all:frontend/dist` + `wails.Run`（1280×800）。**暂不建 `app.go`**：绑定方法与生命周期钩子自 Phase 1 随 `internal/app` 一起提供，避免无行为的占位代码 |
| `projects/file-pilot/Makefile` | `dev` / `build` / `test` / `test-race` / `fmt` / `vet`（`fmt` 用 `gofmt -l -w .`，仅作用于本项目） |
| `projects/file-pilot/README.md` | 用法、环境前置（WebView2、wails CLI、gcc）、依赖锁定记录、验收记录 |
| `projects/file-pilot/frontend/` | Vue3 + TS + Vite 7 + Tailwind v4（`@tailwindcss/vite`）骨架；`src/App.vue` 为 Phase 0 占位页（已标注由 Phase 1 替换） |
| `projects/file-pilot/.gitignore` | 仅补根 `.gitignore` 未覆盖的 `node_modules/` |

**约束执行情况**：

- ✅ Wails 版本锁定 tag `v2.16.0`（CLI 亦为 `wails@v2.16.0`，未用 master）。
- ✅ 前端 npm 依赖记录于 `README.md`（P1-9）。
- ✅ 根目录及其他 projects 无改动（见本文 §6 自检记录）。
- ⚠️ `wails build` 会执行 `go mod tidy`，**未被 import 的依赖会被移除**——`modernc.org/sqlite` 因此曾被清出 `go.mod`，由本阶段保留的 `phase0_v7_verify_test.go`（真实 import）维持；Phase 1 落地 `internal/database` 后该测试并入并删除。

### T0.3 关键技术验证（结论必须回填本文档）

| # | 验证项 | 目的 | 方法与预期 |
|---|---|---|---|
| V1 | **`os.Rename` 覆盖语义**（✅ 已核实） | 决定「绝不覆盖」如何实现 | 已核实 Go 1.25.4 源码：`os.Rename` → `MoveFileEx(..., MOVEFILE_REPLACE_EXISTING)`，**会覆盖已存在文件**。备选：`syscall.MoveFile`（`MoveFileW`，不替换，但无长路径归一） |
| V2 | Windows 隐藏/系统属性读取 | 决定是否需要 `x/sys/windows` | 尝试用标准库 `syscall` 读取文件属性（`GetFileAttributes` + `FILE_ATTRIBUTE_HIDDEN/SYSTEM`）；若不可行 → 触发依赖白名单追加 |
| V3 | 文件创建时间读取 | R-2 落地 | Windows 通过平台 API 取 `CreationTime`；失败置 NULL |
| V4 | 长路径（>260）行为 | 防「路径超长」失败 | 构造超长路径，验证 Go os API 的 `\\?\` 归一是否生效 |
| V5 | junction / reparse point 表现 | 防越界/防环 | 构造 junction，确认其在 Go 中的 `DirEntry`/`FileInfo` 表现，并验证跳过策略 |
| V6 | Wails 独立 module + 根 go.mod 共存 | 防工具链冲突 | `wails build` / `wails dev` 在独立 module 下可跑通 |
| V7 | SQLite 驱动无 CGO 构建 | 防构建链依赖 | `CGO_ENABLED=0` 下可编译并打开 DB |

**V1 已核实证据（写入结论）**：

- `D:\Program Files\Go\src\os\file_windows.go` → `os.rename` 调用 `internal/syscall/windows.Rename`。
- `...\internal\syscall\windows\syscall_windows.go:364` → `return MoveFileEx(from, to, MOVEFILE_REPLACE_EXISTING)`。
- 结论：**不能依赖 `os.Rename` 失败来防覆盖**。

**V2 ~ V5 验证结论（2026-10-15 实机 + Go 1.25.4 源码核对）**：

| # | 结论 | 证据 |
|---|---|---|
| V2 | Windows 隐藏/系统属性**可用纯标准库读取**，无需 `x/sys/windows` | `fi.Sys().(*syscall.Win32FileAttributeData).FileAttributes` + `syscall.FILE_ATTRIBUTE_HIDDEN(0x2)` / `FILE_ATTRIBUTE_SYSTEM(0x4)`；Go 源码 `os/types_windows.go:83`、`os/atime_windows.go` 即此用法；实机构造隐藏文件读取 `hidden=true` |
| V3 | 文件创建时间**可用纯标准库读取**（R-2 从「最好能取」升为「可取」） | 同一结构体 `CreationTime syscall.Filetime` → `.Nanoseconds()`；实机读出 `ctime=2026-10-15T09:15:18+08:00` |
| V4 | Go 的 `\\?\` 长路径归一有效 | 321 字符路径 `MkdirAll` / `WriteFile` / `os.Stat` / `os.Rename` 全部成功（`os/stat_windows.go:29 fixLongPath`） |
| V5 | junction 在 Go 中的判定明确 | `os.Lstat(junction)` → `mode=?rw-rw-rw-`、`IsDir=false`、`IsSymlink=false`、**`ModeIrregular=true`**；`ReadDir` 的 `DirEntry.IsDir()` 也是 `false`。**跳过规则定为：`ModeSymlink ∥ ModeIrregular` 即跳过**，与 fast-stat 口径一致（junction 若不处理会被当成普通文件计数） |

**V6 结论（工程骨架可构建）**：

- `wails build`（production，windows/amd64）一次通过：生成 bindings → 安装前端依赖 → 编译前端 → 生成 assets → 编译应用，产出 `build/bin/file-pilot.exe`，耗时 20.8s。
- 独立 module 与根 `go.mod` 共存无冲突；根仓库不受影响。

**V7 结论（无 CGO 构建 + SQLite 驱动可用）**：

- `CGO_ENABLED=0 go build .` 通过（产物 5.89 MB）。
- `CGO_ENABLED=0 go test -run TestPhase0V7 -v .` **PASS**：真实 `sql.Open` → `Ping` → 建表 → 写入 → 查询回读全部成功（测试文件 `phase0_v7_verify_test.go`，Phase 1 并入 `internal/database` 后删除）。

**V8（新增）race 检测器可用性 —— 发现环境级问题**：

- **现象**：`go test -race ./...` 的测试二进制**进程启动即静默退出 `0xC0000139` (STATUS_ENTRYPOINT_NOT_FOUND)**，无任何 stdout/stderr、无应用事件日志。
- **影响面**：与依赖包无关——**仅含标准库**的测试包同样复现；带 cgo 的包则通过。二分定位到**链接方式**：默认（内部链接）失败，外部链接通过。
- **解法（已落地 Makefile）**：`go test -race -ldflags=-linkmode=external ./...`。实测标准库包、`modernc.org/libc`、本项目包**全部通过**。
- **环境事实**：Go 1.25.4、`GOTOOLCHAIN=local`、gcc = `D:\Program Files\mingw64\bin\gcc.exe` **7.3.0（MinGW DLL 均为 2018-03-19）**，已在 PATH。
- **残余风险**：未验证「升级 MinGW 到新版后内部链接是否恢复」；若后续在其他机器复现，优先升级 MinGW 而非继续依赖 `-linkmode=external`。

### T0.4 分类算法口径裁决（🔴 P0-4）✅ 已裁决

- **裁决（2026-10-15）：采用 [`../requirements-analysis.md`](../requirements-analysis.md) §2.1 口径 B**（语义优先选类 + 跨类别匹配总分封顶 100）。理由：`requirements.md` §5.2 的 4 个示例按口径 A 字面自相矛盾，口径 B 全部吻合。
- ✅ **已获需求方授权并执行（2026-10-15，修订号 R3）**：`requirements.md` §5.2 改写为口径 B 算法（语义优先选类 + 跨类别命中总分封顶 100），校验示例扩为 6 条；§5.6 原因规则同步改为「取决定类别的命中项中分值最高者」并给出并列裁决顺序；§十五 标记为已复核采纳；修订记录新增 R3 行。

### T0.5 复核 `requirements.md` §十五（R-1 ~ R-9）✅ 全部采纳

**裁决（2026-10-15）：R-1 ~ R-9 按建议口径全部采纳**，`requirements.md` §十五 无需修改：

| # | 内容 | 裁决 |
|---|---|---|
| R-1 | 分类仅 7 类、7 类不扩 | ✅ 采纳 |
| R-2 | 文件创建时间「最好能取」→ **可取**（V3 验证） | ✅ 采纳，实现为非空写入 |
| R-3 | 扫描中不剪动态并发 | ✅ 采纳 |
| R-4 | 保留历史任务 20 次 | ✅ 采纳 |
| R-5 | 「可能可释放」阈值 1 GiB | ✅ 采纳 |
| R-6 | 预览列表 500 条分批 | ✅ 采纳 |
| R-7 | 关键词表口径 | ✅ 采纳 |
| R-8 | 前端不做自动化测试门禁 | ✅ 采纳 |
| R-9 | 仅撤销最近一次整理 | ✅ 采纳 |

---

## 3. 交付物清单

| # | 交付物 | 类型 | 状态 |
|---|---|---|---|
| 1 | `dependencies.md` 新增 §2.2（授权后） | 规范变更 | ✅ |
| 2 | `go.mod` / `go.sum` | 工程 | ✅ |
| 3 | `wails.json` / `main.go`（`app.go` 按 T0.2 说明推迟至 Phase 1） | 工程 | ✅ |
| 4 | `Makefile` / `README.md` / `frontend/` / `.gitignore` 骨架 | 工程 | ✅ |
| 5 | 技术验证结论（V1–V8）回填本文档 | 文档 | ✅ |
| 6 | 分类算法口径 + R-1~R-9 裁决记录 | 文档 | ✅ |
| 7 | `requirements.md` §5.2 按口径 B 修订 | 需求文档变更 | ✅（授权后执行，修订号 R3） |

---

## 4. Definition of Done

```text
✓ P0-1 依赖白名单已更新（dependencies.md §2.2）——待 Code Review
✓ P0-2 覆盖防护方案已定：方案①存在性预检 + os.Rename（含 V1 结论）
✓ P0-3 平台属性采集路径已定：纯标准库 Win32FileAttributeData（含 V2/V3 结论），依赖范围明确且未扩大
✓ P0-4 分类算法口径已定（口径 B）；requirements.md §5.2 / §5.6 已按授权同步修订（R3）
✓ requirements.md §十五 R-1~R-9 已复核并全部采纳
✓ 独立 module 骨架可 build / wails build 跑通（V6）
✓ CGO_ENABLED=0 构建通过（V7，含 SQLite 驱动实测）
✓ 根目录与其他 projects 无意外改动（git diff 自检，见 §6）
✓ 如实报告：已执行验证 / 未验证项 / 风险（见 §5、§7）
✓ C-2 / C-3 已关闭（2026-10-15 需求方裁决）
⏳ C-1 Code Review（dependencies.md §2.2）待需求方 —— 阻塞进入 Phase 1
⏳ C-4 体验验证（build/bin/file-pilot.exe）待需求方 —— 阻塞进入 Phase 1
```

---

## 5. 风险与残余问题

| 风险 / 残余问题 | 影响 | 现状与缓解 |
|---|---|---|
| ~~`x/sys/windows` 需入白名单~~ | — | ✅ 已消除：V2/V3 证明标准库足够 |
| Wails v2 与独立 module 共存异常 | 阻塞全部开发 | ✅ V6 已验证通过 |
| `modernc.org/sqlite` 传递依赖较多 | 违反极简原则 | 已在 `dependencies.md` §2.2 与项目 README 列明；待 Code Review 确认 |
| `github.com/pkg/errors` 出现在依赖图 | 与黑名单冲突 | 仅 **wails 内部传递依赖**，本项目不直接 import（黑名单针对项目自身代码）；已在 README 记录，待 Review 确认可接受 |
| **V8：本机 `-race` 内部链接失败** | 并发测试无法默认执行 | 已用 `-ldflags=-linkmode=external` 绕过并内置 Makefile；**未验证**升级 MinGW 后是否恢复 |
| 分类口径裁决与需求方直觉不一致 | 返工 | 已提供可复算用例表（analysis §2.1），需求方已确认口径 B |
| `wails build` 触发 `go mod tidy` 清理未引用依赖 | `go.mod` 中 sqlite 被移除 | 由 `phase0_v7_verify_test.go` 真实 import 维持；Phase 1 落地 `internal/database` 后并入删除 |
| 生产二进制未做真机安装/运行体验 | 交付风险 | 属「体验验证」环节，由需求方在本阶段审核时执行 |

---

## 6. 执行记录与自检（2026-10-15，本机实机）

**已执行并有输出的命令**：

```text
✓ go mod init github.com/wenhao26/go-studio/projects/file-pilot
✓ go get github.com/wailsapp/wails/v2@v2.16.0
✓ go get modernc.org/sqlite@v1.59.0（v1.60.1 因 go>=1.26 被拒，已换 1.59.0）
✓ go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
✓ npm install -D tailwindcss @tailwindcss/vite（67 packages，0 vulnerabilities）
✓ wails build                → build/bin/file-pilot.exe，20.815s
✓ gofmt -l .                 → 无输出
✓ go vet ./...               → exit 0
✓ go test ./...              → [no test files]（骨架阶段无业务代码）
✓ CGO_ENABLED=0 go build .   → exit 0，5,889,536 bytes
✓ CGO_ENABLED=0 go test -run TestPhase0V7 -v .   → PASS
✓ go test -race -ldflags=-linkmode=external .     → PASS
✓ 临时验证程序（V2~V5）在系统临时目录运行并读出预期值，随后删除
```

**未执行 / 未验证**：

```text
- wails dev 热重载联调：未执行（无前端业务页面，属 Phase 1 T1.9 体验环节）
- 升级 MinGW 后 `-race` 内部链接是否恢复：未验证
- 真机安装包（NSIS）与 WebView2 缺失场景：未验证（Phase 4 分发事项）
- 前端 `npm run build` 单独执行：由 wails build 内部完成，未单独重跑
```

**git 自检**（实测）：

```text
git status --short:
   M docs/development/dependencies.md     ← 本次授权的规范变更，+28 行
?? projects/file-pilot/                   ← 本阶段全部新增

git diff --cached 为空（无暂存、无提交，遵循 AGENTS.md §39）
git check-ignore 确认：frontend/node_modules、frontend/dist 已被忽略
根 go.mod / go.sum / 其他 projects / docs 其余部分均未改动
```

---

## 7. 待需求方确认（进入 Phase 1 前）

| # | 事项 | 结论 | 状态 |
|---|---|---|---|
| C-1 | **Code Review：`dependencies.md` §2.2** | 含批准清单、未批准清单、传递依赖（`github.com/pkg/errors` 由 wails 引入，项目不直接 import）；按 `docs/README.md` §3，规范变更需经 Review | ⏳ **需求方审核中**（2026-10-15 回复：先审核、稍后反馈） |
| C-2 | **授权修订 `requirements.md` §5.2** | ✅ **已授权并完成**：§5.2 改写为口径 B、§5.6 原因规则同步、示例扩为 6 条、修订记录新增 R3、§十五 标记已采纳 | ✅ 关闭 |
| C-3 | **确认 `-race` 的 `-linkmode=external` 方案** | ✅ **已确认保留**该 flag（Makefile 内置 + 注释原因 + README §4 + 本文 V8）；备选「升级 MinGW 后复测」留作后续问题处理 | ✅ 关闭 |
| C-4 | 体验验证 | `wails build` 产物 `build/bin/file-pilot.exe` 可直接双击运行，窗口显示 Phase 0 占位页 | ⏳ **需求方体验中** |

**关闭 C-1 与 C-4 后即可进入 Phase 1**（[`01-phase-1-scan.md`](01-phase-1-scan.md)）。

