// Package main 是 FilePilot 的组合根：装配依赖、注册 Wails 绑定与事件、
// 管理应用生命周期（业务编排在 internal/app，见其包注释）。
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/app"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/diag"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := run(); err != nil {
		// 启动阶段的不可恢复错误：明示原因后退出（AGENTS.md §10、§65）。
		fmt.Fprintf(os.Stderr, "FilePilot 启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 组合根是调用链起点：Wails 的 OnStartup 之前没有可用 ctx，这里创建的
	// 根 ctx 只用于启动阶段的数据库打开与迁移，业务链路一律使用 Wails 注入的
	// 生命周期 ctx（AGENTS.md §8.3）。
	ctx := context.Background()

	log := openDiag()

	application, err := newApp(ctx, log)
	if err != nil {
		// 装配失败：释放诊断日志（errors.Join 保留两段信息），由 main 报错退出。
		return errors.Join(err, log.Close())
	}
	// wails.Run 返回后兜底释放：正常路径 OnShutdown 已释放，Close 幂等。
	defer application.Close()

	return wails.Run(&options.App{
		Title:  "FilePilot",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        application.Startup,
		OnShutdown:       application.Shutdown,
		// 只绑定契约方法面：App 的生命周期与事件出口方法不得暴露给前端（contracts §5）。
		Bind: []any{app.NewAPI(application)},
	})
}

// openDiag 打开诊断日志；任一步失败时降级为 stderr 输出。
//
// 降级理由：诊断日志是可观测性能力而非业务正确性依赖（NFR-3），
// 日志目录不可写不应让应用无法启动（AGENTS.md §10）。
func openDiag() *diag.Log {
	dir, err := diag.DefaultDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FilePilot: 定位日志目录失败，诊断日志降级为 stderr: %v\n", err)
		return diag.NewStderr()
	}
	log, err := diag.Open(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FilePilot: 打开诊断日志失败，降级为 stderr: %v\n", err)
		return diag.NewStderr()
	}
	return log
}

// newApp 打开并迁移数据库，装配 App；任一步失败即启动失败
// （数据库不可用属于不可恢复错误，明示原因后退出）。
func newApp(ctx context.Context, log *diag.Log) (*app.App, error) {
	dbPath, err := storage.DefaultPath()
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	store, err := storage.Open(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		// 迁移失败：先关闭已打开的数据库再上抛，避免句柄泄漏。
		return nil, errors.Join(err, store.Close())
	}
	return app.New(store, log), nil
}
