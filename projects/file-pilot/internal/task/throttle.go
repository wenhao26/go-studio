package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
)

// 节流参数（requirements §8.3）。
const (
	// progressInterval 是进度事件与日志窗口的推送周期（「≥200ms 间隔」取等号）。
	progressInterval = 200 * time.Millisecond

	// maxLogsPerWindow 是单窗口内非 system 日志的条数上限（「≤20 条」取等号）。
	maxLogsPerWindow = 20
)

// Throttler 实现 scanner.LogSink，并按固定周期推送进度、批量推送日志。
//
// 采样规则（§8.3）：system 日志永不丢弃、不计入配额；scan/classify 日志在
// 单窗口内超过 maxLogsPerWindow 即丢弃并累计 dropped，随后在该窗口的推送里
// 以一条 system 日志披露「已丢弃 N 条明细」，不让采样悄悄发生。
//
// 并发安全：Log 可被多个 worker 并发调用；Start/Stop 由单一 runner 串行调用。
type Throttler struct {
	taskID    int64
	startedAt time.Time
	emit      Emitter
	progress  func() ProgressUpdate
	now       func() time.Time

	// newTicker 注入周期源：testing.md 禁 time.Sleep，测试用手动通道驱动窗口。
	newTicker func(d time.Duration) (tick <-chan time.Time, stop func())

	mu      sync.Mutex
	pending []scanner.LogEntry
	sampled int // 当前窗口已接收的非 system 条数
	dropped int // 当前窗口因采样被丢弃的条数

	started    bool
	stopTicker func()
	stopOnce   sync.Once
	stopCh     chan struct{}
	doneCh     chan struct{}
}

// NewThrottler 构造节流器；调用方必须随后调用 Start，结束时调用 Stop。
//
// progress 返回任务当前的进度快照（任务相关字段；TaskID/ElapsedMS/Rate 由
// 本类型统一填写），可为 nil（如只推送日志）。
func NewThrottler(taskID int64, startedAt time.Time, emit Emitter, progress func() ProgressUpdate) *Throttler {
	return &Throttler{
		taskID:    taskID,
		startedAt: startedAt,
		emit:      emit,
		progress:  progress,
		now:       time.Now,
		newTicker: newTimeTicker,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// newTimeTicker 返回基于 time.Ticker 的周期源。
func newTimeTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

// Start 启动周期推送。
func (t *Throttler) Start() {
	tick, stop := t.newTicker(progressInterval)
	t.stopTicker = stop
	t.started = true

	go func() {
		defer close(t.doneCh)
		for {
			select {
			case <-tick:
				t.tick()
			case <-t.stopCh:
				return
			}
		}
	}()
}

// Stop 停止周期推送，并把窗口内剩余日志发出（收尾必发，保证不丢日志）。
func (t *Throttler) Stop() {
	t.stopOnce.Do(func() { close(t.stopCh) })
	if t.started {
		<-t.doneCh
		t.stopTicker()
	}
	t.flush()
}

// Log 实现 scanner.LogSink：按窗口配额采样后缓存，等待周期批量推送。
func (t *Throttler) Log(kind scanner.LogKind, text string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if kind != scanner.LogSystem {
		if t.sampled >= maxLogsPerWindow {
			t.dropped++
			return
		}
		t.sampled++
	}
	t.pending = append(t.pending, scanner.LogEntry{
		TS:   t.now().UTC().UnixMilli(),
		Kind: kind,
		Text: text,
	})
}

// tick 推送一次进度并清空日志窗口。
func (t *Throttler) tick() {
	if t.progress != nil {
		t.emit.EmitProgress(t.taskID, t.snapshot())
	}
	t.flush()
}

// flush 把窗口内日志连同丢弃披露推送给 Emitter；无内容时不推送。
func (t *Throttler) flush() {
	entries := t.drain()
	if len(entries) == 0 {
		return
	}
	t.emit.EmitLogs(t.taskID, entries)
}

// drain 取出待发送条目并重置窗口；丢弃计数折叠为一条 system 披露日志。
func (t *Throttler) drain() []scanner.LogEntry {
	t.mu.Lock()
	entries := t.pending
	dropped := t.dropped
	t.pending = nil
	t.sampled = 0
	t.dropped = 0
	t.mu.Unlock()

	if dropped > 0 {
		entries = append(entries, scanner.LogEntry{
			TS:   t.now().UTC().UnixMilli(),
			Kind: scanner.LogSystem,
			Text: fmt.Sprintf("已丢弃 %d 条明细", dropped),
		})
	}
	return entries
}

// snapshot 汇总当前进度与速率（裁决 D-A4：rate = 已扫描 / 已耗时秒数，取整）。
//
// 任务相关字段由 progress 源填写；TaskID 与 ElapsedMS 在此统一覆盖，
// rate 仅在扫描进度（Scanned>0）时计算，整理/撤销的 Percent 由源自行提供。
func (t *Throttler) snapshot() ProgressUpdate {
	p := ProgressUpdate{}
	if t.progress != nil {
		p = t.progress()
	}
	p.TaskID = t.taskID

	elapsed := t.now().Sub(t.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	p.ElapsedMS = elapsed.Milliseconds()
	if seconds := elapsed.Seconds(); seconds > 0 && p.Scanned > 0 {
		p.Rate = int64(float64(p.Scanned) / seconds)
	}
	return p
}
