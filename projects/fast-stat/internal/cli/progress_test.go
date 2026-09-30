package cli

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/report"
	"github.com/wenhao26/go-studio/projects/fast-stat/internal/scan"
)

// errWriter 模拟不可写的输出目标。
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRates(t *testing.T) {
	tests := []struct {
		name      string
		prev      scan.ProgressInfo
		cur       scan.ProgressInfo
		dt        time.Duration
		wantFiles float64
		wantBytes float64
	}{
		{
			name:      "normal delta over one second",
			prev:      scan.ProgressInfo{FilesFound: 0, TotalSizeBytes: 0},
			cur:       scan.ProgressInfo{FilesFound: 100, TotalSizeBytes: 2048},
			dt:        time.Second,
			wantFiles: 100,
			wantBytes: 2048,
		},
		{
			name:      "half second doubles the rate",
			prev:      scan.ProgressInfo{FilesFound: 0, TotalSizeBytes: 0},
			cur:       scan.ProgressInfo{FilesFound: 50, TotalSizeBytes: 1024},
			dt:        500 * time.Millisecond,
			wantFiles: 100,
			wantBytes: 2048,
		},
		{
			name: "zero duration yields zero rate to avoid division by zero",
			prev: scan.ProgressInfo{FilesFound: 0, TotalSizeBytes: 0},
			cur:  scan.ProgressInfo{FilesFound: 10, TotalSizeBytes: 10},
			dt:   0,
		},
		{
			name: "negative duration yields zero rate",
			prev: scan.ProgressInfo{FilesFound: 0},
			cur:  scan.ProgressInfo{FilesFound: 10},
			dt:   -time.Second,
		},
		{
			name: "counter regression is clamped instead of producing negative rates",
			prev: scan.ProgressInfo{FilesFound: 100, TotalSizeBytes: 2048},
			cur:  scan.ProgressInfo{FilesFound: 10, TotalSizeBytes: 20},
			dt:   time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFiles, gotBytes := rates(tt.prev, tt.cur, tt.dt)
			if gotFiles != tt.wantFiles {
				t.Errorf("filesPerSec = %v, want %v", gotFiles, tt.wantFiles)
			}
			if gotBytes != tt.wantBytes {
				t.Errorf("bytesPerSec = %v, want %v", gotBytes, tt.wantBytes)
			}
		})
	}
}

func TestProgressRenderer_UpdateShowsMeasurableStateOnly(t *testing.T) {
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	p := newProgressRenderer(&buf, 200)
	p.startedAt = base
	p.prevAt = base

	p.update(base.Add(time.Second), scan.ProgressInfo{
		DirectoriesVisited: 10,
		FilesFound:         1200,
		TotalSizeBytes:     1048576,
		CurrentDir:         "/a/b",
	})

	got := buf.String()
	if !strings.HasPrefix(got, "\r") {
		t.Errorf("进度行应以 \\r 开头以便覆盖上一帧，got %q", got)
	}

	for _, want := range []string{"10 dirs", "1,200 files", "1.00 MB", "1,200 files/s", "1.00 MB/s", "1.00s", "/a/b"} {
		if !strings.Contains(got, want) {
			t.Errorf("进度行缺少 %q: %q", want, got)
		}
	}

	// 决策 P0-5：不展示百分比与 ETA。
	for _, forbidden := range []string{"%", "ETA", "eta"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("进度行不应出现 %q: %q", forbidden, got)
		}
	}
}

// TestProgressRenderer_ClearsShorterLine 验证缩短的帧会补空格清行，
// 否则上一帧的尾部字符会残留在终端上。
func TestProgressRenderer_ClearsShorterLine(t *testing.T) {
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	p := newProgressRenderer(&buf, 200)
	p.startedAt = base
	p.prevAt = base

	p.update(base.Add(time.Second), scan.ProgressInfo{FilesFound: 1200, TotalSizeBytes: 1048576, CurrentDir: "/a/long/path"})
	first := strings.TrimPrefix(buf.String(), "\r")
	firstWidth := report.DisplayWidth(first)

	buf.Reset()
	p.update(base.Add(2*time.Second), scan.ProgressInfo{FilesFound: 1200, TotalSizeBytes: 1048576})

	second := strings.TrimPrefix(buf.String(), "\r")
	trimmed := strings.TrimRight(second, " ")
	padding := len(second) - len(trimmed)

	if padding <= 0 {
		t.Fatalf("缩短的帧应补空格清行，实际 padding=%d", padding)
	}
	if want := firstWidth - report.DisplayWidth(trimmed); padding != want {
		t.Errorf("padding = %d, want %d", padding, want)
	}
}

