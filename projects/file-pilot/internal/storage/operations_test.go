package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// appendOp 追加一条操作记录（测试辅助）；失败状态带固定原因。
func appendOp(t *testing.T, store *Store, taskID int64, action, status string, size int64) {
	t.Helper()
	reason := ""
	if status == OperationFailed {
		reason = "失败原因"
	}
	if err := store.AppendOperation(context.Background(), OperationRecord{
		TaskID: taskID, Action: action, SrcPath: "src", DstPath: "dst",
		Status: status, Error: reason, SizeBytes: size, CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("AppendOperation: %v", err)
	}
}

// TestOperations_RoundTripAndAggregate 覆盖 operations 的写入、成功过滤、失败
// 明细与聚合（FR-9/FR-10/FR-11）。
func TestOperations_RoundTripAndAggregate(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	taskID, err := store.CreateTask(ctx, "organize", 1000)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	appendOp(t, store, taskID, ActionMove, OperationSuccess, 10)
	appendOp(t, store, taskID, ActionMove, OperationFailed, 0)
	appendOp(t, store, taskID, ActionMove, OperationSuccess, 5)
	appendOp(t, store, taskID, ActionUndo, OperationSuccess, 10)

	// 成功 move 只含两条成功的 move（不含 failed、不含 undo）。
	moves, err := store.ListSuccessfulMoves(ctx, taskID)
	if err != nil {
		t.Fatalf("ListSuccessfulMoves: %v", err)
	}
	if len(moves) != 2 {
		t.Fatalf("successful moves = %d, want 2", len(moves))
	}

	// 失败明细只含 move 的失败项。
	fails, err := store.ListFailedOperations(ctx, taskID, ActionMove)
	if err != nil {
		t.Fatalf("ListFailedOperations: %v", err)
	}
	if len(fails) != 1 || fails[0].Error != "失败原因" {
		t.Fatalf("move failures = %+v", fails)
	}

	// 聚合：成功 2 / 失败 1 / 已移动 15。
	sum, err := store.SummarizeOperations(ctx, taskID, ActionMove)
	if err != nil {
		t.Fatalf("SummarizeOperations: %v", err)
	}
	if sum.SuccessCount != 2 || sum.FailedCount != 1 || sum.MovedSizeBytes != 15 {
		t.Fatalf("summary = %+v, want 2/1/15", sum)
	}

	// undo 聚合独立。
	undoSum, err := store.SummarizeOperations(ctx, taskID, ActionUndo)
	if err != nil {
		t.Fatalf("SummarizeOperations(undo): %v", err)
	}
	if undoSum.SuccessCount != 1 || undoSum.FailedCount != 0 {
		t.Fatalf("undo summary = %+v, want 1/0", undoSum)
	}
}

func TestOperations_Validation(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	if err := store.AppendOperation(ctx, OperationRecord{Action: ActionMove, Status: OperationSuccess}); err == nil {
		t.Error("want error for non-positive task id")
	}
	taskID, _ := store.CreateTask(ctx, "organize", 1000)
	if err := store.AppendOperation(ctx, OperationRecord{TaskID: taskID, Status: OperationSuccess}); err == nil {
		t.Error("want error for empty action")
	}
	if err := store.AppendOperation(ctx, OperationRecord{TaskID: taskID, Action: ActionMove}); err == nil {
		t.Error("want error for empty status")
	}
	if _, err := store.ListSuccessfulMoves(ctx, 0); err == nil {
		t.Error("want error for ListSuccessfulMoves task 0")
	}
	if _, err := store.SummarizeOperations(ctx, taskID, ""); err == nil {
		t.Error("want error for empty summarize action")
	}
	if _, err := store.ListFailedOperations(ctx, 0, ActionMove); err == nil {
		t.Error("want error for ListFailedOperations task 0")
	}
	if _, err := store.ListFailedOperations(ctx, taskID, ""); err == nil {
		t.Error("want error for empty list action")
	}
}

// TestGetLatestUndoableOrganize 覆盖撤销目标选择（P1-2）：必须挑「有成功
// move 的最新的 organize」，撤销任务不得被选中。
func TestGetLatestUndoableOrganize(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	scanID, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("CreateScan: %v", err)
	}

	// 第一个整理任务：一次成功（可撤销）。
	first, _ := store.CreateTask(ctx, "organize", 1000)
	if err := store.SetTaskScan(ctx, first, scanID); err != nil {
		t.Fatalf("SetTaskScan: %v", err)
	}
	appendOp(t, store, first, ActionMove, OperationSuccess, 1)

	// 第二个整理任务：最新且有成功 move → 应被选中。
	second, _ := store.CreateTask(ctx, "organize", 2000)
	if err := store.SetTaskScan(ctx, second, scanID); err != nil {
		t.Fatalf("SetTaskScan: %v", err)
	}
	appendOp(t, store, second, ActionMove, OperationSuccess, 2)

	got, err := store.GetLatestUndoableOrganize(ctx, scanID)
	if err != nil {
		t.Fatalf("GetLatestUndoableOrganize: %v", err)
	}
	if got.ID != second {
		t.Fatalf("selected task = %d, want %d (latest)", got.ID, second)
	}
	if got.Undone {
		t.Fatal("fresh organize should not be undone")
	}

	// 撤销任务（只有 undo 记录）不得被选中。
	undoTask, _ := store.CreateTask(ctx, "organize", 3000)
	if err := store.SetTaskScan(ctx, undoTask, scanID); err != nil {
		t.Fatalf("SetTaskScan: %v", err)
	}
	if err := store.SetTaskParent(ctx, undoTask, second); err != nil {
		t.Fatalf("SetTaskParent: %v", err)
	}
	appendOp(t, store, undoTask, ActionUndo, OperationSuccess, 2)

	got, err = store.GetLatestUndoableOrganize(ctx, scanID)
	if err != nil {
		t.Fatalf("GetLatestUndoableOrganize after undo: %v", err)
	}
	if got.ID != second {
		t.Fatalf("selected = %d, want %d (undo task must be excluded)", got.ID, second)
	}
}

