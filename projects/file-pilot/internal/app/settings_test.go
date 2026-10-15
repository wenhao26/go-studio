package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/diag"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// TestGetSettings_Defaults：键缺失时返回默认配置与空最近目录列表（FR-1/FU-2）。
func TestGetSettings_Defaults(t *testing.T) {
	a, _ := newTestApp(t)

	s, err := a.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.Workers != scanner.DefaultConfig().Workers {
		t.Errorf("workers = %d, want default %d", s.Workers, scanner.DefaultConfig().Workers)
	}
	if s.ScanHidden || s.ScanSystem || s.SkipLarge {
		t.Errorf("switches must default to false: %+v", s.Config)
	}
	if s.RecentDirs == nil || len(s.RecentDirs) != 0 {
		t.Errorf("recent_dirs = %#v, want non-nil empty slice", s.RecentDirs)
	}
}

func TestSaveSettings_RoundTrip(t *testing.T) {
	a, _ := newTestApp(t)

	want := scanner.Config{Workers: 8, ScanHidden: true, ScanSystem: false, SkipLarge: true}
	if err := a.SaveSettings(want); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	got, err := a.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.Config != want {
		t.Errorf("config = %+v, want %+v", got.Config, want)
	}
}

// TestSaveSettings_InvalidWorkers：非法配置拒绝保存，且按契约返回裸哨兵
// （message 即前端匹配键）。
func TestSaveSettings_InvalidWorkers(t *testing.T) {
	a, _ := newTestApp(t)

	for _, workers := range []int{0, -1, 65} {
		err := a.SaveSettings(scanner.Config{Workers: workers})
		if !errors.Is(err, scanner.ErrInvalidWorkers) {
			t.Errorf("workers=%d: err = %v, want ErrInvalidWorkers", workers, err)
		}
		if err.Error() != "workers must be between 1 and 64" {
			t.Errorf("workers=%d: message = %q, want bare sentinel key", workers, err.Error())
		}
	}
}

