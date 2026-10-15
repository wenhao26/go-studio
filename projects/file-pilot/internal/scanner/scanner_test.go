package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/classifier"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// --- 测试替身 ---------------------------------------------------------------

// fakeFS 是 fsx.FileSystem 的内存实现：目录树可编程构造，
// 权限拒绝/读取失败/元数据缺失这些真实 FS 上不稳定的分支可确定性注入。
type fakeFS struct {
	mu       sync.Mutex
	dirs     map[string][]fs.DirEntry
	readErrs map[string]error
	onRead   func(dir string)
}

func newFakeFS() *fakeFS {
	return &fakeFS{
		dirs:     map[string][]fs.DirEntry{},
		readErrs: map[string]error{},
	}
}

func (f *fakeFS) addDir(path string, entries ...fs.DirEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dirs[path] = entries
}

func (f *fakeFS) failReadDir(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readErrs[path] = err
}

func (f *fakeFS) ReadDir(dir string) ([]fs.DirEntry, error) {
	f.mu.Lock()
	hook := f.onRead
	err := f.readErrs[dir]
	entries := f.dirs[dir]
	_, known := f.dirs[dir]
	f.mu.Unlock()

	if hook != nil {
		hook(dir)
	}
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, &fs.PathError{Op: "open", Path: dir, Err: fs.ErrNotExist}
	}
	return entries, nil
}

// Lstat 只对已注册目录成功：未注册路径返回 not-exist，
// 使「根不存在」这类场景可确定性构造。
func (f *fakeFS) Lstat(path string) (fs.FileInfo, error) {
	f.mu.Lock()
	_, isDir := f.dirs[path]
	f.mu.Unlock()

	if isDir {
		return fakeInfo{name: filepath.Base(path), mode: fs.ModeDir | 0o755}, nil
	}
	return nil, &fs.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
}

func (f *fakeFS) Stat(path string) (fs.FileInfo, error) {
	return f.Lstat(path)
}

// fakeDirEntry 是可编程的目录条目。
type fakeDirEntry struct {
	name    string
	typ     fs.FileMode
	fi      fs.FileInfo
	infoErr error
}

func (e fakeDirEntry) Name() string      { return e.name }
func (e fakeDirEntry) IsDir() bool       { return e.typ.IsDir() }
func (e fakeDirEntry) Type() fs.FileMode { return e.typ }
func (e fakeDirEntry) Info() (fs.FileInfo, error) {
	if e.infoErr != nil {
		return nil, e.infoErr
	}
	if e.fi == nil {
		return nil, fs.ErrNotExist
	}
	return e.fi, nil
}

// fakeInfo 是可编程的文件元数据；Sys() 返回 nil，
// 因此 fsx.AttributesOf 走「读不到即零值」路径（不猜测）。
type fakeInfo struct {
	name string
	size int64
	mode fs.FileMode
	mod  time.Time
}

func (i fakeInfo) Name() string       { return i.name }
func (i fakeInfo) Size() int64        { return i.size }
func (i fakeInfo) Mode() fs.FileMode  { return i.mode }
func (i fakeInfo) ModTime() time.Time { return i.mod }
func (i fakeInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeInfo) Sys() any           { return nil }

func fileEntry(name string, size int64) fakeDirEntry {
	return fakeDirEntry{
		name: name,
		fi: fakeInfo{name: name, size: size, mode: 0o644,
			mod: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
	}
}

func dirEntry(name string) fakeDirEntry {
	return fakeDirEntry{name: name, typ: fs.ModeDir, fi: fakeInfo{name: name, mode: fs.ModeDir | 0o755}}
}

func linkEntry(name string) fakeDirEntry {
	return fakeDirEntry{name: name, typ: fs.ModeSymlink}
}

func statFailEntry(name string) fakeDirEntry {
	return fakeDirEntry{name: name, infoErr: &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrPermission}}
}

