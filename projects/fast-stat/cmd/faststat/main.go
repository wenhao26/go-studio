// Command faststat 统计本地目录的目录数、文件数与总大小。
//
// 本命令是 fast-stat 项目的组合根：只负责依赖组装、信号处理与进程退出，
// 不包含任何统计或渲染逻辑（分别位于 internal/scan 与 internal/report）。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/cli"
)

// defaultTerminalWidth 是探测不到终端列宽时使用的默认值。
//
// 与 internal/cli 中的默认值保持一致；这里重复声明是为了让 main 不必暴露
// 内部包常量，代价只是两个常量需要同步。
const defaultTerminalWidth = 120

func main() {
	os.Exit(run())
}

// run 承载 main 的主体逻辑。
//
// 单独抽出是为了让 defer（日志刷新、信号注销）在 os.Exit 之前执行：
// os.Exit 会跳过所有 defer，直接写在 main 里会让清理逻辑失效。
func run() int {
	logLevel := zap.NewAtomicLevelAt(zap.WarnLevel)

	logger, err := newLogger(logLevel)
	if err != nil {
		// 日志系统自身不可用，此时只能直接写 stderr。
		fmt.Fprintf(os.Stderr, "faststat: init logger: %v\n", err)
		return cli.ExitError
	}
	// 退出前尽量刷新缓冲。stderr 上的 Sync 常返回 "invalid argument"，
	// 属于已知无害情况，因此显式忽略。
	defer func() { _ = logger.Sync() }()

	// 把取消权交给信号：Ctrl-C / SIGTERM 会取消 ctx，扫描随即优雅停止并
	// 输出已统计到的部分结果。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := cli.New(cli.Config{
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		Logger:           logger,
		LogLevel:         &logLevel,
		StdoutIsTerminal: isTerminal(os.Stdout),
		TerminalWidth:    terminalWidth(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "faststat: %v\n", err)
		return cli.ExitError
	}

	return app.Run(ctx, os.Args[1:])
}

// newLogger 构造写入 stderr 的控制台风格 zap logger。
//
// 采用 console 编码而非 JSON：本工具是人机交互的 CLI，诊断输出的可读性优先；
// 机器可读的结构化输出由 --format json 的报告负责。日志不写 stdout，
// 以免污染可被管道解析的报告。
func newLogger(level zap.AtomicLevel) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Level = level
	cfg.Encoding = "console"
	cfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	cfg.OutputPaths = []string{"stderr"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	// CLI 的运行期错误已有明确的人类可读描述，堆栈对使用者没有价值。
	cfg.DisableStacktrace = true

	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return logger, nil
}

// isTerminal 判断文件是否连接到终端。
//
// IsCygwinTerminal 覆盖 Git Bash / MSYS2 等 Windows 上的终端环境，
// 这些场景下 IsTerminal 会返回 false。
func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// terminalWidth 尽力探测终端列宽。
//
// 这里不引入 golang.org/x/term（不在依赖白名单内），因此优先读取 COLUMNS
// 环境变量；探测失败时回退到默认宽度。该值只影响进度行的截断长度，
// 探测不准不会影响统计结果的正确性。
func terminalWidth() int {
	if v, ok := os.LookupEnv("COLUMNS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultTerminalWidth
}
