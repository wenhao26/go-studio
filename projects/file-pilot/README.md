# FilePilot · 智能文件整理助手

面向个人用户的 Windows 桌面应用：**扫描 → 智能分类 → 预览 → 整理（移动）→ 撤销**。
Wails v2 + Vue3/TypeScript/Vite/TailwindCSS + 纯 Go SQLite。

- 需求权威来源：[`requirements.md`](requirements.md)；深度分析：[`requirements-analysis.md`](requirements-analysis.md)
- 开发阶段计划与契约：[`development/`](development/)（**v1 = Phase 1–3 已交付完成**）
- 依赖规范：[`../../docs/development/dependencies.md`](../../docs/development/dependencies.md) §2.2（file-pilot 项目级批准记录）

> **状态（2026-10-15）**：**v1 已交付完成并通过需求方验收**——扫描（Phase 1）、智能分类（Phase 2）、整理与撤销（Phase 3）全链路实现，前端页面 P1–P4 具备。
> Phase 4 为 v1 之外的增强 backlog（[`development/04-phase-4-enhancement-backlog.md`](development/04-phase-4-enhancement-backlog.md)），**不实现**。
> 构建产物：`build/bin/file-pilot.exe`。前端交互按 [`development/test-and-acceptance.md`](development/test-and-acceptance.md) §6 手动清单验收（R-8：前端无自动化测试）。

---

## 1. 项目结构

```text
projects/file-pilot/
├── main.go                      # Wails 应用入口（窗口配置、绑定注册、生命周期）
├── wails.json                   # Wails 构建配置（前端构建命令、产物名）
├── go.mod / go.sum              # Go 依赖锁定（版本见 §4.1）
├── Makefile                     # 命令入口（dev / build / test / test-race / fmt / vet）
├── build.ps1 / build.bat        # 一键打包脚本（PowerShell / cmd）
├── .gitignore                   # 项目级忽略规则（补充仓库根 .gitignore）
├── README.md                    # 本文件
├── requirements.md              # 需求权威来源
├── requirements-analysis.md     # 需求深度分析
│
├── build/                       # Wails 构建资源（非产物；产物 build/bin/ 已被忽略）
│   ├── appicon.png              # 应用图标源图
│   ├── darwin/                  # macOS 平台配置（v1 仅构建 Windows，保留默认）
│   └── windows/
│       ├── icon.ico             # 应用图标
│       ├── info.json            # 版本 / 公司信息（写入 exe 资源）
│       ├── wails.exe.manifest   # 应用清单（DPI 等）
│       └── installer/           # NSIS 安装包脚本
│
├── development/                 # 阶段计划、契约、测试与验收（文档）
│   ├── README.md                # 阶段总览与状态
│   ├── 00-phase-0-prerequisites.md
│   ├── 01-phase-1-scan.md
│   ├── 02-phase-2-classification.md
│   ├── 03-phase-3-organize-undo.md
│   ├── 04-phase-4-enhancement-backlog.md   # v1 之外的增强 backlog（不实现）
│   ├── contracts.md             # 跨阶段契约（接口 / 数据 / 事件 / 错误）
│   └── test-and-acceptance.md   # 测试策略与手动验收清单
│
├── frontend/                    # 前端（Vue3 + TypeScript + Vite + TailwindCSS）
│   ├── index.html               # 页面骨架
│   ├── package.json / package-lock.json
│   ├── tsconfig.json / tsconfig.node.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── main.ts              # 应用挂载入口
│   │   ├── App.vue              # 应用外壳（顶部导航 + 路由出口 + 恢复横幅）
│   │   ├── router.ts            # 路由表（4 条路由，裁决 D-A5）
│   │   ├── style.css            # Tailwind 入口
│   │   ├── vite-env.d.ts
│   │   ├── lib/                 # 前端工具与状态（无 UI）
│   │   │   ├── task.ts          # 任务状态 + 事件桥接（task:status|progress|log）
│   │   │   ├── errors.ts        # 哨兵错误 → 友好文案映射（contracts §4）
│   │   │   └── format.ts        # 展示层格式化（大小 / 耗时 / 速率 / 日志时间）
│   │   ├── views/               # 页面 P1–P4
│   │   │   ├── HomeView.vue     # P1 首页（目录选择、配置、最近目录、开始扫描）
│   │   │   ├── ScanView.vue     # P2 扫描中（进度 + 日志 + 取消）
│   │   │   ├── ResultsView.vue  # P3 结果（分类汇总 / 详情、开始整理）
│   │   │   └── OrganizeView.vue # P4 整理（进度 / 报告 / 撤销）
│   │   └── assets/fonts/        # 内嵌字体（Nunito，附 OFL 许可）
│   └── wailsjs/                 # Wails 自动生成的绑定（勿手改；wails generate module 重建）
│       ├── go/app/API.js|.d.ts  # 绑定方法（contracts §5）
│       ├── go/app/models.ts     # 绑定的 Go 模型类型
│       └── runtime/             # Wails 运行时（事件监听等）
│
└── internal/                    # Go 后端（依赖方向 app → task → service → storage → fsx）
    ├── app/                     # 绑定层 = Transport/Handler + 事件桥接
    │   ├── app.go               # App 生命周期与装配
    │   ├── api.go               # 暴露给前端的薄委托（contracts §5，全集）
    │   ├── bindings.go          # 边界 DTO 与绑定实现
    │   ├── settings.go          # 扫描配置 + 最近目录（含 RemoveRecentDir）
    │   ├── run.go               # 扫描任务 runner
    │   ├── run_organize.go      # 整理 / 撤销任务 runner
    │   ├── events.go            # task:status|progress|log 事件发射
    │   ├── adapter.go           # 任务管理器适配
    │   ├── errors.go            # 错误映射与哨兵
    │   └── *_test.go
    ├── task/                    # 任务管理器（状态机、单活动约束、取消、节流器）
    ├── scanner/                 # 扫描 Service（递归遍历、跳过规则、属性采集）
    ├── classifier/              # 分类 Service（规则引擎：扩展名 / 关键词 / 路径）
    ├── organizer/               # 整理 / 撤销 Service（安全移动、冲突改名）
    ├── storage/                 # Repository（SQLite）
    │   └── migrations/0001_init.sql   # SQL 迁移
    ├── fsx/                     # Infrastructure：文件系统抽象 + 平台属性
    ├── units/                   # 字节格式化口径（与前端 format.ts 对齐）
    └── diag/                    # 诊断日志（zap + 自实现滚动，NFR-3）
```

