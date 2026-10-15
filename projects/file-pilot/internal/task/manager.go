package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
)

// recoveryCause 是异常恢复时写入的任务失败原因（FR-12：标记「应用中断」）。
const recoveryCause = "应用中断"

// startFailureCause 是启动中途失败的回收原因（不经过状态机转换，见 reapFailedStart）。
const startFailureCause = "任务启动失败"

// Spec 是创建任务的输入。
type Spec struct {
	// Type 是任务类型。
	Type Type

	// Root 是扫描任务的根目录（仅 TypeScan 需要）。
	Root string

	// Workers 是扫描并发数（仅 TypeScan 需要），范围 [1,64]。
	Workers int

	// ScanID 是整理/撤销任务关联的扫描 id（仅 TypeOrganize 需要；0 表示无）。
	// 扫描任务忽略此字段——其 scan_id 由 Begin 现场创建（避免外部传入不可信 id）。
	ScanID int64

	// ParentTaskID 是撤销任务指向的被撤销原整理任务 id（仅撤销任务需要；0 表示无）。
	ParentTaskID int64
}

// StatusEvent 是 task:status 事件的来源载荷（contracts §8.2）。
//
// ScanID 是任务关联的扫描记录 id（organize/无扫描任务为 0）：结果页据此
// 查询汇总/详情（D-C1），故随每次状态转换推送。
type StatusEvent struct {
	TaskID int64
	Type   Type
	Status Status
	ScanID int64
	Error  string
}

// ProgressUpdate 是一次进度推送（contracts §3.8：字段按任务类型取舍）。
//
// 扫描（不确定型）：Scanned/Skipped/Rate/CurrentDir。
// 整理/撤销（确定型）：Done/Total/Percent/CurrentFile/TargetCategory。
// 两个分支共用 ElapsedMS；TaskID/Rate 由 Throttler 统一填写。
type ProgressUpdate struct {
	TaskID    int64
	ElapsedMS int64
	Rate      int64 // 扫描：已扫描文件数 / 已耗时秒数（裁决 D-A4，取整）

	// 扫描专属
	Scanned    int64
	Skipped    int64
	CurrentDir string

	// 整理/撤销专属
	Done           int64
	Total          int64
	Percent        int
	CurrentFile    string
	TargetCategory string
}

// Emitter 是任务层向上传递事件的出口。
//
// 接口由消费方 task 定义（contracts §2），实现于 app（events.go）：
// 转成 snake_case 载荷并调用 runtime.EventsEmit。
type Emitter interface {
	EmitStatus(ev StatusEvent)
	EmitProgress(taskID int64, p ProgressUpdate)
	EmitLogs(taskID int64, entries []scanner.LogEntry)
}

// Handle 代表一次已启动的任务，交给调用方（app 的扫描 runner）驱动到终态。
//
// 使用约定：runner 必须 defer h.Close()，并在结束时调用 Manager.Finish。
type Handle struct {
	// ID 是 tasks.id。
	ID int64
	// Type 是任务类型。
	Type Type
	// ScanID 是关联的扫描记录 id（非 scan 任务为 0）。
	ScanID int64
	// Root 是扫描根目录（非 scan 任务为空串）。
	Root string

	startedAt time.Time
	status    Status // 受 Manager.mu 保护

	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	doneOnce sync.Once
}

// Context 返回任务的 ctx：任务被取消或等待收尾时结束，worker 由此协作退出。
func (h *Handle) Context() context.Context { return h.ctx }

// StartedAt 返回任务启动时刻，供节流器计算 elapsed 与 rate。
func (h *Handle) StartedAt() time.Time { return h.startedAt }

// Done 在 runner 结束（调用 Close）后关闭，供优雅退出等待。
func (h *Handle) Done() <-chan struct{} { return h.done }

// Close 标记任务运行结束（幂等），唤醒等待收尾的 Shutdown。
func (h *Handle) Close() {
	h.doneOnce.Do(func() { close(h.done) })
}

// ActiveTask 是当前活动任务的只读快照（GetActiveTask 的数据来源）。
type ActiveTask struct {
	ID        int64
	Type      Type
	ScanID    int64
	Root      string
	Status    Status
	StartedAt time.Time
}

