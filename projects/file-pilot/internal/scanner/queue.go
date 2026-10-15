package scanner

import "sync"

// workQueue 是并发安全的 LIFO 目录栈（复用 fast-stat/internal/scan/queue.go
// 的已验证设计，development/01-phase-1-scan.md T1.4 要求）。
//
// 为什么不用 channel：生产者（发现子目录的 worker）与消费者是同一批 worker。
// 若使用有缓冲 channel 且 worker 在发送时阻塞，存在一条真实可达的死锁路径——
// 所有 worker 同时阻塞在发送、缓冲区已满、且没有任何 goroutine 在接收。
// 把「出队」与「回填」收敛到同一把锁之后，等待只可能发生在
// 「栈为空但仍有任务在处理」这一种情形，从结构上消除死锁（AGENTS.md §18 八问）。
//
// 为什么用 LIFO 而非 FIFO：深度优先遍历让相邻目录的访问在时间上聚集，
// 对机械硬盘的寻道与带预读的 SSD 都更友好；待处理路径的堆积量只与
// 「当前分支深度 × 每层待处理目录数」相关，而不是整棵树的宽度。
//
// 唤醒一律用 Broadcast 而非 Signal：等待条件是多个状态的组合
// （栈为空 && 仍有在途任务 && 未关闭），done 使条件成立时往往有多个等待者
// 需要同时退出；Signal 只唤醒一个，其余会永久阻塞在 Wait 上。
type workQueue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	stack    []string
	inFlight int
	closed   bool
}

// newWorkQueue 构造一个空队列。
func newWorkQueue() *workQueue {
	q := &workQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// push 把待处理目录压入栈。
//
// 本方法只在锁内做切片追加，不等待任何外部条件，调用方不存在死锁风险。
func (q *workQueue) push(dirs ...string) {
	if len(dirs) == 0 {
		return
	}
	q.mu.Lock()
	q.stack = append(q.stack, dirs...)
	q.mu.Unlock()
	q.cond.Broadcast()
}

// pop 弹出一个待处理目录。
//
// 返回 (dir, true) 表示取到任务，且 inFlight 已 +1，处理方**必须**调用 done；
// 返回 (dir, false) 表示队列已关闭或任务耗尽且无在途任务，worker 应退出。
//
// 阻塞只可能发生在「栈为空但仍有 inFlight 任务」这一种情形，
// 该状态必然被某个持有任务的 worker 的 done 解除，因此不会死锁。
func (q *workQueue) pop() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for {
		if q.closed {
			return "", false
		}
		if n := len(q.stack); n > 0 {
			dir := q.stack[n-1]
			q.stack = q.stack[:n-1]
			q.inFlight++
			return dir, true
		}
		if q.inFlight == 0 {
			return "", false
		}
		q.cond.Wait()
	}
}

// done 标记一个在途任务结束并唤醒等待者。
//
// 与 pop 必须成对出现：遗漏会让其他 worker 永久阻塞在 pop 上。
// 因此 processDir 里用 defer 保证所有返回路径都覆盖。
func (q *workQueue) done() {
	q.mu.Lock()
	q.inFlight--
	q.mu.Unlock()
	q.cond.Broadcast()
}

// close 关闭队列并唤醒全部等待者（取消路径调用）。
func (q *workQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}
