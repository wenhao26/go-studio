package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/organizer"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/task"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/units"
)

// emitOrganizeFinalLog 在节流器 Stop 之后**直接**推送一条收尾系统日志。
//
// 必须绕过节流器：Throttler.Log 在 Stop 之后只入缓冲、不再冲刷（Stop 只做一次
// flush），继续用 th.Log 会静默丢日志。口径与 runScan 的 emitFinalLogs 一致。
func (a *App) emitOrganizeFinalLog(taskID int64, text string) {
	a.EmitLogs(taskID, []scanner.LogEntry{{
		TS:   time.Now().UTC().UnixMilli(),
		Kind: scanner.LogSystem,
		Text: text,
	}})
}

// organizeProgress 是整理/撤销任务的确定型进度快照源（D-D7）。
//
// 与扫描的「查询式」进度不同，整理的总数在执行前已知（整理=计划条数，
// 撤销=成功 move 条数），因此这里直接累计 done，无需读扫描器状态。
// 并发安全：OnItem 由 organizer 单 goroutine 调用，但 Throttler 的 tick
// 在另一 goroutine 读取，故用互斥锁保护。
type organizeProgress struct {
	mu      sync.Mutex
	total   int64
	done    int64
	current string
	cat     string
}

// newOrganizeProgress 构造进度源；total 为待处理总数。
func newOrganizeProgress(total int64) *organizeProgress {
	return &organizeProgress{total: total}
}

// observe 记录一条已处理项（成功或失败都计入 done）。
func (p *organizeProgress) observe(r organizer.ItemResult) {
	p.mu.Lock()
	p.done++
	p.current = r.SrcPath
	p.cat = r.Category
	p.mu.Unlock()
}

// snapshot 返回当前进度；TaskID/ElapsedMS/Rate 由 Throttler 统一填写。
func (p *organizeProgress) snapshot() task.ProgressUpdate {
	p.mu.Lock()
	defer p.mu.Unlock()
	percent := 0
	if p.total > 0 {
		percent = int(p.done * 100 / p.total)
	}
	return task.ProgressUpdate{
		Done:           p.done,
		Total:          p.total,
		Percent:        percent,
		CurrentFile:    p.current,
		TargetCategory: p.cat,
	}
}

// runOrganize 驱动一次整理到终态（FR-9）。
//
// goroutine 契约（AGENTS.md §17）：只在 StartOrganize 内启动；生命周期由
// Manager.Finish + defer h.Close() 收敛，取消信号经 h.Context() 进入 organizer。
//
// 日志口径（D-D5）：只用 kind=system，记录启动/收尾汇总与失败明细；
// 成功项不逐条记录（operations 表 + 报告即审计来源）。
func (a *App) runOrganize(h *task.Handle, plan organizer.Plan) {
	defer h.Close()

	prog := newOrganizeProgress(int64(len(plan.Items)))
	th := task.NewThrottler(h.ID, h.StartedAt(), a, prog.snapshot)
	th.Start()
	th.Log(scanner.LogSystem, fmt.Sprintf("整理任务启动: 目标 %s（共 %d 个文件）", plan.TargetRoot, len(plan.Items)))

	onItem := func(r organizer.ItemResult) {
		prog.observe(r)
		if r.Status == organizer.StatusFailed {
			th.Log(scanner.LogSystem, fmt.Sprintf("整理失败: %s · %s", r.SrcPath, r.Error))
		}
	}

	rep, runErr := a.organizer.Move(h.Context(), h.ID, plan, organizer.Hooks{OnItem: onItem})
	th.Stop()

	to := task.StatusCompleted
	switch {
	case runErr != nil:
		to = task.StatusFailed
	case rep.Canceled:
		to = task.StatusCanceled
	}

	switch {
	case runErr != nil:
		a.emitOrganizeFinalLog(h.ID, fmt.Sprintf("整理任务失败: %v", runErr))
	case rep.Canceled:
		a.emitOrganizeFinalLog(h.ID, fmt.Sprintf("整理任务取消: 已完成 %d / %d 个文件", rep.Success, len(plan.Items)))
	default:
		a.emitOrganizeFinalLog(h.ID, fmt.Sprintf("整理完成: 成功 %d · 失败 %d · 已移动 %s · 总数 %d",
			rep.Success, rep.Failed, units.FormatBytes(rep.MovedSizeBytes), len(plan.Items)))
	}

	if err := a.tasks.Finish(h.ID, to, runErr, task.ScanStats{}); err != nil {
		a.diag.Error("finish organize task", zap.Int64("task_id", h.ID), zap.Error(err))
	}
}

// runUndo 驱动一次撤销到终态（FR-11）。
//
// 采用与整理相同的收敛结构；差别在于收尾时按 D-D6 决定是否把原整理任务
// 标记为 undone：仅「正常完成」才标记，取消不标记（允许安全重试）。
func (a *App) runUndo(h *task.Handle, originalTaskID int64, moves []storage.OperationRecord) {
	defer h.Close()

	total := int64(len(moves))
	prog := newOrganizeProgress(total)
	th := task.NewThrottler(h.ID, h.StartedAt(), a, prog.snapshot)
	th.Start()
	th.Log(scanner.LogSystem, fmt.Sprintf("撤销任务启动: 回移 %d 个文件", total))

	onItem := func(r organizer.ItemResult) {
		prog.observe(r)
		if r.Status == organizer.StatusFailed {
			th.Log(scanner.LogSystem, fmt.Sprintf("撤销失败: %s · %s", r.SrcPath, r.Error))
		}
	}

	rep, runErr := a.organizer.Undo(h.Context(), h.ID, moves, organizer.Hooks{OnItem: onItem})
	th.Stop()

	to := task.StatusCompleted
	switch {
	case runErr != nil:
		to = task.StatusFailed
	case rep.Canceled:
		to = task.StatusCanceled
	}

	// D-D6：仅正常完成时标记原任务已撤销；取消/失败保留可重试性。
	// 使用 context.WithoutCancel：任务即将终态，但标记写库不应被取消信号打断。
	if !rep.Canceled && runErr == nil {
		if err := a.store.MarkTaskUndone(context.WithoutCancel(h.Context()), originalTaskID); err != nil {
			a.diag.Error("mark original task undone",
				zap.Int64("orig_task_id", originalTaskID), zap.Error(err))
		}
	}

	switch {
	case runErr != nil:
		a.emitOrganizeFinalLog(h.ID, fmt.Sprintf("撤销任务失败: %v", runErr))
	case rep.Canceled:
		a.emitOrganizeFinalLog(h.ID, fmt.Sprintf("撤销任务取消: 已完成 %d / %d 个文件", rep.Success, total))
	default:
		a.emitOrganizeFinalLog(h.ID, fmt.Sprintf("撤销完成: 成功 %d · 失败 %d · 已回移 %s · 总数 %d",
			rep.Success, rep.Failed, units.FormatBytes(rep.MovedSizeBytes), total))
	}

	if err := a.tasks.Finish(h.ID, to, runErr, task.ScanStats{}); err != nil {
		a.diag.Error("finish undo task", zap.Int64("task_id", h.ID), zap.Error(err))
	}
}
