package app

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/classifier"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/organizer"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/task"
)

// defaultPageSize 是分页默认大小（contracts §5：默认 100/页）。
const defaultPageSize = 100

// ChooseDirectory 打开系统原生目录选择器（FR-1）。
//
// 取消语义（wails v2.16 源码核实）：用户取消时返回
// ("", nil)，故空路径即取消，以 ErrDirectoryPickCanceled 上报。
func (a *App) ChooseDirectory() (string, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return "", err
	}

	// OpenDirectoryDialog 本身即目录选择器（v2 无 CanChoose* 字段，v1 遗留）。
	opts := runtime.OpenDialogOptions{
		Title: "选择扫描目录",
	}
	path, err := runtime.OpenDirectoryDialog(ctx, opts)
	if err != nil {
		// 对话框系统级失败：未映射，前端展示通用文案 + 诊断日志。
		a.diag.Error("open directory dialog", zap.Error(err))
		return "", fmt.Errorf("open directory dialog: %w", err)
	}
	if path == "" {
		return "", ErrDirectoryPickCanceled
	}
	return path, nil
}

// StartScan 启动一次扫描任务（FR-1/FU-2），返回 task_id。
//
// 边界校验（所有失败都不产生数据库行）：
//   - 根目录经 normalizeRoot（SEC-5：路径是不可信输入）；
//   - 配置经 cfg.Validate（异常命中 ErrInvalidWorkers 裸哨兵）；
//   - 单活动约束由 task.Manager 保证（异常命中 ErrTaskRunning 裸哨兵）。
//
// 校验通过后记录最近目录（失败只记日志，不阻断扫描）并异步驱动 runner。
func (a *App) StartScan(rootPath string, cfg scanner.Config) (int64, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return 0, err
	}

	root, err := normalizeRoot(rootPath)
	if err != nil {
		return 0, err
	}
	if err := cfg.Validate(); err != nil {
		return 0, bareError(err, scanner.ErrInvalidWorkers)
	}

	h, err := a.tasks.Begin(ctx, task.Spec{
		Type:    task.TypeScan,
		Root:    root,
		Workers: cfg.Workers,
	})
	if err != nil {
		return 0, bareError(err, task.ErrTaskRunning)
	}

	// 最近目录（FR-13）为便利性数据：写入失败不影响扫描启动。
	if err := a.rememberDir(ctx, root); err != nil {
		a.diag.Error("remember recent dir", zap.String("root", root), zap.Error(err))
	}

	go a.runScan(h, cfg)
	return h.ID, nil
}

// CancelTask 取消任务（协作式）。
//
// 语义（task.Manager.Cancel）：活动任务触发取消并返回 nil；已终态返回 nil
// （幂等）；不存在返回账未映射错误（前端通用文案）。
func (a *App) CancelTask(taskID int64) error {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return err
	}
	if err := a.tasks.Cancel(ctx, taskID); err != nil {
		a.diag.Error("cancel task", zap.Int64("task_id", taskID), zap.Error(err))
		return err
	}
	return nil
}