> 分层依赖方向与边界见 [`development/contracts.md`](development/contracts.md) §2；`frontend/wailsjs/` 为 Wails 生成物，**勿手改**。各阶段文档见 [`development/`](development/)。

---

## 2. 环境前置

| 前置 | 版本要求 | 本机验证值 | 用途 |
|---|---|---|---|
| Go | 1.25.x（与根 `go.mod` 对齐） | 1.25.4（`GOTOOLCHAIN=local`） | 后端编译 |
| Node.js | ≥ 20 | v24.14.0 / npm 11.9.0 | 前端构建 |
| Wails CLI | 与库同 tag（**v2.16.0**） | v2.16.0 | 构建/联调 |
| MinGW-w64 gcc | 需在 PATH | 7.3.0（`D:\Program Files\mingw64\bin`） | 仅 `-race` 测试需要 |
| WebView2 Runtime | 随 Windows 更新 | 见 `wails doctor` | 运行窗口 |

安装 / 校验 Wails CLI：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
wails version
wails doctor        # 校验 Go / Node / WebView2
```

---

## 3. 常用命令

`Makefile` 提供统一入口；本机为 Windows PowerShell，等价命令如下（两种方式等价，任选）：

| 作用 | Makefile | PowerShell |
|---|---|---|
| 开发联调（热重载） | `make dev` | `wails dev` |
| 一键打包 | — | `.\build.ps1` 或 `build.bat` |
| 生产构建 | `make build` | `wails build` |
| 单元测试 | `make test` | `go test ./...` |
| 并发检测 | `make test-race` | `go test -race -ldflags=-linkmode=external ./...` |
| 格式化（仅本项目） | `make fmt` | `gofmt -l -w .` |
| 静态检查 | `make vet` | `go vet ./...` |

> 上述命令均需在 `projects/file-pilot/` 目录下执行。

---

## 4. 依赖锁定记录

### 4.1 Go（`go.mod`，白名单见 `docs/development/dependencies.md` §2.2）

| 依赖 | 版本 | 说明 |
|---|---|---|
| `github.com/wailsapp/wails/v2` | **v2.16.0**（tag） | 桌面框架；CLI 与库保持同版本 |
| `modernc.org/sqlite` | **v1.59.0** | 纯 Go、无 CGO。**锁定原因**：v1.60.x 要求 `go >= 1.26`，与根 `go.mod` 的 `go 1.25` 冲突；v1.59.0 是兼容 `go 1.25` 的最高版本 |
| 其余 | 见 `go.mod` 的 `// indirect` | 均为 wails / sqlite 传递依赖；其中 `github.com/pkg/errors` 为 **wails 内部传递依赖**（`wails/.../dispatcher`），本项目代码不直接 import |

不引入 `golang.org/x/sys/windows`：Windows 隐藏/系统属性与创建时间可经
`fi.Sys().(*syscall.Win32FileAttributeData)` 用标准库读取（Phase 0 V2/V3 验证）。

### 4.2 前端 npm（`frontend/package.json` + `package-lock.json` 已入库）

| 依赖 | 版本 | 类型 |
|---|---|---|
| `vue` | ^3.5.0 | runtime |
| `vue-router` | ^4.6.4 | runtime（裁决 D-A5：承载 requirements §九 的 4 条路由） |
| `vite` | ^7.0.0 | dev |
| `@vitejs/plugin-vue` | ^6.0.0 | dev |
| `typescript` | ^5.6.3 | dev |
| `vue-tsc` | ^2.1.0 | dev（构建前类型检查） |
| `tailwindcss` + `@tailwindcss/vite` | ^4.3.3 | dev（Tailwind v4，经 Vite 插件接入） |

