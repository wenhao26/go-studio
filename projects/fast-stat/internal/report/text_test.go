package report

import (
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/scan"
)

// sampleInput 返回一份与 PRD §三 示例数值一致的报告输入。
func sampleInput() Input {
	return Input{
		TargetPath: "/data/projects",
		Status:     StatusCompleted,
		Snapshot: scan.Snapshot{
			Root:               "/data/projects",
			DirectoriesVisited: 12450,
			FilesFound:         148203,
			TotalSizeBytes:     5175123456,
			SkippedErrors:      3,
			Duration:           1240 * time.Millisecond,
		},
		ToolVersion: "1.0.0",
		Workers:     8,
	}
}

// textLines 把渲染结果拆成非空行。
func textLines(out string) []string {
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

// TestText_AllLinesHaveEqualDisplayWidth 是排版的核心断言。
//
// 若改用 len() 或码点数计算宽度，含中文与 emoji 的行宽度会与边框行不一致，
// 该测试即会失败。这正是 PRD 示例能对齐、而朴素实现必然错位的原因。
func TestText_AllLinesHaveEqualDisplayWidth(t *testing.T) {
	want := contentWidth + 2 // 左右各一条竖线

	for _, status := range []Status{StatusCompleted, StatusCanceled} {
		in := sampleInput()
		in.Status = status

		for _, verbose := range []bool{false, true} {
			in.Verbose = verbose
			out := Text(in)

			for i, line := range textLines(out) {
				if got := DisplayWidth(line); got != want {
					t.Errorf("status=%v verbose=%v 第 %d 行显示宽度 = %d, 期望 %d: %q",
						status, verbose, i, got, want, line)
				}
			}
		}
	}
}

// TestText_BorderCharactersArePaired 断言边框首尾字符是合法组合。
//
// PRD §三 示例的分隔行使用了 "╠……╝"（左三叉 + 右下角），这属于非法组合；
// 本项目按决策修正为 "╠……╣"。该断言防止缺陷被重新引入。
func TestText_BorderCharactersArePaired(t *testing.T) {
	validPairs := map[string]string{
		"╔": "╗",
		"║": "║",
		"╠": "╣",
		"╚": "╝",
	}

	for i, line := range textLines(Text(sampleInput())) {
		runes := []rune(line)
		if len(runes) == 0 {
			t.Fatalf("第 %d 行为空", i)
		}

		head, tail := string(runes[0]), string(runes[len(runes)-1])
		want, ok := validPairs[head]
		if !ok {
			t.Errorf("第 %d 行使用了未知的左边框字符 %q: %q", i, head, line)
			continue
		}
		if tail != want {
			t.Errorf("第 %d 行边框不匹配: 左 %q 右 %q，期望右 %q", i, head, tail, want)
		}
	}
}

func TestText_ContainsMandatoryMetrics(t *testing.T) {
	out := Text(sampleInput())

	// 四项指标：三项来自 PRD §二.4，第四项由决策 P0-3 定为统计指标。
	mandatory := []string{
		"扫描次数 (Directories Visited)",
		"文件个数 (Total Files Found)",
		"路径总大小 (Total Size)",
		"跳过错误数 (Skipped Errors)",
	}
	for _, want := range mandatory {
		if !strings.Contains(out, want) {
			t.Errorf("报告缺少指标 %q", want)
		}
	}

	values := []string{"12,450", "148,203", "4.82 GB", "🚀 FASTSTAT SCAN REPORT"}
	for _, want := range values {
		if !strings.Contains(out, want) {
			t.Errorf("报告缺少数值 %q", want)
		}
	}

	if !strings.Contains(out, "/data/projects") {
		t.Error("报告缺少目标路径")
	}
	if !strings.Contains(out, "Completed (Duration: 1.24s)") {
		t.Error("报告缺少状态与耗时")
	}
}

// TestText_ContentRowsAreIndented 固化 PRD 示例的排版细节：
// 内容行是 "║ 内容"，而非紧贴边框的 "║内容"。
func TestText_ContentRowsAreIndented(t *testing.T) {
	for i, line := range textLines(Text(sampleInput())) {
		switch []rune(line)[0] {
		case '╔', '╠', '╚':
			continue // 边框行无需缩进
		}
		if !strings.HasPrefix(line, borderVertical+" ") {
			t.Errorf("第 %d 行内容未按 PRD 缩进: %q", i, line)
		}
	}
}

func TestText_TitleIsCentered(t *testing.T) {
	lines := textLines(Text(sampleInput()))

	if len(lines) < 2 {
		t.Fatalf("报告行数过少: %d", len(lines))
	}

	title := strings.Trim(lines[1], "║")
	trimmed := strings.TrimSpace(title)
	if trimmed != reportTitle {
		t.Errorf("标题内容 = %q, 期望 %q", trimmed, reportTitle)
	}

	// 居中要求左右留白宽度差不超过 1 列（奇数差值时右侧多一列）。
	// 空白字符是 ASCII，因此按字节计数与按列计数一致。
	left := len(title) - len(strings.TrimLeft(title, " "))
	right := len(title) - len(strings.TrimRight(title, " "))
	if left > right+1 || right > left+1 {
		t.Errorf("标题未居中：左留白 %d，右留白 %d", left, right)
	}
}

func TestText_CanceledStatus(t *testing.T) {
	in := sampleInput()
	in.Status = StatusCanceled

	out := Text(in)
	if !strings.Contains(out, "Canceled") {
		t.Error("取消状态未体现在报告中")
	}
	if strings.Contains(out, "Completed") {
		t.Error("取消状态不应显示为 Completed")
	}
}

// TestText_TerminalInjectionIsNeutralized 断言路径中的控制字符不会进入输出。
func TestText_TerminalInjectionIsNeutralized(t *testing.T) {
	in := sampleInput()
	in.TargetPath = "/tmp/\x1b]0;pwned\x07\x1b[31mred"

	out := Text(in)
	for _, forbidden := range []string{"\x1b", "\x07", "\r"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("输出中残留控制字符 %q", forbidden)
		}
	}
	if !strings.Contains(out, "pwned") {
		t.Error("净化应保留可打印内容")
	}
}

