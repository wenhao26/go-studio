package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/fsx"
)

// fakeInfo 是 fs.FileInfo 的测试替身。
type fakeInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return f.size }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

// fakeFS 是 fsx.FileSystem 的测试替身。
//
// 错误分支（权限拒绝、读取中途失败）无法在真实文件系统上稳定构造，
// 这正是 internal/fsx 存在抽象的原因。
type fakeFS struct {
	entries map[string][]fs.DirEntry
	infos   map[string]fs.FileInfo
	readErr map[string]error
	statErr map[string]error

	// linkStat 模拟「跟随链接一次」后的结果，用于识别 Windows 目录联接。
	linkStat    map[string]fs.FileInfo
	linkStatErr map[string]error
}

func (f *fakeFS) ReadDir(dir string) ([]fs.DirEntry, error) {
	if err, ok := f.readErr[dir]; ok {
		// 模拟读取中途失败：仍返回已经读出的条目。
		return f.entries[dir], err
	}
	entries, ok := f.entries[dir]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: dir, Err: fs.ErrNotExist}
	}
	return entries, nil
}

func (f *fakeFS) Lstat(path string) (fs.FileInfo, error) {
	if err, ok := f.statErr[path]; ok {
		return nil, err
	}
	info, ok := f.infos[path]
	if !ok {
		return nil, &fs.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
	}
	return info, nil
}

func (f *fakeFS) Stat(path string) (fs.FileInfo, error) {
	if err, ok := f.linkStatErr[path]; ok {
		return nil, err
	}
	info, ok := f.linkStat[path]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
	}
	return info, nil
}

// 构造辅助：把 FileInfo 直接转换为 DirEntry，模拟 readdir 已填充类型信息的情况。
func dirEntry(name string) fs.DirEntry {
	return fs.FileInfoToDirEntry(fakeInfo{name: name, mode: fs.ModeDir | 0o755})
}

func fileEntry(name string, size int64) fs.DirEntry {
	return fs.FileInfoToDirEntry(fakeInfo{name: name, size: size, mode: 0o644})
}

func symlinkEntry(name string) fs.DirEntry {
	return fs.FileInfoToDirEntry(fakeInfo{name: name, mode: fs.ModeSymlink | 0o777})
}

func specialEntry(name string) fs.DirEntry {
	return fs.FileInfoToDirEntry(fakeInfo{name: name, mode: fs.ModeNamedPipe | 0o644})
}

// irregularEntry 模拟 Windows 的目录联接（junction）。
//
// 实测：junction 在 Go 中既不是 ModeSymlink 也不是目录，而是 ModeIrregular
// （Type() 形如 "?---------"，IsDir() 为 false），且 lstat 大小为 0。
func irregularEntry(name string) fs.DirEntry {
	return fs.FileInfoToDirEntry(fakeInfo{name: name, mode: fs.ModeIrregular})
}

// irregularFileEntry 模拟「不可识别但不是目录」的条目，例如云盘按需文件：
// 元数据里记录了真实大小，只是类型不是普通文件。
func irregularFileEntry(name string, size int64) fs.DirEntry {
	return fs.FileInfoToDirEntry(fakeInfo{name: name, size: size, mode: fs.ModeIrregular})
}

func dirInfo(name string) fs.FileInfo {
	return fakeInfo{name: name, mode: fs.ModeDir | 0o755}
}

func TestScan_NestedTree(t *testing.T) {
	root := "rootdir"
	dirA := filepath.Join(root, "a")
	dirB := filepath.Join(root, "b")
	dirDeep := filepath.Join(dirB, "deep")

	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{
			root:    {dirEntry("a"), dirEntry("b"), fileEntry("x.txt", 100), symlinkEntry("link")},
			dirA:    {fileEntry("a1", 10), fileEntry("a2", 20)},
			dirB:    {dirEntry("deep")},
			dirDeep: {fileEntry("d1", 5)},
		},
		infos: map[string]fs.FileInfo{root: dirInfo(root)},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}

	if snap.DirectoriesVisited != 4 {
		t.Errorf("DirectoriesVisited = %d, want 4", snap.DirectoriesVisited)
	}
	if snap.FilesFound != 4 {
		t.Errorf("FilesFound = %d, want 4（符号链接不计入）", snap.FilesFound)
	}
	if snap.TotalSizeBytes != 135 {
		t.Errorf("TotalSizeBytes = %d, want 135", snap.TotalSizeBytes)
	}
	if snap.SkippedErrors != 0 {
		t.Errorf("SkippedErrors = %d, want 0", snap.SkippedErrors)
	}
	if snap.Root != root {
		t.Errorf("Root = %q, want %q", snap.Root, root)
	}
}

