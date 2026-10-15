package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
)

// fixedNow 是测试时钟：Manager.now 注入后所有时间断言确定（testing.md 禁 sleep）。
var fixedNow = time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC)

// fakeStore 是 Store 的内存实现：记录调用序列、可注入失败，不碰真实 SQLite。
type fakeStore struct {
	mu    sync.Mutex
	calls []string

	nextTask int64
	nextScan int64
	tasks    map[int64]*Task
	scans    map[int64]ScanOutcome

	failCreateTask  bool
	failCreateScan  bool
	failSetTaskScan bool
	failSetStatus   bool
	failFinishScan  bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{tasks: map[int64]*Task{}, scans: map[int64]ScanOutcome{}}
}

func (f *fakeStore) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *fakeStore) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeStore) CreateTask(ctx context.Context, typ Type, startedAt int64) (int64, error) {
	f.record("CreateTask")
	if f.failCreateTask {
		return 0, errors.New("fake: create task failed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextTask++
	f.tasks[f.nextTask] = &Task{ID: f.nextTask, Type: typ, Status: StatusPending, StartedAt: startedAt}
	return f.nextTask, nil
}

func (f *fakeStore) CreateScan(ctx context.Context, root string, startedAt int64, workers int) (int64, error) {
	f.record("CreateScan")
	if f.failCreateScan {
		return 0, errors.New("fake: create scan failed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextScan++
	return f.nextScan, nil
}

func (f *fakeStore) SetTaskScan(ctx context.Context, id, scanID int64) error {
	f.record("SetTaskScan")
	if f.failSetTaskScan {
		return errors.New("fake: set task scan failed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("fake: task %d not found", id)
	}
	row.ScanID = scanID
	return nil
}

func (f *fakeStore) SetTaskParent(ctx context.Context, id, parentID int64) error {
	f.record("SetTaskParent")
	return nil
}

func (f *fakeStore) SetTaskStatus(ctx context.Context, id int64, to Status, finishedAt int64, cause string) error {
	f.record(fmt.Sprintf("SetTaskStatus:%s", to))
	if f.failSetStatus {
		return errors.New("fake: set status failed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("fake: task %d not found", id)
	}
	row.Status = to
	row.FinishedAt = finishedAt
	row.Error = cause
	return nil
}

func (f *fakeStore) FinishScan(ctx context.Context, scanID int64, outcome ScanOutcome) error {
	f.record(fmt.Sprintf("FinishScan:%s", outcome.Status))
	if f.failFinishScan {
		return errors.New("fake: finish scan failed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scans[scanID] = outcome
	return nil
}

func (f *fakeStore) FailUnfinishedTasks(ctx context.Context, finishedAt int64, cause string) (int64, error) {
	f.record("FailUnfinishedTasks")
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, row := range f.tasks {
		if row.Status == StatusPending || row.Status == StatusRunning {
			row.Status = StatusFailed
			row.FinishedAt = finishedAt
			row.Error = cause
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) FailRunningScans(ctx context.Context, finishedAt int64) (int64, error) {
	f.record("FailRunningScans")
	return 1, nil
}

func (f *fakeStore) GetTask(ctx context.Context, id int64) (*Task, error) {
	f.record("GetTask")
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.tasks[id]
	if !ok {
		return nil, fmt.Errorf("fake: task %d: %w", id, ErrTaskNotFound)
	}
	cp := *row
	return &cp, nil
}

// fakeEmitter 记录任务层推送的事件；onLogs 是测试钩子，用于与异步窗口同步。
type fakeEmitter struct {
	mu         sync.Mutex
	statuses   []StatusEvent
	progresses []ProgressUpdate
	logs       [][]scanner.LogEntry
	onLogs     func()
}

func (e *fakeEmitter) EmitStatus(ev StatusEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.statuses = append(e.statuses, ev)
}

func (e *fakeEmitter) EmitProgress(taskID int64, p ProgressUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.progresses = append(e.progresses, p)
}

func (e *fakeEmitter) EmitLogs(taskID int64, entries []scanner.LogEntry) {
	e.mu.Lock()
	e.logs = append(e.logs, append([]scanner.LogEntry(nil), entries...))
	hook := e.onLogs
	e.mu.Unlock()
	if hook != nil {
		hook()
	}
}

func (e *fakeEmitter) statusList() []StatusEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]StatusEvent(nil), e.statuses...)
}

func (e *fakeEmitter) progressList() []ProgressUpdate {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]ProgressUpdate(nil), e.progresses...)
}

func (e *fakeEmitter) logList() [][]scanner.LogEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([][]scanner.LogEntry, len(e.logs))
	for i := range e.logs {
		out[i] = append([]scanner.LogEntry(nil), e.logs[i]...)
	}
	return out
}

func newTestManager(t *testing.T) (*Manager, *fakeStore, *fakeEmitter) {
	t.Helper()
	store, emit := newFakeStore(), &fakeEmitter{}
	m := NewManager(store, emit)
	m.now = func() time.Time { return fixedNow }
	return m, store, emit
}

func beginScan(t *testing.T, m *Manager) *Handle {
	t.Helper()
	h, err := m.Begin(context.Background(), Spec{Type: TypeScan, Root: `C:\data`, Workers: 4})
	if err != nil {
		t.Fatalf("Begin scan: %v", err)
	}
	return h
}

func TestBegin_Scan_CreatesRowsInOrder(t *testing.T) {
	m, store, emit := newTestManager(t)

	h := beginScan(t, m)

	wantCalls := []string{"CreateScan", "CreateTask", "SetTaskScan", "SetTaskStatus:running"}
	got := store.callList()
	if len(got) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", got, wantCalls)
	}
	for i := range wantCalls {
		if got[i] != wantCalls[i] {
			t.Fatalf("calls = %v, want %v", got, wantCalls)
		}
	}

	if h.ID != 1 || h.ScanID != 1 || h.Root != `C:\data` || h.Type != TypeScan {
		t.Fatalf("unexpected handle: %+v", h)
	}
	if !h.StartedAt().Equal(fixedNow) {
		t.Fatalf("startedAt = %v, want %v", h.StartedAt(), fixedNow)
	}

	active := m.Active()
	if active == nil || active.ID != h.ID || active.Status != StatusRunning || active.ScanID != h.ScanID {
		t.Fatalf("unexpected active: %+v", active)
	}

	statuses := emit.statusList()
	if len(statuses) != 1 || statuses[0].Status != StatusRunning || statuses[0].TaskID != h.ID {
		t.Fatalf("unexpected status events: %+v", statuses)
	}
}

func TestBegin_SingleActiveConstraint(t *testing.T) {
	m, store, _ := newTestManager(t)
	beginScan(t, m)

	before := len(store.callList())
	_, err := m.Begin(context.Background(), Spec{Type: TypeScan, Root: `C:\other`, Workers: 4})
	if !errors.Is(err, ErrTaskRunning) {
		t.Fatalf("want ErrTaskRunning, got %v", err)
	}
	// 单活动约束必须在任何写库之前执行：失败路径不产生新行。
	if after := len(store.callList()); after != before {
		t.Fatalf("store touched after rejected Begin: %v", store.callList())
	}
}

func TestBegin_Organize_HasNoScanRow(t *testing.T) {
	m, store, _ := newTestManager(t)

	h, err := m.Begin(context.Background(), Spec{Type: TypeOrganize})
	if err != nil {
		t.Fatalf("Begin organize: %v", err)
	}
	if h.ScanID != 0 || h.Root != "" {
		t.Fatalf("organize handle must not carry scan: %+v", h)
	}
	for _, call := range store.callList() {
		if call == "CreateScan" || call == "SetTaskScan" {
			t.Fatalf("unexpected call for organize: %s", call)
		}
	}
}

func TestBegin_InvalidSpec(t *testing.T) {
	m, _, _ := newTestManager(t)

	if _, err := m.Begin(nil, Spec{Type: TypeScan}); err == nil {
		t.Error("want error for nil context")
	}
	if _, err := m.Begin(context.Background(), Spec{Type: "bogus"}); err == nil {
		t.Error("want error for unknown type")
	}
}

func TestBegin_CreateScanFails_NoTaskRow(t *testing.T) {
	m, store, _ := newTestManager(t)
	store.failCreateScan = true

	if _, err := m.Begin(context.Background(), Spec{Type: TypeScan, Root: `C:\data`, Workers: 4}); err == nil {
		t.Fatal("want error when CreateScan fails")
	}
	for _, call := range store.callList() {
		if call == "CreateTask" {
			t.Fatal("task row must not be created when scan creation failed")
		}
	}
	if m.Active() != nil {
		t.Fatal("active must stay nil after failed Begin")
	}
}

func TestBegin_CreateTaskFails_ReapsScan(t *testing.T) {
	m, store, _ := newTestManager(t)
	store.failCreateTask = true

	if _, err := m.Begin(context.Background(), Spec{Type: TypeScan, Root: `C:\data`, Workers: 4}); err == nil {
		t.Fatal("want error when CreateTask fails")
	}
	found := false
	for _, call := range store.callList() {
		if call == "FinishScan:failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("orphan scan row must be reaped as failed, calls=%v", store.callList())
	}
}

func TestBegin_SetTaskScanFails_ReapsBothRows(t *testing.T) {
	m, store, _ := newTestManager(t)
	store.failSetTaskScan = true

	_, err := m.Begin(context.Background(), Spec{Type: TypeScan, Root: `C:\data`, Workers: 4})
	if err == nil {
		t.Fatal("want error when SetTaskScan fails")
	}
	calls := store.callList()
	var reapedTask, reapedScan bool
	for _, call := range calls {
		if call == "SetTaskStatus:failed" {
			reapedTask = true
		}
		if call == "FinishScan:failed" {
			reapedScan = true
		}
	}
	if !reapedTask || !reapedScan {
		t.Fatalf("both rows must be reaped, calls=%v", calls)
	}
	if m.Active() != nil {
		t.Fatal("active must stay nil after failed Begin")
	}
}

// TestBegin_TransitionFails_ReleasesActive：pending→running 落库失败时，
// 单活动约束必须被释放，否则应用从此无法开始任何任务。
func TestBegin_TransitionFails_ReleasesActive(t *testing.T) {
	m, store, _ := newTestManager(t)
	store.failSetStatus = true

	if _, err := m.Begin(context.Background(), Spec{Type: TypeScan, Root: `C:\data`, Workers: 4}); err == nil {
		t.Fatal("want error when transition to running fails")
	}
	if m.Active() != nil {
		t.Fatal("active must be released after failed transition")
	}

	store.failSetStatus = false
	if _, err := m.Begin(context.Background(), Spec{Type: TypeScan, Root: `C:\data`, Workers: 4}); err != nil {
		t.Fatalf("Begin after recovery must succeed: %v", err)
	}
}

func TestFinish_Completed_PersistsStatsAndEmits(t *testing.T) {
	m, store, emit := newTestManager(t)
	h := beginScan(t, m)

	stats := ScanStats{
		Duration:       2 * time.Second,
		FileCount:      100,
		DirCount:       10,
		TotalSizeBytes: 4096,
		SkippedCount:   3,
	}
	if err := m.Finish(h.ID, StatusCompleted, nil, stats); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	outcome, ok := store.scans[h.ScanID]
	if !ok {
		t.Fatal("scan outcome not persisted")
	}
	if outcome.Status != StatusCompleted || outcome.FileCount != 100 || outcome.DurationMS != 2000 {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	// 裁决 D-A4：avg_speed = 文件数 / 耗时秒数。
	if outcome.AvgSpeed != 50 {
		t.Fatalf("avg speed = %v, want 50", outcome.AvgSpeed)
	}

	row, err := store.GetTask(context.Background(), h.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if row.Status != StatusCompleted || row.FinishedAt != fixedNow.UnixMilli() || row.Error != "" {
		t.Fatalf("unexpected task row: %+v", row)
	}

	statuses := emit.statusList()
	last := statuses[len(statuses)-1]
	if last.Status != StatusCompleted {
		t.Fatalf("last status event = %+v, want completed", last)
	}

	if m.Active() != nil {
		t.Fatal("active must be nil after Finish")
	}
	if h.Context().Err() == nil {
		t.Fatal("task ctx must be canceled after Finish")
	}
}

func TestFinish_Organize_SkipsScanPersistence(t *testing.T) {
	m, store, _ := newTestManager(t)
	h, err := m.Begin(context.Background(), Spec{Type: TypeOrganize})
	if err != nil {
		t.Fatalf("Begin organize: %v", err)
	}

	if err := m.Finish(h.ID, StatusCompleted, nil, ScanStats{}); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	for _, call := range store.callList() {
		if call == "FinishScan:completed" {
			t.Fatal("organize task must not write scans row")
		}
	}
}

func TestFinish_Canceled_NilCause(t *testing.T) {
	m, _, _ := newTestManager(t)
	h := beginScan(t, m)

	if err := m.Finish(h.ID, StatusCanceled, nil, ScanStats{}); err != nil {
		t.Fatalf("Finish canceled: %v", err)
	}
}

// TestFinish_IllegalTransition：running 不能回到 running，拒绝而非忽略（§6.2）。
func TestFinish_IllegalTransition(t *testing.T) {
	m, _, _ := newTestManager(t)
	h := beginScan(t, m)

	err := m.Finish(h.ID, StatusRunning, nil, ScanStats{})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
	// 非法转换不得改动状态：任务仍是活动任务。
	if m.Active() == nil {
		t.Fatal("illegal Finish must not release active task")
	}
}

func TestFinish_UnknownTask(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.Finish(999, StatusCompleted, nil, ScanStats{}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

// TestFinish_PersistFails_StillReleases：落库失败也必须释放单活动约束并推送
// 终态事件，否则 UI 停在 running 且应用卡死。
func TestFinish_PersistFails_StillReleases(t *testing.T) {
	m, store, emit := newTestManager(t)
	h := beginScan(t, m)

	store.failSetStatus = true
	if err := m.Finish(h.ID, StatusCompleted, nil, ScanStats{}); err == nil {
		t.Fatal("want error when persist fails")
	}
	if m.Active() != nil {
		t.Fatal("active must be released even when persist fails")
	}
	statuses := emit.statusList()
	last := statuses[len(statuses)-1]
	if last.Status != StatusCompleted {
		t.Fatalf("terminal event must be emitted anyway, got %+v", last)
	}
}

func TestCancel_ActiveTask_TriggersCooperativeCancel(t *testing.T) {
	m, _, _ := newTestManager(t)
	h := beginScan(t, m)

	if err := m.Cancel(context.Background(), h.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if h.Context().Err() == nil {
		t.Fatal("task ctx must be canceled")
	}
	// 取消后 runner 收敛终态；Manager 不越权替 runner 写 canceled。
	if m.Active() == nil {
		t.Fatal("active must remain until runner finishes")
	}
}

func TestCancel_UnknownTask(t *testing.T) {
	m, _, _ := newTestManager(t)
	if err := m.Cancel(context.Background(), 404); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

// TestCancel_AlreadyTerminal_Idempotent：前端可能在完成瞬间点击取消，
// 幂等返回 nil 而非报错（contracts §5）。
func TestCancel_AlreadyTerminal_Idempotent(t *testing.T) {
	m, _, _ := newTestManager(t)
	h := beginScan(t, m)
	if err := m.Finish(h.ID, StatusCompleted, nil, ScanStats{}); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if err := m.Cancel(context.Background(), h.ID); err != nil {
		t.Fatalf("cancel after terminal must be idempotent, got %v", err)
	}
}

// TestCancel_UnfinishedButNotActive：单进程内不应出现的状态，保守拒绝。
func TestCancel_UnfinishedButNotActive(t *testing.T) {
	m, store, _ := newTestManager(t)
	id, err := store.CreateTask(context.Background(), TypeScan, fixedNow.UnixMilli())
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := store.SetTaskStatus(context.Background(), id, StatusRunning, 0, ""); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	if err := m.Cancel(context.Background(), id); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

// TestRecover_MarksUnfinished：FR-12 崩溃恢复走批量修复，原因标记「应用中断」。
func TestRecover_MarksUnfinished(t *testing.T) {
	m, store, _ := newTestManager(t)
	running, err := store.CreateTask(context.Background(), TypeScan, fixedNow.UnixMilli())
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := store.SetTaskStatus(context.Background(), running, StatusRunning, 0, ""); err != nil {
		t.Fatalf("seed running: %v", err)
	}

	tasks, scans, err := m.Recover(context.Background())
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if tasks != 1 || scans != 1 {
		t.Fatalf("recovered tasks=%d scans=%d, want 1/1", tasks, scans)
	}

	row, err := store.GetTask(context.Background(), running)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if row.Status != StatusFailed || row.Error != "应用中断" {
		t.Fatalf("unexpected recovered row: %+v", row)
	}
}

func TestRecover_RejectsWhenActive(t *testing.T) {
	m, _, _ := newTestManager(t)
	beginScan(t, m)
	if _, _, err := m.Recover(context.Background()); err == nil {
		t.Fatal("want error when a task is active")
	}
}

func TestActive_None(t *testing.T) {
	m, _, _ := newTestManager(t)
	if got := m.Active(); got != nil {
		t.Fatalf("active = %+v, want nil", got)
	}
}

func TestWait_NoActive(t *testing.T) {
	m, _, _ := newTestManager(t)
	if !m.Wait(time.Millisecond) {
		t.Fatal("Wait without active task must return true")
	}
}

// TestWait_TimesOutThenSucceeds：未收尾时超时返回 false，Close 后返回 true。
// （timeout 本身即被测行为，不使用 sleep 同步。）
func TestWait_TimesOutThenSucceeds(t *testing.T) {
	m, _, _ := newTestManager(t)
	h := beginScan(t, m)

	if m.Wait(time.Millisecond) {
		t.Fatal("Wait must time out before runner finishes")
	}
	h.Close()
	if !m.Wait(time.Millisecond) {
		t.Fatal("Wait must succeed after Close")
	}
}

func TestCancelActive_NoOpWithoutTask(t *testing.T) {
	m, _, _ := newTestManager(t)
	m.CancelActive() // 无活动任务时不 panic、不报错
}

func TestHandle_Close_Idempotent(t *testing.T) {
	m, _, _ := newTestManager(t)
	h := beginScan(t, m)
	h.Close()
	h.Close()
}
