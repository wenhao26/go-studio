package units

import (
	"math"
	"testing"
)

// TestFormatBytes 锁定大小格式化口径（requirements §2.5 / fast-stat P0-2/P0-4）。
// 档位边界用例「测试即文档」：不足 1MB 一律按 KB 展示。
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
		{"just below MB", 1048575, "1024.00 KB"},
		{"exactly MB", 1048576, "1.00 MB"},
		{"just below GB", 1073741823, "1024.00 MB"},
		{"exactly GB", 1073741824, "1.00 GB"},
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

func TestFormatBytes_Extremes(t *testing.T) {
	got := FormatBytes(math.MaxInt64)
	if got[:1] == "-" {
		t.Errorf("FormatBytes(MaxInt64) = %q, 不应为负", got)
	}
	if want := " GB"; got[len(got)-len(want):] != want {
		t.Errorf("FormatBytes(MaxInt64) = %q, 应落在 GB 档", got)
	}
}

func TestFormatCount_ThousandsSeparator(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{9999, "9,999"},
		{148203, "148,203"},
		{1000000, "1,000,000"},
	}
	for _, tt := range tests {
		if got := formatCount(tt.in); got != tt.want {
			t.Errorf("formatCount(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
