package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/report"
	"github.com/wenhao26/go-studio/projects/fast-stat/internal/scan"
)

// progressInterval 是进度行的刷新间隔。
//
// 150ms 是「视觉上连贯」与「重绘开销可忽略」之间的折中：刷新更频繁只会在
// 小目录上做无意义的终端写入。
const progressInterval = 150 * time.Millisecond

// spinnerFrames 是进度指示器的帧序列。
//
// 刻意使用 ASCII 而非 braille 或 emoji：这些字形在部分终端字体中缺失，
// 或宽度不确定，会让进度行抖动。
var spinnerFrames = []rune{'-', '\\', '|', '/'}

// progressRenderer 把扫描进度渲染为单行覆盖式输出。
//
// 设计约束（对应需求决策 P0-5）：
//   - 不展示百分比与 ETA：遍历结束前总工作量未知，任何百分比都只能被编造；
//   - 只展示可观测量：已扫描目录/文件/大小、瞬时速率、已耗时、当前目录；
//   - 当前目录来源于文件名，属于不可信输入，输出前必须净化并截断。
type progressRenderer struct {
	out       io.Writer
	width     int
	frame     int
	startedAt time.Time
	prev      scan.ProgressInfo
	prevAt    time.Time
	lastLen   int
}

// newProgressRenderer 构造进度渲染器。width <= 0 时不做截断。
func newProgressRenderer(out io.Writer, width int) *progressRenderer {
	now := time.Now()
	return &progressRenderer{
		out:       out,
		width:     width,
		startedAt: now,
		prevAt:    now,
	}
}

// update 渲染一帧进度。
//
// now 由调用方传入而不是内部取 time.Now()，使速率与耗时的计算可以在测试中
// 用确定的时间点验证，无需依赖真实时钟流逝。
func (p *progressRenderer) update(now time.Time, cur scan.ProgressInfo) {
	filesPerSec, bytesPerSec := rates(p.prev, cur, now.Sub(p.prevAt))
	p.prev = cur
	p.prevAt = now

	elapsed := now.Sub(p.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}

	p.frame++
	p.write(p.line(cur, filesPerSec, bytesPerSec, elapsed))
}

// finish 清除进度行，避免其残留在最终报告上方。
//
// 未被清除时报告会紧跟在进度行之后输出，导致报告首行被覆盖成半截内容。
func (p *progressRenderer) finish() {
	if p.lastLen <= 0 {
		return
	}
	_, _ = fmt.Fprintf(p.out, "\r%s\r", strings.Repeat(" ", p.lastLen))
	p.lastLen = 0
}

// line 组装单行进度文本。
func (p *progressRenderer) line(cur scan.ProgressInfo, filesPerSec, bytesPerSec float64, elapsed time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%c %s dirs | %s files | %s | %s files/s | %s/s | %s",
		spinnerFrames[p.frame%len(spinnerFrames)],
		report.FormatCount(cur.DirectoriesVisited),
		report.FormatCount(cur.FilesFound),
		report.FormatBytes(cur.TotalSizeBytes),
		report.FormatCount(int64(filesPerSec)),
		report.FormatBytes(int64(bytesPerSec)),
		report.FormatDuration(elapsed),
	)

	if cur.CurrentDir != "" {
		b.WriteString(" | ")
		b.WriteString(report.SanitizeText(cur.CurrentDir))
	}

	return report.Truncate(b.String(), p.width)
}

// write 以 \r 覆盖上一帧，并用空格清掉残留字符。
//
// 写入失败不向上传播：进度输出失败说明终端已不可写，此时统计结果仍然有效，
// 不应因为渲染问题让整次扫描失败。
func (p *progressRenderer) write(line string) {
	n := report.DisplayWidth(line)

	var padding int
	if p.lastLen > n {
		padding = p.lastLen - n
	}
	if _, err := fmt.Fprintf(p.out, "\r%s%s", line, strings.Repeat(" ", padding)); err != nil {
		return
	}
	p.lastLen = n
}

// rates 计算两次采样之间的瞬时速率。
//
// 计数器单调递增；若样本乱序（dt <= 0 或计数回退），返回 0 而不是负数——
// 负速率会让进度行出现 "-123 files/s" 这类无意义输出。
func rates(prev, cur scan.ProgressInfo, dt time.Duration) (filesPerSec, bytesPerSec float64) {
	if dt <= 0 {
		return 0, 0
	}
	secs := dt.Seconds()

	deltaFiles := cur.FilesFound - prev.FilesFound
	deltaBytes := cur.TotalSizeBytes - prev.TotalSizeBytes
	if deltaFiles < 0 {
		deltaFiles = 0
	}
	if deltaBytes < 0 {
		deltaBytes = 0
	}
	return float64(deltaFiles) / secs, float64(deltaBytes) / secs
}