// memSink 是 FileSink 的内存实现：按批记录落库内容，可注入失败。
type memSink struct {
	mu       sync.Mutex
	batches  [][]storage.FileRecord
	failWith error
}

func (s *memSink) InsertFiles(ctx context.Context, scanID int64, rows []storage.FileRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failWith != nil {
		return s.failWith
	}
	s.batches = append(s.batches, append([]storage.FileRecord(nil), rows...))
	return nil
}

func (s *memSink) all() []storage.FileRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []storage.FileRecord
	for _, b := range s.batches {
		out = append(out, b...)
	}
	return out
}

func (s *memSink) batchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.batches)
}

// memLogSink 是 LogSink 的内存实现；hook 用于在首条日志时注入外部动作（如取消）。
type memLogSink struct {
	mu      sync.Mutex
	entries []LogEntry
	hook    func()
}

func (s *memLogSink) Log(kind LogKind, text string) {
	s.mu.Lock()
	s.entries = append(s.entries, LogEntry{Kind: kind, Text: text})
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (s *memLogSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// runScan 辅助：以默认配置执行一次扫描。
func runScanOn(t *testing.T, fsys *fakeFS, sink FileSink, logs LogSink, cfg Config) (Result, error) {
	t.Helper()
	s := New(fsys, classifier.New())
	return s.Scan(context.Background(), Options{
		Root: rootPath, ScanID: 1, Config: cfg, Files: sink, Logs: logs,
	})
}

// rootPath 是所有测试树的根（仅合成路径，不触碰真实文件系统）。
var rootPath = filepath.Join("C:", "root")

// --- 正常路径 ---------------------------------------------------------------

func TestScan_BasicTree(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath,
		fileEntry("a.jpg", 2),
		dirEntry("sub"),
		fileEntry("b.pdf", 5),
	)
	fsys.addDir(filepath.Join(rootPath, "sub"),
		fileEntry("c.txt", 3),
		fileEntry("d.png", 4),
	)

	sink := &memSink{}
	logs := &memLogSink{}
	res, err := runScanOn(t, fsys, sink, logs, Config{Workers: 2})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if res.FileCount != 4 || res.DirCount != 2 || res.TotalSizeBytes != 14 || res.Skipped != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Root != rootPath || res.Workers != 2 {
		t.Fatalf("unexpected identity fields: %+v", res)
	}

	rows := sink.all()
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	// 分类走 Phase 2 规则引擎；此处文件均为扩展名命中（无关键词/路径规则），
	// 故类别与置信度与扩展名映射一致。
	wantCategory := map[string]string{"a.jpg": "图片", "d.png": "图片", "b.pdf": "文档", "c.txt": "文档"}
	for _, row := range rows {
		if want := wantCategory[row.Name]; row.Category != want {
			t.Errorf("row %s category = %q, want %q", row.Name, row.Category, want)
		}
		if row.Source != "rule" || row.Confidence != 60 {
			t.Errorf("row %s classification = %s/%d, want rule/60", row.Name, row.Source, row.Confidence)
		}
	}
	if logs.count() == 0 {
		t.Error("scan logs must be emitted")
	}
}

func TestScan_ProgressSnapshotAfterRun(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, fileEntry("a.jpg", 2), dirEntry("sub"))
	fsys.addDir(filepath.Join(rootPath, "sub"), fileEntry("b.txt", 3))

	s := New(fsys, classifier.New())
	if _, err := s.Scan(context.Background(), Options{
		Root: rootPath, ScanID: 1, Config: Config{Workers: 1}, Files: &memSink{},
	}); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// 未运行时 Progress 为零值；运行后保留最近一次快照。
	p := s.Progress()
	if p.Files != 2 || p.Dirs != 2 || p.SizeBytes != 5 {
		t.Fatalf("unexpected progress: %+v", p)
	}

	var fresh Scanner
	if got := fresh.Progress(); got != (Progress{}) {
		t.Fatalf("zero scanner progress = %+v, want zero", got)
	}
}

// --- 跳过口径 ---------------------------------------------------------------

