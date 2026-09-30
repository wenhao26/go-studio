// Package scan 实现 fast-stat 的核心目录统计逻辑。
//
// 本包只负责目录遍历编排、并发控制、指标聚合与错误收集，不承担任何终端渲染或
// 输出格式化职责（分别属于 internal/report 与 internal/cli）。
//
// 并发模型：固定数量的 worker goroutine 反复从共享的 LIFO 目录栈取任务，
// 出队与回填由 workQueue 串行化。因此不会为每个目录创建一个 goroutine，
// 也不存在「生产者被队列阻塞」的死锁路径。
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/fsx"
)

const (
	// maxErrorDetails 限制保留的错误明细条数，使内存占用与目录规模无关。
	maxErrorDetails = 20

	// MaxWorkers 是并发度的硬上限。并发度可能来自外部输入，必须收敛，
	// 否则异常参数会耗尽文件描述符与调度资源。
	MaxWorkers = 1024

	// defaultWorkerCap 是默认并发度的上限。
	//
	// 上限存在的理由是 I/O 特征而非 CPU：在机械硬盘上，并发遍历会因磁头反复
	// 寻道导致吞吐低于单线程顺序遍历，因此默认值不应随 CPU 核数无限放大。
	defaultWorkerCap = 16
)

// DefaultWorkers 返回默认并发度，取 min(NumCPU, defaultWorkerCap)，最小为 1。
func DefaultWorkers() int {
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	if n > defaultWorkerCap {
		n = defaultWorkerCap
	}
	return n
}

// runState 保存单次扫描的实时可变状态。
//
// 计数器使用原子类型，使 Progress 可以在 Scan 执行期间被其他 goroutine 无锁读取；
// 错误明细只在 Scan 内部被并发写入，因此用互斥锁保护。
type runState struct {
	dirs    atomic.Int64
	files   atomic.Int64
	size    atomic.Int64
	errs    atomic.Int64
	current atomic.Pointer[string]

	mu      sync.Mutex
	details []ScanError
}

// record 记录一次被跳过的错误。
func (st *runState) record(path string, err error) {
	st.errs.Add(1)

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.details) < maxErrorDetails {
		st.details = append(st.details, ScanError{Path: path, Err: err})
	}
}

// errorDetails 返回当前错误明细的副本，避免调用方与内部状态共享底层数组。
func (st *runState) errorDetails() []ScanError {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.details) == 0 {
		return nil
	}
	out := make([]ScanError, len(st.details))
	copy(out, st.details)
	return out
}

// Scanner 执行目录统计扫描。
//
// 零值不可用，必须通过 New 构造。
//
// 并发约束：同一个 Scanner 不应被并发调用 Scan；但 Progress 可以在 Scan 执行
// 期间被其他 goroutine 并发调用。
type Scanner struct {
	fsys fsx.FileSystem
	run  atomic.Pointer[runState]
}

// New 构造 Scanner。
func New(fsys fsx.FileSystem) *Scanner {
	return &Scanner{fsys: fsys}
}

// Scan 并发遍历 opts.Root 并返回统计结果。
//
// 错误语义：
//   - 根路径不存在、不可访问或不是目录：返回致命错误，不产生有效统计结果；
//   - 单个子目录读取失败（如权限拒绝）：不中断扫描，计入 SkippedErrors 并保留明细；
//   - ctx 取消：尽快停止遍历，返回包装了 context 错误的错误，以及已经统计出的
//     部分结果（调用方仍可使用该结果）。
//
// ctx 必须是第一个参数并由调用方负责取消；本方法不会泄漏 goroutine。
func (s *Scanner) Scan(ctx context.Context, opts Options) (Snapshot, error) {
	if s == nil || s.fsys == nil {
		return Snapshot{}, errors.New("scan: scanner is not initialized")
	}
	if ctx == nil {
		return Snapshot{}, errors.New("scan: nil context")
	}

	root := filepath.Clean(opts.Root)
	if root == "" {
		root = "."
	}

	// 根路径必须先验证：不可用的根会让整个扫描毫无意义，此时应当立即失败，
	// 而不是产出一份「0 个文件」的报告。
	info, err := s.fsys.Lstat(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("stat root %q: %w", root, err)
	}
	if !info.IsDir() {
		return Snapshot{}, fmt.Errorf("scan root %q: %w", root, ErrNotDirectory)
	}

	// 内部派生 ctx：既保证唤醒 goroutine 在 Scan 返回时必定退出，
	// 也不会把取消信号泄漏回调用方。
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	st := &runState{}
	s.run.Store(st)

	workers := opts.Workers
	if workers <= 0 {
		workers = DefaultWorkers()
	}
	if workers > MaxWorkers {
		workers = MaxWorkers
	}

	q := newWorkQueue()
	q.push(root)

	// 取消后必须唤醒所有阻塞在 pop 里的 worker，否则它们会一直等到有新任务为止。
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		<-runCtx.Done()
		q.close()
	}()

	startedAt := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(runCtx, st, q)
		}()
	}
	wg.Wait()

	// 显式取消并等待唤醒 goroutine 退出，避免在 Scan 返回后残留 goroutine。
	cancel()
	<-watcherDone

	snap := Snapshot{
		Root:               root,
		DirectoriesVisited: st.dirs.Load(),
		FilesFound:         st.files.Load(),
		TotalSizeBytes:     st.size.Load(),
		SkippedErrors:      st.errs.Load(),
		Duration:           time.Since(startedAt),
		ErrorDetails:       st.errorDetails(),
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return snap, fmt.Errorf("scan %q: %w", root, ctxErr)
	}
	return snap, nil
}