func TestGetLatestUndoableOrganize_None(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	scanID, _ := store.CreateScan(ctx, `C:\data`, 1000, 4)

	// 无 organize 任务。
	if _, err := store.GetLatestUndoableOrganize(ctx, scanID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}

	// 有 organize 但只有失败 move → 仍无可撤销。
	taskID, _ := store.CreateTask(ctx, "organize", 1000)
	_ = store.SetTaskScan(ctx, taskID, scanID)
	appendOp(t, store, taskID, ActionMove, OperationFailed, 0)
	if _, err := store.GetLatestUndoableOrganize(ctx, scanID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}

	if _, err := store.GetLatestUndoableOrganize(ctx, 0); err == nil {
		t.Error("want error for invalid scan id")
	}
}

func TestMarkTaskUndone(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	id, _ := store.CreateTask(ctx, "organize", 1000)

	if err := store.MarkTaskUndone(ctx, id); err != nil {
		t.Fatalf("MarkTaskUndone: %v", err)
	}
	row, err := store.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if !row.Undone {
		t.Fatal("undone = false, want true")
	}
	if err := store.MarkTaskUndone(ctx, 9999); err == nil {
		t.Error("want error for missing task")
	}
}

func TestSetTaskParent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	parent, _ := store.CreateTask(ctx, "organize", 1000)
	child, _ := store.CreateTask(ctx, "organize", 2000)

	if err := store.SetTaskParent(ctx, child, parent); err != nil {
		t.Fatalf("SetTaskParent: %v", err)
	}
	row, err := store.GetTask(ctx, child)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if row.ParentTaskID != parent {
		t.Fatalf("parent = %d, want %d", row.ParentTaskID, parent)
	}
	if err := store.SetTaskParent(ctx, 0, parent); err == nil {
		t.Error("want error for task id 0")
	}
	if err := store.SetTaskParent(ctx, child, 0); err == nil {
		t.Error("want error for parent id 0")
	}
}

