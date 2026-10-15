package organizer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/fsx"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// ---- 测试脚手架 ----

// fakeFS 包装真实文件系统，允许按用例注入错误分支（权限/占用等无法在真实
// 文件系统上稳定构造，AGENTS.md §32.2 要求覆盖错误路径）。
type fakeFS struct {
	rename func(oldpath, newpath string) error
	mkdir  func(path string, perm fs.FileMode) error
	lstat  func(path string) (fs.FileInfo, error)
}

func (f fakeFS) Rename(oldpath, newpath string) error {
	if f.rename != nil {
		return f.rename(oldpath, newpath)
	}
	return os.Rename(oldpath, newpath)
}

func (f fakeFS) MkdirAll(path string, perm fs.FileMode) error {
	if f.mkdir != nil {
		return f.mkdir(path, perm)
	}
	return os.MkdirAll(path, perm)
}

func (f fakeFS) Lstat(path string) (fs.FileInfo, error) {
	if f.lstat != nil {
		return f.lstat(path)
	}
	return os.Lstat(path)
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "file-pilot.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

// newTaskRow 建一条 organize 任务行（operations.task_id 有外键，必须先建行）。
func newTaskRow(t *testing.T, store *storage.Store) int64 {
	t.Helper()
	id, err := store.CreateTask(context.Background(), "organize", 1000)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return id
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// ---- 整理：正常路径 ----
// TestMove_MovesAndRecords：两条文件的正常整理 —— 文件落到类别目录、
// 逐条成功记录落库、已移动数据量对账一致。
func TestMove_MovesAndRecords(t *testing.T) {
	store := openTestStore(t)
	taskID := newTaskRow(t, store)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.jpg"), "AAA")
	writeFile(t, filepath.Join(root, "b.pdf"), "BBBB")

	o := New(fsx.OSFileSystem{}, store)
	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", SizeBytes: 3, Category: "图片"},
		{Path: filepath.Join(root, "b.pdf"), Name: "b.pdf", SizeBytes: 4, Category: "文档"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := o.EnsureTargetDirs(plan); err != nil {
		t.Fatalf("EnsureTargetDirs: %v", err)
	}

	var results []ItemResult
	rep, err := o.Move(context.Background(), taskID, plan, Hooks{
		OnTotal: func(total int64) {
			if total != 2 {
				t.Errorf("OnTotal = %d, want 2", total)
			}
		},
		OnItem: func(r ItemResult) { results = append(results, r) },
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if rep.Success != 2 || rep.Failed != 0 || rep.MovedSizeBytes != 7 || rep.Canceled {
		t.Fatalf("report = %+v, want success=2 failed=0 moved=7", rep)
	}
	if len(results) != 2 {
		t.Fatalf("OnItem calls = %d, want 2", len(results))
	}

	// 文件已移动到目标类别目录，内容不变。
	if got := readFile(t, filepath.Join(root, fsx.OrganizeDirName, "图片", "a.jpg")); got != "AAA" {
		t.Errorf("moved a.jpg content = %q, want AAA", got)
	}
	if got := readFile(t, filepath.Join(root, fsx.OrganizeDirName, "文档", "b.pdf")); got != "BBBB" {
		t.Errorf("moved b.pdf content = %q, want BBBB", got)
	}
	if _, err := os.Lstat(filepath.Join(root, "a.jpg")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("source a.jpg should be gone, stat err = %v", err)
	}

	// 逐条记录 + 聚合对账（FR-10）。
	moves, err := store.ListSuccessfulMoves(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ListSuccessfulMoves: %v", err)
	}
	if len(moves) != 2 {
		t.Fatalf("successful moves = %d, want 2", len(moves))
	}
	summary, err := store.SummarizeOperations(context.Background(), taskID, storage.ActionMove)
	if err != nil {
		t.Fatalf("SummarizeOperations: %v", err)
	}
	if summary.SuccessCount != 2 || summary.FailedCount != 0 || summary.MovedSizeBytes != 7 {
		t.Fatalf("summary = %+v, want success=2 failed=0 moved=7", summary)
	}
}

// ---- 整理：绝不覆盖 ----

// TestMove_ConflictRenameNeverOverwrites：目标已存在时必须改名，既有文件
// 内容一字未改（SEC-1「绝不覆盖」核心断言）。
func TestMove_ConflictRenameNeverOverwrites(t *testing.T) {
	store := openTestStore(t)
	taskID := newTaskRow(t, store)
	root := t.TempDir()

	targetDir := filepath.Join(root, fsx.OrganizeDirName, "图片")
	writeFile(t, filepath.Join(targetDir, "a.jpg"), "PREEXISTING")
	writeFile(t, filepath.Join(root, "a.jpg"), "NEWCONTENT")

	o := New(fsx.OSFileSystem{}, store)
	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", SizeBytes: 10, Category: "图片"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := o.EnsureTargetDirs(plan); err != nil {
		t.Fatalf("EnsureTargetDirs: %v", err)
	}

	rep, err := o.Move(context.Background(), taskID, plan, Hooks{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if rep.Success != 1 || rep.Failed != 0 {
		t.Fatalf("report = %+v, want success=1", rep)
	}

	// 既有文件未被覆盖。
	if got := readFile(t, filepath.Join(targetDir, "a.jpg")); got != "PREEXISTING" {
		t.Errorf("preexisting file overwritten: %q", got)
	}
	// 新文件落到 a (1).jpg。
	if got := readFile(t, filepath.Join(targetDir, "a (1).jpg")); got != "NEWCONTENT" {
		t.Errorf("renamed file content = %q, want NEWCONTENT", got)
	}
}

// TestMove_NoUniqueNameFails：冲突改名尝试上限耗尽时该条失败（不无限枚举）。
func TestMove_NoUniqueNameFails(t *testing.T) {
	store := openTestStore(t)
	taskID := newTaskRow(t, store)
	root := t.TempDir()
	targetDir := filepath.Join(root, fsx.OrganizeDirName, "图片")
	writeFile(t, filepath.Join(targetDir, "a.jpg"), "x")
	writeFile(t, filepath.Join(targetDir, "a (1).jpg"), "x")
	writeFile(t, filepath.Join(root, "a.jpg"), "NEW")

	o := New(fsx.OSFileSystem{}, store)
	o.maxConflict = 1 // 只尝试 (1)，已被占用 → 无可用名

	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", SizeBytes: 3, Category: "图片"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := o.EnsureTargetDirs(plan); err != nil {
		t.Fatalf("EnsureTargetDirs: %v", err)
	}

	rep, err := o.Move(context.Background(), taskID, plan, Hooks{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if rep.Success != 0 || rep.Failed != 1 {
		t.Fatalf("report = %+v, want failed=1", rep)
	}
	fails, err := store.ListFailedOperations(context.Background(), taskID, storage.ActionMove)
	if err != nil {
		t.Fatalf("ListFailedOperations: %v", err)
	}
	if len(fails) != 1 || !strings.Contains(fails[0].Error, "同名文件过多") {
		t.Fatalf("failures = %+v, want 同名文件过多", fails)
	}
	// 源文件仍在原处（未移动）。
	if got := readFile(t, filepath.Join(root, "a.jpg")); got != "NEW" {
		t.Errorf("source moved unexpectedly: %q", got)
	}
}

// ---- 整理：错误分支 ----

// TestEnsureTargetDirs_Failure：目标目录创建失败返回 ErrTargetDirCreate，
// 且不发生任何移动（「零移动」）。
func TestEnsureTargetDirs_Failure(t *testing.T) {
	store := openTestStore(t)
	taskID := newTaskRow(t, store)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.jpg"), "AAA")

	o := New(fakeFS{mkdir: func(string, fs.FileMode) error { return fs.ErrPermission }}, store)
	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", SizeBytes: 3, Category: "图片"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if err := o.EnsureTargetDirs(plan); !errors.Is(err, ErrTargetDirCreate) {
		t.Fatalf("EnsureTargetDirs err = %v, want ErrTargetDirCreate", err)
	}
	// 源文件未动、无操作记录。
	if _, err := os.Lstat(filepath.Join(root, "a.jpg")); err != nil {
		t.Errorf("source must remain: %v", err)
	}
	moves, err := store.ListSuccessfulMoves(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ListSuccessfulMoves: %v", err)
	}
	if len(moves) != 0 {
		t.Fatalf("moves = %d, want 0 (zero-move on precheck failure)", len(moves))
	}
}

// TestMove_SingleFailureContinues：单个文件失败不中断整体，失败原因与成功项
// 都被记录。
func TestMove_SingleFailureContinues(t *testing.T) {
	store := openTestStore(t)
	taskID := newTaskRow(t, store)
	root := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		writeFile(t, filepath.Join(root, n), n)
	}

	o := New(fakeFS{rename: func(oldpath, newpath string) error {
		if filepath.Base(oldpath) == "b.jpg" {
			return fs.ErrPermission
		}
		return os.Rename(oldpath, newpath)
	}}, store)

	cands := make([]storage.OrganizeCandidate, 0, 3)
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		cands = append(cands, storage.OrganizeCandidate{
			Path: filepath.Join(root, n), Name: n, SizeBytes: 3, Category: "图片",
		})
	}
	plan, err := BuildPlan(root, cands)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := o.EnsureTargetDirs(plan); err != nil {
		t.Fatalf("EnsureTargetDirs: %v", err)
	}

	rep, err := o.Move(context.Background(), taskID, plan, Hooks{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if rep.Success != 2 || rep.Failed != 1 {
		t.Fatalf("report = %+v, want success=2 failed=1", rep)
	}
	fails, err := store.ListFailedOperations(context.Background(), taskID, storage.ActionMove)
	if err != nil {
		t.Fatalf("ListFailedOperations: %v", err)
	}
	if len(fails) != 1 || fails[0].Error != "权限不足" {
		t.Fatalf("failures = %+v, want one 权限不足", fails)
	}
	if _, err := os.Lstat(filepath.Join(root, fsx.OrganizeDirName, "图片", "b.jpg")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("b.jpg should not be moved, stat err = %v", err)
	}
}

// TestMove_LstatPermissionReason：目标存在性检查遇到非 NotExist 错误时按失败
// 处理（不误判为「不存在」而冒险覆盖）。
func TestMove_LstatPermissionReason(t *testing.T) {
	store := openTestStore(t)
	taskID := newTaskRow(t, store)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.jpg"), "AAA")

	o := New(fakeFS{lstat: func(path string) (fs.FileInfo, error) {
		return nil, fs.ErrPermission
	}}, store)
	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", SizeBytes: 3, Category: "图片"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	rep, err := o.Move(context.Background(), taskID, plan, Hooks{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if rep.Failed != 1 {
		t.Fatalf("report = %+v, want failed=1", rep)
	}
	fails, _ := store.ListFailedOperations(context.Background(), taskID, storage.ActionMove)
	if len(fails) != 1 || fails[0].Error != "无法访问目标目录（权限不足）" {
		t.Fatalf("failures = %+v", fails)
	}
}

// TestMove_CancelKeepsCompleted：取消后已完成项保留、剩余项不再执行。
func TestMove_CancelKeepsCompleted(t *testing.T) {
	store := openTestStore(t)
	taskID := newTaskRow(t, store)
	root := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		writeFile(t, filepath.Join(root, n), n)
	}

	o := New(fsx.OSFileSystem{}, store)
	cands := make([]storage.OrganizeCandidate, 0, 3)
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		cands = append(cands, storage.OrganizeCandidate{
			Path: filepath.Join(root, n), Name: n, SizeBytes: 3, Category: "图片",
		})
	}
	plan, err := BuildPlan(root, cands)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := o.EnsureTargetDirs(plan); err != nil {
		t.Fatalf("EnsureTargetDirs: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := 0
	rep, err := o.Move(ctx, taskID, plan, Hooks{OnItem: func(ItemResult) {
		done++
		if done == 1 {
			cancel() // 首条完成后取消，后续应停止
		}
	}})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if !rep.Canceled {
		t.Fatalf("report = %+v, want Canceled", rep)
	}
	if rep.Success != 1 {
		t.Fatalf("success = %d, want 1 (completed work preserved)", rep.Success)
	}
}

// ---- 撤销 ----

// TestUndo_ReversesAndSkipsOccupied：成功回移已完成项；原路径被占用时跳过且
// 绝不覆盖占用者。
func TestUndo_ReversesAndSkipsOccupied(t *testing.T) {
	store := openTestStore(t)
	organizeTask := newTaskRow(t, store)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.jpg"), "AAA")
	writeFile(t, filepath.Join(root, "b.jpg"), "BBB")

	o := New(fsx.OSFileSystem{}, store)
	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", SizeBytes: 3, Category: "图片"},
		{Path: filepath.Join(root, "b.jpg"), Name: "b.jpg", SizeBytes: 3, Category: "图片"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := o.EnsureTargetDirs(plan); err != nil {
		t.Fatalf("EnsureTargetDirs: %v", err)
	}
	if _, err := o.Move(context.Background(), organizeTask, plan, Hooks{}); err != nil {
		t.Fatalf("Move: %v", err)
	}

	// 占用 b 的原路径。
	writeFile(t, filepath.Join(root, "b.jpg"), "OCCUPANT")

	moves, err := store.ListSuccessfulMoves(context.Background(), organizeTask)
	if err != nil {
		t.Fatalf("ListSuccessfulMoves: %v", err)
	}
	undoTask := newTaskRow(t, store)
	rep, err := o.Undo(context.Background(), undoTask, moves, Hooks{})
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if rep.Success != 1 || rep.Failed != 1 {
		t.Fatalf("report = %+v, want success=1 failed=1", rep)
	}

	// a 回移成功。
	if got := readFile(t, filepath.Join(root, "a.jpg")); got != "AAA" {
		t.Errorf("a.jpg content = %q, want AAA", got)
	}
	if _, err := os.Lstat(filepath.Join(root, fsx.OrganizeDirName, "图片", "a.jpg")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("organized a.jpg should be gone, err = %v", err)
	}
	// b 的占用者未被覆盖。
	if got := readFile(t, filepath.Join(root, "b.jpg")); got != "OCCUPANT" {
		t.Errorf("occupant overwritten: %q", got)
	}

	// undo 记录：src=整理后位置、dst=原位置（物理方向）。
	undoFails, err := store.ListFailedOperations(context.Background(), undoTask, storage.ActionUndo)
	if err != nil {
		t.Fatalf("ListFailedOperations(undo): %v", err)
	}
	if len(undoFails) != 1 || undoFails[0].Error != "原路径已被占用" {
		t.Fatalf("undo failures = %+v, want 原路径已被占用", undoFails)
	}
	if filepath.Base(undoFails[0].DstPath) != "b.jpg" {
		t.Fatalf("undo dst_path = %q, want original b.jpg", undoFails[0].DstPath)
	}
	sum, err := store.SummarizeOperations(context.Background(), undoTask, storage.ActionUndo)
	if err != nil {
		t.Fatalf("SummarizeOperations(undo): %v", err)
	}
	if sum.SuccessCount != 1 || sum.FailedCount != 1 {
		t.Fatalf("undo summary = %+v, want success=1 failed=1", sum)
	}
}

// TestUndo_MissingSourceFails：整理后位置已不存在时该条失败。
func TestUndo_MissingSourceFails(t *testing.T) {
	store := openTestStore(t)
	organizeTask := newTaskRow(t, store)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.jpg"), "AAA")

	o := New(fsx.OSFileSystem{}, store)
	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", SizeBytes: 3, Category: "图片"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := o.EnsureTargetDirs(plan); err != nil {
		t.Fatalf("EnsureTargetDirs: %v", err)
	}
	if _, err := o.Move(context.Background(), organizeTask, plan, Hooks{}); err != nil {
		t.Fatalf("Move: %v", err)
	}
	// 删除整理后文件。
	if err := os.Remove(filepath.Join(root, fsx.OrganizeDirName, "图片", "a.jpg")); err != nil {
		t.Fatalf("remove organized file: %v", err)
	}

	moves, _ := store.ListSuccessfulMoves(context.Background(), organizeTask)
	undoTask := newTaskRow(t, store)
	rep, err := o.Undo(context.Background(), undoTask, moves, Hooks{})
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if rep.Success != 0 || rep.Failed != 1 {
		t.Fatalf("report = %+v, want failed=1", rep)
	}
	fails, _ := store.ListFailedOperations(context.Background(), undoTask, storage.ActionUndo)
	if len(fails) != 1 || fails[0].Error != "目标不存在" {
		t.Fatalf("undo failures = %+v, want 目标不存在", fails)
	}
}

// ---- 计划校验（SEC-3） ----

// TestBuildPlan_Validation：非法类别 / 含路径分隔符的文件名 / 空路径 / 空根
// 都必须被拒绝。
func TestBuildPlan_Validation(t *testing.T) {
	root := t.TempDir()
	sep := string(filepath.Separator)

	tests := []struct {
		name string
		root string
		c    storage.OrganizeCandidate
	}{
		{name: "empty root", root: "  ", c: storage.OrganizeCandidate{Path: filepath.Join(root, "a"), Name: "a", Category: "图片"}},
		{name: "invalid category", root: root, c: storage.OrganizeCandidate{Path: filepath.Join(root, "a"), Name: "a", Category: "不存在"}},
		{name: "name with separator", root: root, c: storage.OrganizeCandidate{Path: filepath.Join(root, "a"), Name: "sub" + sep + "a.jpg", Category: "图片"}},
		{name: "dot name", root: root, c: storage.OrganizeCandidate{Path: filepath.Join(root, "a"), Name: "..", Category: "图片"}},
		{name: "empty path", root: root, c: storage.OrganizeCandidate{Path: "  ", Name: "a.jpg", Category: "图片"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildPlan(tt.root, []storage.OrganizeCandidate{tt.c}); err == nil {
				t.Fatalf("BuildPlan(%s) = nil error, want rejection", tt.name)
			}
		})
	}
}

// TestBuildPlan_TargetUnderRoot：合法计划的目标目录必须落在
// <root>/FilePilot整理/<类别> 之下。
func TestBuildPlan_TargetUnderRoot(t *testing.T) {
	root := t.TempDir()
	plan, err := BuildPlan(root, []storage.OrganizeCandidate{
		{Path: filepath.Join(root, "a.jpg"), Name: "a.jpg", Category: "图片"},
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	wantRoot := filepath.Join(root, fsx.OrganizeDirName)
	if plan.TargetRoot != wantRoot {
		t.Fatalf("TargetRoot = %q, want %q", plan.TargetRoot, wantRoot)
	}
	if plan.Items[0].DstDir != filepath.Join(wantRoot, "图片") {
		t.Fatalf("DstDir = %q", plan.Items[0].DstDir)
	}
}
