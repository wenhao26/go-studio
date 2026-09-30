package scan

import "sync"

// workQueue 是一个并发安全的 LIFO 目录栈。
//
// 为什么不用 channel：
// 生产者（发现子目录的 worker）与消费者是同一批 worker。若使用有缓冲 channel
// 且 worker 在发送时阻塞，会存在一条真实可达的死锁路径——所有 worker 同时阻塞在
// 发送、缓冲区已满、且没有任何 goroutine 在接收。把「出队」与「回填」都收敛到
// 同一把锁之后，等待只可能发生在「栈为空但仍有任务在处理」这一种情形，
// 从结构上消除了死锁。
//
// 为什么用 LIFO 而非 FIFO：
// 深度优先遍历让相邻目录的访问在时间上聚集，对机械硬盘的寻道与带预读的 SSD
// 都更友好；同时待处理路径的堆积量只与「当前分支深度 × 每层待处理目录数」相关，
// 而不是整棵树的宽度。
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
// 本方法只在锁内做切片追加，不会等待任何外部条件，因此调用方不存在死锁风险。
func (q *workQueue) push(dirs ...string) {
	if len(dirs) == 0 {
		return
	}
	q.mu.Lock()
	q.stack = append(q.stack, dirs...)
	q.mu.Unlock()
	q.cond.Broadcast()
}

// pop 取出下一个待处理目录。
//
// 返回 ok=false 表示遍历已结束：栈为空且没有正在处理的任务，或被 close 唤醒。
// 调用方每成功 pop 一次，必须恰好调用一次 done。
func (q *workQueue) pop() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.stack) == 0 && q.inFlight > 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.stack) == 0 {
		return "", false
	}

	dir := q.stack[len(q.stack)-1]
	q.stack = q.stack[:len(q.stack)-1]
	q.inFlight++
	return dir, true
}

// done 标记一个已出队的目录处理完毕。
//
// 必须在把该目录的子目录 push 之后调用：否则会出现「栈为空且 inFlight 归零」的
// 瞬时状态，让其他 worker 误判遍历结束而漏扫整棵子树。
func (q *workQueue) done() {
	q.mu.Lock()
	if q.inFlight > 0 {
		q.inFlight--
	}
	q.mu.Unlock()
	q.cond.Broadcast()
}

// close 唤醒所有等待者，用于响应上下文取消。
//
// close 只表示「不再等待新任务」，栈中剩余任务是否继续处理由 worker 决定
// （worker 会在每轮循环开头检查 ctx，因此取消后会立即退出）。
func (q *workQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}
