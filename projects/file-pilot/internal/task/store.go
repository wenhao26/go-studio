package task

import (
	"context"
	"time"
)

// Task 是任务层的持久化视图。
type Task struct {
	ID         int64
	Type       Type
	Status     Status
	ScanID     int64 // 0 表示未关联扫描
	StartedAt  int64 // UTC 毫秒
	FinishedAt int64 // UTC 毫秒；未结束为 0
	Error      string
}

// ScanStats 是扫描任务结束时的统计，由 scanner.Result 换算而来。
type ScanStats struct {
	Duration       time.Duration
	FileCount      int64
	DirCount       int64
	TotalSizeBytes int64
	SkippedCount   int64
}

// ScanOutcome 是写入 scans 终态行的完整统计（Manager 计算 avg_speed 与时间列）。
type ScanOutcome struct {
	Status         Status
	FinishedAt     int64
	DurationMS     int64
	FileCount      int64
	DirCount       int64
	TotalSizeBytes int64
	SkippedCount   int64
	AvgSpeed       float64
}

// Store 是任务层需要的持久化能力。
//
// 接口由消费方 task 定义（contracts §2）；实现方 storage.Store 不 import task，
// 因此由装配层 app 提供薄适配器完成类型映射（task.Type/Status ↔ string 等）。
type Store interface {
	// CreateTask 建立 pending 状态任务行，返回 task_id。
	CreateTask(ctx context.Context, typ Type, startedAt int64) (int64, error)

	// CreateScan 建立 running 状态扫描行，返回 scan_id（仅 scan 任务）。
	CreateScan(ctx context.Context, root string, startedAt int64, workers int) (int64, error)

	// SetTaskScan 关联任务与扫描记录。
	SetTaskScan(ctx context.Context, id, scanID int64) error

	// SetTaskParent 关联撤销任务与其被撤销的原整理任务（P1-1）。
	SetTaskParent(ctx context.Context, id, parentID int64) error

	// SetTaskStatus 写入状态转换结果（合法性由本包校验）。
	SetTaskStatus(ctx context.Context, id int64, to Status, finishedAt int64, cause string) error

	// FinishScan 写入扫描终态统计。
	FinishScan(ctx context.Context, scanID int64, outcome ScanOutcome) error

	// FailUnfinishedTasks 批量修复遗留 pending/running 任务（FR-12 崩溃恢复）。
	FailUnfinishedTasks(ctx context.Context, finishedAt int64, cause string) (int64, error)

	// FailRunningScans 批量修复遗留 running 扫描行（FR-12 崩溃恢复）。
	FailRunningScans(ctx context.Context, finishedAt int64) (int64, error)

	// GetTask 查询任务；不存在时返回 ErrTaskNotFound。
	GetTask(ctx context.Context, id int64) (*Task, error)
}