// Manager 维护「同一时刻至多一个活动任务」的约束（requirements §6.3），
// 并集中执行状态转换与 DB 落库（T1.1：避免状态分裂）。
type Manager struct {
	mu     sync.Mutex
	active *Handle
	store  Store
	emit   Emitter
	now    func() time.Time // 可注入，测试可构造确定的时间
}

// NewManager 构造 Manager。
func NewManager(store Store, emit Emitter) *Manager {
	return &Manager{store: store, emit: emit, now: time.Now}
}

// Begin 创建任务并转入 running；单活动约束在此执行（contracts §5）。
//
// 创建顺序是「先扫描行、后任务行」：单活动检查通过后才写库，且任务行一旦
// 建立，下一次写库就是 pending→running，尽量压缩 pending 残留窗口。
// 中途失败按 best-effort 回收已创建的行；回收失败用 errors.Join 汇总上报，不吞错。
func (m *Manager) Begin(parent context.Context, spec Spec) (*Handle, error) {
	if parent == nil {
		return nil, errors.New("task: nil parent context")
	}
	switch spec.Type {
	case TypeScan, TypeOrganize:
	default:
		return nil, fmt.Errorf("task: unknown type %q", spec.Type)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		return nil, ErrTaskRunning
	}

	now := m.now().UTC()
	startedAt := now.UnixMilli()

	// 扫描任务的 scan_id 现场创建；整理/撤销任务关联调用方指定的既有扫描。
	var scanID int64
	createdScan := false
	if spec.Type == TypeScan {
		id, err := m.store.CreateScan(parent, spec.Root, startedAt, spec.Workers)
		if err != nil {
			return nil, fmt.Errorf("task: create scan record: %w", err)
		}
		scanID = id
		createdScan = true
	} else {
		scanID = spec.ScanID
	}

	taskID, err := m.store.CreateTask(parent, spec.Type, startedAt)
	if err != nil {
		// 任务行未建立：只回收本任务创建过的扫描行，避免残留 running 的孤儿扫描。
		return nil, errors.Join(
			fmt.Errorf("task: create task: %w", err),
			m.reapFailedStart(parent, 0, scanID, createdScan),
		)
	}

	if scanID != 0 {
		if err := m.store.SetTaskScan(parent, taskID, scanID); err != nil {
			return nil, errors.Join(
				fmt.Errorf("task: link task %d to scan %d: %w", taskID, scanID, err),
				m.reapFailedStart(parent, taskID, scanID, createdScan),
			)
		}
	}
	if spec.ParentTaskID != 0 {
		if err := m.store.SetTaskParent(parent, taskID, spec.ParentTaskID); err != nil {
			return nil, errors.Join(
				fmt.Errorf("task: link task %d to parent %d: %w", taskID, spec.ParentTaskID, err),
				m.reapFailedStart(parent, taskID, scanID, createdScan),
			)
		}
	}

	h := &Handle{
		ID:        taskID,
		Type:      spec.Type,
		ScanID:    scanID,
		Root:      spec.Root,
		startedAt: now,
		status:    StatusPending,
		done:      make(chan struct{}),
	}
	h.ctx, h.cancel = context.WithCancel(parent)

	if err := m.transitionLocked(h, StatusRunning, ""); err != nil {
		// 启动失败必须回收：否则行停在 pending/running，且任务 ctx 泄漏。
		h.cancel()
		return nil, errors.Join(err, m.reapFailedStart(parent, taskID, scanID, createdScan))
	}

	m.active = h
	return h, nil
}

// reapFailedStart 回收「启动中途失败」留下的行。
//
// 这些任务从未进入 running、也没有 Handle，无法经过合法转换表
// （pending→failed 是非法转换），因此按崩溃修复语义直接落 failed。
// 回收本身也可能失败（数据库不可用），此时返回错误交由调用方 Join 上报。
//
// 只有 createdScan 为 true（扫描任务现场创建的扫描行）才回收扫描行：
// 整理/撤销任务关联的是既有扫描，误置其终态会破坏已完成扫描的统计。
func (m *Manager) reapFailedStart(ctx context.Context, taskID, scanID int64, createdScan bool) error {
	finishedAt := m.now().UTC().UnixMilli()
	var errs []error
	if taskID != 0 {
		if err := m.store.SetTaskStatus(ctx, taskID, StatusFailed, finishedAt, startFailureCause); err != nil {
			errs = append(errs, fmt.Errorf("task: reap task %d: %w", taskID, err))
		}
	}
	if createdScan && scanID != 0 {
		if err := m.store.FinishScan(ctx, scanID, ScanOutcome{
			Status:     StatusFailed,
			FinishedAt: finishedAt,
		}); err != nil {
			errs = append(errs, fmt.Errorf("task: reap scan %d: %w", scanID, err))
		}
	}
	return errors.Join(errs...)
}

