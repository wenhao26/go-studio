package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// CreateScan 建立一条扫描记录（status=running），返回 scan_id。
//
// 统计列在扫描开始时均为 0：进度期间不回写 scans（进度读内存快照），
// 终态统计由 FinishScan 一次性落库，避免扫描期的高频写放大。
func (s *Store) CreateScan(ctx context.Context, rootPath string, startedAt int64, maxWorkers int) (int64, error) {
	if rootPath == "" {
		return 0, fmt.Errorf("create scan: empty root path")
	}
	if maxWorkers < 1 {
		return 0, fmt.Errorf("create scan: invalid max workers %d", maxWorkers)
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO scans (root_path, status, started_at, max_workers)
VALUES (?, 'running', ?, ?)`, rootPath, startedAt, maxWorkers)
	if err != nil {
		return 0, fmt.Errorf("insert scan %q: %w", rootPath, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert scan %q: last insert id: %w", rootPath, err)
	}
	return id, nil
}

// FinishScan 写入扫描终态与统计（FR-5 的持久化出口）。
func (s *Store) FinishScan(ctx context.Context, scanID int64, outcome ScanOutcome) error {
	if scanID <= 0 {
		return fmt.Errorf("finish scan: invalid scan id %d", scanID)
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE scans
SET status        = ?,
    finished_at   = ?,
    duration_ms   = ?,
    file_count    = ?,
    dir_count     = ?,
    total_size_bytes = ?,
    skipped_count = ?,
    avg_speed     = ?
WHERE id = ?`,
		outcome.Status, outcome.FinishedAt, outcome.DurationMS,
		outcome.FileCount, outcome.DirCount, outcome.TotalSizeBytes,
		outcome.SkippedCount, outcome.AvgSpeed, scanID)
	if err != nil {
		return fmt.Errorf("finish scan %d: %w", scanID, err)
	}
	return nil
}

// FailRunningScans 把遗留 running 的扫描记录批量置为 failed（FR-12 异常恢复）。
//
// 这是崩溃修复而非状态机转换：这些行没有活动 Handle，且 duration/速度无法
// 可信重建，因此只写状态与恢复时刻，统计列保持原样（NULL 表示未知，不编造）。
func (s *Store) FailRunningScans(ctx context.Context, finishedAt int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
UPDATE scans
SET status      = 'failed',
    finished_at = ?,
    duration_ms = COALESCE(duration_ms, MAX(0, ? - started_at))
WHERE status = 'running'`, finishedAt, finishedAt)
	if err != nil {
		return 0, fmt.Errorf("mark running scans failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark running scans failed: rows affected: %w", err)
	}
	return n, nil
}

// GetScan 按 ID 查询扫描记录；不存在时返回 sql.ErrNoRows。
func (s *Store) GetScan(ctx context.Context, id int64) (*ScanRow, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, root_path, status, started_at, COALESCE(finished_at, 0), COALESCE(duration_ms, 0),
       file_count, dir_count, total_size_bytes, skipped_count, COALESCE(avg_speed, 0), max_workers
FROM scans
WHERE id = ?`, id)

	var out ScanRow
	if err := row.Scan(
		&out.ID, &out.RootPath, &out.Status, &out.StartedAt, &out.FinishedAt, &out.DurationMS,
		&out.FileCount, &out.DirCount, &out.TotalSizeBytes, &out.SkippedCount, &out.AvgSpeed, &out.MaxWorkers,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("get scan %d: %w", id, err)
	}
	return &out, nil
}

// PruneOldScans 保留最近 keep 次扫描并清理更早的数据（FR-13 历史自动清理）。
//
// 删除顺序遵守外键：files → operations → tasks → scans；返回被删除的扫描数。
func (s *Store) PruneOldScans(ctx context.Context, keep int) (int64, error) {
	if keep < 1 {
		return 0, fmt.Errorf("prune scans: keep must be >= 1, got %d", keep)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin prune scans: %w", err)
	}
	// commit 成功后 Rollback 返回 sql.ErrTxDone，属预期路径；见 applyMigration 同款说明。
	defer func() { _ = tx.Rollback() }()

	var oldIDs = `SELECT id FROM scans WHERE id NOT IN (SELECT id FROM scans ORDER BY id DESC LIMIT ?)`

	if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE scan_id IN (`+oldIDs+`)`, keep); err != nil {
		return 0, fmt.Errorf("prune old files: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM operations WHERE task_id IN (SELECT id FROM tasks WHERE scan_id IN (`+oldIDs+`))`, keep); err != nil {
		return 0, fmt.Errorf("prune old operations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE scan_id IN (`+oldIDs+`)`, keep); err != nil {
		return 0, fmt.Errorf("prune old tasks: %w", err)
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM scans WHERE id NOT IN (SELECT id FROM scans ORDER BY id DESC LIMIT ?)`, keep)
	if err != nil {
		return 0, fmt.Errorf("prune old scans: %w", err)
	}
	removed, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune old scans: rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune scans: %w", err)
	}
	return removed, nil
}
