package report

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/scan"
)

// 报告排版常量。
const (
	// contentWidth 是内容区的目标显示列数（不含左右边框），与 PRD 示例一致。
	//
	// 取固定宽度而非自适应：边框宽度随数据变化会让不同目录的报告无法并排比较，
	// 而超长内容由 Truncate 收敛，因此固定宽度不会破坏对齐。
	contentWidth = 56

	// headerLabelWidth 是元信息区标签的对齐宽度。
	headerLabelWidth = 12

	// ellipsis 是截断标记。
	ellipsis = "…"

	// reportTitle 是带 emoji 的报告标题。
	reportTitle = "🚀 FASTSTAT SCAN REPORT"

	// asciiTitle 是纯文本模式下的报告标题。
	asciiTitle = "FASTSTAT SCAN REPORT"

	// toolName 是报告中记录的工具名。
	toolName = "faststat"
)

// 边框字符。
//
// PRD §三 示例的分隔行使用了 "╠……╝"（左三叉连接符 + 右下角），这是不合法的
// 组合；本项目按决策修正为 "╠……╣"。box 只接受成对的左右字符，
// 因此不再可能构造出非法组合。
const (
	borderTopLeft     = "╔"
	borderTopRight    = "╗"
	borderMiddleLeft  = "╠"
	borderMiddleRight = "╣"
	borderBottomLeft  = "╚"
	borderBottomRight = "╝"
	borderVertical    = "║"
	borderHorizontal  = "═"
)

// metric 描述一项统计指标的展示形式。
//
// 标签与取值函数绑定在同一个结构里，而不是分成两个平行切片：平行切片在新增
// 指标时极易错位，而错位后的报告会「看起来正常但数字对不上」。
type metric struct {
	// emoji 是默认图标，均选自 Unicode Wide 区（在终端恒为 2 列）。
	emoji string

	// label 是中文与英文标签。
	label string

	// value 从快照中取出该项指标的展示值。
	value func(snapshot scan.Snapshot) string
}

// metrics 是统计指标的唯一数据源，顺序与 PRD §二.4 一致。
//
// 其中「跳过错误数」由需求决策 P0-3 确定为第四项统计指标
// （PRD 原文仅列三项，但 §四.2 又要求在报告中展示错误数）。
var metrics = []metric{
	{
		emoji: "📂",
		label: "扫描次数 (Directories Visited)",
		value: func(s scan.Snapshot) string { return FormatCount(s.DirectoriesVisited) },
	},
	{
		emoji: "📄",
		label: "文件个数 (Total Files Found)",
		value: func(s scan.Snapshot) string { return FormatCount(s.FilesFound) },
	},
	{
		emoji: "📦",
		label: "路径总大小 (Total Size)",
		value: func(s scan.Snapshot) string { return FormatBytes(s.TotalSizeBytes) },
	},
	{
		emoji: "🚫",
		label: "跳过错误数 (Skipped Errors)",
		value: func(s scan.Snapshot) string { return FormatCount(s.SkippedErrors) },
	},
}

// Text 把报告输入渲染为终端文本。
//
// 默认输出带边框的居中标题排版；ASCIISymbols 去掉 emoji 图标，
// PlainLayout 进一步去掉边框与区块分隔。
//
// 对齐基于显示宽度（东亚宽字符与 emoji 按 2 列计），而非字节数或码点数，
// 因此含中文与 emoji 的行同样对齐；任何超出内容区宽度的文本都会被截断，
// 保证边框在任何输入下都不错位。
func Text(in Input) string {
	if in.PlainLayout {
		return plainText(in)
	}

	bx := newBox(contentWidth)

	var b strings.Builder
	b.WriteString(bx.border(borderTopLeft, borderTopRight))
	b.WriteString(bx.centered(textTitle(in.ASCIISymbols)))

	b.WriteString(bx.border(borderMiddleLeft, borderMiddleRight))
	for _, row := range textHeaderRows(in) {
		b.WriteString(bx.content(row))
	}

	b.WriteString(bx.border(borderMiddleLeft, borderMiddleRight))
	for _, row := range textMetricRows(in) {
		b.WriteString(bx.content(row))
	}

	if extra := textVerboseRows(in); len(extra) > 0 {
		b.WriteString(bx.border(borderMiddleLeft, borderMiddleRight))
		for _, row := range extra {
			b.WriteString(bx.content(row))
		}
	}

	b.WriteString(bx.border(borderBottomLeft, borderBottomRight))
	return b.String()
}

