package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/task"
)

// seedScanTree 构造一棵真实扫描树：
//
//	root/a.jpg (2) · root/b.pdf (3) · root/sub/c.txt (4) · root/.secret (隐藏, 1)
func seedScanTree(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	files := map[string]int{
		"a.jpg":                       2,
		"b.pdf":                       3,
		filepath.Join("sub", "c.txt"): 4,
		".secret":                     1,
	}
	for name, size := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// TestStartScan_EndToEnd 以真实文件系统 + 真实 SQLite 跑通完整链路：
// StartScan → 扫描 → 事件 → 终态落库 → 摘要/分页查询。
func TestStartScan_EndToEnd(t *testing.T) {
	a, rec := newTestApp(t)
	ctx := context.Background()
	root := seedScanTree(t)

	taskID, err := a.StartScan(root, scanner.Config{Workers: 2})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if taskID <= 0 {
		t.Fatalf("task id = %d, want > 0", taskID)
	}
	// 单活动约束由 task.Manager 保证（manager_test 已覆盖 Begin 的
	// ErrTaskRunning 分支）；此处若重复 StartScan 会产生并发扫描竞态，不在此测试。

	final := waitTerminal(t, rec)
	if final.TaskID != taskID {
		t.Fatalf("terminal event task_id = %d, want %d", final.TaskID, taskID)
	}
	if final.Status != "completed" {
		t.Fatalf("final status = %q, want completed（事件：%+v；日志：%v）",
			final.Status, rec.statusList(), rec.logTexts())
	}

	// 状态事件序列必须是 running → completed（终态必达，contracts §6）。
	statuses := rec.statusList()
	if len(statuses) < 2 {
		t.Fatalf("status events = %+v, want running + completed", statuses)
	}
	if statuses[0].Status != "running" {
		t.Fatalf("first status = %q, want running", statuses[0].Status)
	}
	last := statuses[len(statuses)-1]
	if last.Status != "completed" || last.Error != "" {
		t.Fatalf("last status = %+v, want completed with empty error", last)
	}

	// 摘要（FR-5）：3 个文件、2 个目录（root+sub）、9 字节、隐藏文件跳过 1。
	taskRow, err := a.store.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	summary, err := a.GetScanSummary(taskRow.ScanID)
	if err != nil {
		t.Fatalf("GetScanSummary: %v", err)
	}
	if summary.Status != "completed" || summary.FileCount != 3 || summary.DirCount != 2 ||
		summary.TotalSizeBytes != 9 || summary.SkippedCount != 1 {
		t.Fatalf("summary = %+v, want completed/3/2/9/1", summary)
	}
	// D-D9：整理二次确认的数量口径（可整理 = excluded=0）；本次无排除，应与总数一致。
	if summary.OrganizableCount != summary.FileCount {
		t.Fatalf("organizable_count = %d, want %d", summary.OrganizableCount, summary.FileCount)
	}
	if summary.MaxWorkers != 2 {
		t.Fatalf("max_workers = %d, want 2", summary.MaxWorkers)
	}
	if summary.DurationMS < 0 || summary.AvgSpeed <= 0 {
		t.Fatalf("timing fields invalid: %+v", summary)
	}

	// D-C1：task:status 载荷携带 scan_id，结果页据此查询汇总/详情。
	if statuses[0].ScanID != taskRow.ScanID || last.ScanID != taskRow.ScanID {
		t.Fatalf("status scan_id = %d/%d, want %d",
			statuses[0].ScanID, last.ScanID, taskRow.ScanID)
	}

	// 分页与类别过滤（P2-9）。
	page, err := a.ListFiles(taskRow.ScanID, "", 1, 100)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if page.TotalCount != 3 || len(page.Items) != 3 || page.TotalPage != 1 {
		t.Fatalf("page = %+v, want 3 items/1 page", page)
	}
	// 非法分页参数被钳制为默认值（contracts §5）。
	clamped, err := a.ListFiles(taskRow.ScanID, "", 0, 0)
	if err != nil {
		t.Fatalf("ListFiles clamped: %v", err)
	}
	if clamped.Page != 1 || clamped.PageSize != defaultPageSize {
		t.Fatalf("clamped page = %d/%d, want 1/%d", clamped.Page, clamped.PageSize, defaultPageSize)
	}

	images, err := a.ListFiles(taskRow.ScanID, "图片", 1, 100)
	if err != nil {
		t.Fatalf("ListFiles by category: %v", err)
	}
	if images.TotalCount != 1 || len(images.Items) != 1 || images.Items[0].Name != "a.jpg" {
		t.Fatalf("images = %+v, want exactly a.jpg", images.Items)
	}

	// 恢复与收尾状态（FR-14）。
	active, err := a.GetActiveTask()
	if err != nil {
		t.Fatalf("GetActiveTask: %v", err)
	}
	if active != nil {
		t.Fatalf("active = %+v, want nil after completion", active)
	}

	// 最近目录已记录（FR-13）。
	settings, err := a.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if len(settings.RecentDirs) == 0 || settings.RecentDirs[0] != root {
		t.Fatalf("recent_dirs = %+v, want [%s] first", settings.RecentDirs, root)
	}

	// 已终态任务的取消幂等返回 nil（contracts §5）。
	if err := a.CancelTask(taskID); err != nil {
		t.Fatalf("cancel terminal task must be idempotent, got %v", err)
	}
}

// TestGetScanSummary_UnknownScan：未知扫描返回未映射错误（前端通用文案）。
func TestGetScanSummary_UnknownScan(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.GetScanSummary(424242); err == nil {
		t.Fatal("want error for unknown scan")
	}
}

// TestListFiles_UnknownScan：未知扫描的文件列表为空结果而非 panic。
func TestListFiles_UnknownScan(t *testing.T) {
	a, _ := newTestApp(t)
	page, err := a.ListFiles(424242, "", 1, 100)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if page.TotalCount != 0 || len(page.Items) != 0 {
		t.Fatalf("page = %+v, want empty", page)
	}
}

// TestShutdown_ReleasesResources：优雅退出后库与日志释放，重复关闭幂等。
func TestShutdown_ReleasesResources(t *testing.T) {
	store := newStore(t)
	a := New(store, nil)
	a.publish = newEventRecorder().publish
	a.Startup(context.Background())

	a.Shutdown(context.Background())

	// 关闭后查询必须报错（库已释放）。
	if _, err := a.GetScanSummary(1); err == nil {
		t.Fatal("store must be closed after Shutdown")
	}
	// 幂等：main 的 defer 兜底路径再次调用不产生二次错误。
	a.Close()

	// 关闭后事件出口静默跳过（无接收方），不 panic。
	a.EmitStatus(task.StatusEvent{TaskID: 1, Type: task.TypeScan, Status: task.StatusCompleted})
}
