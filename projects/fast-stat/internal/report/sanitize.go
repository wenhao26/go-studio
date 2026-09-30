package report

import "strings"

// SanitizeText 移除字符串中的控制字符，用于阻断终端转义序列注入。
//
// 为什么必须做这件事：输出内容包含由文件系统提供的路径与文件名，属于不可信
// 输入。若直接写到终端，攻击者可以在文件名中嵌入 ESC 序列来清屏、篡改终端
// 标题、写剪贴板（OSC 52），或用 \r 覆盖已输出的行、把报告数字伪装成任意值。
//
// 被剥离的字符：
//   - C0 控制符（U+0000–U+001F，含 \n \r \t）；
//   - DEL（U+007F）；
//   - C1 控制符（U+0080–U+009F，含 NEL）；
//   - 行分隔符 U+2028 与段分隔符 U+2029（会破坏单行布局）。
//
// 未处理的方向控制符（如 U+202E）无法伪造数值，但可以重排已渲染文本的视觉
// 顺序，属于已知限制，已在项目 README 的「已知限制」中记录。
//
// 无控制字符时零分配返回原字符串。
func SanitizeText(s string) string {
	for i, r := range s {
		if !isUnsafeRune(r) {
			continue
		}

		// 首次命中才分配：绝大多数路径不含控制字符，走的是零分配路径。
		var b strings.Builder
		b.Grow(len(s))
		b.WriteString(s[:i])
		for _, rest := range s[i:] {
			if !isUnsafeRune(rest) {
				b.WriteRune(rest)
			}
		}
		return b.String()
	}
	return s
}

// Truncate 把字符串按显示宽度截断到 width 列，超出时追加省略号。
//
// 使用显示宽度而非字节数或码点数：中文与 emoji 占 2 列，按字节截断会把字符切坏，
// 按码点数截断会让边框错位。
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if stringWidth(s) <= width {
		return s
	}
	return truncateByWidth(s, width)
}

// isUnsafeRune 判断一个码点是否应当在终端输出前被剥离。
func isUnsafeRune(r rune) bool {
	switch {
	case r < 0x20:
		return true
	case r == 0x7F:
		return true
	case r >= 0x80 && r <= 0x9F:
		return true
	case r == 0x2028 || r == 0x2029:
		return true
	default:
		return false
	}
}
