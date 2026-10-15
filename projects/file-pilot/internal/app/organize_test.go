package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/classifier"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/fsx"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/organizer"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

func writeAppFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// seedOrganizeScan 建一棵真实目录树 + 一条 completed 扫描行 + 两个文件行
// （a.jpg→图片、b.pdf→文档），返回 scanID 与根目录。
func seedOrganizeScan(t *testing.T, a *App) (int64, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	writeAppFile(t, filepath.Join(root, "a.jpg"), 3)
	writeAppFile(t, filepath.Join(root, "b.pdf"), 4)

	scanID, err := a.store.CreateScan(ctx, root, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	rows := []storage.FileRecord{
		{
			Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", Ext: "jpg",
			SizeBytes: 3, MtimeMS: 1, Category: classifier.CategoryImages,
			Confidence: 90, Source: classifier.SourceRule, Reason: "扩展名命中",
		},
		{
			Path: filepath.Join(root, "b.pdf"), Name: "b.pdf", Ext: "pdf",
			SizeBytes: 4, MtimeMS: 1, Category: classifier.CategoryDocs,
			Confidence: 90, Source: classifier.SourceRule, Reason: "扩展名命中",
		},
	}
	if err := a.store.InsertFiles(ctx, scanID, rows); err != nil {
		t.Fatalf("insert files: %v", err)
	}
	if err := a.store.FinishScan(ctx, scanID, storage.ScanOutcome{
		Status: "completed", FinishedAt: 2000, FileCount: 2, TotalSizeBytes: 7,
	}); err != nil {
		t.Fatalf("finish scan: %v", err)
	}
	return scanID, root
}

// TestStartOrganize_EndToEnd 跑通「整理 → 报告 → 撤销 → 重复撤销」完整闭环
// （真实文件系统 + 真实 SQLite）。
func TestStartOrganize_EndToEnd(t *testing.T) {
	a, rec := newTestApp(t)
	scanID, root := seedOrganizeScan(t, a)

	taskID, err := a.StartOrganize(scanID)
	if err != nil {
		t.Fatalf("StartOrganize: %v", err)
	}
	final := waitTerminal(t, rec)
	if final.TaskID != taskID || final.Status != "completed" || final.Type != "organize" {
		t.Fatalf("organize terminal = %+v, want completed/organize/%d", final, taskID)
	}

	// 文件落到类别目录。
	if _, err := os.Lstat(filepath.Join(root, fsx.OrganizeDirName, "图片", "a.jpg")); err != nil {
		t.Fatalf("a.jpg not organized: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, fsx.OrganizeDirName, "文档", "b.pdf")); err != nil {
		t.Fatalf("b.pdf not organized: %v", err)
	}

	// 报告（FR-10）：成功 2、失败 0、已移动 7 字节。
	report, err := a.GetOrganizeReport(taskID)
	if err != nil {
		t.Fatalf("GetOrganizeReport: %v", err)
	}
	if report.Status != "completed" || report.SuccessCount != 2 || report.FailedCount != 0 ||
		report.MovedSizeBytes != 7 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, want completed/2/0/7/no failures", report)
	}
	// D-D7 收尾日志：必须在 Stop 之后直接推送（回归：曾用 th.Log after Stop 而丢失）。
	if !containsLog(rec.logTexts(), "整理完成") {
		t.Fatalf("final organize summary log missing; logs = %v", rec.logTexts())
	}

	// 撤销（FR-11）。
	undoID, err := a.UndoLastOrganize(scanID)
	if err != nil {
		t.Fatalf("UndoLastOrganize: %v", err)
	}
	if undoFinal := waitTerminal(t, rec); undoFinal.TaskID != undoID || undoFinal.Status != "completed" {
		t.Fatalf("undo terminal = %+v, want completed/%d", undoFinal, undoID)
	}
	if !containsLog(rec.logTexts(), "撤销完成") {
		t.Fatalf("final undo summary log missing; logs = %v", rec.logTexts())
	}
	// 文件回到原处，整理目录内不再有它们。
	if _, err := os.Lstat(filepath.Join(root, "a.jpg")); err != nil {
		t.Fatalf("a.jpg not restored: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, fsx.OrganizeDirName, "图片", "a.jpg")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("organized a.jpg should be gone, err = %v", err)
	}

	// 重复撤销：原任务已 undone → ErrAlreadyUndone。
	if _, err := a.UndoLastOrganize(scanID); !errors.Is(err, ErrAlreadyUndone) {
		t.Fatalf("second undo err = %v, want ErrAlreadyUndone", err)
	}
}

// TestStartOrganize_ScanIncomplete：未完成或未知扫描一律拒绝（FR-9）。
func TestStartOrganize_ScanIncomplete(t *testing.T) {
	a, _ := newTestApp(t)
	ctx := context.Background()
	root := t.TempDir()

	// running 扫描（未 FinishScan）。
	runningID, err := a.store.CreateScan(ctx, root, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	if _, err := a.StartOrganize(runningID); !errors.Is(err, ErrScanIncomplete) {
		t.Fatalf("running scan err = %v, want ErrScanIncomplete", err)
	}
	// 未知扫描。
	if _, err := a.StartOrganize(424242); !errors.Is(err, ErrScanIncomplete) {
		t.Fatalf("unknown scan err = %v, want ErrScanIncomplete", err)
	}
}

// TestStartOrganize_TargetDirCreateFailsZeroMove：目标目录无法创建时返回
// ErrTargetDirCreate，且零移动、零任务。
func TestStartOrganize_TargetDirCreateFailsZeroMove(t *testing.T) {
	a, _ := newTestApp(t)
	ctx := context.Background()
	root := t.TempDir()
	writeAppFile(t, filepath.Join(root, "a.jpg"), 3)
	// 在目标根路径上放一个同名文件，使 MkdirAll 失败。
	writeAppFile(t, filepath.Join(root, fsx.OrganizeDirName), 1)

	scanID, err := a.store.CreateScan(ctx, root, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	if err := a.store.InsertFiles(ctx, scanID, []storage.FileRecord{{
		Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", Ext: "jpg",
		SizeBytes: 3, MtimeMS: 1, Category: classifier.CategoryImages,
		Confidence: 90, Source: classifier.SourceRule, Reason: "扩展名命中",
	}}); err != nil {
		t.Fatalf("insert files: %v", err)
	}
	if err := a.store.FinishScan(ctx, scanID, storage.ScanOutcome{Status: "completed", FinishedAt: 2000}); err != nil {
		t.Fatalf("finish scan: %v", err)
	}

	if _, err := a.StartOrganize(scanID); !errors.Is(err, organizer.ErrTargetDirCreate) {
		t.Fatalf("StartOrganize err = %v, want ErrTargetDirCreate", err)
	}
	// 源文件未动、无活动任务。
	if _, err := os.Lstat(filepath.Join(root, "a.jpg")); err != nil {
		t.Fatalf("source must remain: %v", err)
	}
	active, err := a.GetActiveTask()
	if err != nil {
		t.Fatalf("GetActiveTask: %v", err)
	}
	if active != nil {
		t.Fatalf("active = %+v, want nil (no task created)", active)
	}
}

// TestUndoLastOrganize_NothingToUndo：该扫描没有任何成功整理 → ErrNothingToUndo。
func TestUndoLastOrganize_NothingToUndo(t *testing.T) {
	a, _ := newTestApp(t)
	scanID, _ := seedOrganizeScan(t, a)
	if _, err := a.UndoLastOrganize(scanID); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("err = %v, want ErrNothingToUndo", err)
	}
}
