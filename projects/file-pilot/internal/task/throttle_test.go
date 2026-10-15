package task

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
)

// newTestThrottler 构造测试用节流器：时间源注入，窗口由测试同步驱动，
// 全程无 sleep（testing.md）。未调用 Start 时可直接 th.tick() 推进窗口。
func newTestThrottler(progress func() ProgressUpdate) (*Throttler, *fakeEmitter) {
	emit := &fakeEmitter{}
	th := NewThrottler(7, fixedNow, emit, progress)
	th.now = func() time.Time { return fixedNow }
	return th, emit
}

// startWithManualTicker 启动节流器并接管周期源，返回手动触发的窗口通道。
// setup 在 Start 之前执行（goroutine 启动前完成共享状态赋值，避免测试竞态）。
func startWithManualTicker(t *testing.T, th *Throttler, setup func()) chan time.Time {
	t.Helper()
	tick := make(chan time.Time)
	stopped := 0
	th.newTicker = func(d time.Duration) (<-chan time.Time, func()) {
		if d != progressInterval {
			t.Errorf("ticker interval = %v, want %v", d, progressInterval)
		}
		return tick, func() { stopped++ }
	}
	if setup != nil {
		setup()
	}
	th.Start()
	t.Cleanup(func() {
		th.Stop()
		if stopped == 0 {
			t.Error("ticker was never stopped")
		}
	})
	return tick
}

// TestThrottler_WindowCapAndDisclosure 覆盖 §8.3：单窗口非 system 日志 ≤20 条，
// 超出部分丢弃并以一条 system 日志披露。
func TestThrottler_WindowCapAndDisclosure(t *testing.T) {
	th, emit := newTestThrottler(func() ProgressUpdate {
		return ProgressUpdate{Scanned: 42, Skipped: 3, CurrentDir: `C:\data`}
	})

	for i := 0; i < 25; i++ {
		th.Log(scanner.LogScan, fmt.Sprintf("扫描: f%02d", i))
	}
	th.tick()

	if len(emit.progressList()) != 1 {
		t.Fatalf("progress events = %d, want 1", len(emit.progressList()))
	}
	p := emit.progressList()[0]
	if p.TaskID != 7 || p.Scanned != 42 || p.Skipped != 3 || p.CurrentDir != `C:\data` {
		t.Fatalf("unexpected progress: %+v", p)
	}

	batches := emit.logList()
	if len(batches) != 1 {
		t.Fatalf("log batches = %d, want 1", len(batches))
	}
	entries := batches[0]
	if len(entries) != maxLogsPerWindow+1 {
		t.Fatalf("entries = %d, want %d（20 条明细 + 1 条披露）", len(entries), maxLogsPerWindow+1)
	}
	disclosure := entries[len(entries)-1]
	if disclosure.Kind != scanner.LogSystem || disclosure.Text != "已丢弃 5 条明细" {
		t.Fatalf("unexpected disclosure: %+v", disclosure)
	}
}

// TestThrottler_SystemLogsNeverSampled：system 日志不计入配额、永不丢弃。
func TestThrottler_SystemLogsNeverSampled(t *testing.T) {
	th, emit := newTestThrottler(nil)

	const n = 25
	for i := 0; i < n; i++ {
		th.Log(scanner.LogSystem, fmt.Sprintf("系统: %d", i))
	}
	th.tick()

	entries := emit.logList()[0]
	if len(entries) != n {
		t.Fatalf("entries = %d, want %d（system 不采样）", len(entries), n)
	}
}

// TestThrottler_MixedKinds_CountOnlyNonSystem：混合场景下配额只约束 scan/classify。
func TestThrottler_MixedKinds_CountOnlyNonSystem(t *testing.T) {
	th, emit := newTestThrottler(nil)

	for i := 0; i < maxLogsPerWindow; i++ {
		th.Log(scanner.LogScan, fmt.Sprintf("扫描: %d", i))
	}
	th.Log(scanner.LogSystem, "系统: 关键信息")
	th.Log(scanner.LogClassify, "分析: 被丢弃")
	th.tick()

	entries := emit.logList()[0]
	// 20 scan + 1 system + 1 披露（classify 因配额已满被丢弃）。
	if len(entries) != maxLogsPerWindow+2 {
		t.Fatalf("entries = %d, want %d", len(entries), maxLogsPerWindow+2)
	}
	if entries[maxLogsPerWindow].Text != "系统: 关键信息" {
		t.Fatalf("system entry must survive, got %+v", entries[maxLogsPerWindow])
	}
	if last := entries[len(entries)-1]; last.Text != "已丢弃 1 条明细" {
		t.Fatalf("unexpected disclosure: %+v", last)
	}
}

