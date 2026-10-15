// Package scanner 实现递归扫描流水线（requirements FR-3）。
//
// 并发模型沿用 fast-stat 已验证的「固定 worker 池 + LIFO 目录栈（mutex + sync.Cond）」：
// 不为每个目录创建 goroutine，出队与回填收敛在同一把锁之后，从结构上消除
// 「生产者阻塞于队列」的死锁路径（AGENTS.md §18）。
//
// 职责边界：本包只做遍历、跳过判定、元数据采集、即时分类与批量落库；
// 进度/日志的节流属于 internal/task，状态机与事件属于 internal/app。
package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/fsx"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/units"
)

// 跳过原因短语：写入跳过明细与日志，保持短语而非原始错误文本，
// 避免日志里出现冗长的 PathError；原始错误属于诊断日志的范畴。
const (
	skipReasonLink       = "符号链接/目录联接（不跟随）"
	skipReasonBlacklist  = "内置黑名单目录"
	skipReasonHiddenDir  = "隐藏目录（开关跳过）"
	skipReasonHiddenFile = "隐藏文件（开关跳过）"
	skipReasonSystemDir  = "系统目录（开关跳过）"
	skipReasonSystemFile = "系统文件（开关跳过）"
	skipReasonLargeFile  = "大文件 > 1GiB（开关跳过）"
	skipReasonStat       = "无法读取文件属性"
	skipReasonUnreadable = "目录不可读"
)

// blacklistedDirs 是内置黑名单目录名（requirements FR-2），
// 与隐藏/系统开关无关，任何层级始终跳过（不进入统计与整理）。
var blacklistedDirs = []string{"$Recycle.Bin", "System Volume Information"}

// isBlacklistDir 判断目录名是否命中内置黑名单（Windows 路径大小写不敏感，用 EqualFold）。
func isBlacklistDir(name string) bool {
	for _, b := range blacklistedDirs {
		if strings.EqualFold(name, b) {
			return true
		}
	}
	return false
}

// extensionOf 返回小写、不含点的扩展名；无扩展名返回空串（requirements-analysis §1.4）。
//
// 点在首字符视为「点前缀文件名」而非扩展名（".gitignore" 无扩展名）。
func extensionOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

// runState 保存单次扫描的实时可变状态。
//
// 计数器使用原子类型，使 Progress 可在 Scan 执行期间被其他 goroutine 无锁读取；
// 跳过明细与落库缓冲各自用互斥锁保护，且**不在持锁期间做数据库 IO**。
type runState struct {
	dirs    atomic.Int64
	files   atomic.Int64
	size    atomic.Int64
	skipped atomic.Int64
	current atomic.Pointer[string]

	detailMu sync.Mutex
	details  []SkipDetail

	bufMu sync.Mutex
	buf   []storage.FileRecord

	// storeErr 只记录首次落库失败；storeOnce 保证并发 worker 只写一次。
	storeOnce sync.Once
	storeErr  error

	// cancel 是本次运行的内部取消函数：落库失败后停止派发，
	// 避免继续产生无法持久化的数据（由 Scan 在启动 worker 前注入，只读使用）。
	cancel context.CancelFunc
}

// recordDetail 记一次跳过：累计计数并保留前 maxSkipDetails 条明细。
func (st *runState) recordDetail(path, reason string) {
	st.skipped.Add(1)

	st.detailMu.Lock()
	defer st.detailMu.Unlock()
	if len(st.details) < maxSkipDetails {
		st.details = append(st.details, SkipDetail{Path: path, Reason: reason})
	}
}

// detailsCopy 返回明细副本，避免与内部状态共享底层数组。
func (st *runState) detailsCopy() []SkipDetail {
	st.detailMu.Lock()
	defer st.detailMu.Unlock()
	if len(st.details) == 0 {
		return nil
	}
	out := make([]SkipDetail, len(st.details))
	copy(out, st.details)
	return out
}

// appendFile 把一条已分类的文件行追加进落库缓冲。
func (st *runState) appendFile(row storage.FileRecord) {
	st.bufMu.Lock()
	st.buf = append(st.buf, row)
	st.bufMu.Unlock()
}

// setStoreErr 记录首次落库失败并停止派发（幂等，仅首次生效）。
func (st *runState) setStoreErr(err error) {
	st.storeOnce.Do(func() {
		st.storeErr = err
		if st.cancel != nil {
			st.cancel()
		}
	})
}

