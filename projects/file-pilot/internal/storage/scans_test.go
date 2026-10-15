package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestCreateScan_RoundTrip(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	id, err := store.CreateScan(ctx, `C:\data`, 1000, 8)
	if err != nil {
		t.Fatalf("CreateScan: %v", err)
	}
	if id <= 0 {
		t.Fatalf("scan id = %d, want > 0", id)
	}

	row, err := store.GetScan(ctx, id)
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if row.RootPath != `C:\data` || row.Status != "running" || row.MaxWorkers != 8 {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.StartedAt != 1000 || row.FinishedAt != 0 {
		t.Fatalf("unexpected timestamps: %+v", row)
	}
}

func TestFinishScan_WritesOutcome(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	id, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("CreateScan: %v", err)
	}
	outcome := ScanOutcome{
		Status: "completed", FinishedAt: 3500, DurationMS: 2500,
		FileCount: 120, DirCount: 30, TotalSizeBytes: 4096, SkippedCount: 3, AvgSpeed: 48,
	}
	if err := store.FinishScan(ctx, id, outcome); err != nil {
		t.Fatalf("FinishScan: %v", err)
	}

	row, err := store.GetScan(ctx, id)
	if err != nil {
		t.Fatalf("GetScan: %v", err)
	}
	if row.Status != "completed" || row.FinishedAt != 3500 || row.DurationMS != 2500 ||
		row.FileCount != 120 || row.DirCount != 30 || row.TotalSizeBytes != 4096 ||
		row.SkippedCount != 3 || row.AvgSpeed != 48 {
		t.Fatalf("unexpected row after finish: %+v", row)
	}
}

func TestCreateScan_Validation(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if _, err := store.CreateScan(ctx, "", 1000, 4); err == nil {
		t.Error("want error for empty root")
	}
	if _, err := store.CreateScan(ctx, `C:\data`, 1000, 0); err == nil {
		t.Error("want error for workers < 1")
	}
}

func TestFinishScan_InvalidID(t *testing.T) {
	store := openTestStore(t)
	if err := store.FinishScan(context.Background(), 0, ScanOutcome{}); err == nil {
		t.Fatal("want error for scan id 0")
	}
}

func TestGetScan_NotFound(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.GetScan(context.Background(), 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows, got %v", err)
	}
}

// TestFailRunningScans_OnlyRunning 覆盖 FR-12 崩溃恢复：只修 running 行，
// 统计列保持原样（NULL 表示未知，不编造）。
func TestFailRunningScans_OnlyRunning(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	runningID, err := store.CreateScan(ctx, `C:\a`, 1000, 4)
	if err != nil {
		t.Fatalf("create running scan: %v", err)
	}
	doneID, err := store.CreateScan(ctx, `C:\b`, 1000, 4)
	if err != nil {
		t.Fatalf("create finished scan: %v", err)
	}
	if err := store.FinishScan(ctx, doneID, ScanOutcome{
		Status: "completed", FinishedAt: 2000, DurationMS: 1000, FileCount: 5,
	}); err != nil {
		t.Fatalf("finish scan: %v", err)
	}

	n, err := store.FailRunningScans(ctx, 9000)
	if err != nil {
		t.Fatalf("FailRunningScans: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows affected = %d, want 1", n)
	}

	row, err := store.GetScan(ctx, runningID)
	if err != nil {
		t.Fatalf("get running scan: %v", err)
	}
	if row.Status != "failed" || row.FinishedAt != 9000 {
		t.Fatalf("unexpected recovered row: %+v", row)
	}
	if row.DurationMS != 8000 {
		t.Fatalf("duration_ms = %d, want 8000（9000 - started_at）", row.DurationMS)
	}

	done, err := store.GetScan(ctx, doneID)
	if err != nil {
		t.Fatalf("get finished scan: %v", err)
	}
	if done.Status != "completed" || done.FileCount != 5 {
		t.Fatalf("finished scan must stay untouched: %+v", done)
	}
}

// TestPruneOldScans_Cascades 覆盖 FR-13：保留最近 keep 次，级联清理
// files/operations/tasks，且不影响保留的扫描。
func TestPruneOldScans_Cascades(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	const total = 22
	const keep = 20

	var oldestScan, oldestTask int64
	for i := 0; i < total; i++ {
		scanID, err := store.CreateScan(ctx, `C:\root`, int64(1000+i), 4)
		if err != nil {
			t.Fatalf("create scan %d: %v", i, err)
		}
		if err := store.FinishScan(ctx, scanID, ScanOutcome{Status: "completed", FinishedAt: int64(2000 + i)}); err != nil {
			t.Fatalf("finish scan %d: %v", i, err)
		}
		if err := store.InsertFiles(ctx, scanID, []FileRecord{{
			Path: `C:\root\a.jpg`, Name: "a.jpg", Ext: "jpg", SizeBytes: 1,
			MtimeMS: 1, Category: "图片", Confidence: 60, Source: "rule", Reason: "扩展名",
		}}); err != nil {
			t.Fatalf("insert file %d: %v", i, err)
		}
		taskID, err := store.CreateTask(ctx, "scan", int64(1000+i))
		if err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
		if err := store.SetTaskScan(ctx, taskID, scanID); err != nil {
			t.Fatalf("link task %d: %v", i, err)
		}
		if i == 0 {
			oldestScan, oldestTask = scanID, taskID
			// operations 表由 Phase 3 写入；这里直接写一行验证级联删除。
			if _, err := store.db.ExecContext(ctx,
				`INSERT INTO operations (task_id, action, src_path, dst_path, status, created_at)
				 VALUES (?, 'move', ?, ?, 'success', 1)`,
				taskID, `C:\root\a.jpg`, `C:\root\FilePilot整理\图片\a.jpg`); err != nil {
				t.Fatalf("insert operation: %v", err)
			}
		}
	}

	removed, err := store.PruneOldScans(ctx, keep)
	if err != nil {
		t.Fatalf("PruneOldScans: %v", err)
	}
	if removed != total-keep {
		t.Fatalf("removed = %d, want %d", removed, total-keep)
	}

	if _, err := store.GetScan(ctx, oldestScan); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("oldest scan should be deleted, got %v", err)
	}
	if _, err := store.GetTask(ctx, oldestTask); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("oldest task should be cascade deleted, got %v", err)
	}
	var leftover int
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM operations WHERE task_id = ?`, oldestTask).Scan(&leftover); err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if leftover != 0 {
		t.Fatalf("operations of pruned task = %d, want 0", leftover)
	}

	var scans, files, tasks int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans`).Scan(&scans); err != nil {
		t.Fatalf("count scans: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files`).Scan(&files); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&tasks); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if scans != keep || files != keep || tasks != keep {
		t.Fatalf("kept scans=%d files=%d tasks=%d, want %d each", scans, files, tasks, keep)
	}
}

func TestPruneOldScans_Validation(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.PruneOldScans(context.Background(), 0); err == nil {
		t.Fatal("want error for keep < 1")
	}
}