// plainText 渲染无边框版本，便于把报告追加进日志文件。
func plainText(in Input) string {
	var b strings.Builder

	b.WriteString(textTitle(in.ASCIISymbols))
	b.WriteString("\n\n")

	for _, row := range textHeaderRows(in) {
		b.WriteString(row)
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	for _, row := range textMetricRows(in) {
		b.WriteString(row)
		b.WriteByte('\n')
	}

	if extra := textVerboseRows(in); len(extra) > 0 {
		b.WriteByte('\n')
		for _, row := range extra {
			b.WriteString(row)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// textTitle 返回报告标题。纯文本模式下去掉 emoji。
func textTitle(asciiOnly bool) string {
	if asciiOnly {
		return asciiTitle
	}
	return reportTitle
}

// displayLabel 返回指标标签。
//
// asciiOnly 为 true 时省略图标：emoji 在部分终端字体中宽度不确定或直接缺失，
// 去掉图标比换成 ASCII 占位符更不容易误导用户。
func (m metric) displayLabel(asciiOnly bool) string {
	if asciiOnly || m.emoji == "" {
		return m.label
	}
	return m.emoji + " " + m.label
}

// textHeaderRows 构造元信息区：目标路径与最终状态。
func textHeaderRows(in Input) []string {
	return []string{
		kvLine("Target Path", SanitizeText(in.TargetPath), headerLabelWidth),
		kvLine("Status", fmt.Sprintf("%s (Duration: %s)", in.Status, FormatDuration(in.Snapshot.Duration)), headerLabelWidth),
	}
}

// textMetricRows 构造统计指标区。
func textMetricRows(in Input) []string {
	pairs := make([][2]string, 0, len(metrics))
	for _, m := range metrics {
		pairs = append(pairs, [2]string{m.displayLabel(in.ASCIISymbols), m.value(in.Snapshot)})
	}
	return alignedRows(pairs)
}

// textVerboseRows 构造诊断区，仅在 Verbose 时输出。
//
// 只输出真实测量到的内容：生效参数与错误明细。此处刻意不输出「计时分解」，
// 因为本工具没有分阶段计时数据，编造一个分解表比不输出更糟。
func textVerboseRows(in Input) []string {
	if !in.Verbose {
		return nil
	}

	rows := []string{kvLine("Workers", strconv.Itoa(in.Workers), headerLabelWidth)}
	if len(in.Snapshot.ErrorDetails) == 0 {
		return rows
	}

	rows = append(rows, fmt.Sprintf("Skipped Error Details: %d of %d shown",
		len(in.Snapshot.ErrorDetails), in.Snapshot.SkippedErrors))
	for _, detail := range in.Snapshot.ErrorDetails {
		rows = append(rows, "  "+SanitizeText(detail.Path)+": "+SanitizeText(errorText(detail.Err)))
	}
	return rows
}

// alignedRows 把「标签: 值」对渲染为冒号对齐的行。
func alignedRows(pairs [][2]string) []string {
	width := 0
	for _, p := range pairs {
		if n := stringWidth(p[0]); n > width {
			width = n
		}
	}

	rows := make([]string, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, kvLine(p[0], p[1], width))
	}
	return rows
}

// kvLine 生成 "标签 : 值" 形式的一行，标签按显示宽度右补空格对齐。
func kvLine(label, value string, labelWidth int) string {
	return padRight(label, labelWidth) + " : " + value
}
