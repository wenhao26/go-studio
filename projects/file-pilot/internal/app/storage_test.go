package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/diag"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// newStorageApp 构造带真实临时库与日志文件的 App，并把「打开目录」替换为
// 记录器：单测不得真的弹出系统窗口（与 publish 注入同一模式）。
func newStorageApp(t *testing.T) (a *App, dbPath, logPath string, opened *[]string) {
	t.Helper()

	dbPath = filepath.Join(t.TempDir(), "file-pilot.db")
	store, err := storage.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	logDir := t.TempDir()
	logger, err := diag.Open(logDir)
	if err != nil {
		_ = store.Close()
		t.Fatalf("open diag: %v", err)
	}

	a = New(store, logger)
	opened = &[]string{}
	a.openFolder = func(dir string) error {
		*opened = append(*opened, dir)
		return nil
	}
	t.Cleanup(a.Close)
	return a, dbPath, filepath.Join(logDir, diag.DefaultFileName), opened
}

// TestGetStorageInfo_ReportsActualPaths 验证展示值来自真正打开的文件，
// 而不是根据环境变量重新推断的路径。
func TestGetStorageInfo_ReportsActualPaths(t *testing.T) {
	a, dbPath, logPath, _ := newStorageApp(t)

	info, err := a.GetStorageInfo()
	if err != nil {
		t.Fatalf("GetStorageInfo: %v", err)
	}
	if info.DataPath != dbPath {
		t.Errorf("data_path = %q, want %q", info.DataPath, dbPath)
	}
	if info.LogPath != logPath {
		t.Errorf("log_path = %q, want %q", info.LogPath, logPath)
	}
}

// TestOpenStorageFolder_OpensContainingDir 验证点击后打开的是文件所在目录。
func TestOpenStorageFolder_OpensContainingDir(t *testing.T) {
	a, dbPath, logPath, opened := newStorageApp(t)

	if err := a.OpenStorageFolder(storageKindData); err != nil {
		t.Fatalf("open data folder: %v", err)
	}
	if err := a.OpenStorageFolder(storageKindLog); err != nil {
		t.Fatalf("open log folder: %v", err)
	}

	want := []string{filepath.Dir(dbPath), filepath.Dir(logPath)}
	if len(*opened) != len(want) {
		t.Fatalf("opened = %v, want %v", *opened, want)
	}
	for i := range want {
		if (*opened)[i] != want[i] {
			t.Errorf("opened[%d] = %q, want %q", i, (*opened)[i], want[i])
		}
	}
}

// TestOpenStorageFolder_InvalidKind 验证非法 kind 直接拒绝，不触发外部进程。
func TestOpenStorageFolder_InvalidKind(t *testing.T) {
	a, _, _, opened := newStorageApp(t)

	for _, kind := range []string{"", "bogus", "../data", "DATA"} {
		if err := a.OpenStorageFolder(kind); err == nil {
			t.Errorf("kind %q: want error, got nil", kind)
		}
	}
	if len(*opened) != 0 {
		t.Errorf("opened = %v, want none", *opened)
	}
}

// TestOpenStorageFolder_MissingDir 验证目录已不存在时返回裸哨兵且不打开。
func TestOpenStorageFolder_MissingDir(t *testing.T) {
	a, _, logPath, opened := newStorageApp(t)
	logDir := filepath.Dir(logPath)

	// Windows 下必须先释放日志文件句柄，才能删除其所在目录。
	if err := a.diag.Close(); err != nil {
		t.Fatalf("close diag: %v", err)
	}
	if err := os.RemoveAll(logDir); err != nil {
		t.Fatalf("remove log dir: %v", err)
	}

	err := a.OpenStorageFolder(storageKindLog)
	if !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("err = %v, want ErrStorageUnavailable", err)
	}
	if len(*opened) != 0 {
		t.Errorf("opened = %v, want none", *opened)
	}
}

// TestStorageInfo_DegradedLog 验证日志降级为 stderr 时不伪造路径，且打开入口
// 返回不可用（前端据此禁用按钮）。
func TestStorageInfo_DegradedLog(t *testing.T) {
	a := New(nil, nil) // 无数据库 + 日志降级 stderr
	t.Cleanup(a.Close)

	info, err := a.GetStorageInfo()
	if err != nil {
		t.Fatalf("GetStorageInfo: %v", err)
	}
	if info.DataPath != "" || info.LogPath != "" {
		t.Errorf("unexpected paths: %+v", info)
	}

	for _, kind := range []string{storageKindData, storageKindLog} {
		if err := a.OpenStorageFolder(kind); !errors.Is(err, ErrStorageUnavailable) {
			t.Errorf("kind %q: err = %v, want ErrStorageUnavailable", kind, err)
		}
	}
}
