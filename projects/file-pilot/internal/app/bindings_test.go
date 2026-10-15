package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
)

// TestNormalizeRoot 校验前端传入的不可信路径（SEC-5）。
func TestNormalizeRoot(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	missing := filepath.Join(dir, "missing")

	tests := []struct {
		name    string
		input   string
		wantErr error
		abs     string // 非空时期望返回值
	}{
		{name: "empty", input: "", wantErr: ErrDirNotFound},
		{name: "blank", input: "   ", wantErr: ErrDirNotFound},
		{name: "missing", input: missing, wantErr: ErrDirNotFound},
		{name: "file not dir", input: file, wantErr: ErrDirNotDirectory},
		{name: "valid dir", input: dir, abs: dir},
		{name: "trailing separator", input: dir + string(filepath.Separator), abs: dir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRoot(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeRoot: %v", err)
			}
			if got != tt.abs {
				t.Fatalf("root = %q, want %q", got, tt.abs)
			}
		})
	}
}

// TestStartScan_ValidationErrorsAreBareSentinels：StartScan 的校验失败必须
// 按契约返回裸哨兵（message 即前端匹配键），且不产生任何数据库行。
func TestStartScan_ValidationErrorsAreBareSentinels(t *testing.T) {
	a, _ := newTestApp(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name    string
		root    string
		cfg     scanner.Config
		wantMsg string
	}{
		{name: "dir not found", root: filepath.Join(dir, "missing"), cfg: scanner.Config{Workers: 4},
			wantMsg: "directory not found"},
		{name: "not a directory", root: file, cfg: scanner.Config{Workers: 4},
			wantMsg: "path is not a directory"},
		{name: "invalid workers", root: dir, cfg: scanner.Config{Workers: 0},
			wantMsg: "workers must be between 1 and 64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := a.StartScan(tt.root, tt.cfg)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if id != 0 {
				t.Fatalf("task id = %d, want 0 on validation failure", id)
			}
			if err.Error() != tt.wantMsg {
				t.Fatalf("message = %q, want %q（裸哨兵）", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestTotalPage(t *testing.T) {
	tests := []struct {
		total    int64
		pageSize int
		want     int64
	}{
		{total: 0, pageSize: 100, want: 0},
		{total: 1, pageSize: 100, want: 1},
		{total: 100, pageSize: 100, want: 1},
		{total: 101, pageSize: 100, want: 2},
		{total: 250, pageSize: 100, want: 3},
		{total: 25, pageSize: 10, want: 3},
	}
	for _, tt := range tests {
		if got := totalPage(tt.total, tt.pageSize); got != tt.want {
			t.Errorf("totalPage(%d, %d) = %d, want %d", tt.total, tt.pageSize, got, tt.want)
		}
	}
}

// TestCancelTask_UnknownTask：未知任务的哨兵原样上抛（无映射 → 前端通用文案）。
func TestCancelTask_UnknownTask(t *testing.T) {
	a, _ := newTestApp(t)
	if err := a.CancelTask(987654); err == nil {
		t.Fatal("want error for unknown task")
	}
}
