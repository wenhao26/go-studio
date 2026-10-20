package app

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/classifier"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/diag"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/fsx"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/organizer"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/task"
)

// retainedScans 是保留的最近扫描次数（FR-13：超出自动清理历史扫描级联数据）。
const retainedScans = 20

// shutdownGrace 是优雅退出时等待活动任务收尾的上限（NFR-2）。
//
// NFR-2 要求 500ms 内开始收尾（CancelActive 立即触发，满足）；这里是等待
// runner 完成「尾批落库 + 终态写入」的硬上限，3s 覆盖机械盘最坏情况下的
// 单事务写入；超时后不强杀，未收敛的行由下次启动的 Recover 修正（FR-12）。
const shutdownGrace = 3 * time.Second

// App 是 Wails 绑定对象与生命周期宿主：装配 storage/task/scanner/diag，
// 并实现 task.Emitter（事件出口，events.go）。
//
// ctx 是 Wails 注入的生命周期 ctx（OnStartup 收到、OnShutdown 结束）：
// 启动/关闭时写、事件路径只读，用 RWMutex 保护而不依赖调用时序假设。
type App struct {
	store     *storage.Store
	diag      *diag.Log
	scanner   *scanner.Scanner
	tasks     *task.Manager
	organizer *organizer.Organizer

	mu        sync.RWMutex
	ctx       context.Context
	recovery  *RecoveryNotice // 启动恢复结果（FR-12），无恢复时为 nil
	closeOnce sync.Once

	// publish 是事件发布的注入点：生产实现为 publishEvent（Wails 运行时广播），
	// 测试注入记录器——Wails 运行时不在场时 EventsEmit 会 panic。
	publish func(name string, payload any)

	// openFolder 是打开目录的注入点：生产实现为 openFolderNative（资源管理器），
	// 测试注入记录器——单测不得真的弹出系统窗口。
	openFolder func(dir string) error
}

// New 装配 App。
//
// logger 为 nil 时降级为 stderr：诊断日志缺失不应导致后续路径空指针，
// 但调用方（main）通常已自行处理 Open 失败。
func New(store *storage.Store, logger *diag.Log) *App {
	if logger == nil {
		logger = diag.NewStderr()
	}
	a := &App{
		store:     store,
		diag:      logger,
		scanner:   scanner.New(fsx.OSFileSystem{}, classifier.New()),
		organizer: organizer.New(fsx.OSFileSystem{}, store),
	}
	// App 实现 task.Emitter：任务层的状态/进度/日志事件经此出口发布。
	a.tasks = task.NewManager(storeAdapter{store: store}, a)
	// 事件发布的默认实现走 Wails 运行时；测试注入记录器（events.go）。
	a.publish = a.publishEvent
	// 打开目录的默认实现走系统资源管理器；测试注入记录器（storage.go）。
	a.openFolder = openFolderNative
	return a
}

// Startup 在窗口就绪后执行：记录生命周期 ctx、异常恢复、历史清理。
func (a *App) Startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()

	// FR-12：上次异常退出遗留的 pending/running 任务与 running 扫描行置 failed。
	// 恢复结果留存在内存中，供前端启动时经 GetRecoveryNotice 取用并提示用户。
	tasks, scans, err := a.tasks.Recover(ctx)
	if err != nil {
		a.diag.Error("recover interrupted tasks", zap.Error(err))
	} else if tasks > 0 || scans > 0 {
		a.mu.Lock()
		a.recovery = &RecoveryNotice{RecoveredTasks: tasks, RecoveredScans: scans}
		a.mu.Unlock()
		a.diag.Warn("recovered interrupted tasks", zap.Int64("tasks", tasks), zap.Int64("scans", scans))
	}

	// FR-13：保留最近 retainedScans 次扫描，超出的级联清理。
	removed, err := a.store.PruneOldScans(ctx, retainedScans)
	if err != nil {
		a.diag.Error("prune old scans", zap.Error(err))
	} else if removed > 0 {
		a.diag.Info("pruned old scans", zap.Int64("removed", removed))
	}
}

// Shutdown 优雅退出（AGENTS.md §62）：取消活动任务、等待收尾、释放资源。
func (a *App) Shutdown(ctx context.Context) {
	a.tasks.CancelActive()
	if !a.tasks.Wait(shutdownGrace) {
		// 超时说明 runner 仍在收尾：此时关库会制造二次错误，进程随后退出，
		// 未完成的终态写入由下次启动的 Recover 收敛（FR-12），故只记录。
		a.diag.Error("graceful shutdown timed out", zap.Duration("grace", shutdownGrace))
		return
	}
	a.Close()
}

// Close 释放数据库与日志资源；幂等（Shutdown 与 main 兜底路径都会调用）。
func (a *App) Close() {
	a.closeOnce.Do(func() {
		// 顺序：先关数据库（可能仍被 Shutdown 等待中的写入使用），再关日志，
		// 保证关库产生的错误还能写进诊断日志。
		if err := a.store.Close(); err != nil {
			a.diag.Error("close database", zap.Error(err))
		}
		if err := a.diag.Close(); err != nil {
			// 诊断日志自身关闭失败：stderr 兜底，不再经日志通道。
			fmt.Fprintf(os.Stderr, "FilePilot: close diag log: %v\n", err)
		}
	})
}

// lifecycle 返回生命周期 ctx；未启动时返回 nil（事件出口据此跳过）。
func (a *App) lifecycle() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ctx
}

// requireLifecycle 返回生命周期 ctx；未就绪时报错而非伪造 context.Background
// （AGENTS.md §8.3：绑定调用一定发生在 OnStartup 之后，缺失说明状态异常）。
func (a *App) requireLifecycle() (context.Context, error) {
	ctx := a.lifecycle()
	if ctx == nil {
		return nil, errNotStarted
	}
	return ctx, nil
}
