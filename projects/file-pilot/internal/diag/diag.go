// Package diag 提供诊断日志（NFR-3、requirements §3.4.6）：JSON 结构化输出、
// ISO8601 时间戳、caller、error 级别堆栈；文件超过 10 MiB 滚动为 .log.old
// （保留 1 个）。
//
// 诊断日志与 UI 日志（contracts §3.9，面向用户）职责不同：这里记录的是不该
// 出现在界面上的细节（完整错误链、内部参数），用于现场排查；文件落在
// %APPDATA%\FilePilot\logs\ 下。
//
// 滚动逻辑在 RotateWriter 内自实现：依赖评审裁决不引入 lumberjack
// （AGENTS.md §31.1：标准库可解决则不新增依赖）。
package diag

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// DefaultLimit 是单个日志文件的大小上限：10 MiB（NFR-3：>10MB 滚动，保留 1 个 .old）。
const DefaultLimit int64 = 10 << 20

// DefaultFileName 是日志文件名。
const DefaultFileName = "file-pilot.log"

// DefaultDirName 是日志目录名，位于用户配置目录下。
const DefaultDirName = "logs"

// Log 是诊断日志入口：内嵌 *zap.Logger，调用点即 a.diag.Error(...)。
//
// 零值不可用；通过 Open（文件）或 NewStderr（降级）构造。
type Log struct {
	*zap.Logger
	w *RotateWriter // NewStderr 构造时为 nil
}

// DefaultDir 返回默认日志目录：<用户配置目录>/FilePilot/logs（requirements §3.4.6）。
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(base, "FilePilot", DefaultDirName), nil
}

// Open 打开（必要时创建）目录下的诊断日志文件。
//
// 任何失败都返回错误：调用方应降级到 NewStderr 并继续启动——日志能力缺失
// 不是应用不可恢复的错误（AGENTS.md §10）。
func Open(dir string) (*Log, error) {
	if dir == "" {
		return nil, errors.New("diag: empty log dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir %q: %w", dir, err)
	}

	w, err := newRotateWriter(filepath.Join(dir, DefaultFileName), DefaultLimit)
	if err != nil {
		return nil, err
	}
	return &Log{Logger: newLogger(w), w: w}, nil
}

// NewStderr 返回退化实现：输出到 stderr，不落文件、不滚动。
//
// 用于日志文件不可用时的降级路径：诊断能力保留（stderr 可见/可重定向），
// 但不会因目录权限等问题阻断启动。
func NewStderr() *Log {
	return &Log{Logger: newLogger(zapcore.AddSync(os.Stderr))}
}

// newLogger 组装 JSON 输出核心：ISO8601 时间（时间戳必须可被工具解析）、
// caller 位置、error 级别堆栈（NFR-3）。
func newLogger(ws zapcore.WriteSyncer) *zap.Logger {
	enc := zap.NewProductionEncoderConfig()
	enc.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(zapcore.NewJSONEncoder(enc), ws, zapcore.InfoLevel)
	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

// Close 落盘并关闭日志文件；可安全重复调用（RotateWriter.Close 幂等）。
func (l *Log) Close() error {
	if l == nil {
		return nil
	}
	// 先 Sync 再 Close：Close 之后 Sync 会拿到无效句柄。
	syncErr := l.Logger.Sync()
	if l.w == nil {
		return syncErr
	}
	return errors.Join(syncErr, l.w.Close())
}

// RotateWriter 是按大小滚动的文件写入器。
//
// 当写入会使文件超过 limit 时（且文件非空），把当前文件改名为
// <path>.old（覆盖旧 .old，仅保留 1 个）并新建空文件继续写。
// 滚动失败时返回错误，绝不静默丢弃日志（AGENTS.md §9.1）。
type RotateWriter struct {
	mu    sync.Mutex
	path  string
	limit int64
	file  *os.File
	size  int64
}

// newRotateWriter 打开日志文件；size 从既有文件长度开始，重启后沿用同一上限。
func newRotateWriter(path string, limit int64) (*RotateWriter, error) {
	if path == "" {
		return nil, errors.New("diag: empty log path")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("diag: invalid rotation limit %d", limit)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		// 打开成功但取不到长度：无法维持滚动上限，宁可失败也不假装正常。
		_ = file.Close()
		return nil, fmt.Errorf("stat log file %q: %w", path, err)
	}
	return &RotateWriter{path: path, limit: limit, file: file, size: info.Size()}, nil
}

// Write 实现 io.Writer：超限时先滚动再写。
func (w *RotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return 0, fmt.Errorf("diag: log %q closed", w.path)
	}
	if w.size > 0 && w.size+int64(len(p)) > w.limit {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return n, fmt.Errorf("write log %q: %w", w.path, err)
	}
	return n, nil
}

// rotateLocked 关闭当前文件、改名到 .old 并新建空文件（调用方持锁）。
func (w *RotateWriter) rotateLocked() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close log for rotation %q: %w", w.path, err)
	}

	old := w.path + ".old"
	// os.Rename 在 Windows 上以 MOVEFILE_REPLACE_EXISTING 覆盖已存在的 .old，
	// 该行为在 Phase 0 已实机验证，因此无需先 Remove。
	if err := os.Rename(w.path, old); err != nil {
		// 改名失败：重开原文件继续写，保住日志通道后再上报错误。
		if reopenErr := w.reopenLocked(); reopenErr != nil {
			return errors.Join(fmt.Errorf("rotate log %q: %w", w.path, err), reopenErr)
		}
		return fmt.Errorf("rotate log %q: %w", w.path, err)
	}
	return w.reopenLocked()
}

// reopenLocked 打开（或新建）日志文件并把 size 归零（调用方持锁）。
func (w *RotateWriter) reopenLocked() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("reopen log %q: %w", w.path, err)
	}
	w.file = file
	w.size = 0
	return nil
}

// Sync 落盘（zapcore.Syncer）。
func (w *RotateWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("sync log %q: %w", w.path, err)
	}
	return nil
}

// Close 关闭日志文件；幂等（二次调用为 no-op）。
func (w *RotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return fmt.Errorf("close log %q: %w", w.path, err)
	}
	return nil
}