func TestScan_SkipsHiddenBlacklistAndOwnOutputDir(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath,
		fileEntry(".secret", 1),
		dirEntry(".git"),
		dirEntry("System Volume Information"),
		dirEntry("FilePilot整理"),
		fileEntry("ok.txt", 7),
	)
	fsys.addDir(filepath.Join(rootPath, ".git"), fileEntry("inner.txt", 1))
	fsys.addDir(filepath.Join(rootPath, "FilePilot整理"), fileEntry("上次结果.jpg", 1))

	sink := &memSink{}
	res, err := runScanOn(t, fsys, sink, nil, Config{Workers: 1})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// FilePilot整理 是「自有输出」排除：既不计入文件/目录，也不计入跳过（FR-5 口径）。
	if res.FileCount != 1 || res.DirCount != 1 {
		t.Fatalf("unexpected counts: %+v", res)
	}
	// 跳过 = 隐藏文件 + 隐藏目录 + 黑名单目录 = 3（不含输出目录）。
	if res.Skipped != 3 {
		t.Fatalf("skipped = %d, want 3", res.Skipped)
	}
	if len(res.Details) != 3 {
		t.Fatalf("details = %+v, want 3 entries", res.Details)
	}
	if len(sink.all()) != 1 || sink.all()[0].Name != "ok.txt" {
		t.Fatalf("unexpected rows: %+v", sink.all())
	}
}

// TestScan_HiddenIncludedWhenEnabled：开关打开时隐藏文件与隐藏目录均纳入（裁决 D-A6）。
func TestScan_HiddenIncludedWhenEnabled(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, fileEntry(".secret", 1), dirEntry(".git"))
	fsys.addDir(filepath.Join(rootPath, ".git"), fileEntry("config", 2))

	sink := &memSink{}
	res, err := runScanOn(t, fsys, sink, nil, Config{Workers: 1, ScanHidden: true})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.FileCount != 2 || res.DirCount != 2 || res.Skipped != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestScan_SymlinkSkippedWithoutDescent(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, linkEntry("alias"), fileEntry("a.txt", 1))
	// 若 symlink 被错误下潜，这里会因目录不存在而产生额外跳过。
	fsys.addDir(filepath.Join(rootPath, "alias"), fileEntry("inner.txt", 1))

	sink := &memSink{}
	res, err := runScanOn(t, fsys, sink, nil, Config{Workers: 1})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.FileCount != 1 || res.Skipped != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Details[0].Reason != skipReasonLink {
		t.Fatalf("skip reason = %q, want %q", res.Details[0].Reason, skipReasonLink)
	}
}

func TestScan_UnreadableDir_CountedAsSkipNotDir(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, dirEntry("locked"), fileEntry("a.txt", 1))
	fsys.failReadDir(filepath.Join(rootPath, "locked"),
		&fs.PathError{Op: "open", Path: "locked", Err: fs.ErrPermission})

	res, err := runScanOn(t, fsys, &memSink{}, nil, Config{Workers: 1})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// P2-3：仅统计成功完整读取的目录；不可读目录必须计为跳过。
	if res.DirCount != 1 || res.Skipped != 1 || res.FileCount != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Details[0].Reason != skipReasonUnreadable {
		t.Fatalf("skip reason = %q", res.Details[0].Reason)
	}
}

func TestScan_StatFailure_Skipped(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, statFailEntry("ghost"), fileEntry("a.txt", 1))

	res, err := runScanOn(t, fsys, &memSink{}, nil, Config{Workers: 1})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.FileCount != 1 || res.Skipped != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Details[0].Reason != skipReasonStat {
		t.Fatalf("skip reason = %q", res.Details[0].Reason)
	}
}

func TestScan_LargeFileSkippedBySwitch(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, fileEntry("huge.iso", 1<<30+1), fileEntry("small.txt", 1))

	res, err := runScanOn(t, fsys, &memSink{}, nil, Config{Workers: 1, SkipLarge: true})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.FileCount != 1 || res.Skipped != 1 || res.TotalSizeBytes != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Details[0].Reason != skipReasonLargeFile {
		t.Fatalf("skip reason = %q", res.Details[0].Reason)
	}
}

