package report

import (
	"strings"
	"testing"
)

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain ascii path", "/var/log/syslog", "/var/log/syslog"},
		{"chinese path untouched", "/数据/目录/文件.txt", "/数据/目录/文件.txt"},
		{"emoji untouched", "🚀 report", "🚀 report"},
		{"strips CSI color escape", "a\x1b[31mred\x1b[0m", "a[31mred[0m"},
		{"strips carriage return", "safe\r4.82 GB", "safe4.82 GB"},
		{"strips line feed", "a\nb", "ab"},
		{"strips tab", "a\tb", "ab"},
		{"strips NUL", "a\x00b", "ab"},
		{"strips DEL", "a\x7fb", "ab"},
		{"strips C1 NEL", "a\u0085b", "ab"},
		{"strips OSC title injection", "\x1b]0;pwned\x07", "]0;pwned"},
		{"strips line separator", "a\u2028b", "ab"},
		{"strips paragraph separator", "a\u2029b", "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeText(tt.in); got != tt.want {
				t.Errorf("SanitizeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestTruncate 同时断言两条性质：精确结果（固化宽度策略）与不变量（保证截断永不越界）。
func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"fits", "abc", 5, "abc"},
		{"exact fit", "abcde", 5, "abcde"},
		{"ascii truncated", "abcdef", 3, "ab…"},
		{"empty input", "", 5, ""},
		{"zero width", "abc", 0, ""},
		{"negative width", "abc", -1, ""},
		{"cjk counted as two columns", "中文中文", 5, "中文…"},
		{"cjk fits exactly", "中文", 4, "中文"},
		{"width one keeps only ellipsis", "abcdef", 1, "…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.in, tt.width)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
			}

			// 不变量：任何情况下都不得超出请求宽度。
			if tt.width > 0 && DisplayWidth(got) > tt.width {
				t.Errorf("Truncate(%q, %d) = %q 宽度 %d 超限", tt.in, tt.width, got, DisplayWidth(got))
			}
			// 不变量：截断结果去掉标记后必须是原串的前缀，不得凭空生成内容。
			if prefix := strings.TrimSuffix(got, ellipsis); !strings.HasPrefix(tt.in, prefix) {
				t.Errorf("Truncate(%q, %d) = %q 不是原串前缀", tt.in, tt.width, got)
			}
		})
	}
}

// TestWidthPolicyIsPinned 固化宽度策略，防止 runewidth 的默认值随运行环境漂移。
//
// runewidth 的默认策略依赖 locale：在未设置 locale 环境变量的 Windows 上，
// IsEastAsian() 返回 true，会让歧义宽度字符按 2 列计算，从而让报告宽度在不同
// 机器上不一致、边框与内容错位。本测试锁定显式策略的效果。
func TestWidthPolicyIsPinned(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		// 歧义宽度（East Asian Width = Ambiguous）：本项目固定按 1 列。
		{"horizontal border is one column", borderHorizontal, 1},
		{"vertical border is one column", borderVertical, 1},
		{"ellipsis is one column", ellipsis, 1},
		// 宽字符（Wide）：与终端渲染一致，恒为 2 列。
		{"chinese is two columns", "中", 2},
		{"emoji is two columns", "🚀", 2},
		{"ascii is one column", "a", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayWidth(tt.in); got != tt.want {
				t.Errorf("DisplayWidth(%q) = %d, want %d（宽度策略可能被环境改写）", tt.in, got, tt.want)
			}
		})
	}
}

// TestDisplayWidthAssumptions 固化报告排版所依赖的宽度假设。
//
// 这些断言把「emoji 与中文按 2 列计」的假设变成可执行契约：一旦 runewidth 的
// 宽度表变更导致假设不再成立，测试会立即失败，而不是让边框在用户终端上静默错位。
func TestDisplayWidthAssumptions(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"rocket emoji", "🚀", 2},
		{"open folder emoji", "📂", 2},
		{"page emoji", "📄", 2},
		{"package emoji", "📦", 2},
		{"prohibited emoji", "🚫", 2},
		{"ascii letters", "abc", 3},
		{"chinese characters", "中文", 4},
		{"mixed", "a中", 3},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayWidth(tt.in); got != tt.want {
				t.Errorf("DisplayWidth(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