// transitionLocked 在持有 m.mu 的前提下执行一次状态转换：校验 → 落库 → 事件。
//
// 只处理任务状态；扫描行的终态统计由 persistTerminal 负责。Begin 的
// pending→running 也走这里，保证运行期转换集中一处。
func (m *Manager) transitionLocked(h *Handle, to Status, cause string) error {
	if !canTransition(h.status, to) {
		return transitionError(h.ID, h.status, to)
	}

	finishedAt := int64(0)
	if IsTerminal(to) {
		finishedAt = m.now().UTC().UnixMilli()
	}
	// ctx 可能已因取消而结束：落库用 WithoutCancel，否则取消路径永远写不进去。
	if err := m.store.SetTaskStatus(context.WithoutCancel(h.ctx), h.ID, to, finishedAt, cause); err != nil {
		return fmt.Errorf("task: persist task %d status %s: %w", h.ID, to, err)
	}

	h.status = to
	m.emit.EmitStatus(StatusEvent{TaskID: h.ID, Type: h.Type, Status: to, ScanID: h.ScanID, Error: cause})
	return nil
}

// Finish 把活动任务导向终态：先写 scans 终态统计（若有），再写任务状态与事件，
// 最后释放单活动约束并结束任务 ctx。
//
// stats 只在带扫描记录的任务上生效；organize 任务传零值即可（ScanID=0 不落库）。
// 即使落库失败也必须释放单活动约束，否则应用会永久卡在「有任务进行中」；
// 此时仍推送终态事件，避免前端永远停在 running（DB 侧下次启动由 Recover 修正）。
func (m *Manager) Finish(id int64, to Status, cause error, stats ScanStats) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	h := m.active
	if h == nil || h.ID != id {
		return fmt.Errorf("task: finish %d: %w", id, ErrTaskNotFound)
	}
	if !canTransition(h.status, to) {
		return transitionError(h.ID, h.status, to)
	}

	finishedAt := m.now().UTC().UnixMilli()
	causeMsg := ""
	if cause != nil {
		causeMsg = cause.Error()
	}

	persistErr := m.persistTerminal(h, to, finishedAt, causeMsg, stats)

	// 状态 → ctx → 单活动占位依次释放；Close 由 runner 的 defer 调用，
	// 保证 Shutdown 等待返回时 runner 已完成全部收尾。
	h.status = to
	h.cancel()
	m.active = nil

	// 终态事件必须推送（contracts §6：每次合法转换一条），无论落库成败：
	// 落库失败时 DB 侧由下次启动的 Recover 修正（FR-12），而 UI 停在 running
	// 的伤害是即时的。
	m.emit.EmitStatus(StatusEvent{TaskID: h.ID, Type: h.Type, Status: to, ScanID: h.ScanID, Error: causeMsg})
	return persistErr
}

// persistTerminal 写入扫描终态统计与任务状态。
func (m *Manager) persistTerminal(h *Handle, to Status, finishedAt int64, cause string, stats ScanStats) error {
	// ctx 可能已因取消而结束：落库必须用 WithoutCancel，否则取消路径永远写不进去。
	ctx := context.WithoutCancel(h.ctx)

	var errs []error
	// 只有扫描任务才会终结其现场创建的 scans 行；整理/撤销任务关联的是
	// 既有已完成扫描，若在此 FinishScan 会用零值统计覆盖它（数据破坏）。
	if h.Type == TypeScan && h.ScanID != 0 {
		outcome := ScanOutcome{
			Status:         to,
			FinishedAt:     finishedAt,
			DurationMS:     stats.Duration.Milliseconds(),
			FileCount:      stats.FileCount,
			DirCount:       stats.DirCount,
			TotalSizeBytes: stats.TotalSizeBytes,
			SkippedCount:   stats.SkippedCount,
			AvgSpeed:       avgSpeed(stats.FileCount, stats.Duration),
		}
		if err := m.store.FinishScan(ctx, h.ScanID, outcome); err != nil {
			errs = append(errs, fmt.Errorf("task: finish scan %d: %w", h.ScanID, err))
		}
	}
	if err := m.store.SetTaskStatus(ctx, h.ID, to, finishedAt, cause); err != nil {
		errs = append(errs, fmt.Errorf("task: persist task %d status %s: %w", h.ID, to, err))
	}
	return errors.Join(errs...)
}