// ActiveTask 是活动任务的边界 DTO（contracts §5：GetActiveTask）。
//
// 与 task.ActiveTask 的差异：跨 Wails 边界用 snake_case 并补 elapsed_ms。
type ActiveTask struct {
	TaskID    int64  `json:"task_id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	ScanID    int64  `json:"scan_id"`
	RootPath  string `json:"root_path"`
	Error     string `json:"error"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

// GetActiveTask 返回当前活动任务快照；无活动任务返回 nil（FR-12）。
//
// 只读内存状态，不要求生命周期 ctx 就绪（与 GetRecoveryNotice 一致）。
func (a *App) GetActiveTask() (*ActiveTask, error) {
	t := a.tasks.Active()
	if t == nil {
		return nil, nil
	}
	return &ActiveTask{
		TaskID:    t.ID,
		Type:      string(t.Type),
		Status:    string(t.Status),
		ScanID:    t.ScanID,
		RootPath:  t.Root,
		ElapsedMS: time.Since(t.StartedAt).Milliseconds(),
	}, nil
}

// RecoveryNotice 是启动恢复提示（contracts §3.10，FR-12）。
type RecoveryNotice struct {
	RecoveredTasks int64 `json:"recovered_tasks"`
	RecoveredScans int64 `json:"recovered_scans"`
}

// GetRecoveryNotice 返回本次启动的异常恢复结果；无恢复时返回 nil（序列化为 null）。
//
// 与 GetActiveTask 一样只读内存状态，不要求生命周期 ctx 就绪。
func (a *App) GetRecoveryNotice() (*RecoveryNotice, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.recovery == nil {
		return nil, nil
	}
	notice := *a.recovery
	return &notice, nil
}

// ScanSummary 是扫描完成统计（contracts §3.6，FR-5）。
//
// OrganizableCount 是参与整理的文件数（excluded=0，FR-9.1），供整理二次确认
// 展示「即将整理: 文件数量 N」；与 FileCount 之差即被排除的文件数（D-D9）。
type ScanSummary struct {
	FileCount        int64   `json:"file_count"`
	DirCount         int64   `json:"dir_count"`
	TotalSizeBytes   int64   `json:"total_size_bytes"`
	SkippedCount     int64   `json:"skipped_count"`
	DurationMS       int64   `json:"duration_ms"`
	AvgSpeed         float64 `json:"avg_speed"`
	MaxWorkers       int     `json:"max_workers"`
	Status           string  `json:"status"`
	OrganizableCount int64   `json:"organizable_count"`
}

// GetScanSummary 返回一次扫描的终态统计；不存在的 scanID 返回未映射错误
// （前端通常携带有效 id 调用，异常情况展示通用文案 + 诊断日志）。
func (a *App) GetScanSummary(scanID int64) (ScanSummary, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return ScanSummary{}, err
	}

	row, err := a.store.GetScan(ctx, scanID)
	if err != nil {
		a.diag.Error("get scan summary", zap.Int64("scan_id", scanID), zap.Error(err))
		return ScanSummary{}, fmt.Errorf("get scan %d: %w", scanID, err)
	}
	organizable, err := a.store.CountOrganizeCandidates(ctx, scanID)
	if err != nil {
		a.diag.Error("count organize candidates", zap.Int64("scan_id", scanID), zap.Error(err))
		return ScanSummary{}, fmt.Errorf("count organize candidates %d: %w", scanID, err)
	}
	return ScanSummary{
		FileCount:        row.FileCount,
		DirCount:         row.DirCount,
		TotalSizeBytes:   row.TotalSizeBytes,
		SkippedCount:     row.SkippedCount,
		DurationMS:       row.DurationMS,
		AvgSpeed:         row.AvgSpeed,
		MaxWorkers:       row.MaxWorkers,
		Status:           row.Status,
		OrganizableCount: organizable,
	}, nil
}

// FileItem 是文件详情列表一行（contracts §3.5）。
type FileItem struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
	Category   string `json:"category"`
	Confidence *int64 `json:"confidence"` // null = 用户手动（FR-8）
	Source     string `json:"source"`     // rule / user
	Reason     string `json:"reason"`
	Excluded   bool   `json:"excluded"`
}

