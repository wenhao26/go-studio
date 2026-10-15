package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/task"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/units"
)

// runScan 驱动一次扫描到终态：节流器 → 扫描 → 终态判定 → 收尾日志 → 落库。
//
// goroutine 契约（AGENTS.md §17）：只在 StartScan 内启动；生命周期由
// Manager.Finish（释放单活动约束）+ defer h.Close()（唤醒 Shutdown 等待）
// 收敛，取消信号经 h.Context() 进入扫描器，不存在无主等待。
func (a *App) runScan(h *task.Handle, cfg scanner.Config) {
	defer h.Close()

	th := task.NewThrottler(h.ID, h.StartedAt(), a, func() task.ProgressUpdate {
		s := a.scanner.Progress()
		return task.ProgressUpdate{Scanned: s.Files, Skipped: s.Skipped, CurrentDir: s.CurrentDir}
	})
	th.Start()
	th.Log(scanner.LogSystem, fmt.Sprintf("扫描任务启动: %s (workers=%d)", h.Root, cfg.Workers))

	res, scanErr := a.scanner.Scan(h.Context(), scanner.Options{
		Root:   h.Root,
		ScanID: h.ScanID,
		Config: cfg,
		Files:  a.store,
		Logs:   th,
	})

	// 先停节流器：Stop 会把窗口内剩余日志发出；此后 finalLogs 直接推送，
	// 不再受窗口采样影响（裁决 D-A7/D-A8：跳过明细必须完整呈现）。
	th.Stop()

	to := task.StatusCompleted
	switch {
	case scanErr == nil:
	case errors.Is(scanErr, context.Canceled):
		to = task.StatusCanceled
	default:
		to = task.StatusFailed
	}

	// 收尾日志先于 task:status 推送：前端收到终态事件后切换页面，
	// 日志区此时应已包含汇总与跳过明细。
	a.emitFinalLogs(h.ID, res, to, scanErr)

	stats := task.ScanStats{
		Duration:       res.Duration,
		FileCount:      res.FileCount,
		DirCount:       res.DirCount,
		TotalSizeBytes: res.TotalSizeBytes,
		SkippedCount:   res.Skipped,
	}
	if err := a.tasks.Finish(h.ID, to, scanErr, stats); err != nil {
		a.diag.Error("finish scan task", zap.Int64("task_id", h.ID), zap.Error(err))
	}

	// 诊断日志（NFR-3）：失败/取消必须可追溯，完整错误链只留在这里。
	switch to {
	case task.StatusFailed:
		a.diag.Error("scan task failed",
			zap.Int64("task_id", h.ID), zap.String("root", h.Root), zap.Error(scanErr))
	case task.StatusCanceled:
		a.diag.Warn("scan task canceled",
			zap.Int64("task_id", h.ID), zap.String("root", h.Root))
	}
}

// emitFinalLogs 推送扫描收尾日志：一行汇总 + 跳过明细（≤20 条）+ 截断披露。
//
// 明细条数上限由 scanner 保证（maxSkipDetails=20）；被截断的剩余跳过数
// 显式披露，不让「明细不全」变成静默事实（FR-5 展示于日志区）。
func (a *App) emitFinalLogs(taskID int64, res scanner.Result, to task.Status, scanErr error) {
	now := time.Now().UTC().UnixMilli()
	entry := func(kind scanner.LogKind, text string) scanner.LogEntry {
		return scanner.LogEntry{TS: now, Kind: kind, Text: text}
	}

	entries := make([]scanner.LogEntry, 0, len(res.Details)+2)
	switch to {
	case task.StatusCompleted:
		entries = append(entries, entry(scanner.LogSystem, fmt.Sprintf(
			"扫描完成: %d 文件 / %d 目录 / 共 %s / 跳过 %d",
			res.FileCount, res.DirCount, units.FormatBytes(res.TotalSizeBytes), res.Skipped)))
	case task.StatusCanceled:
		entries = append(entries, entry(scanner.LogSystem, fmt.Sprintf(
			"扫描已取消: 部分结果 %d 文件 / %d 目录",
			res.FileCount, res.DirCount)))
	case task.StatusFailed:
		entries = append(entries, entry(scanner.LogSystem, fmt.Sprintf("扫描失败: %v", scanErr)))
	}

	for _, d := range res.Details {
		entries = append(entries, entry(scanner.LogScan, fmt.Sprintf("跳过: %s · %s", d.Path, d.Reason)))
	}

	if truncated := res.Skipped - int64(len(res.Details)); truncated > 0 {
		entries = append(entries, entry(scanner.LogSystem, fmt.Sprintf(
			"跳过明细仅保留前 %d 条，其余 %d 条未展示", len(res.Details), truncated)))
	}

	a.EmitLogs(taskID, entries)
}