> npm 依赖不受 Go 白名单约束，但版本变更需同步更新本表（P1-9）。

---

## 5. 已知环境问题

**`-race` 测试必须带 `-ldflags=-linkmode=external`（Makefile 已内置）。**

Go 1.25.4 + 本机 MinGW gcc 7.3.0 下，race 测试二进制走**内部链接**会在进程启动时以
`0xC0000139 (STATUS_ENTRYPOINT_NOT_FOUND)` 静默退出——仅含标准库的测试包同样复现，
与依赖无关；改用外部链接后全部通过。完整现象、二分过程与复现步骤见
[`development/00-phase-0-prerequisites.md`](development/00-phase-0-prerequisites.md) V8。

若其他机器出现同样问题：优先升级 MinGW-w64（而非依赖该 flag）。

---

## 6. 阶段与验收

阶段计划、任务分解、契约与测试策略见 [`development/README.md`](development/README.md)。

Phase 0 已执行的验证（均在本机真实执行）：

| 验证 | 命令 | 结果 |
|---|---|---|
| 构建 | `wails build` | ✅ `build/bin/file-pilot.exe`（20.8s） |
| 无 CGO 构建 | `CGO_ENABLED=0 go build .` | ✅ 5.89 MB |
| SQLite 无 CGO 可用 | `CGO_ENABLED=0 go test -run TestPhase0V7 -v .` | ✅ PASS |
| 静态检查 | `go vet ./...` | ✅ 无告警 |
| 格式化 | `gofmt -l .` | ✅ 无输出 |
| 并发检测 | `go test -race -ldflags=-linkmode=external ./...` | ✅ PASS（见 §5） |

### 6.1 v1 交付验证（2026-10-15，Phase 1–3 完成）

| 验证 | 命令 | 结果 |
|---|---|---|
| 构建 | `wails build` | ✅ `build/bin/file-pilot.exe` |
| 绑定重生成 | `wails generate module` | ✅ 含 14 个绑定方法 |
| 前端类型检查 + 构建 | `npm run build` | ✅ `vue-tsc --noEmit && vite build` |
| 单元测试 | `go test ./...` | ✅ 全部通过 |
| 并发检测 | `go test -race -ldflags=-linkmode=external ./...` | ✅ 全部通过 |
| 静态检查 / 格式 | `go vet ./...` / `gofmt -l .` | ✅ 无告警 / 无输出 |
| 覆盖率（核心 ≥70%） | `go test -cover ./...` | ✅ scanner 89.6 · task 93.4 · classifier 93.3 · organizer 83.1 |
| 覆盖率（工具 ≥85%） | 同上 | ✅ fsx 94.4 · storage 85.0 · units 100 |

> 人工 GUI 交互验收按 [`development/test-and-acceptance.md`](development/test-and-acceptance.md) §6；`file-pilot.exe` 体验验证需真实图形窗口。

---

## 7. 打包与分发

产物为**单文件** `build/bin/file-pilot.exe`：无 CGO（纯 Go SQLite）、内嵌前端静态资源与 WebView2Loader，Windows 下可直接双击运行、可整体拷贝分发。

### 7.1 一键打包（推荐）

在 `projects/file-pilot/` 目录下执行：

```powershell
.\build.ps1            # 完整流程：Go 测试 → 前端构建 → wails build → 产物校验
.\build.ps1 -SkipTests # 跳过 Go 测试（仅重新打包时用）
```

脚本依次执行：前置检查（`go`/`node`/`npm`/`wails` 是否在 PATH）→ 可选 `go test ./...` → `npm run build`（前端类型检查 + 构建）→ `wails build` → 校验产物存在并打印路径与大小。任一步失败即中止并给出提示；脚本不改动 `go.mod`/`go.sum`，不提交 Git。

> 脚本以 **UTF-8 with BOM** 保存（PowerShell 5.1 正确解码中文脚本的必要条件）。请勿转存为无 BOM UTF-8，否则中文字符串会吞掉引号、破坏语法。

等价的 cmd 入口：`build.bat`（同样支持 `-SkipTests`；文件全英文以保证 cmd 代码页兼容）。

### 7.2 手动步骤

```powershell
npm run build          # 前端类型检查 + 构建（在 frontend/ 下）
wails build            # 产出 build/bin/file-pilot.exe
```

`make build` 等价于 `wails build`。

### 7.3 产物

| 项 | 值 |
|---|---|
| 路径 | `build/bin/file-pilot.exe`（`wails.json` 未指定 `outputdirectory`，走 wails v2 默认 `build/bin/`） |
| 类型 | 单文件、无 CGO（纯 Go SQLite + 内嵌 WebView2Loader） |
| 大小 | 约 16 MB（随依赖/资源变化） |
| 运行依赖 | WebView2 Runtime（随 Windows 更新） |

### 7.4 校验

```powershell
.\build\bin\file-pilot.exe   # 启动窗口；或仅检查文件存在与大小
```

> `build/bin/`、`frontend/dist/`、`frontend/node_modules/`、`coverage` 等产物 / 本地依赖已被 `.gitignore` 忽略，不会入库。
