package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runResult 汇总一次 Run 的可观测输出。
type runResult struct {
	code   int
	stdout string
	stderr string
}

func newTestApp(t *testing.T, tty bool) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	app, err := New(Config{
		Stdout:           &stdout,
		Stderr:           &stderr,
		StdoutIsTerminal: tty,
		TerminalWidth:    80,
	})
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	return app, &stdout, &stderr
}

func runApp(t *testing.T, ctx context.Context, tty bool, args ...string) runResult {
	t.Helper()

	app, stdout, stderr := newTestApp(t, tty)
	code := app.Run(ctx, args)
	return runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// fixtureTree 构造一个内容确定的目录树：
//
//	root/a.txt (5) + root/b.txt (6) + root/sub/c.txt (3)
//
// 期望统计：目录 2、文件 3、总大小 14 字节。
func fixtureTree(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}
	files := map[string]string{
		"a.txt":     "hello",
		"b.txt":     "world!",
		"sub/c.txt": "xyz",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("写入 %s 失败: %v", name, err)
		}
	}
	return root
}

// wireRunReport 是 JSON 报告中本测试关心的字段子集。
type wireRunReport struct {
	Status     string `json:"status"`
	Statistics struct {
		DirectoriesVisited int64 `json:"directories_visited"`
		TotalFiles         int64 `json:"total_files"`
		TotalSizeBytes     int64 `json:"total_size_bytes"`
		SkippedErrors      int64 `json:"skipped_errors"`
	} `json:"statistics"`
}

func TestApp_Run_TextReport(t *testing.T) {
	root := fixtureTree(t)

	got := runApp(t, context.Background(), false, "--path", root)

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d (stderr=%q)", got.code, ExitOK, got.stderr)
	}
	if got.stderr != "" {
		t.Errorf("非 TTY 模式下 stderr 应为空，实际 %q", got.stderr)
	}
	for _, want := range []string{
		"🚀 FASTSTAT SCAN REPORT",
		"Target Path",
		"Completed",
		"Directories Visited",
		"Total Files Found",
		"Total Size",
		"Skipped Errors",
		"3",
		"14 Bytes",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("文本报告缺少 %q\n%s", want, got.stdout)
		}
	}
}

func TestApp_Run_JSONReport(t *testing.T) {
	root := fixtureTree(t)

	got := runApp(t, context.Background(), false, "--path", root, "--format", "json")

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d (stderr=%q)", got.code, ExitOK, got.stderr)
	}
	if !json.Valid([]byte(got.stdout)) {
		t.Fatalf("stdout 不是合法 JSON（进度或日志污染了输出）:\n%s", got.stdout)
	}

	var rep wireRunReport
	if err := json.Unmarshal([]byte(got.stdout), &rep); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if rep.Status != "completed" {
		t.Errorf("status = %q, want completed", rep.Status)
	}
	if rep.Statistics.DirectoriesVisited != 2 {
		t.Errorf("directories_visited = %d, want 2", rep.Statistics.DirectoriesVisited)
	}
	if rep.Statistics.TotalFiles != 3 {
		t.Errorf("total_files = %d, want 3", rep.Statistics.TotalFiles)
	}
	if rep.Statistics.TotalSizeBytes != 14 {
		t.Errorf("total_size_bytes = %d, want 14", rep.Statistics.TotalSizeBytes)
	}
	if rep.Statistics.SkippedErrors != 0 {
		t.Errorf("skipped_errors = %d, want 0", rep.Statistics.SkippedErrors)
	}
}

