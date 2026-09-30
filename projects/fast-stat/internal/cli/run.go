// Package cli 是 fast-stat 的 Transport 层。
//
// 职责：解析并校验命令行参数、驱动扫描、渲染进度（stderr）与最终报告（stdout）、
// 把各类失败映射为进程退出码。
//
// 明确不做的事：不直接遍历文件系统（交给 internal/scan）、不做统计聚合、
// 不实现任何业务口径判断。
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/fsx"
	"github.com/wenhao26/go-studio/projects/fast-stat/internal/report"
	"github.com/wenhao26/go-studio/projects/fast-stat/internal/scan"
	"github.com/wenhao26/go-studio/projects/fast-stat/internal/version"
)

// programName 是命令行中显示的程序名，同时决定 cmd 子目录名。
const programName = "faststat"

// 进程退出码。
const (
	// ExitOK 表示扫描完成且报告已成功输出（可能包含被跳过的错误）。
	ExitOK = 0

	// ExitError 表示运行期失败：根路径不可用、输出写入失败，或被用户取消。
	ExitError = 1

	// ExitUsage 表示命令行用法错误。
	ExitUsage = 2
)

// defaultTerminalWidth 是探测不到终端宽度时用于截断进度行的默认列宽。
const defaultTerminalWidth = 120

// Format 是报告输出格式。
type Format string

const (
	// FormatText 输出带边框的终端文本报告（默认）。
	FormatText Format = "text"

	// FormatJSON 输出机器可读的 JSON 报告。
	FormatJSON Format = "json"
)

// Options 是解析并校验后的调用配置，构造后视为只读。
type Options struct {
	// Path 是要统计的根目录，已做 filepath.Clean。
	Path string

	// Format 是输出格式。
	Format Format

	// Output 是报告输出文件路径；为空表示输出到 stdout。
	Output string

	// Threads 是并发遍历的 worker 数量；0 表示自动（min(NumCPU, 16)）。
	Threads int

	// Verbose 控制是否输出诊断区（生效参数与错误明细）。
	Verbose bool

	// NoEmoji 为 true 时用纯文本标签替换报告中的 emoji 图标。
	NoEmoji bool

	// Plain 为 true 时省略报告边框，仅输出文本行。
	Plain bool

	// Version 为 true 时只打印版本信息。
	Version bool
}

// Config 是构造 App 所需的运行时依赖。
type Config struct {
	// Stdout 接收最终报告。
	Stdout io.Writer

	// Stderr 接收进度渲染与诊断输出。
	Stderr io.Writer

	// Logger 是结构化诊断日志；为 nil 时退化为不输出。
	Logger *zap.Logger

	// LogLevel 非 nil 时，开启 --verbose 会把日志级别提升为 Info。
	LogLevel *zap.AtomicLevel

	// StdoutIsTerminal 指示 Stdout 是否连接到终端。
	//
	// 为 false 时禁用进度渲染：管道、重定向与 CI 环境下进度行会污染
	// 输出与日志文件。
	StdoutIsTerminal bool

	// TerminalWidth 是进度行截断使用的终端列宽；<=0 时使用默认值。
	TerminalWidth int
}

// App 封装一次 fast-stat 进程的运行上下文。
type App struct {
	stdout   io.Writer
	stderr   io.Writer
	logger   *zap.Logger
	logLevel *zap.AtomicLevel
	tty      bool
	width    int

	fsys fsx.FileSystem
}

// New 构造 App。Stdout 与 Stderr 必须非 nil。
func New(cfg Config) (*App, error) {
	if cfg.Stdout == nil {
		return nil, errors.New("cli: config Stdout must not be nil")
	}
	if cfg.Stderr == nil {
		return nil, errors.New("cli: config Stderr must not be nil")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	width := cfg.TerminalWidth
	if width <= 0 {
		width = defaultTerminalWidth
	}

	return &App{
		stdout:   cfg.Stdout,
		stderr:   cfg.Stderr,
		logger:   logger,
		logLevel: cfg.LogLevel,
		tty:      cfg.StdoutIsTerminal,
		width:    width,
		fsys:     fsx.OSFileSystem{},
	}, nil
}

// Run 执行一次完整的调用并返回进程退出码。
//
// 本方法不会调用 os.Exit，进程退出由 main 负责，因此可以完整地被测试覆盖。
// args 不包含程序名本身。
func (a *App) Run(ctx context.Context, args []string) int {
	if ctx == nil {
		ctx = context.Background()
	}

	opts, err := Parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		_, _ = fmt.Fprint(a.stdout, Usage())
		return ExitOK
	case err != nil:
		// 用法错误属于「用户与程序约定不符」，用纯文本输出到 stderr，
		// 不走结构化日志：这是一条给人看的提示，不是运行期诊断。
		_, _ = fmt.Fprintf(a.stderr, "%s: %v\n\n%s", programName, err, Usage())
		return ExitUsage
	}

	if opts.Version {
		_, _ = fmt.Fprintf(a.stdout, "%s %s\n", programName, version.String())
		return ExitOK
	}

	if opts.Verbose && a.logLevel != nil {
		a.logLevel.SetLevel(zap.InfoLevel)
	}

	return a.scanAndReport(ctx, opts)
}

