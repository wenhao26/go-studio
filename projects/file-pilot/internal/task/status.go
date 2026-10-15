// Package task 实现任务状态机、单活动约束与扫描期进度/日志节流
// （requirements §六、§8.3、FR-12）。
//
// 依赖方向：task → scanner（进度快照与日志条目类型）；不 import storage、
// 不 import app（contracts §2）。持久化能力以 Store 接口声明，由装配层 app 提供实现。
package task

import (
	"errors"
	"fmt"
)

// Status 是任务状态（contracts §3.3）。
type Status string

const (
	// StatusPending 是任务刚创建、尚未开始执行的状态。
	StatusPending Status = "pending"
	// StatusRunning 是执行中的状态。
	StatusRunning Status = "running"
	// StatusCompleted 是成功结束的终态。
	StatusCompleted Status = "completed"
	// StatusFailed 是失败结束的终态。
	StatusFailed Status = "failed"
	// StatusCanceled 是被取消结束的终态。
	StatusCanceled Status = "canceled"
	// StatusPaused 是预留状态：v1 不可达（requirements §十四 D-4）。
	// 保留它是为了让状态集合与需求一致，但合法转换表不含任何 paused 项，
	// 因此从/到它的转换都会被拒绝。
	StatusPaused Status = "paused"
)

// Type 是任务类型（contracts §3.4）。
type Type string

const (
	// TypeScan 是扫描任务（含分类流水线）。
	TypeScan Type = "scan"
	// TypeOrganize 是整理任务（含撤销），Phase 3 启用。
	TypeOrganize Type = "organize"
)

// 哨兵错误（contracts §4）。
//
// Go 侧消息是稳定的英文短语：它们同时是前端「哨兵 → 友好文案」映射表的
// 匹配键（requirements §8.4）。前端只展示映射结果，未命中映射的一律展示
// 通用文案并把原始错误写入诊断日志，绝不把 Go 错误直接堆给用户。
var (
	// ErrTaskRunning 单活动约束：已有任务在执行。
	ErrTaskRunning = errors.New("task already running")

	// ErrInvalidTransition 非法状态转换（requirements §6.2：拒绝而非忽略）。
	ErrInvalidTransition = errors.New("invalid task status transition")

	// ErrTaskNotFound 任务不存在（无对应行，或取消时目标任务不是活动任务）。
	ErrTaskNotFound = errors.New("task not found")
)

// allowedTransitions 是合法转换表（requirements §6.2）。
//
// pending 只能进入 running 或 canceled；running 只能进入三个终态；
// 终态不可再转换；paused 没有任何项，任何涉及它的转换都非法。
var allowedTransitions = map[Status][]Status{
	StatusPending: {StatusRunning, StatusCanceled},
	StatusRunning: {StatusCompleted, StatusFailed, StatusCanceled},
}

// canTransition 判断 from→to 是否合法。
func canTransition(from, to Status) bool {
	for _, next := range allowedTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// transitionError 构造被拒绝转换的错误（保留 ErrInvalidTransition 供 errors.Is 判别）。
func transitionError(id int64, from, to Status) error {
	return fmt.Errorf("task %d: %s → %s: %w", id, from, to, ErrInvalidTransition)
}

// IsTerminal 判断状态是否为终态。
func IsTerminal(s Status) bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}