func TestScan_RootWithTrailingSeparatorIsCleaned(t *testing.T) {
	root := "rootdir"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: {fileEntry("a", 1)}},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root + string(filepath.Separator)})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.Root != root {
		t.Errorf("Root = %q, want %q（应被 Clean 规范化）", snap.Root, root)
	}
}

func TestScan_EmptyDirectory(t *testing.T) {
	root := "empty"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: {}},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.DirectoriesVisited != 1 || snap.FilesFound != 0 || snap.TotalSizeBytes != 0 {
		t.Errorf("空目录统计异常: %+v", snap)
	}
	if snap.SkippedErrors != 0 {
		t.Errorf("空目录不应产生错误: %d", snap.SkippedErrors)
	}
}

func TestScan_SkipsSymlinks(t *testing.T) {
	root := "symroot"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{
			root: {symlinkEntry("to-dir"), symlinkEntry("to-file"), fileEntry("real", 7)},
		},
		infos: map[string]fs.FileInfo{root: dirInfo(root)},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.FilesFound != 1 {
		t.Errorf("FilesFound = %d, want 1（符号链接不计入）", snap.FilesFound)
	}
	if snap.TotalSizeBytes != 7 {
		t.Errorf("TotalSizeBytes = %d, want 7", snap.TotalSizeBytes)
	}
	if snap.DirectoriesVisited != 1 {
		t.Errorf("DirectoriesVisited = %d, want 1（不跟随符号链接目录）", snap.DirectoriesVisited)
	}
}

// TestScan_SkipsDirectoryLinks 覆盖 Windows 目录联接（junction）的识别。
//
// 缺少该判断时，junction 会被当成 0 字节文件计入文件数，让 Windows 上的
// 文件统计系统性偏大——这是实测发现并修复的缺陷。
func TestScan_SkipsDirectoryLinks(t *testing.T) {
	root := "linkroot"
	linkPath := filepath.Join(root, "loop")

	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{
			root: {irregularEntry("loop"), fileEntry("real.txt", 7)},
		},
		infos: map[string]fs.FileInfo{root: dirInfo(root)},
		// 跟随一次后发现指向目录 -> 判定为目录联接，按符号链接策略跳过。
		linkStat: map[string]fs.FileInfo{linkPath: dirInfo("loop")},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.FilesFound != 1 {
		t.Errorf("FilesFound = %d, want 1（目录联接不应计为文件）", snap.FilesFound)
	}
	if snap.TotalSizeBytes != 7 {
		t.Errorf("TotalSizeBytes = %d, want 7", snap.TotalSizeBytes)
	}
	if snap.DirectoriesVisited != 1 {
		t.Errorf("DirectoriesVisited = %d, want 1（不跟随目录联接）", snap.DirectoriesVisited)
	}
	if snap.SkippedErrors != 0 {
		t.Errorf("SkippedErrors = %d, want 0（跳过链接不算错误）", snap.SkippedErrors)
	}
}

// TestScan_CountsIrregularEntriesWhenNotDirectory 覆盖「不可识别条目实际不是目录」：
// 例如 Windows 上云盘的按需文件，仍应计入文件数。
func TestScan_CountsIrregularEntriesWhenNotDirectory(t *testing.T) {
	root := "irregularroot"
	entryPath := filepath.Join(root, "placeholder")

	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{
			root: {irregularFileEntry("placeholder", 42)},
		},
		infos:    map[string]fs.FileInfo{root: dirInfo(root)},
		linkStat: map[string]fs.FileInfo{entryPath: fakeInfo{name: "placeholder", size: 42, mode: 0o644}},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.FilesFound != 1 {
		t.Errorf("FilesFound = %d, want 1", snap.FilesFound)
	}
	if snap.TotalSizeBytes != 42 {
		t.Errorf("TotalSizeBytes = %d, want 42", snap.TotalSizeBytes)
	}
}

