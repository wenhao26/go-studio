package fsx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileSystem_ReadDir(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.txt", "a.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("准备测试文件失败: %v", err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("准备测试目录失败: %v", err)
	}

	entries, err := OSFileSystem{}.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir 返回错误: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("条目数 = %d, want 3", len(entries))
	}

	// 不依赖顺序：实现刻意跳过了 os.ReadDir 的排序以节省开销。
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		seen[e.Name()] = true
	}
	for _, want := range []string{"a.txt", "b.txt", "sub"} {
		if !seen[want] {
			t.Errorf("缺少条目 %q", want)
		}
	}
}

func TestOSFileSystem_ReadDirErrors(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		_, err := OSFileSystem{}.ReadDir(filepath.Join(t.TempDir(), "missing"))
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("错误应可判定为 fs.ErrNotExist，got %v", err)
		}
	})

	t.Run("not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "afile")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("准备测试文件失败: %v", err)
		}
		if _, err := (OSFileSystem{}).ReadDir(file); err == nil {
			t.Error("对普通文件调用 ReadDir 应返回错误")
		}
	})
}

func TestOSFileSystem_Lstat(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("准备测试文件失败: %v", err)
	}

	info, err := OSFileSystem{}.Lstat(file)
	if err != nil {
		t.Fatalf("Lstat 返回错误: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("Size = %d, want 5", info.Size())
	}
	if info.IsDir() {
		t.Error("普通文件不应被判定为目录")
	}

	if _, err := (OSFileSystem{}).Lstat(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("不存在的路径应返回 fs.ErrNotExist，got %v", err)
	}
}

// TestOSFileSystem_LstatDoesNotFollowSymlink 验证「不跟随符号链接」的实现约定。
//
// Windows 上创建符号链接需要特权，失败时跳过该断言而不是让测试变成
// 「取决于运行账户权限」的不稳定用例。
func TestOSFileSystem_LstatDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
		t.Fatalf("准备目标文件失败: %v", err)
	}

	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("当前环境不支持创建符号链接，跳过: %v", err)
	}

	info, err := OSFileSystem{}.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat 返回错误: %v", err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat 应返回符号链接自身的信息，got mode=%v", info.Mode())
	}
}

// TestOSFileSystem_StatFollowsLinks 验证 Stat 会跟随链接（用于识别目录联接）。
func TestOSFileSystem_StatFollowsLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("准备目标目录失败: %v", err)
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("当前环境不支持创建符号链接，跳过: %v", err)
	}

	// Lstat 看到的是链接本身，Stat 看到的是目标。
	lstat, err := (OSFileSystem{}).Lstat(link)
	if err != nil {
		t.Fatalf("Lstat 返回错误: %v", err)
	}
	if lstat.IsDir() {
		t.Error("Lstat 不应跟随符号链接")
	}

	stat, err := (OSFileSystem{}).Stat(link)
	if err != nil {
		t.Fatalf("Stat 返回错误: %v", err)
	}
	if !stat.IsDir() {
		t.Error("Stat 应跟随符号链接并返回目标目录信息")
	}

	if _, err := (OSFileSystem{}).Stat(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat 不存在的路径应返回 fs.ErrNotExist，got %v", err)
	}
}

func TestWriteFileAtomic_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	want := []byte(`{"ok":true}` + "\n")

	if err := WriteFileAtomic(path, want, DefaultFilePerm); err != nil {
		t.Fatalf("WriteFileAtomic 返回错误: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取结果失败: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("内容 = %q, want %q", got, want)
	}
	assertNoTempFiles(t, dir)
}

func TestWriteFileAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatalf("准备旧文件失败: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("new content"), DefaultFilePerm); err != nil {
		t.Fatalf("WriteFileAtomic 返回错误: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取结果失败: %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("内容 = %q, want new content", got)
	}
	assertNoTempFiles(t, dir)
}

func TestWriteFileAtomic_MissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "out.json")
	if err := WriteFileAtomic(path, []byte("x"), DefaultFilePerm); err == nil {
		t.Error("目标目录不存在时应返回错误")
	}
}

// TestWriteFileAtomic_TargetIsDirectory 覆盖 rename 失败路径：
// 若目标路径已存在且是目录，rename 会失败，此时必须清理临时文件。
func TestWriteFileAtomic_TargetIsDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "adir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("准备目录失败: %v", err)
	}

	if err := WriteFileAtomic(target, []byte("x"), DefaultFilePerm); err == nil {
		t.Error("目标为目录时应返回错误")
	}
	assertNoTempFiles(t, dir)
}

// assertNoTempFiles 断言目录中没有残留的临时文件。
//
// 半截临时文件既不美观也会让人误以为有并发写入，因此失败路径必须清理。
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	for _, e := range entries {
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			t.Errorf("发现残留临时文件 %q", e.Name())
		}
	}
}