// Progress 返回当前（或最近一次）扫描的实时进度快照。
//
// 可与 Scan 并发调用。从未执行过 Scan 时返回零值快照。
// 返回的 CurrentDir 未经终端净化，展示前必须由表现层处理。
func (s *Scanner) Progress() ProgressInfo {
	if s == nil {
		return ProgressInfo{}
	}
	st := s.run.Load()
	if st == nil {
		return ProgressInfo{}
	}

	p := ProgressInfo{
		DirectoriesVisited: st.dirs.Load(),
		FilesFound:         st.files.Load(),
		TotalSizeBytes:     st.size.Load(),
		SkippedErrors:      st.errs.Load(),
	}
	if cur := st.current.Load(); cur != nil {
		p.CurrentDir = *cur
	}
	return p
}

// worker 反复从队列取目录并处理，直到队列耗尽或被取消。
func (s *Scanner) worker(ctx context.Context, st *runState, q *workQueue) {
	for {
		if ctx.Err() != nil {
			return
		}
		dir, ok := q.pop()
		if !ok {
			return
		}
		s.processDir(ctx, st, dir, q)
	}
}

// processDir 处理单个目录：读取条目、累计文件指标、把子目录回填到队列。
//
// done 必须覆盖所有返回路径：遗漏会让其他 worker 永久阻塞在 pop 上。
func (s *Scanner) processDir(ctx context.Context, st *runState, dir string, q *workQueue) {
	defer q.done()

	if ctx.Err() != nil {
		return
	}
	st.current.Store(&dir)

	entries, err := s.fsys.ReadDir(dir)
	if err != nil {
		// 读取失败的目录不计入 DirectoriesVisited，但错误必须被记录，
		// 否则用户会得到一份「总数偏小却显示 Completed」的误导性报告。
		st.record(dir, err)
	} else {
		st.dirs.Add(1)
	}

	// 即使读取中途失败，已读出的条目也是真实存在的，仍然参与统计，
	// 避免因为一次 I/O 错误丢掉本可观测的数据。
	children := make([]string, 0, 4)
	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}

		name := entry.Name()
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			// v1 策略：不跟随符号链接。既避免环形链接导致无限遍历，
			// 也避免遍历越出用户指定的目标目录。符号链接不计入任何指标。
			continue
		case entry.IsDir():
			children = append(children, filepath.Join(dir, name))
		default:
			s.countEntry(st, dir, name, entry)
		}
	}

	// 先回填子目录再由 defer 调用 done：顺序颠倒会让其他 worker 提前退出。
	if len(children) > 0 {
		q.push(children...)
	}
}

// countEntry 累计单个非目录条目的指标。
func (s *Scanner) countEntry(st *runState, dir, name string, entry fs.DirEntry) {
	path := filepath.Join(dir, name)

	info, err := entry.Info()
	if err != nil {
		// 条目已被目录列表确认存在，因此仍计入文件数；
		// 但拿不到元数据，只能按 0 字节处理，并记录为被跳过的错误。
		st.files.Add(1)
		st.record(path, err)
		return
	}

	// Windows 上的目录联接（junction）在 Go 中既不是 ModeSymlink 也不是目录，
	// 而是 ModeIrregular（实测：Type() 为 "?---------"，IsDir() 为 false）。
	// 若不做区分，junction 会被当成 0 字节文件计入文件数，让 Windows 上的
	// 文件统计系统性偏大。
	//
	// 判据是「跟随一次后是否为目录」：是目录即为目录联接，按符号链接策略跳过；
	// 无法判定时按普通条目处理，宁可多算也不少算。
	if info.Mode()&fs.ModeIrregular != 0 && s.isDirectoryLink(path) {
		return
	}

	st.files.Add(1)
	// 逻辑大小（st_size）。Size() 对目录通常是块大小，但此处不会遇到目录。
	if size := info.Size(); size > 0 {
		st.size.Add(size)
	}
}

// isDirectoryLink 判断一个不可识别的条目是否指向目录（Windows 目录联接等）。
//
// 只在 ModeIrregular 这个罕见分支上被调用，因此不会给常规扫描路径增加 syscall。
func (s *Scanner) isDirectoryLink(path string) bool {
	info, err := s.fsys.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
