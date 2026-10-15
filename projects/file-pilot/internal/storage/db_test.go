package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openTestStore 打开一个迁移完成的临时库。
func openTestStore(t *testing.T) *Store {
	t.Helper()
	store := openRawTestStore(t)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

// openRawTestStore 打开一个未迁移的临时库（迁移相关测试用）。
func openRawTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "file-pilot.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

// TestOpen_AppliesAndVerifiesPragmas 复核运行参数确实生效（裁决 D-A2）。
func TestOpen_AppliesAndVerifiesPragmas(t *testing.T) {
	store := openRawTestStore(t)

	var journal string
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if !strings.EqualFold(journal, "wal") {
		t.Errorf("journal_mode = %q, want wal", journal)
	}

	var timeout int64
	if err := store.db.QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if timeout != busyTimeoutMS {
		t.Errorf("busy_timeout = %d, want %d", timeout, busyTimeoutMS)
	}

	var fk int64
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// 单连接串行：并发写不再依赖 busy retry，行为可预测（裁决 D-A2）。
	if got := store.db.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestOpen_EmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("want error for empty path, got nil")
	}
}

func TestOpen_CreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "file-pilot.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestDefaultPath_LocatesFilePilotDir(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if !strings.Contains(path, "FilePilot") || !strings.HasSuffix(path, "file-pilot.db") {
		t.Fatalf("path %q should end with FilePilot/file-pilot.db", path)
	}
}

func TestClose_NilSafeAndIdempotent(t *testing.T) {
	var nilStore *Store
	if err := nilStore.Close(); err != nil {
		t.Fatalf("nil store close: %v", err)
	}

	store := openRawTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// database/sql 的 Close 可重复调用；本包语义要求幂等。
	if err := store.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

// TestOpen_CorruptFileFailsPragma 复核「参数设置失败即报错」的另一半：
// 文件不是合法 SQLite 库时 journal_mode 复核失败，Open 必须返回错误
// 而不是带着一个假装配置好的句柄继续运行（裁决 D-A2）。
func TestOpen_CorruptFileFailsPragma(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file-pilot.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database at all"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	if _, err := Open(context.Background(), path); err == nil {
		t.Fatal("want error for corrupt database file, got nil")
	}
}

// TestOpen_ParentPathIsFile 复核目录创建失败路径。
func TestOpen_ParentPathIsFile(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if _, err := Open(context.Background(), filepath.Join(blocker, "file-pilot.db")); err == nil {
		t.Fatal("want error when parent dir path is a file, got nil")
	}
}

// TestMethods_FailFastWhenDatabaseClosed 用「已关闭的库」注入依赖故障，
// 断言每个方法都返回错误而非 panic 或静默成功（testing.md：依赖错误路径）。
func TestMethods_FailFastWhenDatabaseClosed(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	calls := []struct {
		name string
		fn   func() error
	}{
		{"Migrate", func() error { return store.Migrate(ctx) }},
		{"CreateScan", func() error { _, err := store.CreateScan(ctx, `C:\a`, 1, 1); return err }},
		{"FinishScan", func() error { return store.FinishScan(ctx, 1, ScanOutcome{Status: "completed"}) }},
		{"GetScan", func() error { _, err := store.GetScan(ctx, 1); return err }},
		{"FailRunningScans", func() error { _, err := store.FailRunningScans(ctx, 1); return err }},
		{"PruneOldScans", func() error { _, err := store.PruneOldScans(ctx, 5); return err }},
		{"InsertFiles", func() error { return store.InsertFiles(ctx, 1, []FileRecord{{Name: "x"}}) }},
		{"ListFiles", func() error { _, _, err := store.ListFiles(ctx, 1, "", 10, 0); return err }},
		{"CreateTask", func() error { _, err := store.CreateTask(ctx, "scan", 1); return err }},
		{"SetTaskStatus", func() error { return store.SetTaskStatus(ctx, 1, "running", 0, "") }},
		{"SetTaskScan", func() error { return store.SetTaskScan(ctx, 1, 1) }},
		{"FailUnfinishedTasks", func() error { _, err := store.FailUnfinishedTasks(ctx, 1, "应用中断"); return err }},
		{"GetTask", func() error { _, err := store.GetTask(ctx, 1); return err }},
		{"GetSetting", func() error { _, err := store.GetSetting(ctx, "scan_config"); return err }},
		{"SetSetting", func() error { return store.SetSetting(ctx, "scan_config", "{}") }},
	}
	for _, c := range calls {
		if err := c.fn(); err == nil {
			t.Errorf("%s: want error on closed database, got nil", c.name)
		}
	}
}