func TestApp_Run_OutputFile(t *testing.T) {
	root := fixtureTree(t)
	out := filepath.Join(t.TempDir(), "result.json")

	got := runApp(t, context.Background(), false,
		"--path", root, "--format", "json", "--output", out)

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d (stderr=%q)", got.code, ExitOK, got.stderr)
	}
	if got.stdout != "" {
		t.Errorf("指定 --output 时 stdout 应为空，实际 %q", got.stdout)
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("读取输出文件失败: %v", err)
	}
	if !json.Valid(content) {
		t.Fatalf("输出文件不是合法 JSON:\n%s", content)
	}

	// 目录中不应残留临时文件（原子写入的清理路径）。
	entries, err := os.ReadDir(filepath.Dir(out))
	if err != nil {
		t.Fatalf("读取输出目录失败: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("输出目录条目数 = %d, want 1（不应残留临时文件）", len(entries))
	}
}

func TestApp_Run_MissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	got := runApp(t, context.Background(), false, "--path", missing)

	if got.code != ExitError {
		t.Errorf("退出码 = %d, want %d", got.code, ExitError)
	}
	if got.stdout != "" {
		t.Errorf("致命错误时不应输出报告，实际 %q", got.stdout)
	}
	if !strings.Contains(got.stderr, "faststat:") {
		t.Errorf("stderr 应包含错误说明，实际 %q", got.stderr)
	}
}

func TestApp_Run_PathIsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}

	got := runApp(t, context.Background(), false, "--path", file)

	if got.code != ExitError {
		t.Errorf("退出码 = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "not a directory") {
		t.Errorf("stderr 应说明根路径不是目录，实际 %q", got.stderr)
	}
}

func TestApp_Run_UsageError(t *testing.T) {
	got := runApp(t, context.Background(), false, "--format", "xml")

	if got.code != ExitUsage {
		t.Errorf("退出码 = %d, want %d", got.code, ExitUsage)
	}
	if !strings.Contains(got.stderr, "用法") {
		t.Errorf("stderr 应包含用法说明，实际 %q", got.stderr)
	}
}

func TestApp_Run_Help(t *testing.T) {
	got := runApp(t, context.Background(), false, "--help")

	if got.code != ExitOK {
		t.Errorf("退出码 = %d, want %d", got.code, ExitOK)
	}
	if !strings.Contains(got.stdout, "用法") {
		t.Errorf("stdout 应包含用法说明，实际 %q", got.stdout)
	}
}

func TestApp_Run_Version(t *testing.T) {
	got := runApp(t, context.Background(), false, "--version")

	if got.code != ExitOK {
		t.Errorf("退出码 = %d, want %d", got.code, ExitOK)
	}
	if !strings.Contains(got.stdout, programName) {
		t.Errorf("stdout 应包含程序名，实际 %q", got.stdout)
	}
}

// TestApp_Run_Canceled 验证取消语义：输出部分结果、状态为 canceled、退出码非 0。
func TestApp_Run_Canceled(t *testing.T) {
	root := fixtureTree(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := runApp(t, ctx, false, "--path", root, "--format", "json")

	if got.code != ExitError {
		t.Errorf("退出码 = %d, want %d", got.code, ExitError)
	}
	if !json.Valid([]byte(got.stdout)) {
		t.Fatalf("取消后仍应输出合法 JSON:\n%s", got.stdout)
	}

	var rep wireRunReport
	if err := json.Unmarshal([]byte(got.stdout), &rep); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if rep.Status != "canceled" {
		t.Errorf("status = %q, want canceled", rep.Status)
	}
}

func TestApp_Run_VerboseText(t *testing.T) {
	root := fixtureTree(t)

	got := runApp(t, context.Background(), false, "--path", root, "--verbose")

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d", got.code, ExitOK)
	}
	if !strings.Contains(got.stdout, "Workers") {
		t.Errorf("verbose 报告应包含生效参数:\n%s", got.stdout)
	}
}

// TestApp_Run_NonTTYSuppressesProgress 验证重定向/CI 场景下进度行不会污染输出。
func TestApp_Run_NonTTYSuppressesProgress(t *testing.T) {
	root := fixtureTree(t)

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			got := runApp(t, context.Background(), false, "--path", root, "--format", format)

			if strings.Contains(got.stderr, "\r") {
				t.Errorf("非 TTY 模式不应渲染进度: %q", got.stderr)
			}
			if strings.Contains(got.stdout, "\r") {
				t.Errorf("stdout 不应包含进度控制字符: %q", got.stdout)
			}
		})
	}
}