// storeError 返回首次落库失败（若有）。
func (st *runState) storeError() error {
	// storeErr 只在 storeOnce 内写入；读取发生在全部 worker 退出之后（wg.Wait 提供
	// happens-before 边），此处不额外加锁。
	return st.storeErr
}

// scanRun 是一次扫描的只读运行参数（Scan 构造，worker 并发只读）。
type scanRun struct {
	root       string
	targetRoot string
	workers    int
	scanID     int64
	cfg        Config
	files      FileSink
	logs       LogSink
	classifier Classifier
}

// log 投递一条日志；logs 为空时安全跳过。
func (r *scanRun) log(kind LogKind, text string) {
	if r.logs == nil {
		return
	}
	r.logs.Log(kind, text)
}

// Scanner 执行目录扫描。
//
// 零值不可用，必须通过 New 构造。同一个 Scanner 不应被并发 Scan
// （应用层的单活动任务约束保证这一点，task.Manager）；Progress 可并发调用。
type Scanner struct {
	fsys       fsx.FileSystem
	classifier Classifier
	run        atomic.Pointer[runState]
}

// New 构造 Scanner。
//
// fsys 或 cls 为 nil 时 Scan 返回错误而非 panic，使依赖缺失以普通错误暴露
// （AGENTS.md §11：不依赖「这里不会 nil」的隐式假设）。
func New(fsys fsx.FileSystem, cls Classifier) *Scanner {
	return &Scanner{fsys: fsys, classifier: cls}
}

// Scan 并发遍历 opts.Root，产出统计并把已分类文件批量写入 files 表。
//
// 错误语义：
//   - 根路径不存在/不可访问/不是目录：致命错误，不产生有效结果；
//   - 单个条目读取失败（权限拒绝等）：不中断扫描，计入 Skipped 并保留明细；
//   - 落库失败：停止派发，返回包装了该错误的错误（任务置 failed）；
//   - ctx 取消：尽快退出，尾批仍落库以保留部分结果，返回包装 context 错误的错误。
//
// ctx 必须由调用方负责取消；本方法不会泄漏 goroutine。
func (s *Scanner) Scan(ctx context.Context, opts Options) (Result, error) {
	if s == nil || s.fsys == nil || s.classifier == nil {
		return Result{}, errors.New("scanner: not initialized")
	}
	if ctx == nil {
		return Result{}, errors.New("scanner: nil context")
	}
	if opts.Files == nil {
		return Result{}, errors.New("scanner: missing file sink")
	}
	// 防御性校验：正常入口在 StartScan 已校验，这里保证绕过入口时也不静默钳制。
	if err := opts.Config.Validate(); err != nil {
		return Result{}, err
	}

	root := filepath.Clean(opts.Root)
	info, err := s.fsys.Lstat(root)
	if err != nil {
		return Result{}, fmt.Errorf("stat root %q: %w", root, err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("scan root %q: %w", root, ErrNotDirectory)
	}

	// 内部派生 ctx：既保证取消能唤醒阻塞在 pop 上的 worker，
	// 也不会把取消信号泄漏回调用方（defer cancel 覆盖全部返回路径）。
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	st := &runState{cancel: cancel}
	s.run.Store(st)

	run := &scanRun{
		root:       root,
		targetRoot: filepath.Join(root, fsx.OrganizeDirName),
		workers:    opts.Config.Workers,
		scanID:     opts.ScanID,
		cfg:        opts.Config,
		files:      opts.Files,
		logs:       opts.Logs,
		classifier: s.classifier,
	}

	q := newWorkQueue()
	q.push(root)

	// 取消后必须唤醒所有阻塞在 pop 里的 worker，否则它们会一直等到有新任务。
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		<-runCtx.Done()
		q.close()
	}()

	startedAt := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < run.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(runCtx, st, q, run)
		}()
	}
	wg.Wait()

	// 显式取消并等待唤醒 goroutine 退出，避免 Scan 返回后残留 goroutine。
	cancel()
	<-watcherDone

	// 尾批必须落库：取消时也要保留已扫描部分（FR-3/FR-5「已取消 · 部分结果」），
	// 因此这里使用 WithoutCancel——取消信号不应吞掉用户可见的部分结果。
	s.flushBatch(context.WithoutCancel(ctx), st, run)

	snap := Result{
		Root:           root,
		FileCount:      st.files.Load(),
		DirCount:       st.dirs.Load(),
		TotalSizeBytes: st.size.Load(),
		Skipped:        st.skipped.Load(),
		Duration:       time.Since(startedAt),
		Workers:        run.workers,
		Details:        st.detailsCopy(),
	}

	// 落库失败优先于取消：它是真实的数据丢失，不应被「已取消」掩盖。
	if storeErr := st.storeError(); storeErr != nil {
		return snap, fmt.Errorf("scan %q: %w", root, storeErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return snap, fmt.Errorf("scan %q: %w", root, ctxErr)
	}
	return snap, nil
}