// TestScan_SkipDetailsCapped：明细最多 20 条，但 skipped 计数必须完整（FR-5）。
func TestScan_SkipDetailsCapped(t *testing.T) {
	fsys := newFakeFS()
	entries := make([]fs.DirEntry, 0, 30)
	for i := 0; i < 30; i++ {
		entries = append(entries, fileEntry(fmt.Sprintf(".hidden%02d", i), 1))
	}
	fsys.addDir(rootPath, entries...)

	res, err := runScanOn(t, fsys, &memSink{}, nil, Config{Workers: 1})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Skipped != 30 {
		t.Fatalf("skipped = %d, want 30", res.Skipped)
	}
	if len(res.Details) != maxSkipDetails {
		t.Fatalf("details = %d, want %d", len(res.Details), maxSkipDetails)
	}
}

// --- 取消与错误路径 ---------------------------------------------------------

// TestScan_Cancel_PartialResultsPersisted：取消后尽快退出，尾批仍落库
// （FR-3/FR-5「已取消 · 部分结果」）。
func TestScan_Cancel_PartialResultsPersisted(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, fileEntry("a.jpg", 2), fileEntry("b.jpg", 3), dirEntry("sub"))
	fsys.addDir(filepath.Join(rootPath, "sub"), fileEntry("c.jpg", 4))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 首条日志（即首个文件入缓冲后）触发取消：单 worker 下后续条目不再处理。
	logs := &memLogSink{}
	logs.hook = cancel

	s := New(fsys, classifier.New())
	sink := &memSink{}
	res, err := s.Scan(ctx, Options{
		Root: rootPath, ScanID: 1, Config: Config{Workers: 1}, Files: sink, Logs: logs,
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if res.FileCount != 1 || len(sink.all()) != 1 {
		t.Fatalf("partial results must persist: %+v rows=%d", res, len(sink.all()))
	}
}

func TestScan_StoreError_AbortsScan(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath, fileEntry("a.jpg", 2), dirEntry("sub"))
	fsys.addDir(filepath.Join(rootPath, "sub"), fileEntry("b.jpg", 3))

	sink := &memSink{failWith: errors.New("db exploded")}
	res, err := runScanOn(t, fsys, sink, nil, Config{Workers: 1})
	if err == nil {
		t.Fatal("want error when sink fails")
	}
	if !errors.Is(err, sink.failWith) {
		t.Fatalf("error must wrap sink failure, got %v", err)
	}
	// 落库失败优先于取消：统计仍返回，任务由上层置 failed。
	if res.FileCount < 1 {
		t.Fatalf("result should still carry partial counts: %+v", res)
	}
}