// TestGetSettings_CorruptConfigReturnsBareSentinel：持久化数据损坏时按契约
// 返回裸 ErrConfigInvalid（前端映射「请检查扫描配置」），不静默回退默认值。
func TestGetSettings_CorruptConfigReturnsBareSentinel(t *testing.T) {
	a, _ := newTestApp(t)
	ctx := context.Background()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid json", raw: "{oops"},
		{name: "out of range workers", raw: `{"workers":0,"scan_hidden":false,"scan_system":false,"skip_large":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.store.SetSetting(ctx, settingScanConfig, tt.raw); err != nil {
				t.Fatalf("seed corrupt config: %v", err)
			}
			_, err := a.GetSettings()
			if !errors.Is(err, ErrConfigInvalid) {
				t.Fatalf("err = %v, want ErrConfigInvalid", err)
			}
			if err.Error() != "invalid scan config" {
				t.Fatalf("message = %q, want bare sentinel key", err.Error())
			}
		})
	}
}

// TestRememberDir_DedupAndCap：去重、最新在前、上限 20 条（FR-13）。
func TestRememberDir_DedupAndCap(t *testing.T) {
	a, _ := newTestApp(t)
	ctx := context.Background()

	const total = 25
	for i := 0; i < total; i++ {
		root := fmt.Sprintf(`C:\root%02d`, i)
		if err := a.rememberDir(ctx, root); err != nil {
			t.Fatalf("rememberDir %d: %v", i, err)
		}
	}
	// 重复写入同一次目录不产生副本。
	if err := a.rememberDir(ctx, fmt.Sprintf(`C:\root%02d`, total-1)); err != nil {
		t.Fatalf("rememberDir repeat: %v", err)
	}

	dirs, err := a.loadRecentDirs(ctx)
	if err != nil {
		t.Fatalf("loadRecentDirs: %v", err)
	}
	if len(dirs) != recentDirsLimit {
		t.Fatalf("len(recent_dirs) = %d, want %d", len(dirs), recentDirsLimit)
	}
	if dirs[0] != fmt.Sprintf(`C:\root%02d`, total-1) {
		t.Fatalf("newest dir = %q, want %q", dirs[0], fmt.Sprintf(`C:\root%02d`, total-1))
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if seen[d] {
			t.Fatalf("duplicate entry %q", d)
		}
		seen[d] = true
	}
}

// TestRemoveRecentDir：移除最近目录——删存在的项、保留顺序、幂等删不存在的项。
func TestRemoveRecentDir(t *testing.T) {
	a, _ := newTestApp(t)
	ctx := context.Background()

	seed := []string{`C:\beta`, `C:\alpha`, `C:\gamma`}
	for _, d := range seed {
		if err := a.rememberDir(ctx, d); err != nil {
			t.Fatalf("rememberDir %s: %v", d, err)
		}
	}
	// rememberDir 最新在前，故列表应为 gamma, alpha, beta。
	before, err := a.loadRecentDirs(ctx)
	if err != nil {
		t.Fatalf("loadRecentDirs: %v", err)
	}
	wantBefore := []string{`C:\gamma`, `C:\alpha`, `C:\beta`}
	if fmt.Sprint(before) != fmt.Sprint(wantBefore) {
		t.Fatalf("before = %#v, want %#v", before, wantBefore)
	}

	// 删除中间项后其余顺序不变。
	if err := a.RemoveRecentDir(`C:\alpha`); err != nil {
		t.Fatalf("RemoveRecentDir: %v", err)
	}
	got, err := a.loadRecentDirs(ctx)
	if err != nil {
		t.Fatalf("loadRecentDirs: %v", err)
	}
	want := []string{`C:\gamma`, `C:\beta`}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after = %#v, want %#v", got, want)
	}

	// 幂等：删除不存在的项返回 nil 且不改动列表。
	if err := a.RemoveRecentDir(`C:\missing`); err != nil {
		t.Fatalf("RemoveRecentDir(missing): %v", err)
	}
	got, err = a.loadRecentDirs(ctx)
	if err != nil {
		t.Fatalf("loadRecentDirs: %v", err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after idempotent = %#v, want %#v", got, want)
	}

	// 空目录是防御性 no-op。
	if err := a.RemoveRecentDir(""); err != nil {
		t.Fatalf("RemoveRecentDir(empty): %v", err)
	}
}

// TestBindings_NotStarted：启动完成前调用绑定返回明确错误而非空指针/弹窗。
func TestBindings_NotStarted(t *testing.T) {
	store := newStore(t)
	a := New(store, nil) // nil logger 走 stderr 降级
	t.Cleanup(a.Close)

	if _, err := a.ChooseDirectory(); !errors.Is(err, errNotStarted) {
		t.Errorf("ChooseDirectory err = %v, want errNotStarted", err)
	}
	if _, err := a.GetSettings(); !errors.Is(err, errNotStarted) {
		t.Errorf("GetSettings err = %v, want errNotStarted", err)
	}
	if err := a.SaveSettings(scanner.DefaultConfig()); !errors.Is(err, errNotStarted) {
		t.Errorf("SaveSettings err = %v, want errNotStarted", err)
	}
	if _, err := a.StartScan(t.TempDir(), scanner.DefaultConfig()); !errors.Is(err, errNotStarted) {
		t.Errorf("StartScan err = %v, want errNotStarted", err)
	}
	if err := a.CancelTask(1); !errors.Is(err, errNotStarted) {
		t.Errorf("CancelTask err = %v, want errNotStarted", err)
	}
	if _, err := a.GetScanSummary(1); !errors.Is(err, errNotStarted) {
		t.Errorf("GetScanSummary err = %v, want errNotStarted", err)
	}
	if _, err := a.ListFiles(1, "", 1, 100); !errors.Is(err, errNotStarted) {
		t.Errorf("ListFiles err = %v, want errNotStarted", err)
	}
}

// TestStartup_RecoversInterruptedState：FR-12（遗留 running 置 failed）
// 与 FR-13（保留最近 20 次扫描）在同一启动流程中生效。
func TestStartup_RecoversInterruptedState(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// 先种 25 个已完成的历史扫描（id 较小），再种崩溃现场——
	// PruneOldScans 按插入序保留最近 keep 次，现场扫描必须是最新的一条。
	for i := 0; i < 25; i++ {
		id, err := store.CreateScan(ctx, `C:\old`, int64(1000+i), 4)
		if err != nil {
			t.Fatalf("seed old scan %d: %v", i, err)
		}
		if err := store.FinishScan(ctx, id, storage.ScanOutcome{Status: "completed", FinishedAt: int64(2000 + i)}); err != nil {
			t.Fatalf("finish old scan %d: %v", i, err)
		}
	}

	// 遗留的崩溃现场：1 个 running 扫描 + 1 个 running 任务。
	runningScan, err := store.CreateScan(ctx, `C:\crashed`, 900_000, 4)
	if err != nil {
		t.Fatalf("seed running scan: %v", err)
	}
	runningTask, err := store.CreateTask(ctx, "scan", 900_000)
	if err != nil {
		t.Fatalf("seed running task: %v", err)
	}
	if err := store.SetTaskScan(ctx, runningTask, runningScan); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := store.SetTaskStatus(ctx, runningTask, "running", 0, ""); err != nil {
		t.Fatalf("seed running status: %v", err)
	}

	a := New(store, diag.NewStderr())
	a.Startup(context.Background())
	t.Cleanup(a.Close)

	taskRow, err := store.GetTask(ctx, runningTask)
	if err != nil {
		t.Fatalf("get recovered task: %v", err)
	}
	if taskRow.Status != "failed" || taskRow.Error != "应用中断" {
		t.Fatalf("recovered task = %+v, want failed/应用中断", taskRow)
	}
	scanRow, err := store.GetScan(ctx, runningScan)
	if err != nil {
		t.Fatalf("get recovered scan: %v", err)
	}
	if scanRow.Status != "failed" {
		t.Fatalf("recovered scan = %+v, want failed", scanRow)
	}

	// FR-12：恢复结果经 GetRecoveryNotice 暴露给前端（无恢复时期望 nil）。
	notice, err := a.GetRecoveryNotice()
	if err != nil {
		t.Fatalf("GetRecoveryNotice: %v", err)
	}
	if notice == nil || notice.RecoveredTasks != 1 || notice.RecoveredScans != 1 {
		t.Fatalf("recovery notice = %+v, want tasks=1 scans=1", notice)
	}

	// 保留计数：逐个探测全部种子扫描，存活数必须恰为 retainedScans。
	//（现场扫描插入序最新，必然在保留集内。）
	survived := 0
	for id := int64(1); id <= 26; id++ {
		if _, err := store.GetScan(ctx, id); err == nil {
			survived++
		} else if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("get scan %d: %v", id, err)
		}
	}
	if survived != retainedScans {
		t.Fatalf("surviving scans = %d, want %d（FR-13 保留最近 20 次）", survived, retainedScans)
	}
}

// TestGetRecoveryNotice_NoRecovery：干净启动不产生恢复提示，返回 null。
func TestGetRecoveryNotice_NoRecovery(t *testing.T) {
	a, _ := newTestApp(t)

	notice, err := a.GetRecoveryNotice()
	if err != nil {
		t.Fatalf("GetRecoveryNotice: %v", err)
	}
	if notice != nil {
		t.Fatalf("recovery notice = %+v, want nil", notice)
	}
}