// avgSpeed 计算全程平均扫描速度（files/s；裁决 D-A4：与进度 rate 同源口径）。
func avgSpeed(files int64, d time.Duration) float64 {
	seconds := d.Seconds()
	if seconds <= 0 {
		return 0
	}
	return float64(files) / seconds
}

// Cancel 请求取消任务。
//
// 语义：对活动任务触发协作式取消并返回 nil——真正的终态由 runner 观察到
// ctx 取消后经 Finish 写入；任务已终态时返回 nil（幂等：前端可能在完成
// 瞬间点击取消）；任务不存在返回 ErrTaskNotFound。
func (m *Manager) Cancel(ctx context.Context, id int64) error {
	m.mu.Lock()
	h := m.active
	m.mu.Unlock()

	if h != nil && h.ID == id {
		h.cancel()
		return nil
	}

	row, err := m.store.GetTask(ctx, id)
	if err != nil {
		// 适配层已把「不存在」映射为 ErrTaskNotFound。
		return fmt.Errorf("task: cancel %d: %w", id, err)
	}
	if IsTerminal(row.Status) {
		return nil
	}
	// 存在非终态行却不是活动任务：单进程内不应发生（活动约束保证），
	// 保守按未知任务拒绝，不做任何状态写入。
	return fmt.Errorf("task: cancel %d: status=%s not active: %w", id, row.Status, ErrTaskNotFound)
}

// Recover 把上次进程异常退出遗留的 pending/running 任务与 running 扫描行
// 批量置为 failed（FR-12：原因标记「应用中断」）。
//
// 这是批量修复而非状态机转换：这些行没有活动 Handle，无法经过合法转换表。
// 返回受影响的任务数与扫描行数。
func (m *Manager) Recover(ctx context.Context) (int64, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		// 防御：启动阶段不可能已有活动任务；出现即说明被误用，拒绝执行。
		return 0, 0, fmt.Errorf("task: recover called while task %d active", m.active.ID)
	}

	finishedAt := m.now().UTC().UnixMilli()
	tasks, err := m.store.FailUnfinishedTasks(ctx, finishedAt, recoveryCause)
	if err != nil {
		return 0, 0, fmt.Errorf("task: recover unfinished tasks: %w", err)
	}
	scans, err := m.store.FailRunningScans(ctx, finishedAt)
	if err != nil {
		return tasks, 0, fmt.Errorf("task: recover running scans: %w", err)
	}
	return tasks, scans, nil
}

// Active 返回当前活动任务快照；无活动任务返回 nil。
func (m *Manager) Active() *ActiveTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return nil
	}
	h := m.active
	return &ActiveTask{
		ID:        h.ID,
		Type:      h.Type,
		ScanID:    h.ScanID,
		Root:      h.Root,
		Status:    h.status,
		StartedAt: h.startedAt,
	}
}

// CancelActive 取消当前活动任务（无活动任务时为 no-op），供优雅退出使用。
func (m *Manager) CancelActive() {
	m.mu.Lock()
	h := m.active
	m.mu.Unlock()
	if h != nil {
		h.cancel()
	}
}

// Wait 等待活动任务收尾（runner 调用 Close 后返回），最多等待 timeout；
// 返回 true 表示已收尾，false 表示等待超时。
func (m *Manager) Wait(timeout time.Duration) bool {
	m.mu.Lock()
	h := m.active
	m.mu.Unlock()
	if h == nil {
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-h.Done():
		return true
	case <-timer.C:
		return false
	}
}