func TestScan_Validation(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath)

	tests := []struct {
		name    string
		opts    Options
		wantErr error
	}{
		{name: "nil files sink", opts: Options{Root: rootPath, Config: Config{Workers: 1}}},
		{name: "invalid workers", opts: Options{Root: rootPath, Config: Config{Workers: 0}, Files: &memSink{}}, wantErr: ErrInvalidWorkers},
		{name: "root missing", opts: Options{Root: filepath.Join(rootPath, "nope"), Config: Config{Workers: 1}, Files: &memSink{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(fsys, classifier.New())
			_, err := s.Scan(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestScan_RootIsFile(t *testing.T) {
	// Lstat 返回普通文件：必须报 ErrNotDirectory 而不是空扫成功。
	s := New(fileOnlyFS{}, classifier.New())
	_, err := s.Scan(context.Background(), Options{
		Root: rootPath, Config: Config{Workers: 1}, Files: &memSink{},
	})
	if !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("err = %v, want ErrNotDirectory", err)
	}
}

// fileOnlyFS 的根是普通文件（Lstat 非目录）。
type fileOnlyFS struct{}

func (fileOnlyFS) ReadDir(dir string) ([]fs.DirEntry, error) {
	return nil, errors.New("not a directory")
}

func (fileOnlyFS) Lstat(path string) (fs.FileInfo, error) {
	return fakeInfo{name: filepath.Base(path), size: 1}, nil
}

func (fileOnlyFS) Stat(path string) (fs.FileInfo, error) {
	return fakeInfo{name: filepath.Base(path), size: 1}, nil
}

func TestScan_NotInitialized(t *testing.T) {
	var s Scanner
	if _, err := s.Scan(context.Background(), Options{Root: rootPath, Files: &memSink{}}); err == nil {
		t.Fatal("want error for uninitialized scanner")
	}
	if _, err := New(fsysStub{}, nil).Scan(context.Background(), Options{Root: rootPath, Files: &memSink{}}); err == nil {
		t.Fatal("want error for nil classifier")
	}
}

// fsysStub 只用于构造未初始化场景，方法不应被调用。
type fsysStub struct{}

func (fsysStub) ReadDir(string) ([]fs.DirEntry, error) { panic("not used") }
func (fsysStub) Lstat(string) (fs.FileInfo, error)     { panic("not used") }
func (fsysStub) Stat(string) (fs.FileInfo, error)      { panic("not used") }

func TestScan_NilContext(t *testing.T) {
	fsys := newFakeFS()
	fsys.addDir(rootPath)
	s := New(fsys, classifier.New())
	if _, err := s.Scan(nil, Options{Root: rootPath, Files: &memSink{}}); err == nil {
		t.Fatal("want error for nil context")
	}
}

// --- 队列 -------------------------------------------------------------------

func TestWorkQueue_LIFOAndCompletion(t *testing.T) {
	q := newWorkQueue()
	q.push("a", "b", "c")

	// LIFO：最后压入的先出队；每次弹出后按契约调用 done 归还在途计数。
	for _, want := range []string{"c", "b", "a"} {
		got, ok := q.pop()
		if !ok || got != want {
			t.Fatalf("pop = (%q,%v), want (%q,true)", got, ok, want)
		}
		q.done()
	}
	// 无在途任务且队列为空：pop 返回 false（worker 退出条件）。
	if _, ok := q.pop(); ok {
		t.Fatal("pop on empty queue must return false")
	}
}

// TestWorkQueue_WaitsUntilDone：栈空但仍有在途任务时阻塞，
// done 之后继续出队（pop/done 必须成对）。
func TestWorkQueue_WaitsUntilDone(t *testing.T) {
	q := newWorkQueue()
	q.push("only")

	got, ok := q.pop()
	if !ok || got != "only" {
		t.Fatalf("pop = (%q,%v)", got, ok)
	}

	popped := make(chan string, 1)
	go func() {
		dir, ok := q.pop()
		if !ok {
			popped <- ""
			return
		}
		popped <- dir
	}()

	// 队列空且 inFlight=1：上面的 pop 阻塞；done 必须唤醒它。
	//（channel 未就绪即 done 安全：唤醒发生在 done 之后。）
	q.done()
	if dir := <-popped; dir != "" {
		t.Fatalf("second pop = %q, want blocked until done", dir)
	}
}

func TestWorkQueue_CloseWakesWaiters(t *testing.T) {
	q := newWorkQueue()
	q.push("only")
	if _, ok := q.pop(); !ok {
		t.Fatal("pop must succeed")
	}

	closed := make(chan bool, 1)
	go func() {
		_, ok := q.pop()
		closed <- ok
	}()

	q.close()
	if ok := <-closed; ok {
		t.Fatal("pop after close must return false")
	}
	q.done() // 补齐 inFlight（close 不替代 done 的配对语义）
}

func TestWorkQueue_PushEmptyNoop(t *testing.T) {
	q := newWorkQueue()
	q.push()
	if _, ok := q.pop(); ok {
		t.Fatal("empty push must not create tasks")
	}
}