// TestScan_CountsIrregularEntriesWhenStatFails 覆盖无法判定时的取舍：
// 宁可多算也不少算，同时不因 stat 失败而中断。
func TestScan_CountsIrregularEntriesWhenStatFails(t *testing.T) {
	root := "irregularfailroot"
	entryPath := filepath.Join(root, "mystery")

	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{
			root: {irregularEntry("mystery")},
		},
		infos:       map[string]fs.FileInfo{root: dirInfo(root)},
		linkStatErr: map[string]error{entryPath: &fs.PathError{Op: "stat", Path: entryPath, Err: fs.ErrPermission}},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.FilesFound != 1 {
		t.Errorf("FilesFound = %d, want 1（无法判定时按普通条目计入）", snap.FilesFound)
	}
	if snap.SkippedErrors != 0 {
		t.Errorf("SkippedErrors = %d, want 0（Stat 失败不是扫描错误）", snap.SkippedErrors)
	}
}

func TestScan_CountsSpecialFiles(t *testing.T) {
	root := "specialroot"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: {specialEntry("fifo"), fileEntry("regular", 3)}},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.FilesFound != 2 {
		t.Errorf("FilesFound = %d, want 2（特殊文件计入文件数）", snap.FilesFound)
	}
	if snap.TotalSizeBytes != 3 {
		t.Errorf("TotalSizeBytes = %d, want 3", snap.TotalSizeBytes)
	}
}

// TestScan_SkippedDirectoryDoesNotAbortScan 验证 PRD §四.2：
// 权限不足的目录不能让程序崩溃或中断。
func TestScan_SkippedDirectoryDoesNotAbortScan(t *testing.T) {
	root := "permroot"
	denied := filepath.Join(root, "denied")
	allowed := filepath.Join(root, "allowed")

	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{
			root:    {dirEntry("denied"), dirEntry("allowed")},
			allowed: {fileEntry("ok", 11)},
		},
		infos: map[string]fs.FileInfo{root: dirInfo(root)},
		readErr: map[string]error{
			denied: &fs.PathError{Op: "open", Path: denied, Err: fs.ErrPermission},
		},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("权限错误不应中断扫描，got %v", err)
	}
	if snap.SkippedErrors != 1 {
		t.Errorf("SkippedErrors = %d, want 1", snap.SkippedErrors)
	}
	if snap.DirectoriesVisited != 2 {
		t.Errorf("DirectoriesVisited = %d, want 2（被拒目录不计入）", snap.DirectoriesVisited)
	}
	if snap.FilesFound != 1 {
		t.Errorf("FilesFound = %d, want 1", snap.FilesFound)
	}
	if len(snap.ErrorDetails) != 1 {
		t.Fatalf("ErrorDetails 条数 = %d, want 1", len(snap.ErrorDetails))
	}
	if !errors.Is(snap.ErrorDetails[0], fs.ErrPermission) {
		t.Error("错误明细应可通过 errors.Is 判定为 fs.ErrPermission")
	}
}

// TestScan_PartialReadKeepsEntries 固化文档化行为：
// 读取中途失败时，已经读出的条目仍参与统计，但该目录不计入目录数。
func TestScan_PartialReadKeepsEntries(t *testing.T) {
	root := "partialroot"
	sub := filepath.Join(root, "sub")

	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{
			root: {dirEntry("sub"), fileEntry("top", 2)},
			sub:  {fileEntry("partial", 9)},
		},
		infos: map[string]fs.FileInfo{root: dirInfo(root)},
		readErr: map[string]error{
			sub: &fs.PathError{Op: "readdirent", Path: sub, Err: errors.New("io error")},
		},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.FilesFound != 2 {
		t.Errorf("FilesFound = %d, want 2（部分读取的条目已计入）", snap.FilesFound)
	}
	if snap.TotalSizeBytes != 11 {
		t.Errorf("TotalSizeBytes = %d, want 11", snap.TotalSizeBytes)
	}
	if snap.SkippedErrors != 1 {
		t.Errorf("SkippedErrors = %d, want 1", snap.SkippedErrors)
	}
	if snap.DirectoriesVisited != 1 {
		t.Errorf("DirectoriesVisited = %d, want 1（读取失败的目录不计入）", snap.DirectoriesVisited)
	}
}

