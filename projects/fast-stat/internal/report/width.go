package report

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// 本文件是 runewidth 依赖在本工程中的唯一落点，并显式固定终端宽度计算策略。
//
// 为什么必须引入该依赖：Go 标准库不提供任何「终端显示宽度」能力。东亚宽字符
// 与 emoji 在等宽终端占 2 列，用 len()（字节数）或 utf8.RuneCountInString
// （码点数）计算宽度都会让含中文/emoji 的行必然错位——这正是 PRD 示例能对齐、
// 而朴素实现做不到的原因。
//
// 为什么必须显式固定策略：
// runewidth 的默认策略随运行环境变化。实测在未设置任何 locale 环境变量的
// Windows 上 IsEastAsian() 返回 true，DefaultCondition.EastAsianWidth 随之为
// true，于是「歧义宽度」字符（Unicode East Asian Width = Ambiguous，例如
// 制表符 ═ 与省略号 …）被算作 2 列。后果是同一份代码在不同机器上产出宽度不同
// 的报告、边框与内容宽度互不一致，测试也无法稳定。
//
// 固定为 EastAsianWidth=false 的依据：
//   - 中文与 emoji 属于 Wide 而非 Ambiguous，无论该开关如何都按 2 列计算，
//     因此中文对齐在所有平台都正确；
//   - 制表符与省略号按 1 列，与现代终端（Windows Terminal、VS Code 终端、
//     iTerm2 默认配置）的实际渲染一致；
//   - 复现 PRD §三 示例的 58 列宽度；
//   - 输出在任何机器与 CI 上完全一致，可被测试稳定断言。
//
// 已知限制：若使用把歧义宽度字符渲染为 2 列的终端（部分传统 CJK 终端），
// 边框视觉上会比内容宽——但盒子内部仍然自洽，因为所有宽度都出自同一策略。
var cond = &runewidth.Condition{
	EastAsianWidth:     false,
	StrictEmojiNeutral: true,
}

// stringWidth 返回字符串在等宽终端下的显示列数。
func stringWidth(s string) int {
	return cond.StringWidth(s)
}

// DisplayWidth 返回字符串在等宽终端下的显示列数。
//
// 导出给进度渲染使用：进度行「先按宽度截断、再按宽度补空格清行」，
// 必须与 Truncate 使用同一套宽度口径，否则清理长度会算错。
func DisplayWidth(s string) int {
	return stringWidth(s)
}

// truncateByWidth 把字符串截断到 width 列，并以省略号结尾。
func truncateByWidth(s string, width int) string {
	return cond.Truncate(s, width, ellipsis)
}

// padRight 在字符串右侧补空格，使其显示宽度达到 width。
//
// 已超宽时原样返回：补齐只能变宽，不能变窄。
func padRight(s string, width int) string {
	if n := stringWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// center 把字符串在 width 列内居中，左右用空格填充。
//
// 奇数差值时右侧多一个空格，与 PRD 示例的居中效果一致。
func center(s string, width int) string {
	n := stringWidth(s)
	if n >= width {
		return s
	}
	left := (width - n) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-n-left)
}

// box 描述报告盒子的几何尺寸。
//
// 几何尺寸在构造时一次性算出，而不是在渲染时各自重复计算：边框行与内容行
// 只要有一处口径不同，盒子就会错位。把它们收敛到同一个结构体，
// 由构造保证一致性，而不是依赖调用方自觉。
type box struct {
	// innerCols 是内容区的显示列数。
	innerCols int

	// horizontalCount 是水平边框需要重复的字符数。
	horizontalCount int
}

// newBox 按目标列数构造盒子几何。
//
// 水平边框由字符重复而成，单个字符的宽度未必整除目标列数（歧义宽度字符
// 可能是 1 或 2 列），因此向上取整后再以实际宽度为准回填 innerCols，
// 让内容区与边框行在任何宽度策略下都严格等宽。
func newBox(targetCols int) box {
	if targetCols < 1 {
		targetCols = 1
	}

	charWidth := stringWidth(borderHorizontal)
	if charWidth < 1 {
		charWidth = 1
	}

	count := (targetCols + charWidth - 1) / charWidth
	return box{
		innerCols:       count * charWidth,
		horizontalCount: count,
	}
}

// contentIndent 是内容相对左边框的缩进列数，与 PRD 示例的排版一致
// （示例为 "║ Target Path  : ..."，而不是紧贴边框）。
const contentIndent = 1

// border 渲染一条完整边框行。
func (b box) border(left, right string) string {
	return left + strings.Repeat(borderHorizontal, b.horizontalCount) + right + "\n"
}

// content 渲染一行内容：两侧补竖线，按内容区宽度截断并补齐。
func (b box) content(row string) string {
	avail := b.innerCols - contentIndent
	return borderVertical + strings.Repeat(" ", contentIndent) +
		padRight(Truncate(row, avail), avail) + borderVertical + "\n"
}

// centered 渲染一行居中内容，用于标题。
//
// 不施加 contentIndent：标题在完整内容区内居中，额外缩进会让它看起来偏右。
func (b box) centered(s string) string {
	return borderVertical + center(s, b.innerCols) + borderVertical + "\n"
}