// FilePage 是 ListFiles 的返回（contracts §3.5 分页口径）。
type FilePage struct {
	Items      []FileItem `json:"items"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalCount int64      `json:"total_count"`
	TotalPage  int64      `json:"total_page"`
}

// ListFiles 分页查询某次扫描的文件行；category="" 表示全部（P2-9）。
//
// 越界页返回空 items 但 total_* 仍准确：前端据此收敛页码，无需区分
// 「页码非法」与「没有数据」。
func (a *App) ListFiles(scanID int64, category string, page, pageSize int) (FilePage, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return FilePage{}, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}

	offset := (page - 1) * pageSize
	items, total, err := a.store.ListFiles(ctx, scanID, category, pageSize, offset)
	if err != nil {
		a.diag.Error("list files", zap.Int64("scan_id", scanID), zap.String("category", category), zap.Error(err))
		return FilePage{}, fmt.Errorf("list files scan %d: %w", scanID, err)
	}

	out := FilePage{
		Items:      make([]FileItem, len(items)),
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPage:  totalPage(total, pageSize),
	}
	for i, row := range items {
		out.Items[i] = FileItem{
			ID:         row.ID,
			Name:       row.Name,
			Path:       row.Path,
			SizeBytes:  row.SizeBytes,
			Category:   row.Category,
			Confidence: row.Confidence,
			Source:     row.Source,
			Reason:     row.Reason,
			Excluded:   row.Excluded,
		}
	}
	return out, nil
}

// totalPage 计算总页数；无数据时为 0（前端按 0 处理空态）。
func totalPage(total int64, pageSize int) int64 {
	if total <= 0 {
		return 0
	}
	return (total + int64(pageSize) - 1) / int64(pageSize)
}

// CategoryCount 是一个类别的文件计数（contracts §5：固定 7 类顺序）。
type CategoryCount struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

// GetCategorySummary 返回某次扫描按类别的文件计数（FR-7 汇总）。
//
// 固定 7 类顺序（classifier.Categories：图片/视频/文档/财务/工作/学习/其他），
// 未出现的类别计数为 0——保证前端汇总视图布局稳定（D-C4）。
func (a *App) GetCategorySummary(scanID int64) ([]CategoryCount, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return nil, err
	}

	counts, err := a.store.GetCategoryCounts(ctx, scanID)
	if err != nil {
		a.diag.Error("get category summary", zap.Int64("scan_id", scanID), zap.Error(err))
		return nil, fmt.Errorf("get category summary scan %d: %w", scanID, err)
	}

	out := make([]CategoryCount, 0, len(classifier.Categories))
	for _, category := range classifier.Categories {
		out = append(out, CategoryCount{Category: category, Count: counts[category]})
	}
	return out, nil
}

// SetFilePlan 更新单文件的整理计划（FR-8：改类别 / 排除 / 恢复），即时落库。
//
// 语义（T2.6）：
//   - 改类别（category 与当前不同）：source=user、confidence=NULL、reason=用户手动指定；
//   - 仅排除/恢复（category 与当前相同）：只改 excluded，保留原来源/置信度/原因
//     （排除一个规则命中的文件不应把它变成「用户手动指定」）。
//
// 校验（contracts §4）：category ∈ 7 类，否则 ErrCategoryInvalid；fileID 不存在，
// 否则 ErrFileNotInScan。归属由结果页保证（签名不含 scanID，D-C2）。
func (a *App) SetFilePlan(fileID int64, category string, excluded bool) error {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return err
	}
	if !classifier.IsValidCategory(category) {
		a.diag.Warn("set file plan: invalid category",
			zap.Int64("file_id", fileID), zap.String("category", category))
		return ErrCategoryInvalid
	}

	current, err := a.store.GetFilePlan(ctx, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotInScan
		}
		a.diag.Error("read file plan", zap.Int64("file_id", fileID), zap.Error(err))
		return fmt.Errorf("read file plan %d: %w", fileID, err)
	}

	plan := storage.FilePlanUpdate{
		ID:         fileID,
		Category:   current.Category,
		Source:     current.Source,
		Confidence: current.Confidence,
		Reason:     current.Reason,
		Excluded:   excluded,
	}
	if category != current.Category {
		plan.Category = category
		plan.Source = classifier.SourceUser
		plan.Confidence = nil
		plan.Reason = classifier.ReasonManual
	}

	if err := a.store.UpdateFilePlan(ctx, plan); err != nil {
		a.diag.Error("update file plan", zap.Int64("file_id", fileID), zap.Error(err))
		return fmt.Errorf("update file plan %d: %w", fileID, err)
	}
	return nil
}

// OrganizeFailure 是整理/撤销失败明细（contracts §3.7）。
type OrganizeFailure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// OrganizeReport 是整理报告（FR-10；contracts §3.7）。
type OrganizeReport struct {
	TaskID         int64             `json:"task_id"`
	Status         string            `json:"status"`
	SuccessCount   int64             `json:"success_count"`
	FailedCount    int64             `json:"failed_count"`
	DurationMS     int64             `json:"duration_ms"`
	MovedSizeBytes int64             `json:"moved_size_bytes"`
	Failures       []OrganizeFailure `json:"failures"`
}

// StartOrganize 启动整理任务（FR-9），返回 task_id。
//
// 语义（D-D3）：
//   - 前端二次确认后调用；
//   - 必须是 completed 的扫描（未完成/不存在 → ErrScanIncomplete）；
//   - 同步预检目标目录（MkdirAll）：失败 → organizer.ErrTargetDirCreate（裸哨兵）；
//   - 单活动任务约束失败 → ErrTaskRunning。
func (a *App) StartOrganize(scanID int64) (int64, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return 0, err
	}

	scan, err := a.store.GetScan(ctx, scanID)
	if err != nil {
		a.diag.Error("start organize: scan not found", zap.Int64("scan_id", scanID), zap.Error(err))
		return 0, ErrScanIncomplete
	}
	if scan.Status != string(task.StatusCompleted) {
		return 0, ErrScanIncomplete
	}

	cands, err := a.store.ListOrganizeCandidates(ctx, scanID)
	if err != nil {
		a.diag.Error("start organize: list candidates", zap.Int64("scan_id", scanID), zap.Error(err))
		return 0, fmt.Errorf("list organize candidates scan %d: %w", scanID, err)
	}

	plan, err := organizer.BuildPlan(scan.RootPath, cands)
	if err != nil {
		a.diag.Error("start organize: build plan", zap.Int64("scan_id", scanID), zap.Error(err))
		return 0, fmt.Errorf("build organize plan scan %d: %w", scanID, err)
	}

	if err := a.organizer.EnsureTargetDirs(plan); err != nil {
		a.diag.Error("start organize: ensure target dirs", zap.Int64("scan_id", scanID), zap.Error(err))
		// ErrTargetDirCreate 为裸哨兵（contracts §4 要求原样返回）。
		return 0, bareError(err, organizer.ErrTargetDirCreate)
	}

	h, err := a.tasks.Begin(ctx, task.Spec{
		Type:   task.TypeOrganize,
		ScanID: scanID,
		Root:   scan.RootPath,
	})
	if err != nil {
		return 0, bareError(err, task.ErrTaskRunning)
	}

	go a.runOrganize(h, plan)
	return h.ID, nil
}

// GetOrganizeReport 返回整理任务的报告（FR-10；contracts §3.7）。
//
// 同时服务整理任务与撤销任务：
//   - 整理任务（parent_task_id == 0）→ 基于 action='move'，失败 path 为原文件；
//   - 撤销任务（parent_task_id != 0）→ 基于 action='undo'，失败 path 为整理后位置。
func (a *App) GetOrganizeReport(taskID int64) (OrganizeReport, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return OrganizeReport{}, err
	}

	t, err := a.store.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.diag.Warn("get organize report: task not found", zap.Int64("task_id", taskID))
		} else {
			a.diag.Error("get organize report", zap.Int64("task_id", taskID), zap.Error(err))
		}
		return OrganizeReport{}, fmt.Errorf("get task %d: %w", taskID, err)
	}

	action := storage.ActionMove
	if t.ParentTaskID != 0 {
		action = storage.ActionUndo
	}

	summary, err := a.store.SummarizeOperations(ctx, taskID, action)
	if err != nil {
		a.diag.Error("summarize operations", zap.Int64("task_id", taskID), zap.String("action", action), zap.Error(err))
		return OrganizeReport{}, fmt.Errorf("summarize operations task %d: %w", taskID, err)
	}
	fails, err := a.store.ListFailedOperations(ctx, taskID, action)
	if err != nil {
		a.diag.Error("list failed operations", zap.Int64("task_id", taskID), zap.String("action", action), zap.Error(err))
		return OrganizeReport{}, fmt.Errorf("list failed operations task %d: %w", taskID, err)
	}

	var dur int64
	if t.FinishedAt != 0 && t.StartedAt != 0 && t.FinishedAt >= t.StartedAt {
		dur = t.FinishedAt - t.StartedAt
	}

	out := OrganizeReport{
		TaskID:         taskID,
		Status:         t.Status,
		SuccessCount:   summary.SuccessCount,
		FailedCount:    summary.FailedCount,
		DurationMS:     dur,
		MovedSizeBytes: summary.MovedSizeBytes,
		Failures:       make([]OrganizeFailure, 0, len(fails)),
	}
	for _, f := range fails {
		out.Failures = append(out.Failures, OrganizeFailure{Path: f.SrcPath, Reason: f.Error})
	}
	return out, nil
}

// UndoLastOrganize 撤销上一次可撤销的整理（FR-11），返回撤销任务 task_id。
//
// 选取规则（D-D2）：
//   - 最近一个存在成功 move 的 organize 任务（按 id 最大）；
//   - 该任务已 undone=1 → ErrAlreadyUndone；
//   - 不存在 → ErrNothingToUndo。
func (a *App) UndoLastOrganize(scanID int64) (int64, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return 0, err
	}
	if scanID <= 0 {
		return 0, fmt.Errorf("undo last organize: invalid scan id %d", scanID)
	}

	t, err := a.store.GetLatestUndoableOrganize(ctx, scanID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNothingToUndo
		}
		a.diag.Error("undo last organize: find target", zap.Int64("scan_id", scanID), zap.Error(err))
		return 0, fmt.Errorf("find undoable organize scan %d: %w", scanID, err)
	}
	if t.Undone {
		return 0, ErrAlreadyUndone
	}

	moves, err := a.store.ListSuccessfulMoves(ctx, t.ID)
	if err != nil {
		a.diag.Error("undo last organize: list moves", zap.Int64("orig_task_id", t.ID), zap.Error(err))
		return 0, fmt.Errorf("list successful moves task %d: %w", t.ID, err)
	}

	h, err := a.tasks.Begin(ctx, task.Spec{
		Type:         task.TypeOrganize,
		ScanID:       t.ScanID,
		ParentTaskID: t.ID,
	})
	if err != nil {
		return 0, bareError(err, task.ErrTaskRunning)
	}

	go a.runUndo(h, t.ID, moves)
	return h.ID, nil
}

// normalizeRoot 把前端传入的目录路径归一化为绝对路径并校验可用性。
//
// 路径是不可信输入（SEC-5）：这里只做存在性与类型校验；扫描期间的
// 逐层校验（符号链接、隐藏属性等）由 scanner/fsx 负责。
func normalizeRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", ErrDirNotFound
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root path: %w", err)
	}
	abs = filepath.Clean(abs)

	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrDirNotFound
		}
		return "", fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return "", ErrDirNotDirectory
	}
	return abs, nil
}