// Progress 返回当前（或最近一次）扫描的实时进度快照，可与 Scan 并发调用。
func (s *Scanner) Progress() Progress {
	if s == nil {
		return Progress{}
	}
	st := s.run.Load()
	if st == nil {
		return Progress{}
	}
	p := Progress{
		Files:     st.files.Load(),
		Dirs:      st.dirs.Load(),
		SizeBytes: st.size.Load(),
		Skipped:   st.skipped.Load(),
	}
	if cur := st.current.Load(); cur != nil {
		p.CurrentDir = *cur
	}
	return p
}

// worker 反复从队列取目录处理，直到队列耗尽或被取消。
func (s *Scanner) worker(ctx context.Context, st *runState, q *workQueue, run *scanRun) {
	for {
		if ctx.Err() != nil {
			return
		}
		dir, ok := q.pop()
		if !ok {
			return
		}
		s.processDir(ctx, st, q, run, dir)
	}
}

// processDir 处理单个目录：读取条目、判定跳过、采集文件、回填子目录。
//
// defer q.done() 覆盖所有返回路径：遗漏会让其他 worker 永久阻塞在 pop 上。
func (s *Scanner) processDir(ctx context.Context, st *runState, q *workQueue, run *scanRun, dir string) {
	defer q.done()

	if ctx.Err() != nil {
		return
	}
	st.current.Store(&dir)

	entries, err := s.fsys.ReadDir(dir)
	if err != nil {
		// 读取失败的目录不计入 dir_count（P2-3：仅统计成功完整读取的目录），
		// 但必须记为跳过，否则会得到「统计偏小却显示 completed」的误导性结果。
		s.record(run, st, dir, skipReasonUnreadable)
	} else {
		st.dirs.Add(1)
	}

	// 即使读取中途失败，已读出的条目也是真实存在的，仍然参与统计。
	children := make([]string, 0, 4)
	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}
		name := entry.Name()
		path := filepath.Join(dir, name)

		// 符号链接：不下潜、不纳入、计入跳过（FR-3、SEC-2 防环与防逃逸）。
		if entry.Type()&fs.ModeSymlink != 0 {
			s.record(run, st, path, skipReasonLink)
			continue
		}

		fi, infoErr := entry.Info()
		if infoErr != nil {
			// 属性不可得：跳过而非编造 size/mtime（files 表要求真实元数据）。
			s.record(run, st, path, skipReasonStat)
			continue
		}

		if entry.IsDir() {
			if s.shouldDescend(run, st, path, name, fi) {
				children = append(children, path)
			}
			continue
		}

		// Windows 目录联接实测为 ModeIrregular 而非目录（Phase 0 V5）：
		// 跟随一次判断它是否指向目录，是则按符号链接策略跳过（FR-3、SEC-2）。
		if fi.Mode()&fs.ModeIrregular != 0 && s.isDirectoryLink(path) {
			s.record(run, st, path, skipReasonLink)
			continue
		}

		attrs := fsx.AttributesOf(fi)
		hidden := fsx.IsHiddenName(name) || attrs.Hidden
		switch {
		case hidden && !run.cfg.ScanHidden:
			s.record(run, st, path, skipReasonHiddenFile)
		case attrs.System && !run.cfg.ScanSystem:
			s.record(run, st, path, skipReasonSystemFile)
		case run.cfg.SkipLarge && fi.Size() > largeFileThreshold:
			s.record(run, st, path, skipReasonLargeFile)
		default:
			s.addFile(run, st, ctx, path, name, fi, attrs)
		}
	}

	// 先回填子目录、再由 defer 调用 done：顺序颠倒会让其他 worker 提前退出。
	if len(children) > 0 && ctx.Err() == nil {
		q.push(children...)
	}
}