func TestScan_RootNotFound(t *testing.T) {
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{},
		infos:   map[string]fs.FileInfo{},
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: "missing"})
	if err == nil {
		t.Fatal("根路径不存在时应返回错误")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("错误应可判定为 fs.ErrNotExist，got %v", err)
	}
	if snap.DirectoriesVisited != 0 || snap.FilesFound != 0 {
		t.Error("致命错误时不应产生统计结果")
	}
}

func TestScan_RootIsFile(t *testing.T) {
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{},
		infos:   map[string]fs.FileInfo{"afile": fakeInfo{name: "afile", size: 1, mode: 0o644}},
	}

	_, err := New(fsys).Scan(context.Background(), Options{Root: "afile"})
	if !errors.Is(err, ErrNotDirectory) {
		t.Errorf("错误应可判定为 ErrNotDirectory，got %v", err)
	}
}

func TestScan_ErrorDetailsAreBounded(t *testing.T) {
	root := "manyerrors"
	const deniedCount = 30

	children := make([]fs.DirEntry, 0, deniedCount)
	readErr := make(map[string]error, deniedCount)
	for i := 0; i < deniedCount; i++ {
		name := fmt.Sprintf("d%02d", i)
		children = append(children, dirEntry(name))
		path := filepath.Join(root, name)
		readErr[path] = &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
	}

	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: children},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
		readErr: readErr,
	}

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if snap.SkippedErrors != deniedCount {
		t.Errorf("SkippedErrors = %d, want %d", snap.SkippedErrors, deniedCount)
	}
	if len(snap.ErrorDetails) != maxErrorDetails {
		t.Errorf("ErrorDetails 条数 = %d, want %d（应被限流）", len(snap.ErrorDetails), maxErrorDetails)
	}
}

func TestScan_NilGuards(t *testing.T) {
	t.Run("nil scanner", func(t *testing.T) {
		var s *Scanner
		if _, err := s.Scan(context.Background(), Options{Root: "."}); err == nil {
			t.Error("nil scanner 应返回错误而不是 panic")
		}
	})

	t.Run("uninitialized filesystem", func(t *testing.T) {
		s := New(nil)
		if _, err := s.Scan(context.Background(), Options{Root: "."}); err == nil {
			t.Error("未注入文件系统时应返回错误")
		}
	})

	t.Run("nil context", func(t *testing.T) {
		s := New(&fakeFS{})
		if _, err := s.Scan(nil, Options{Root: "."}); err == nil { //nolint:staticcheck // 显式验证 nil ctx 的防御分支
			t.Error("nil ctx 应返回错误")
		}
	})

	t.Run("nil receiver progress", func(t *testing.T) {
		var s *Scanner
		if got := s.Progress(); got != (ProgressInfo{}) {
			t.Errorf("nil scanner 的 Progress 应返回零值，got %+v", got)
		}
	})
}

func TestScan_ContextCanceled(t *testing.T) {
	root := "cancelroot"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: {fileEntry("a", 1)}},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	snap, err := New(fsys).Scan(ctx, Options{Root: root})
	if err == nil {
		t.Fatal("已取消的 ctx 应导致错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("错误应可判定为 context.Canceled，got %v", err)
	}
	if snap.DirectoriesVisited != 0 {
		t.Errorf("取消后不应统计目录，got %d", snap.DirectoriesVisited)
	}
}

func TestScan_ReusableScanner(t *testing.T) {
	root := "reuse"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: {fileEntry("a", 4)}},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	s := New(fsys)
	for i := 0; i < 2; i++ {
		snap, err := s.Scan(context.Background(), Options{Root: root})
		if err != nil {
			t.Fatalf("第 %d 次 Scan 返回错误: %v", i, err)
		}
		// 计数器必须在每次 Scan 开始时重置，否则第二次会翻倍。
		if snap.FilesFound != 1 || snap.TotalSizeBytes != 4 {
			t.Fatalf("第 %d 次统计异常: %+v", i, snap)
		}
	}
}

func TestScan_WorkersOptionIsRespected(t *testing.T) {
	root := "workers"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: {fileEntry("a", 1), fileEntry("b", 2)}},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	for _, workers := range []int{0, 1, 4, -1, MaxWorkers + 1000} {
		snap, err := New(fsys).Scan(context.Background(), Options{Root: root, Workers: workers})
		if err != nil {
			t.Fatalf("Workers=%d 时返回错误: %v", workers, err)
		}
		if snap.FilesFound != 2 || snap.TotalSizeBytes != 3 {
			t.Errorf("Workers=%d 统计异常: %+v", workers, snap)
		}
	}
}

