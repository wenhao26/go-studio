package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestCreateTask_PendingStart(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	id, err := store.CreateTask(ctx, "scan", 1000)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if id <= 0 {
		t.Fatalf("task id = %d, want > 0", id)
	}

	row, err := store.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if row.Type != "scan" || row.Status != "pending" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.StartedAt != 1000 || row.FinishedAt != 0 || row.ScanID != 0 || row.Error != "" {
		t.Fatalf("unexpected zero-state fields: %+v", row)
	}
}

func TestSetTaskStatus_RoundTrip(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	id, err := store.CreateTask(ctx, "scan", 1000)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := store.SetTaskStatus(ctx, id, "running", 0, ""); err != nil {
		t.Fatalf("SetTaskStatus running: %v", err)
	}
	if err := store.SetTaskStatus(ctx, id, "failed", 2000, "读取根目录失败"); err != nil {
		t.Fatalf("SetTaskStatus failed: %v", err)
	}

	row, err := store.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if row.Status != "failed" || row.FinishedAt != 2000 || row.Error != "读取根目录失败" {
		t.Fatalf("unexpected row: %+v", row)
	}
}

// TestSetTaskStatus_EmptyCauseStoresNULL：空原因写 NULL，读回空串，
// 避免把「无错误」持久化成空字符串语义混淆。
func TestSetTaskStatus_EmptyCauseStoresNULL(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	id, err := store.CreateTask(ctx, "scan", 1000)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := store.SetTaskStatus(ctx, id, "completed", 1500, ""); err != nil {
		t.Fatalf("SetTaskStatus: %v", err)
	}

	var cause any
	if err := store.db.QueryRowContext(ctx, `SELECT error FROM tasks WHERE id = ?`, id).Scan(&cause); err != nil {
		t.Fatalf("read error: %v", err)
	}
	if cause != nil {
		t.Fatalf("error = %v, want NULL", cause)
	}

	row, err := store.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if row.Error != "" {
		t.Fatalf("error = %q, want empty", row.Error)
	}
}

func TestCreateTask_Validation(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.CreateTask(context.Background(), "", 1000); err == nil {
		t.Fatal("want error for empty type")
	}
}

func TestSetTaskStatus_Validation(t *testing.T) {
	store := openTestStore(t)
	if err := store.SetTaskStatus(context.Background(), 1, "", 0, ""); err == nil {
		t.Fatal("want error for empty status")
	}
}

func TestSetTaskScan_Link(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	scanID, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("CreateScan: %v", err)
	}
	taskID, err := store.CreateTask(ctx, "scan", 1000)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := store.SetTaskScan(ctx, taskID, scanID); err != nil {
		t.Fatalf("SetTaskScan: %v", err)
	}

	row, err := store.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if row.ScanID != scanID {
		t.Fatalf("scan_id = %d, want %d", row.ScanID, scanID)
	}
}

func TestSetTaskScan_Validation(t *testing.T) {
	store := openTestStore(t)
	if err := store.SetTaskScan(context.Background(), 0, 1); err == nil {
		t.Error("want error for task id 0")
	}
	if err := store.SetTaskScan(context.Background(), 1, 0); err == nil {
		t.Error("want error for scan id 0")
	}
}

// TestFailUnfinishedTasks_OnlyUnfinished 覆盖 FR-12 崩溃恢复的批量修复。
func TestFailUnfinishedTasks_OnlyUnfinished(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	pending, _ := store.CreateTask(ctx, "scan", 1000)
	running, _ := store.CreateTask(ctx, "scan", 1000)
	done, _ := store.CreateTask(ctx, "scan", 1000)
	if err := store.SetTaskStatus(ctx, running, "running", 0, ""); err != nil {
		t.Fatalf("set running: %v", err)
	}
	if err := store.SetTaskStatus(ctx, done, "completed", 1200, ""); err != nil {
		t.Fatalf("set completed: %v", err)
	}

	n, err := store.FailUnfinishedTasks(ctx, 9000, "应用中断")
	if err != nil {
		t.Fatalf("FailUnfinishedTasks: %v", err)
	}
	if n != 2 {
		t.Fatalf("rows affected = %d, want 2", n)
	}

	for _, id := range []int64{pending, running} {
		row, err := store.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("get task %d: %v", id, err)
		}
		if row.Status != "failed" || row.Error != "应用中断" || row.FinishedAt != 9000 {
			t.Fatalf("unexpected recovered task: %+v", row)
		}
	}

	kept, err := store.GetTask(ctx, done)
	if err != nil {
		t.Fatalf("get completed task: %v", err)
	}
	if kept.Status != "completed" {
		t.Fatalf("completed task must stay untouched: %+v", kept)
	}
}

func TestFailUnfinishedTasks_RequiresCause(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.FailUnfinishedTasks(context.Background(), 1, ""); err == nil {
		t.Fatal("want error for empty cause")
	}
}

func TestGetTask_NotFound(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.GetTask(context.Background(), 42); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows, got %v", err)
	}
}
