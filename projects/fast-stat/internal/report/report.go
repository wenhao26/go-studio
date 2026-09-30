// Package report 把扫描结果渲染为终端文本或 JSON。
//
// 本包是纯函数式表现层：不读取文件系统、不感知并发、不持有可变全局状态，
// 因此可以完整地用表格驱动测试覆盖。
//
// 输入契约统一为 Input：新增输出格式时只需增加一个渲染函数，
// 不需要把 scan 包的内部细节泄漏给调用方。
package report

import (
	"strconv"
	"time"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/scan"
)

// Status 表示一次扫描的最终状态。
type Status int

const (
	// StatusCompleted 表示扫描正常完成，可能包含被跳过的错误。
	StatusCompleted Status = iota

	// StatusCanceled 表示扫描被信号或上下文取消，统计结果为部分结果。
	StatusCanceled
)

// String 返回供终端展示的状态文案。
func (s Status) String() string {
	switch s {
	case StatusCanceled:
		return "Canceled"
	default:
		return "Completed"
	}
}

// JSONValue 返回供 JSON 输出的状态值。
func (s Status) JSONValue() string {
	switch s {
	case StatusCanceled:
		return "canceled"
	default:
		return "completed"
	}
}

// Input 是渲染报告所需的全部数据，也是 report 包对外的唯一输入契约。
type Input struct {
	// TargetPath 是实际统计的根路径。渲染前会被净化并截断。
	TargetPath string

	// Status 是扫描的最终状态。
	Status Status

	// Snapshot 是扫描统计结果。
	Snapshot scan.Snapshot

	// ToolVersion 是构建注入的版本号，写入 JSON 报告。
	ToolVersion string

	// Workers 是本次运行实际使用的并发度，仅在 Verbose 时展示。
	Workers int

	// Verbose 为 true 时追加生效参数与错误明细。
	Verbose bool

	// ASCIISymbols 为 true 时用纯文本标签替换 emoji 图标。
	//
	// 存在的理由：emoji 在部分终端字体中宽度不确定或直接缺失，会让报告错位。
	// 注意本字段的零值语义——false 表示「使用默认的 emoji 图标」，
	// 因此不会出现「忘记设置就退化成纯文本」的意外。
	ASCIISymbols bool

	// PlainLayout 为 true 时省略区块边框，仅输出文本行。
	//
	// 适用于把报告写入日志文件的场景。零值同样表示「使用默认的带边框排版」。
	PlainLayout bool
}

// FormatDuration 把耗时格式化为人类可读的字符串。
//
// 规则：
//   - 小于 1 秒：保留一位小数的毫秒（如 "12.5ms"）；
//   - 大于等于 1 秒：保留两位小数的秒（如 "1.24s"）。
//
// 之所以在亚秒区间切换到毫秒：目录扫描常常在毫秒级完成，统一按两位小数的秒
// 显示会得到 "0.00s"，让用户误以为工具没有测量耗时。
// 注意 JSON 报告的 duration_seconds 仍按秒（两位小数）输出，契约不变。
//
// 负数按 0 处理：耗时来自单调时钟差值，正常情况下不会为负。
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return strconv.FormatFloat(d.Seconds()*1000, 'f', 1, 64) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 2, 64) + "s"
}

// errorText 安全地取出错误文案：error 是接口，必须显式处理 nil。
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
