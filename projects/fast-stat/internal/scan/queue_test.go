package scan

import (
	"testing"
	"time"
)

// timeoutGuard 用于把「本应阻塞」的断言变成失败而不是永久挂起。
//
// 它不是用来等待正确性（那属于禁止的 time.Sleep 用法），而是给死锁兜底：
// 若实现真的阻塞住了，测试应当在 2 秒内失败，而不是让 CI 一直卡住。
const timeoutGuard = 2 * time.Second

func TestWorkQueue_PopReturnsPushedItems(t *testing.T) {
	q := newWorkQueue()
	q.push("a", "b", "c")

	// LIFO：后进先出，便于深度优先遍历获得更好的磁盘局部性。
	for _, want := range []string{"c", "b", "a"} {
		got, ok := q.pop()
		if !ok {
			t.Fatalf("pop 返回 ok=false，期望 %q", want)
		}
		if got != want {
			t.Fatalf("pop = %q, want %q", got, want)
		}
		q.done()
	}

	if _, ok := q.pop(); ok {
		t.Error("栈空且无在途任务时 pop 应返回 ok=false")
	}
}

func TestWorkQueue_PushEmptyIsNoop(t *testing.T) {
	q := newWorkQueue()
	q.push()

	if _, ok := q.pop(); ok {
		t.Error("空 push 不应产生任务")
	}
}

// TestWorkQueue_PopBlocksUntilPush 覆盖「队列空但仍有任务在途」的等待路径。
func TestWorkQueue_PopBlocksUntilPush(t *testing.T) {
	q := newWorkQueue()
	q.push("first")

	if _, ok := q.pop(); !ok {
		t.Fatal("首次 pop 应成功")
	}
	// 此时 inFlight=1、栈为空：后续 pop 必须等待。

	popped := make(chan string, 1)
	go func() {
		if dir, ok := q.pop(); ok {
			popped <- dir
		}
	}()

	select {
	case dir := <-popped:
		t.Fatalf("在仍有任务处理中时 pop 不应立即返回（得到 %q）", dir)
	case <-time.After(50 * time.Millisecond):
		// 符合预期：pop 正在等待。
	}

	q.push("second")
	select {
	case dir := <-popped:
		if dir != "second" {
			t.Fatalf("pop = %q, want second", dir)
		}
	case <-time.After(timeoutGuard):
		t.Fatal("push 之后 pop 未被唤醒")
	}
}

// TestWorkQueue_DoneUnblocksWaiterWhenFinished 覆盖正常结束路径：
// inFlight 归零后等待中的 pop 必须返回 ok=false，否则 worker 无法退出。
func TestWorkQueue_DoneUnblocksWaiterWhenFinished(t *testing.T) {
	q := newWorkQueue()
	q.push("only")

	if _, ok := q.pop(); !ok {
		t.Fatal("首次 pop 应成功")
	}

	finished := make(chan bool, 1)
	go func() {
		_, ok := q.pop()
		finished <- ok
	}()

	select {
	case ok := <-finished:
		t.Fatalf("在 inFlight>0 时 pop 不应返回（ok=%v）", ok)
	case <-time.After(50 * time.Millisecond):
	}

	q.done()

	select {
	case ok := <-finished:
		if ok {
			t.Error("遍历结束后 pop 应返回 ok=false")
		}
	case <-time.After(timeoutGuard):
		t.Fatal("done 之后等待中的 pop 未被唤醒")
	}
}

// TestWorkQueue_CloseUnblocksWaiter 覆盖取消路径：
// close 必须唤醒所有等待者，否则取消后 worker 永久挂起。
func TestWorkQueue_CloseUnblocksWaiter(t *testing.T) {
	q := newWorkQueue()
	q.push("task")

	if _, ok := q.pop(); !ok {
		t.Fatal("首次 pop 应成功")
	}

	finished := make(chan bool, 1)
	go func() {
		_, ok := q.pop()
		finished <- ok
	}()

	select {
	case <-finished:
		t.Fatal("在 inFlight>0 时 pop 不应返回")
	case <-time.After(50 * time.Millisecond):
	}

	q.close()

	select {
	case ok := <-finished:
		if ok {
			t.Error("close 后 pop 应返回 ok=false")
		}
	case <-time.After(timeoutGuard):
		t.Fatal("close 未唤醒等待中的 pop")
	}
}

// TestWorkQueue_CloseKeepsPendingItemsAvailable 固化 close 的语义：
// close 只表示「不再等待新任务」，已经在栈中的任务仍可被取出处理。
func TestWorkQueue_CloseKeepsPendingItemsAvailable(t *testing.T) {
	q := newWorkQueue()
	q.push("pending")
	q.close()

	dir, ok := q.pop()
	if !ok || dir != "pending" {
		t.Fatalf("close 后应仍可取出现有任务，got=(%q, %v)", dir, ok)
	}
	q.done()

	if _, ok := q.pop(); ok {
		t.Error("取完后 pop 应返回 ok=false")
	}
}