// scanAndReport 驱动扫描并把结果渲染到目标位置。
func (a *App) scanAndReport(ctx context.Context, opts Options) int {
	// 并发度在 CLI 层解析出确定值后再下传，这样报告与日志展示的就是实际生效的数值，
	// 而不是「请求值」或二次推导值。
	workers := opts.Threads
	if workers == 0 {
		workers = scan.DefaultWorkers()
	}

	a.logger.Info("scan starting",
		zap.String("root", opts.Path),
		zap.String("format", string(opts.Format)),
		zap.Int("workers", workers),
	)

	scanner := scan.New(a.fsys)
	stopProgress := a.startProgress(scanner, opts)

	// 不加超时：目录统计的耗时由数据规模决定，设置固定的 deadline 只会让
	// 大目录必然失败。取消权交给上游的 ctx（信号）。
	snapshot, scanErr := scanner.Scan(ctx, scan.Options{Root: opts.Path, Workers: workers})

	// 先停掉进度渲染，再输出报告，避免两者交叉写坏终端。
	stopProgress()

	// 取消与致命错误必须区分：取消时统计结果是部分结果但仍可展示，
	// 而根路径不可用时没有任何可信结果可报。
	canceled := errors.Is(scanErr, context.Canceled)
	if scanErr != nil && !canceled {
		a.logger.Error("scan failed",
			zap.String("root", opts.Path),
			zap.Error(scanErr),
		)
		_, _ = fmt.Fprintf(a.stderr, "%s: %v\n", programName, scanErr)
		return ExitError
	}

	status := report.StatusCompleted
	if canceled {
		status = report.StatusCanceled
		a.logger.Warn("scan canceled",
			zap.String("root", opts.Path),
			zap.Int64("files_found", snapshot.FilesFound),
		)
	}

	in := report.Input{
		TargetPath:   opts.Path,
		Status:       status,
		Snapshot:     snapshot,
		ToolVersion:  version.Version,
		Workers:      workers,
		Verbose:      opts.Verbose,
		ASCIISymbols: opts.NoEmoji,
		PlainLayout:  opts.Plain,
	}

	payload, err := render(in, opts.Format)
	if err != nil {
		a.logger.Error("render report failed", zap.Error(err))
		_, _ = fmt.Fprintf(a.stderr, "%s: render report: %v\n", programName, err)
		return ExitError
	}

	if err := a.writeReport(opts.Output, payload); err != nil {
		a.logger.Error("write report failed",
			zap.String("output", opts.Output),
			zap.Error(err),
		)
		_, _ = fmt.Fprintf(a.stderr, "%s: %v\n", programName, err)
		return ExitError
	}

	a.logger.Info("scan finished",
		zap.Int64("directories_visited", snapshot.DirectoriesVisited),
		zap.Int64("files_found", snapshot.FilesFound),
		zap.Int64("skipped_errors", snapshot.SkippedErrors),
	)

	if canceled {
		return ExitError
	}
	return ExitOK
}

// render 按输出格式渲染报告。
func render(in report.Input, format Format) ([]byte, error) {
	if format == FormatJSON {
		return report.JSON(in)
	}
	return []byte(report.Text(in)), nil
}

// writeReport 把报告写入文件或 stdout。
func (a *App) writeReport(output string, payload []byte) error {
	if output == "" {
		if _, err := a.stdout.Write(payload); err != nil {
			return fmt.Errorf("write report to stdout: %w", err)
		}
		return nil
	}
	if err := fsx.WriteFileAtomic(output, payload, fsx.DefaultFilePerm); err != nil {
		return fmt.Errorf("write report to %q: %w", output, err)
	}
	return nil
}

// startProgress 按需启动进度渲染，并返回停止函数。
//
// 进度只在 stdout 是终端且输出为文本时启用：JSON 需要 stdout 可被直接解析，
// 而进度行即使写往 stderr，也会在 CI 日志中制造噪音。
func (a *App) startProgress(scanner *scan.Scanner, opts Options) func() {
	if !a.tty || opts.Format != FormatText {
		return func() {}
	}

	renderer := newProgressRenderer(a.stderr, a.width)
	stop := make(chan struct{})
	done := make(chan struct{})
	ticker := time.NewTicker(progressInterval)

	a.logger.Debug("progress renderer enabled", zap.Int("width", a.width))

	go func() {
		defer close(done)
		// ticker 必须显式 Stop，否则会持续持有资源直到进程退出。
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				renderer.update(now, scanner.Progress())
			}
		}
	}()

	return func() {
		close(stop)
		// 等待渲染 goroutine 退出，确保进度行不会与最终报告交叉输出。
		<-done
		renderer.finish()
	}
}

