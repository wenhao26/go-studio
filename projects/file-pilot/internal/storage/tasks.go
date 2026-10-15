package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// tasksColumns 是读取任务行的显式列清单（AGENTS.md §22.2）。
const tasksColumns = `id, type, status, COALESCE(scan_id, 0), COALESCE(parent_task_id, 0), COALESCE(started_at, 0), COALESCE(finished_at, 0), COALESCE(error, ''), undone`

// CreateTask 建立一条 pending 状态的任务行，返回 task_id。
//
// pending 是任务状态机的起点：task.Manager 随后立即转换为 running
// （requirements §6.2 的合法转换 pending→running）。
func (s *Store) CreateTask(ctx context.Context, typ string, startedAt int64) (int64, error) {
	if typ == "" {
		return 0, fmt.Errorf("create task: empty type")
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO tasks (type, status, started_at, undone)
VALUES (?, 'pending', ?, 0)`, typ, startedAt)
	if err != nil {
		return 0, fmt.Errorf("insert task type=%q: %w", typ, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert task type=%q: last insert id: %w", typ, err)
	}
	return id, nil
}

// SetTaskStatus 更新任务状态、结束时间与错误原因。
//
// 只做写入，不校验合法性：状态机转换表由 task.Manager 负责（消费方定义的规则）。
// finishedAt=0 表示「尚未结束」，写 NULL 而非 0，与 scans 列语义一致；
// cause 为空时同样写 NULL，避免把「无错误」存成空字符串。
func (s *Store) SetTaskStatus(ctx context.Context, id int64, status string, finishedAt int64, cause string) error {
	if status == "" {
		return fmt.Errorf("set task %d status: empty status", id)
	}
	var errText any
	if cause != "" {
		errText = cause
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET status      = ?,
    finished_at = NULLIF(?, 0),
    error       = ?
WHERE id = ?`, status, finishedAt, errText, id)
	if err != nil {
		return fmt.Errorf("set task %d status=%q: %w", id, status, err)
	}
	return nil
}

// SetTaskScan 关联任务与扫描记录（tasks.scan_id）。
func (s *Store) SetTaskScan(ctx context.Context, id, scanID int64) error {
	if id <= 0 || scanID <= 0 {
		return fmt.Errorf("set task scan: invalid ids task=%d scan=%d", id, scanID)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET scan_id = ? WHERE id = ?`, scanID, id)
	if err != nil {
		return fmt.Errorf("set task %d scan %d: %w", id, scanID, err)
	}
	return nil
}

// SetTaskParent 关联撤销任务与其被撤销的原整理任务（tasks.parent_task_id，P1-1）。
func (s *Store) SetTaskParent(ctx context.Context, id, parentID int64) error {
	if id <= 0 || parentID <= 0 {
		return fmt.Errorf("set task parent: invalid ids task=%d parent=%d", id, parentID)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET parent_task_id = ? WHERE id = ?`, parentID, id)
	if err != nil {
		return fmt.Errorf("set task %d parent %d: %w", id, parentID, err)
	}
	return nil
}

// MarkTaskUndone 把原整理任务标记为已撤销（FR-11 判重）。
//
// 只做写入；「何时可撤销」由 app 层依据 GetLatestUndoableOrganize 判定。
func (s *Store) MarkTaskUndone(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE tasks SET undone = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark task %d undone: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark task %d undone: rows affected: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("mark task %d undone: task not found", id)
	}
	return nil
}

// GetLatestUndoableOrganize 返回某次扫描下「最近一次存在成功移动的整理任务」
// （FR-11 / P1-2 选取规则）。
//
// 撤销任务本身也是 organize 类型，但其只有 action='undo' 记录、没有成功的
// move，故被 EXISTS 条件排除——保证不会把撤销任务当成可撤销目标。
// 返回的 Undone 供调用方区分 ErrNothingToUndo 与 ErrAlreadyUndone；
// 不存在时返回 sql.ErrNoRows。
func (s *Store) GetLatestUndoableOrganize(ctx context.Context, scanID int64) (*TaskRow, error) {
	if scanID <= 0 {
		return nil, fmt.Errorf("latest undoable organize: invalid scan id %d", scanID)
	}
	row := s.db.QueryRowContext(ctx, `
SELECT `+tasksColumns+`
FROM tasks t
WHERE t.type = 'organize'
  AND t.scan_id = ?
  AND EXISTS (
      SELECT 1 FROM operations o
      WHERE o.task_id = t.id AND o.action = 'move' AND o.status = 'success'
  )
ORDER BY t.id DESC
LIMIT 1`, scanID)

	var out TaskRow
	err := row.Scan(&out.ID, &out.Type, &out.Status, &out.ScanID, &out.ParentTaskID,
		&out.StartedAt, &out.FinishedAt, &out.Error, &out.Undone)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("latest undoable organize scan %d: %w", scanID, err)
	}
	return &out, nil
}

// FailUnfinishedTasks 把遗留 pending/running 的任务批量置为 failed（FR-12 异常恢复）。
//
// 这是崩溃修复而非状态机转换：这些行没有活动 Handle，无法也不应经过
// 合法转换表（运行期的 pending→failed 依然是非法转换，会被 Manager 拒绝）。
func (s *Store) FailUnfinishedTasks(ctx context.Context, finishedAt int64, cause string) (int64, error) {
	if cause == "" {
		return 0, fmt.Errorf("fail unfinished tasks: empty cause")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET status      = 'failed',
    finished_at = ?,
    error       = ?
WHERE status IN ('pending', 'running')`, finishedAt, cause)
	if err != nil {
		return 0, fmt.Errorf("mark unfinished tasks failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark unfinished tasks failed: rows affected: %w", err)
	}
	return n, nil
}

// GetTask 按 ID 查询任务；不存在时返回 sql.ErrNoRows。
func (s *Store) GetTask(ctx context.Context, id int64) (*TaskRow, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT `+tasksColumns+`
FROM tasks
WHERE id = ?`, id)

	var out TaskRow
	if err := row.Scan(&out.ID, &out.Type, &out.Status, &out.ScanID, &out.ParentTaskID,
		&out.StartedAt, &out.FinishedAt, &out.Error, &out.Undone); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("get task %d: %w", id, err)
	}
	return &out, nil
}