// TestListOrganizeCandidates_ExcludesExcluded 覆盖候选过滤与排序（FR-9.2）。
func TestListOrganizeCandidates_ExcludesExcluded(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	scanID, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("CreateScan: %v", err)
	}
	rows := []FileRecord{
		{Path: `C:\data\a.jpg`, Name: "a.jpg", Ext: "jpg", SizeBytes: 1, MtimeMS: 1, Category: "图片", Confidence: 90, Source: "rule", Reason: "r"},
		{Path: `C:\data\b.pdf`, Name: "b.pdf", Ext: "pdf", SizeBytes: 2, MtimeMS: 1, Category: "文档", Confidence: 90, Source: "rule", Reason: "r"},
		{Path: `C:\data\c.png`, Name: "c.png", Ext: "png", SizeBytes: 3, MtimeMS: 1, Category: "图片", Confidence: 90, Source: "rule", Reason: "r"},
	}
	if err := store.InsertFiles(ctx, scanID, rows); err != nil {
		t.Fatalf("InsertFiles: %v", err)
	}
	// 排除 b.pdf：不应出现在候选中。
	plan, err := store.GetFilePlan(ctx, 2)
	if err != nil {
		t.Fatalf("GetFilePlan: %v", err)
	}
	if err := store.UpdateFilePlan(ctx, FilePlanUpdate{
		ID: 2, Category: plan.Category, Source: plan.Source,
		Confidence: plan.Confidence, Reason: plan.Reason, Excluded: true,
	}); err != nil {
		t.Fatalf("UpdateFilePlan: %v", err)
	}

	cands, err := store.ListOrganizeCandidates(ctx, scanID)
	if err != nil {
		t.Fatalf("ListOrganizeCandidates: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %d, want 2 (excluded filtered)", len(cands))
	}
	// 排序：category 升序（"图片" < "文档" 取决于字节序，这里只断言分组相邻）。
	for _, c := range cands {
		if c.Name == "b.pdf" {
			t.Fatal("excluded file must not be a candidate")
		}
	}
	if _, err := store.ListOrganizeCandidates(ctx, 0); err == nil {
		t.Error("want error for invalid scan id")
	}
}

// TestCountOrganizeCandidates 覆盖「即将整理」数量口径（excluded=0，FR-9.1）。
func TestCountOrganizeCandidates(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	scanID, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("CreateScan: %v", err)
	}
	rows := []FileRecord{
		{Path: `C:\data\a.jpg`, Name: "a.jpg", Ext: "jpg", SizeBytes: 1, MtimeMS: 1, Category: "图片", Confidence: 90, Source: "rule", Reason: "r"},
		{Path: `C:\data\b.pdf`, Name: "b.pdf", Ext: "pdf", SizeBytes: 2, MtimeMS: 1, Category: "文档", Confidence: 90, Source: "rule", Reason: "r"},
		{Path: `C:\data\c.png`, Name: "c.png", Ext: "png", SizeBytes: 3, MtimeMS: 1, Category: "图片", Confidence: 90, Source: "rule", Reason: "r"},
	}
	if err := store.InsertFiles(ctx, scanID, rows); err != nil {
		t.Fatalf("InsertFiles: %v", err)
	}

	// 全部未排除：计数 = 3。
	if n, err := store.CountOrganizeCandidates(ctx, scanID); err != nil || n != 3 {
		t.Fatalf("count = %d (err %v), want 3", n, err)
	}

	// 排除 b.pdf（id=2）：计数 = 2，与候选列表长度一致。
	plan, err := store.GetFilePlan(ctx, 2)
	if err != nil {
		t.Fatalf("GetFilePlan: %v", err)
	}
	if err := store.UpdateFilePlan(ctx, FilePlanUpdate{
		ID: 2, Category: plan.Category, Source: plan.Source,
		Confidence: plan.Confidence, Reason: plan.Reason, Excluded: true,
	}); err != nil {
		t.Fatalf("UpdateFilePlan: %v", err)
	}
	n, err := store.CountOrganizeCandidates(ctx, scanID)
	if err != nil {
		t.Fatalf("CountOrganizeCandidates: %v", err)
	}
	cands, err := store.ListOrganizeCandidates(ctx, scanID)
	if err != nil {
		t.Fatalf("ListOrganizeCandidates: %v", err)
	}
	if n != 2 || int(n) != len(cands) {
		t.Fatalf("count = %d, candidates = %d, want 2/2", n, len(cands))
	}

	if _, err := store.CountOrganizeCandidates(ctx, 0); err == nil {
		t.Error("want error for invalid scan id")
	}
}