func TestProgressRenderer_SanitizesCurrentDir(t *testing.T) {
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	p := newProgressRenderer(&buf, 200)
	p.startedAt = base
	p.prevAt = base

	p.update(base.Add(time.Second), scan.ProgressInfo{CurrentDir: "/tmp/\x1b[31mevil"})

	got := buf.String()
	if strings.Contains(got, "\x1b") {
		t.Errorf("进度行残留控制字符: %q", got)
	}
	if !strings.Contains(got, "evil") {
		t.Error("净化应保留可打印内容")
	}
}

func TestProgressRenderer_TruncatesToWidth(t *testing.T) {
	const width = 40
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	p := newProgressRenderer(&buf, width)
	p.startedAt = base
	p.prevAt = base

	p.update(base.Add(time.Second), scan.ProgressInfo{
		FilesFound: 5,
		CurrentDir: "/very/long/directory/" + strings.Repeat("x", 200),
	})

	line := strings.TrimPrefix(buf.String(), "\r")
	if got := report.DisplayWidth(line); got > width {
		t.Errorf("进度行宽度 = %d, 不应超过 %d", got, width)
	}
}

// TestProgressRenderer_TinyWidth 覆盖终端宽度极小/为 0 的边界：
// 不得 panic，且一旦有宽度上限就不能超限。
func TestProgressRenderer_TinyWidth(t *testing.T) {
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	for _, width := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			var buf bytes.Buffer
			p := newProgressRenderer(&buf, width)
			p.startedAt = base
			p.prevAt = base

			// 只检查 update 产生的这一帧；finish 会额外写入清行输出。
			if width >= 1 {
				line := strings.TrimPrefix(buf.String(), "\r")
				if got := report.DisplayWidth(line); got > width {
					t.Errorf("宽度 %d 时渲染出 %d 列: %q", width, got, line)
				}
			}

			// finish 在任意宽度下都不得 panic。
			buf.Reset()
			p.finish()
		})
	}
}

// TestProgressRenderer_WriteFailureIsTolerated 验证输出不可写时不会 panic，
// 也不会把进度状态误当成已渲染（否则 finish 会去清一个不存在的行）。
func TestProgressRenderer_WriteFailureIsTolerated(t *testing.T) {
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	p := newProgressRenderer(errWriter{}, 80)
	p.startedAt = base
	p.prevAt = base

	p.update(base.Add(time.Second), scan.ProgressInfo{FilesFound: 3})
	if p.lastLen != 0 {
		t.Errorf("写入失败后 lastLen 应为 0，got %d", p.lastLen)
	}

	// 未成功渲染过任何一帧时，finish 不应输出任何内容。
	p.finish()
}

func TestProgressRenderer_FinishClearsLine(t *testing.T) {
	base := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	p := newProgressRenderer(&buf, 200)
	p.startedAt = base
	p.prevAt = base

	p.update(base.Add(time.Second), scan.ProgressInfo{FilesFound: 5})
	rendered := report.DisplayWidth(strings.TrimPrefix(buf.String(), "\r"))
	buf.Reset()

	p.finish()

	got := buf.String()
	if !strings.HasPrefix(got, "\r") || !strings.HasSuffix(got, "\r") {
		t.Errorf("finish 应以 \\r 开头并回到行首: %q", got)
	}
	if want := rendered; len(strings.Trim(got, "\r")) != want {
		t.Errorf("清理空格数 = %d, want %d", len(strings.Trim(got, "\r")), want)
	}
	if p.lastLen != 0 {
		t.Errorf("finish 之后 lastLen 应归零，got %d", p.lastLen)
	}
}