// TestApp_Run_TTYWiring 只验证 TTY 模式的接线不会破坏报告输出。
//
// 小目录的扫描通常在首个 150ms tick 之前就结束了，因此不能断言进度一定被渲染
// ——那样会让测试依赖机器速度。进度渲染本身的正确性由 progress_test.go 覆盖。
func TestApp_Run_TTYWiring(t *testing.T) {
	root := fixtureTree(t)

	got := runApp(t, context.Background(), true, "--path", root)

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d", got.code, ExitOK)
	}
	if !strings.Contains(got.stdout, "FASTSTAT SCAN REPORT") {
		t.Errorf("TTY 模式下报告应完整输出:\n%s", got.stdout)
	}
}

// TestApp_Run_ThreadsIsReported 验证 --threads 会真正下传，
// 且诊断区展示的是实际生效的并发度。
func TestApp_Run_ThreadsIsReported(t *testing.T) {
	root := fixtureTree(t)

	got := runApp(t, context.Background(), false, "--path", root, "--threads", "3", "--verbose")

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d (stderr=%q)", got.code, ExitOK, got.stderr)
	}
	if !strings.Contains(got.stdout, "Workers") || !strings.Contains(got.stdout, "3") {
		t.Errorf("诊断区应展示实际并发度:\n%s", got.stdout)
	}
}

// TestApp_Run_NoEmoji 验证 --no-emoji 的输出不含 emoji。
func TestApp_Run_NoEmoji(t *testing.T) {
	root := fixtureTree(t)

	got := runApp(t, context.Background(), false, "--path", root, "--no-emoji")

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d", got.code, ExitOK)
	}
	for _, emoji := range []string{"🚀", "📂", "📄", "📦", "🚫"} {
		if strings.Contains(got.stdout, emoji) {
			t.Errorf("--no-emoji 模式下仍出现 %q:\n%s", emoji, got.stdout)
		}
	}
	if !strings.Contains(got.stdout, "FASTSTAT SCAN REPORT") {
		t.Error("纯文本标题缺失")
	}
}

// TestApp_Run_Plain 验证 --plain 的输出不含边框，但指标完整。
func TestApp_Run_Plain(t *testing.T) {
	root := fixtureTree(t)

	got := runApp(t, context.Background(), false, "--path", root, "--plain")

	if got.code != ExitOK {
		t.Fatalf("退出码 = %d, want %d", got.code, ExitOK)
	}
	for _, border := range []string{"╔", "║", "═", "╚"} {
		if strings.Contains(got.stdout, border) {
			t.Errorf("--plain 模式下仍出现边框字符 %q:\n%s", border, got.stdout)
		}
	}
	for _, want := range []string{"Target Path", "Directories Visited", "14 Bytes"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("--plain 报告缺少 %q:\n%s", want, got.stdout)
		}
	}
}

func TestNew_RejectsNilWriters(t *testing.T) {
	var buf bytes.Buffer

	if _, err := New(Config{Stderr: &buf}); err == nil {
		t.Error("Stdout 为 nil 时应返回错误")
	}
	if _, err := New(Config{Stdout: &buf}); err == nil {
		t.Error("Stderr 为 nil 时应返回错误")
	}
}

// TestApp_Run_NilContextIsTolerated 验证调用方传 nil ctx 时不会 panic。
func TestApp_Run_NilContextIsTolerated(t *testing.T) {
	root := fixtureTree(t)

	app, stdout, _ := newTestApp(t, false)
	code := app.Run(nil, []string{"--path", root}) //nolint:staticcheck // 显式验证 nil ctx 的防御分支

	if code != ExitOK {
		t.Errorf("退出码 = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stdout.String(), "FASTSTAT SCAN REPORT") {
		t.Error("nil ctx 应退化为 Background")
	}
}