func TestThrottler_StopFlushesRemainder(t *testing.T) {
	th, emit := newTestThrottler(nil)
	startWithManualTicker(t, th, nil)

	th.Log(scanner.LogScan, "扫描: a")
	th.Log(scanner.LogScan, "扫描: b")
	th.Stop()

	batches := emit.logList()
	if len(batches) != 1 || len(batches[0]) != 2 {
		t.Fatalf("stop must flush pending logs, got %+v", batches)
	}
}

func TestThrottler_StopWithoutStartStillFlushes(t *testing.T) {
	th, emit := newTestThrottler(nil)
	th.Log(scanner.LogScan, "扫描: a")
	th.Stop()

	batches := emit.logList()
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("flush without start, got %+v", batches)
	}
}

// TestThrottler_GoroutineTickPath 覆盖 Start 的周期循环本身：
// tick 到达 → 推进度 + 发日志；Stop → 退出循环并停表。
func TestThrottler_GoroutineTickPath(t *testing.T) {
	th, emit := newTestThrottler(func() ProgressUpdate { return ProgressUpdate{Scanned: 9} })

	// flusher 在日志批次送达时通知，使断言与异步窗口同步（无 sleep）。
	flushed := make(chan struct{}, 1)
	tick := startWithManualTicker(t, th, func() {
		emit.onLogs = func() { flushed <- struct{}{} }
	})

	th.Log(scanner.LogScan, "扫描: a")
	tick <- time.Time{}
	<-flushed

	progresses := emit.progressList()
	if len(progresses) != 1 || progresses[0].Scanned != 9 {
		t.Fatalf("unexpected progress: %+v", progresses)
	}
	batches := emit.logList()
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("unexpected logs: %+v", batches)
	}
}

// TestThrottler_RateUsesLifetimeAverage：rate = 已扫描 / 已耗时秒数，取整（裁决 D-A4）。
func TestThrottler_RateUsesLifetimeAverage(t *testing.T) {
	start := fixedNow
	emit := &fakeEmitter{}
	th := NewThrottler(1, start, emit, func() ProgressUpdate {
		return ProgressUpdate{Scanned: 10}
	})
	// 已耗时 2 秒、扫描 10 个文件 → rate = 5。
	th.now = func() time.Time { return start.Add(2 * time.Second) }

	th.tick()

	progresses := emit.progressList()
	if len(progresses) != 1 {
		t.Fatalf("progress events = %d, want 1", len(progresses))
	}
	p := progresses[0]
	if p.Rate != 5 {
		t.Fatalf("rate = %d, want 5", p.Rate)
	}
	if p.ElapsedMS != 2000 {
		t.Fatalf("elapsed = %d, want 2000", p.ElapsedMS)
	}
}

// TestThrottler_NilProgress_EmitsNoProgress：无进度源时只推日志（降级场景）。
func TestThrottler_NilProgress_EmitsNoProgress(t *testing.T) {
	th, emit := newTestThrottler(nil)

	th.Log(scanner.LogScan, "扫描: a")
	th.tick()

	if len(emit.progressList()) != 0 {
		t.Fatalf("progress events = %d, want 0", len(emit.progressList()))
	}
	if len(emit.logList()) != 1 {
		t.Fatalf("log batches = %d, want 1", len(emit.logList()))
	}
}

// TestThrottler_ConcurrentLog：多个 worker 并发写日志（-race 验证无竞态），
// 配额与丢弃计数保持一致。
func TestThrottler_ConcurrentLog(t *testing.T) {
	th, emit := newTestThrottler(nil)

	const workers = 8
	const perWorker = 100
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				th.Log(scanner.LogScan, fmt.Sprintf("w%d-%d", w, i))
			}
		}(w)
	}
	wg.Wait()
	th.tick()

	entries := emit.logList()[0]
	if len(entries) != maxLogsPerWindow+1 {
		t.Fatalf("entries = %d, want %d", len(entries), maxLogsPerWindow+1)
	}
	wantDropped := workers*perWorker - maxLogsPerWindow
	if last := entries[len(entries)-1]; last.Text != fmt.Sprintf("已丢弃 %d 条明细", wantDropped) {
		t.Fatalf("disclosure = %+v, want %d dropped", last, wantDropped)
	}
}
