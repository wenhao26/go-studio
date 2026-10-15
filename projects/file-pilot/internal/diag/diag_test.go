package diag

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestOpen_WritesJSONWithTimeAndCaller(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	log.Info("hello", zap.String("k", "v"))
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, DefaultFileName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	line := strings.TrimSpace(string(raw))
	for _, want := range []string{`"msg":"hello"`, `"k":"v"`, `"level":"info"`, `"caller"`} {
		if !strings.Contains(line, want) {
			t.Errorf("log line missing %s: %s", want, line)
		}
	}
	// ISO8601 时间戳（NFR-3：时间必须可被工具解析），形如 2026-10-11T08:00:00.000Z。
	if !regexp.MustCompile(`\d{4}-\d{2}-\d{2}T`).MatchString(line) {
		t.Errorf("timestamp is not ISO8601: %s", line)
	}
}

// TestRotateWriter_RotatesAndKeepsSingleOld 覆盖 NFR-3 的滚动语义：
// 超限改名为 .old、仅保留 1 个（新一次滚动覆盖旧 .old）、当前文件重新开始。
func TestRotateWriter_RotatesAndKeepsSingleOld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	const limit = 64
	w, err := newRotateWriter(path, limit)
	if err != nil {
		t.Fatalf("newRotateWriter: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	lineA := strings.Repeat("a", 40) + "\n" // 41 字节
	lineB := strings.Repeat("b", 40) + "\n"
	lineC := strings.Repeat("c", 40) + "\n"

	if _, err := w.Write([]byte(lineA)); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if _, err := w.Write([]byte(lineB)); err != nil {
		t.Fatalf("write b: %v", err)
	}
	assertFileContent(t, path, lineB)
	assertFileContent(t, path+".old", lineA)

	// 再次滚动：.old 被覆盖，磁盘上始终只有 1 个 .old。
	if _, err := w.Write([]byte(lineC)); err != nil {
		t.Fatalf("write c: %v", err)
	}
	assertFileContent(t, path, lineC)
	assertFileContent(t, path+".old", lineB)

	if entries, err := os.ReadDir(filepath.Dir(path)); err != nil {
		t.Fatalf("read dir: %v", err)
	} else if len(entries) != 2 {
		t.Fatalf("log dir has %d entries, want 2（当前 + .old）", len(entries))
	}
}

// TestRotateWriter_SurvivesReopen：重启后从既有文件长度继续计数，
// 不会因新建进程而失去滚动上限。
func TestRotateWriter_SurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.log")
	const limit = 64

	w1, err := newRotateWriter(path, limit)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	if _, err := w1.Write([]byte(strings.Repeat("a", 40) + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	w2, err := newRotateWriter(path, limit)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = w2.Close() })
	if _, err := w2.Write([]byte(strings.Repeat("b", 40) + "\n")); err != nil {
		t.Fatalf("write after reopen: %v", err)
	}
	assertFileContent(t, path+".old", strings.Repeat("a", 40)+"\n")
	assertFileContent(t, path, strings.Repeat("b", 40)+"\n")
}

func TestNewRotateWriter_Validation(t *testing.T) {
	if _, err := newRotateWriter("", 10); err == nil {
		t.Error("want error for empty path")
	}
	if _, err := newRotateWriter(filepath.Join(t.TempDir(), "x.log"), 0); err == nil {
		t.Error("want error for non-positive limit")
	}
}

func TestRotateWriter_WriteAfterClose(t *testing.T) {
	w, err := newRotateWriter(filepath.Join(t.TempDir(), "x.log"), 128)
	if err != nil {
		t.Fatalf("newRotateWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("want error when writing after close")
	}
	// 幂等：二次 Close 不报错。
	if err := w.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestRotateWriter_SyncWithoutFile(t *testing.T) {
	w, err := newRotateWriter(filepath.Join(t.TempDir(), "x.log"), 128)
	if err != nil {
		t.Fatalf("newRotateWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("sync after close should be a no-op: %v", err)
	}
}

func TestOpen_EmptyDir(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("want error for empty dir")
	}
}

// TestOpen_MkdirAllFails：目录路径被文件占用时必须报错，不能静默降级。
func TestOpen_MkdirAllFails(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if _, err := Open(blocker); err == nil {
		t.Fatal("want error when log dir path is a file")
	}
}

// TestOpen_LogPathIsDirectory：日志文件路径被目录占用时必须报错。
func TestOpen_LogPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, DefaultFileName), 0o755); err != nil {
		t.Fatalf("mkdir blocker: %v", err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("want error when log path is a directory")
	}
}

func TestNewStderr_CloseIdempotent(t *testing.T) {
	log := NewStderr()
	if err := log.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestDefaultDir(t *testing.T) {
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if !strings.Contains(dir, "FilePilot") || !strings.HasSuffix(dir, DefaultDirName) {
		t.Fatalf("dir %q should be <config>/FilePilot/logs", dir)
	}
}

func TestLogClose_NilSafe(t *testing.T) {
	var log *Log
	if err := log.Close(); err != nil {
		t.Fatalf("nil log close: %v", err)
	}
}

// assertFileContent 断言文件内容恰好等于 want。
func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(raw) != want {
		t.Fatalf("%s content = %q, want %q", path, raw, want)
	}
}
