package app

import (
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/task"
)

// 事件名（contracts §6）。
const (
	eventStatus   = "task:status"
	eventProgress = "task:progress"
	eventLog      = "task:log"
)

// 事件载荷：跨 Wails 边界一律 snake_case（contracts §1）。

// statusPayload 是 task:status 的载荷：任务身份、当前状态与关联扫描。
type statusPayload struct {
	TaskID int64  `json:"task_id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	ScanID int64  `json:"scan_id"`
	Error  string `json:"error"`
}

// progressPayload 是 task:progress 的载荷：扫描与整理两分支字段同结构发送
// （未用分支为零值，前端按任务类型取舍展示）。
type progressPayload struct {
	TaskID    int64 `json:"task_id"`
	ElapsedMS int64 `json:"elapsed_ms"`

	// 扫描（不确定型）
	Scanned    int64  `json:"scanned"`
	Skipped    int64  `json:"skipped"`
	Rate       int64  `json:"rate"`
	CurrentDir string `json:"current_dir"`

	// 整理/撤销（确定型，contracts §3.8）
	Done           int64  `json:"done"`
	Total          int64  `json:"total"`
	Percent        int    `json:"percent"`
	CurrentFile    string `json:"current_file"`
	TargetCategory string `json:"target_category"`
}

// logPayload 是 task:log 的批量载荷：一次窗口冲刷的全部日志条目。
type logPayload struct {
	TaskID  int64              `json:"task_id"`
	Entries []scanner.LogEntry `json:"entries"`
}

// EmitStatus 实现 task.Emitter：每次状态机合法转换推送一条（不节流）。
func (a *App) EmitStatus(ev task.StatusEvent) {
	a.emit(eventStatus, statusPayload{
		TaskID: ev.TaskID,
		Type:   string(ev.Type),
		Status: string(ev.Status),
		ScanID: ev.ScanID,
		Error:  sanitize(ev.Error),
	})
}

// EmitProgress 实现 task.Emitter：周期推送进度（task.Throttler 保证 ≥200ms）。
func (a *App) EmitProgress(taskID int64, p task.ProgressUpdate) {
	a.emit(eventProgress, progressPayload{
		TaskID:         taskID,
		ElapsedMS:      p.ElapsedMS,
		Scanned:        p.Scanned,
		Skipped:        p.Skipped,
		Rate:           p.Rate,
		CurrentDir:     sanitize(p.CurrentDir),
		Done:           p.Done,
		Total:          p.Total,
		Percent:        p.Percent,
		CurrentFile:    sanitize(p.CurrentFile),
		TargetCategory: sanitize(p.TargetCategory),
	})
}

// EmitLogs 实现 task.Emitter：按窗口批量推送日志条目。
func (a *App) EmitLogs(taskID int64, entries []scanner.LogEntry) {
	if len(entries) == 0 {
		return
	}
	// 复制后净化：不修改调用方持有的切片（Throttler 的 pending 已置 nil，
	// 但 finalLogs 等调用方可能复用其底层数组）。
	safe := make([]scanner.LogEntry, len(entries))
	copy(safe, entries)
	for i := range safe {
		safe[i].Text = sanitize(safe[i].Text)
	}
	a.emit(eventLog, logPayload{TaskID: taskID, Entries: safe})
}

// emit 发送事件；生命周期 ctx 不存在时跳过（启动前/关闭后无接收方）。
//
// 事件发送失败不可上报：Wails 的 EventsEmit 无返回值、未订阅时静默丢弃，
// 与「事件不持久化、UI 恢复靠查询接口」（contracts §6）的语义一致。
func (a *App) emit(name string, payload any) {
	if a.lifecycle() == nil {
		return
	}
	a.publish(name, payload)
}

// publishEvent 是生产发布实现：经 Wails 运行时广播给前端。
func (a *App) publishEvent(name string, payload any) {
	runtime.EventsEmit(a.lifecycle(), name, payload)
}

// sanitize 剥离控制字符（SEC-5：文件名/路径属不可信输入，写出前净化，
// 防止伪造日志行与 UI 注入；保留可读文本本身）。
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