// shouldDescend 判断目录是否继续下潜；返回 false 时已按需记入跳过。
//
// 判定顺序：内置黑名单 → 本应用输出目录 → 隐藏 → 系统。
func (s *Scanner) shouldDescend(run *scanRun, st *runState, path, name string, fi fs.FileInfo) bool {
	// 内置黑名单目录始终跳过，与开关无关（FR-2）。
	if isBlacklistDir(name) {
		s.record(run, st, path, skipReasonBlacklist)
		return false
	}

	// 始终排除本应用的输出目录，避免把上次整理结果再次纳入扫描（FR-3、SEC-4）。
	// 它是「自有输出」而非异常跳过，不计入 skipped 统计（FR-5 口径不含它）。
	if path == run.targetRoot {
		return false
	}

	attrs := fsx.AttributesOf(fi)
	// 需求只写了「隐藏/系统文件」开关；隐藏/系统目录按同一开关处理
	//（裁决 D-A6，见 development/01-phase-1-scan.md §6）。
	if (fsx.IsHiddenName(name) || attrs.Hidden) && !run.cfg.ScanHidden {
		s.record(run, st, path, skipReasonHiddenDir)
		return false
	}
	if attrs.System && !run.cfg.ScanSystem {
		s.record(run, st, path, skipReasonSystemDir)
		return false
	}
	return true
}

// addFile 采集元数据、即时分类并写入落库缓冲（FR-3 流水线）。
func (s *Scanner) addFile(run *scanRun, st *runState, ctx context.Context, path, name string, fi fs.FileInfo, attrs fsx.Attributes) {
	ext := extensionOf(name)
	cls := run.classifier.Classify(path, name, ext)

	row := storage.FileRecord{
		Path:       path,
		Name:       name,
		Ext:        ext,
		SizeBytes:  fi.Size(),
		MtimeMS:    fi.ModTime().UTC().UnixMilli(),
		CtimeMS:    attrs.CreatedAt.UnixMilli(),
		HasCtime:   !attrs.CreatedAt.IsZero(),
		Category:   cls.Category,
		Confidence: cls.Confidence,
		Source:     cls.Source,
		Reason:     cls.Reason,
	}

	st.files.Add(1)
	st.size.Add(row.SizeBytes)
	st.appendFile(row)
	s.maybeFlush(ctx, st, run)

	// 两条日志均由 LogSink 采样（§8.3），不在本包做任何采样判断。
	run.log(LogScan, fmt.Sprintf("扫描: %s · %s", name, units.FormatBytes(row.SizeBytes)))
	run.log(LogClassify, fmt.Sprintf("分析: %s · %s · 建议分类: %s · 可信度: %d%%",
		name, cls.Reason, cls.Category, cls.Confidence))
}

// maybeFlush 缓冲达到批量阈值时执行一次事务写入（§七：每 500 条一个事务）。
//
// 取锁只做切片交接，数据库 IO 在锁外执行；并发 flush 由 SQLite 单连接串行化。
func (s *Scanner) maybeFlush(ctx context.Context, st *runState, run *scanRun) {
	st.bufMu.Lock()
	if len(st.buf) < defaultBatchSize {
		st.bufMu.Unlock()
		return
	}
	batch := st.buf
	st.buf = nil
	st.bufMu.Unlock()

	s.insert(ctx, st, run, batch)
}

// flushBatch 无条件写出缓冲中剩余的行（尾批 / 取消后的部分结果）。
func (s *Scanner) flushBatch(ctx context.Context, st *runState, run *scanRun) {
	st.bufMu.Lock()
	batch := st.buf
	st.buf = nil
	st.bufMu.Unlock()

	if len(batch) == 0 {
		return
	}
	s.insert(ctx, st, run, batch)
}

// insert 写入一批 files 行；失败记录到 runState（首个错误决定任务终态）。
func (s *Scanner) insert(ctx context.Context, st *runState, run *scanRun, batch []storage.FileRecord) {
	if err := run.files.InsertFiles(ctx, run.scanID, batch); err != nil {
		st.setStoreErr(fmt.Errorf("insert %d file rows: %w", len(batch), err))
	}
}

// record 记录一次跳过并投递实时日志（kind=scan，受 LogSink 采样）。
func (s *Scanner) record(run *scanRun, st *runState, path, reason string) {
	st.recordDetail(path, reason)
	run.log(LogScan, fmt.Sprintf("跳过: %s · %s", path, reason))
}

// isDirectoryLink 判断不可识别条目是否指向目录（Windows 目录联接等）。
//
// 只在 ModeIrregular 这一罕见分支被调用，不给常规扫描增加 syscall。
func (s *Scanner) isDirectoryLink(path string) bool {
	info, err := s.fsys.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
