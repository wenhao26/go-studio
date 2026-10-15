//go:build windows

package fsx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// setFileAttributes 直接调用标准库 syscall 设置 Windows 文件属性，
// 用于在测试中构造真实带隐藏属性的文件（无法靠文件名表达）。
func setFileAttributes(t *testing.T, path string, attrs uint32) {
	t.Helper()
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString(%q): %v", path, err)
	}
	if err := syscall.SetFileAttributes(ptr, attrs); err != nil {
		t.Fatalf("SetFileAttributes(%q, %#x): %v", path, attrs, err)
	}
}

func TestOSFileSystem_ReadDir_ListsEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	entries, err := OSFileSystem{}.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}

func TestOSFileSystem_ReadDir_NotExistKeepsPathError(t *testing.T) {
	_, err := OSFileSystem{}.ReadDir(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist, got %v", err)
	}
}

func TestOSFileSystem_LstatAndStat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	lfi, err := OSFileSystem{}.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if lfi.IsDir() {
		t.Fatal("Lstat should not report directory for a file")
	}
	if _, err := (OSFileSystem{}).Stat(path); err != nil {
		t.Fatalf("Stat: %v", err)
	}
}

func TestOSFileSystem_MkdirAll_Idempotent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "out", "图片")
	if err := (OSFileSystem{}).MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("Stat(%q) = %v (err %v), want directory", dir, fi, err)
	}
	// 已存在时幂等返回 nil（organizer 预检依赖此语义）。
	if err := (OSFileSystem{}).MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll (already exists): %v", err)
	}
}

func TestOSFileSystem_Rename_MovesFile(t *testing.T) {
	root := t.TempDir()
	dstDir := filepath.Join(root, "out")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(root, "a.jpg")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := filepath.Join(dstDir, "a.jpg")
	if err := (OSFileSystem{}).Rename(src, dst); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("src should be gone, got %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "data" {
		t.Fatalf("dst content = %q (err %v), want data", data, err)
	}
	// 源不存在时返回 *fs.PathError（errors.Is fs.ErrNotExist）。
	if err := (OSFileSystem{}).Rename(filepath.Join(root, "missing"), dst); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Rename missing src err = %v, want fs.ErrNotExist", err)
	}
}

// TestAttributesOf_HiddenFile 覆盖 Windows 隐藏属性与创建时间的真实读取
// （Phase 0 V2/V3 结论的回归测试）。
func TestAttributesOf_HiddenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	setFileAttributes(t, path, syscall.FILE_ATTRIBUTE_HIDDEN)

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	attrs := AttributesOf(fi)
	if !attrs.Hidden {
		t.Error("Hidden = false, want true (FILE_ATTRIBUTE_HIDDEN)")
	}
	if attrs.System {
		t.Error("System = true, want false")
	}
	if attrs.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want creation time")
	}
}

func TestAttributesOf_PlainFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	attrs := AttributesOf(fi)
	if attrs.Hidden || attrs.System {
		t.Errorf("plain file attributes = %+v, want hidden=false system=false", attrs)
	}
	if attrs.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want creation time")
	}
}

func TestAttributesOf_NilFileInfoReturnsZeroValue(t *testing.T) {
	if got := AttributesOf(nil); got != (Attributes{}) {
		t.Errorf("AttributesOf(nil) = %+v, want zero value", got)
	}
}

func TestIsHiddenName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: ".gitignore", want: true},
		{name: ".config", want: true},
		{name: "readme.md", want: false},
		{name: "a.b", want: false},
		{name: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHiddenName(tt.name); got != tt.want {
				t.Errorf("IsHiddenName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