// TestText_LongPathIsTruncated 断言超长路径不会撑破边框。
func TestText_LongPathIsTruncated(t *testing.T) {
	in := sampleInput()
	in.TargetPath = "/very/long/" + strings.Repeat("长", 100)

	out := Text(in)
	for i, line := range textLines(out) {
		if got := DisplayWidth(line); got != contentWidth+2 {
			t.Errorf("第 %d 行显示宽度 = %d, 期望 %d", i, got, contentWidth+2)
		}
	}
	if strings.Contains(out, strings.Repeat("长", 100)) {
		t.Error("超长路径未被截断")
	}
	if !strings.Contains(out, ellipsis) {
		t.Error("截断应包含省略号")
	}
}

func TestText_VerboseRows(t *testing.T) {
	plain := Text(sampleInput())
	if strings.Contains(plain, "Workers") {
		t.Error("非 verbose 模式不应输出诊断区")
	}
	if strings.Contains(plain, "Skipped Error Details") {
		t.Error("非 verbose 模式不应输出错误明细")
	}

	in := sampleInput()
	in.Verbose = true
	in.Snapshot.ErrorDetails = []scan.ScanError{
		{Path: "/data/secret", Err: fs.ErrPermission},
	}

	out := Text(in)
	for _, want := range []string{"Workers", "8", "Skipped Error Details: 1 of 3 shown", "/data/secret"} {
		if !strings.Contains(out, want) {
			t.Errorf("verbose 报告缺少 %q", want)
		}
	}
}

// TestText_VerboseWithoutErrors 覆盖「开启 verbose 但没有错误」的路径。
func TestText_VerboseWithoutErrors(t *testing.T) {
	in := sampleInput()
	in.Verbose = true
	in.Snapshot.SkippedErrors = 0
	in.Snapshot.ErrorDetails = nil

	out := Text(in)
	if !strings.Contains(out, "Workers") {
		t.Error("verbose 模式应输出生效参数")
	}
	if strings.Contains(out, "Skipped Error Details") {
		t.Error("无错误时不应输出明细标题")
	}
}

// TestText_ASCIISymbols 验证 --no-emoji 的降级路径：
// 输出中不得残留任何 emoji，且对齐依然成立。
func TestText_ASCIISymbols(t *testing.T) {
	in := sampleInput()
	in.ASCIISymbols = true

	out := Text(in)

	for _, emoji := range []string{"🚀", "📂", "📄", "📦", "🚫"} {
		if strings.Contains(out, emoji) {
			t.Errorf("--no-emoji 模式下不应出现 %q", emoji)
		}
	}
	if !strings.Contains(out, asciiTitle) {
		t.Errorf("应使用纯文本标题 %q", asciiTitle)
	}
	for _, want := range []string{
		"扫描次数 (Directories Visited)",
		"文件个数 (Total Files Found)",
		"路径总大小 (Total Size)",
		"跳过错误数 (Skipped Errors)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少指标标签 %q", want)
		}
	}

	for i, line := range textLines(out) {
		if got := DisplayWidth(line); got != contentWidth+2 {
			t.Errorf("第 %d 行显示宽度 = %d, 期望 %d: %q", i, got, contentWidth+2, line)
		}
	}
}

// TestText_PlainLayout 验证 --plain 的降级路径：
// 不出现任何边框字符，但四项指标与元信息仍然完整。
func TestText_PlainLayout(t *testing.T) {
	in := sampleInput()
	in.PlainLayout = true

	out := Text(in)

	for _, border := range []string{"╔", "╗", "╠", "╣", "╚", "╝", "║", "═"} {
		if strings.Contains(out, border) {
			t.Errorf("--plain 模式下不应出现边框字符 %q", border)
		}
	}
	for _, want := range []string{
		"🚀 FASTSTAT SCAN REPORT",
		"Target Path",
		"Status",
		"扫描次数 (Directories Visited)",
		"12,450",
		"148,203",
		"4.82 GB",
		"跳过错误数 (Skipped Errors)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--plain 报告缺少 %q\n%s", want, out)
		}
	}
}

// TestText_PlainLayoutWithVerbose 覆盖 --plain 与 --verbose 的组合。
func TestText_PlainLayoutWithVerbose(t *testing.T) {
	in := sampleInput()
	in.PlainLayout = true
	in.Verbose = true
	in.ASCIISymbols = true
	in.Snapshot.ErrorDetails = []scan.ScanError{{Path: "/data/secret", Err: fs.ErrPermission}}

	out := Text(in)

	for _, want := range []string{"FASTSTAT SCAN REPORT", "Workers", "Skipped Error Details", "/data/secret"} {
		if !strings.Contains(out, want) {
			t.Errorf("--plain --verbose 报告缺少 %q\n%s", want, out)
		}
	}
}
