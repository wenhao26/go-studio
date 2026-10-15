package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/task"
)

// storeAdapter 把 storage.Store 适配为 task.Store。
//
// task.Store 的接口由消费方 task 定义（contracts §2），参数类型是 task 侧的
// Type/Status/ScanOutcome；storage.Store 不 import task，因此这一层薄映射
// 放在装配层 app，避免反向依赖与类型纠缠。
type storeAdapter struct {
	store *storage.Store
}

// 编译期断言：适配器必须满足 task.Store。
var _ task.Store = storeAdapter{}

func (a storeAdapter) CreateTask(ctx context.Context, typ task.Type, startedAt int64) (int64, error) {
	return a.store.CreateTask(ctx, string(typ), startedAt)
}

func (a storeAdapter) CreateScan(ctx context.Context, root string, startedAt int64, workers int) (int64, error) {
	return a.store.CreateScan(ctx, root, startedAt, workers)
}

func (a storeAdapter) SetTaskScan(ctx context.Context, id, scanID int64) error {
	return a.store.SetTaskScan(ctx, id, scanID)
}

func (a storeAdapter) SetTaskParent(ctx context.Context, id, parentID int64) error {
	return a.store.SetTaskParent(ctx, id, parentID)
}

func (a storeAdapter) SetTaskStatus(ctx context.Context, id int64, to task.Status, finishedAt int64, cause string) error {
	return a.store.SetTaskStatus(ctx, id, string(to), finishedAt, cause)
}

func (a storeAdapter) FinishScan(ctx context.Context, scanID int64, outcome task.ScanOutcome) error {
	return a.store.FinishScan(ctx, scanID, storage.ScanOutcome{
		Status:         string(outcome.Status),
		FinishedAt:     outcome.FinishedAt,
		DurationMS:     outcome.DurationMS,
		FileCount:      outcome.FileCount,
		DirCount:       outcome.DirCount,
		TotalSizeBytes: outcome.TotalSizeBytes,
		SkippedCount:   outcome.SkippedCount,
		AvgSpeed:       outcome.AvgSpeed,
	})
}

func (a storeAdapter) FailUnfinishedTasks(ctx context.Context, finishedAt int64, cause string) (int64, error) {
	return a.store.FailUnfinishedTasks(ctx, finishedAt, cause)
}

func (a storeAdapter) FailRunningScans(ctx context.Context, finishedAt int64) (int64, error) {
	return a.store.FailRunningScans(ctx, finishedAt)
}

// GetTask 查询任务；storage 的「无行」错误在此映射为 task 层哨兵，
// 使 task.Manager 无需感知 database/sql。
func (a storeAdapter) GetTask(ctx context.Context, id int64) (*task.Task, error) {
	row, err := a.store.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get task %d: %w", id, task.ErrTaskNotFound)
		}
		return nil, fmt.Errorf("get task %d: %w", id, err)
	}
	return &task.Task{
		ID:         row.ID,
		Type:       task.Type(row.Type),
		Status:     task.Status(row.Status),
		ScanID:     row.ScanID,
		StartedAt:  row.StartedAt,
		FinishedAt: row.FinishedAt,
		Error:      row.Error,
	}, nil
}
