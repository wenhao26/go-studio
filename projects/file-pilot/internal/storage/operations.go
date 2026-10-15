package storage

import (
	"context"
	"fmt"
)

// 操作动作与状态（FR-9/FR-11；contracts §5）。
const (
	// ActionMove 是一次文件移动（整理）。
	ActionMove = "move"
	// ActionUndo 是一次反向移动（撤销）。
	ActionUndo = "undo"

	// OperationSuccess 表示该条操作成功。
	OperationSuccess = "success"
	// OperationFailed 表示该条操作失败（含原因）。
	OperationFailed = "failed"
)

// operationsColumns 是读取操作记录的显式列清单（AGENTS.md §22.2）。
const operationsColumns = `task_id, action, src_path, dst_path, status, COALESCE(error, ''), COALESCE(size_bytes, 0), created_at`

// AppendOperation 追加一条 move/undo 记录（FR-9：逐条同步写，不可延迟）。
//
// 调用方负责设置 Action/Status/时间；本方法只做参数化写入与基本校验。
func (s *Store) AppendOperation(ctx context.Context, op OperationRecord) error {
	if op.TaskID <= 0 {
		return fmt.Errorf("append operation: invalid task id %d", op.TaskID)
	}
	if op.Action == "" || op.Status == "" {
		return fmt.Errorf("append operation: empty action/status for task %d", op.TaskID)
	}
	var errText any
	if op.Error != "" {
		errText = op.Error
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO operations (task_id, action, src_path, dst_path, status, error, size_bytes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		op.TaskID, op.Action, op.SrcPath, op.DstPath, op.Status, errText, op.SizeBytes, op.CreatedAt)
	if err != nil {
		return fmt.Errorf("append %s operation task %d: %w", op.Action, op.TaskID, err)
	}
	return nil
}

// ListSuccessfulMoves 返回某任务全部成功的 move 记录（撤销的数据源，FR-11）。
//
// 只取 status='success' 的 move：失败项没有实际移动，无可回移。
// 顺序为 id 升序（撤销按记录顺序逐条反向，结果确定）。
func (s *Store) ListSuccessfulMoves(ctx context.Context, taskID int64) ([]OperationRecord, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("list successful moves: invalid task id %d", taskID)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT `+operationsColumns+`
FROM operations
WHERE task_id = ? AND action = 'move' AND status = 'success'
ORDER BY id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list successful moves task %d: %w", taskID, err)
	}
	defer rows.Close()
	return scanOperations(rows)
}

// OperationSummary 是一个任务某动作的聚合统计（FR-10 报告）。
type OperationSummary struct {
	SuccessCount   int64
	FailedCount    int64
	MovedSizeBytes int64 // 仅成功项 size 之和（P1-10）
}

// SummarizeOperations 聚合某任务某动作（move/undo）的成功/失败数与已移动数据量。
func (s *Store) SummarizeOperations(ctx context.Context, taskID int64, action string) (OperationSummary, error) {
	if taskID <= 0 {
		return OperationSummary{}, fmt.Errorf("summarize operations: invalid task id %d", taskID)
	}
	if action == "" {
		return OperationSummary{}, fmt.Errorf("summarize operations: empty action for task %d", taskID)
	}
	var out OperationSummary
	err := s.db.QueryRowContext(ctx, `
SELECT
    COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE WHEN status = 'failed'  THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE WHEN status = 'success' THEN COALESCE(size_bytes, 0) ELSE 0 END), 0)
FROM operations
WHERE task_id = ? AND action = ?`, taskID, action).
		Scan(&out.SuccessCount, &out.FailedCount, &out.MovedSizeBytes)
	if err != nil {
		return OperationSummary{}, fmt.Errorf("summarize %s operations task %d: %w", action, taskID, err)
	}
	return out, nil
}

// ListFailedOperations 返回某任务某动作的失败记录（FR-10 失败明细入口）。
//
// 按 id 升序，前端可稳定展示「有提示、有原因、有去处」（requirements §九）。
func (s *Store) ListFailedOperations(ctx context.Context, taskID int64, action string) ([]OperationRecord, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("list failed operations: invalid task id %d", taskID)
	}
	if action == "" {
		return nil, fmt.Errorf("list failed operations: empty action for task %d", taskID)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT `+operationsColumns+`
FROM operations
WHERE task_id = ? AND action = ? AND status = 'failed'
ORDER BY id`, taskID, action)
	if err != nil {
		return nil, fmt.Errorf("list failed %s operations task %d: %w", action, taskID, err)
	}
	defer rows.Close()
	return scanOperations(rows)
}

// scanOperations 把查询结果扫描为 OperationRecord 切片。
func scanOperations(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]OperationRecord, error) {
	out := make([]OperationRecord, 0, 16)
	for rows.Next() {
		var op OperationRecord
		if err := rows.Scan(&op.TaskID, &op.Action, &op.SrcPath, &op.DstPath,
			&op.Status, &op.Error, &op.SizeBytes, &op.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan operation row: %w", err)
		}
		out = append(out, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operations: %w", err)
	}
	return out, nil
}