// TestScan_ConcurrentWorkersProduceExactCounts 用宽树反复触发并发路径。
//
// 该用例在 -race 下运行，是「无数据竞争」与「无死锁」的主要证据：
// 若 workQueue 的等待条件写错，这里会挂起或计数错误。
func TestScan_ConcurrentWorkersProduceExactCounts(t *testing.T) {
	const dirsPerLevel = 40
	const filesPerDir = 25
	const fileSize = 10

	root := "wide"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	top := make([]fs.DirEntry, 0, dirsPerLevel)
	for i := 0; i < dirsPerLevel; i++ {
		name := fmt.Sprintf("d%02d", i)
		child := filepath.Join(root, name)
		top = append(top, dirEntry(name))
		fsys.infos[child] = dirInfo(name)

		entries := make([]fs.DirEntry, 0, filesPerDir)
		for j := 0; j < filesPerDir; j++ {
			entries = append(entries, fileEntry(fmt.Sprintf("f%02d", j), fileSize))
		}
		fsys.entries[child] = entries
	}
	fsys.entries[root] = top

	snap, err := New(fsys).Scan(context.Background(), Options{Root: root, Workers: 8})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}

	wantDirs := int64(1 + dirsPerLevel)
	wantFiles := int64(dirsPerLevel * filesPerDir)
	if snap.DirectoriesVisited != wantDirs {
		t.Errorf("DirectoriesVisited = %d, want %d", snap.DirectoriesVisited, wantDirs)
	}
	if snap.FilesFound != wantFiles {
		t.Errorf("FilesFound = %d, want %d", snap.FilesFound, wantFiles)
	}
	if snap.TotalSizeBytes != wantFiles*fileSize {
		t.Errorf("TotalSizeBytes = %d, want %d", snap.TotalSizeBytes, wantFiles*fileSize)
	}
}

// TestScan_RealFilesystem 是对真实文件系统的集成测试，
// 用于验证抽象实现与遍历逻辑在真实 Syscall 行为下的一致性。
func TestScan_RealFilesystem(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("world!"), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	snap, err := New(fsx.OSFileSystem{}).Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}

	if snap.DirectoriesVisited != 2 {
		t.Errorf("DirectoriesVisited = %d, want 2", snap.DirectoriesVisited)
	}
	if snap.FilesFound != 2 {
		t.Errorf("FilesFound = %d, want 2", snap.FilesFound)
	}
	if snap.TotalSizeBytes != 11 {
		t.Errorf("TotalSizeBytes = %d, want 11", snap.TotalSizeBytes)
	}
	if snap.SkippedErrors != 0 {
		t.Errorf("SkippedErrors = %d, want 0", snap.SkippedErrors)
	}
	if snap.Duration < 0 {
		t.Errorf("Duration = %v, 不应为负", snap.Duration)
	}
}

func TestProgress_ZeroBeforeScan(t *testing.T) {
	s := New(&fakeFS{})
	if got := s.Progress(); got != (ProgressInfo{}) {
		t.Errorf("未扫描时 Progress 应为零值，got %+v", got)
	}
}

func TestProgress_ReflectsLastScan(t *testing.T) {
	root := "progressroot"
	fsys := &fakeFS{
		entries: map[string][]fs.DirEntry{root: {fileEntry("a", 8)}},
		infos:   map[string]fs.FileInfo{root: dirInfo(root)},
	}

	s := New(fsys)
	if _, err := s.Scan(context.Background(), Options{Root: root}); err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}

	got := s.Progress()
	if got.DirectoriesVisited != 1 || got.FilesFound != 1 || got.TotalSizeBytes != 8 {
		t.Errorf("Progress 未反映最近一次扫描结果: %+v", got)
	}
}

func TestDefaultWorkers(t *testing.T) {
	got := DefaultWorkers()
	if got < 1 {
		t.Errorf("DefaultWorkers() = %d, 必须 >= 1", got)
	}
	if got > defaultWorkerCap {
		t.Errorf("DefaultWorkers() = %d, 不应超过上限 %d", got, defaultWorkerCap)
	}
}
