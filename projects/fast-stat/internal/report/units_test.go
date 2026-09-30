package report

import (
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0 Bytes"},
		{"one byte", 1, "1 Bytes"},
		{"just below KB", 1023, "1,023 Bytes"},
		{"exactly KB", 1024, "1.00 KB"},
		{"one and a half KB", 1536, "1.50 KB"},
		// 档位边界刻意保留 PRD 定义的档位语义：不足 1MB 一律按 KB 展示，
		// 因此这里输出 1024.00 KB 而不是 1.00 MB。测试即文档。
		{"just below MB", 1048575, "1024.00 KB"},
		{"exactly MB", 1048576, "1.00 MB"},
		{"one and a half MB", 1572864, "1.50 MB"},
		{"just below GB", 1073741823, "1024.00 MB"},
		{"exactly GB", 1073741824, "1.00 GB"},
		// 与 PRD 示例中的 4.82 GB 对齐，确保示例可被复现。
		{"PRD example", 5175123456, "4.82 GB"},
		{"hundred GB", 107374182400, "100.00 GB"},
		{"negative is clamped", -1, "0 Bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatBytes(tt.in); got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0"},
		{"single digit", 7, "7"},
		{"three digits", 999, "999"},
		{"four digits", 1000, "1,000"},
		{"four digits not round", 9999, "9,999"},
		{"PRD directories", 12450, "12,450"},
		{"PRD files", 148203, "148,203"},
		{"two groups", 1000000, "1,000,000"},
		{"negative is clamped", -42, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatCount(tt.in); got != tt.want {
				t.Errorf("FormatCount(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFormatDuration 固化耗时的双档位格式：
// 亚秒用毫秒（避免毫秒级扫描被显示成 "0.00s"），秒级用两位小数（与 PRD 示例一致）。
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero", 0, "0.0ms"},
		{"sub millisecond", 400 * time.Microsecond, "0.4ms"},
		{"milliseconds", 12500 * time.Microsecond, "12.5ms"},
		{"just below one second", 999 * time.Millisecond, "999.0ms"},
		{"exactly one second switches to seconds", time.Second, "1.00s"},
		{"PRD example", 1240 * time.Millisecond, "1.24s"},
		{"half second above one second", 1500 * time.Millisecond, "1.50s"},
		{"ninety seconds", 90 * time.Second, "90.00s"},
		{"negative is clamped", -time.Second, "0.0ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDuration(tt.in); got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