// Parse 解析命令行参数并返回校验后的配置。
//
// 语义（与 PRD 示例及项目决策一致）：
//   - --path 优先于位置参数；两者都缺省时统计当前目录 "."；
//   - 位置参数最多一个，多给视为用法错误；
//   - --format 仅接受 text 与 json；
//   - -h/--help 返回 flag.ErrHelp，由调用方决定退出码。
//
// 这里不使用 flag 自带的错误输出：用法文本由 Usage 统一维护，
// 避免帮助信息出现两套措辞。
func Parse(args []string) (Options, error) {
	flags := flag.NewFlagSet(programName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var (
		pathFlag    = flags.String("path", "", "要统计的目录路径（优先于位置参数）")
		formatFlag  = flags.String("format", string(FormatText), "输出格式：text | json")
		outputFlag  = flags.String("output", "", "报告输出文件路径（缺省输出到 stdout）")
		threadsFlag = flags.Int("threads", 0, "并发遍历的 worker 数量；0 表示自动")
		verboseFlag = flags.Bool("verbose", false, "输出诊断信息（生效参数与错误明细）")
		noEmojiFlag = flags.Bool("no-emoji", false, "用纯文本标签替换 emoji 图标")
		plainFlag   = flags.Bool("plain", false, "省略报告边框，仅输出文本行")
		versionFlag = flags.Bool("version", false, "打印版本信息后退出")
	)

	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}

	rest := flags.Args()
	if len(rest) > 1 {
		return Options{}, fmt.Errorf("最多接受一个位置参数，收到 %d 个: %v", len(rest), rest)
	}

	opts := Options{
		Format:  Format(*formatFlag),
		Output:  *outputFlag,
		Threads: *threadsFlag,
		Verbose: *verboseFlag,
		NoEmoji: *noEmojiFlag,
		Plain:   *plainFlag,
		Version: *versionFlag,
	}

	switch {
	case *pathFlag != "":
		opts.Path = *pathFlag
	case len(rest) == 1:
		opts.Path = rest[0]
	default:
		opts.Path = "."
	}
	// 路径来自外部输入，进入遍历层前先规范化（security.md §44：输入不可信）。
	opts.Path = filepath.Clean(opts.Path)

	switch opts.Format {
	case FormatText, FormatJSON:
	default:
		return Options{}, fmt.Errorf("无效的 --format 值 %q：仅支持 %s | %s",
			*formatFlag, FormatText, FormatJSON)
	}

	// 并发度在 CLI 边界就校验，而不是留给遍历层静默钳制：
	// 用户传了明显错误的数值时应当立即知道，而不是拿到一份「参数被悄悄改过」的报告。
	if opts.Threads < 0 || opts.Threads > scan.MaxWorkers {
		return Options{}, fmt.Errorf("--threads 必须在 0（自动）到 %d 之间，收到 %d",
			scan.MaxWorkers, opts.Threads)
	}

	// --plain 与 JSON 相互矛盾：JSON 报告本身没有排版。静默忽略会让用户
	// 以为自己指定了纯文本输出。
	if opts.Plain && opts.Format == FormatJSON {
		return Options{}, errors.New("--plain 与 --format json 不能同时使用：JSON 报告不带排版")
	}

	return opts, nil
}

// Usage 返回命令行用法说明。
//
// 其中「统计口径」一节是刻意的：口径歧义（逻辑大小 vs 磁盘占用、是否跟随
// 符号链接、是否去重硬链接）是同类工具最容易产生争议的地方，必须在帮助中显式声明。
func Usage() string {
	return programName + ` — 高性能本地目录统计工具

用法:
  ` + programName + ` [路径] [选项]

选项:
  --path <dir>      要统计的目录路径（优先于位置参数；缺省为当前目录）
  --format <fmt>    输出格式：text | json（默认 text）
  --output <file>   把报告写入文件（缺省输出到 stdout）
  --threads <n>     并发遍历的 worker 数量；0 表示自动（min(NumCPU, 16)，上限 ` + strconv.Itoa(scan.MaxWorkers) + `）
  --verbose         输出诊断信息（生效参数与错误明细）
  --no-emoji        用纯文本标签替换 emoji 图标（终端 emoji 宽度异常时使用）
  --plain           省略报告边框，仅输出文本行（便于写入日志；不可与 json 同用）
  --version         打印版本信息后退出
  -h, --help        显示本帮助

退出码:
  0  扫描完成（可能包含被跳过的错误）
  1  运行失败（根路径不可用、报告写入失败、被用户取消）
  2  命令行用法错误

统计口径:
  扫描次数  成功完整读取的目录节点数，含根目录；读取失败的目录不计入
  文件个数  普通文件与特殊文件（FIFO/socket/设备）的数量
            不含目录本身；不跟随、也不计入符号链接与目录联接(junction)
  路径总大小 文件逻辑大小（st_size）之和，非磁盘占用；硬链接不去重
  跳过错误数 因无权限等原因读取失败的路径数

注意:
  本工具按 1024 基数换算并显示为 KB/MB/GB 字样；
  因不跟随符号链接、且不对硬链接去重，统计结果与 du -sh 不保证一致。
`
}
