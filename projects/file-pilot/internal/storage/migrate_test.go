package storage

import (
	"context"
	"testing"
)

func TestMigrate_AppliesOnceAndIsIdempotent(t *testing.T) {
	store := openRawTestStore(t)
	ctx := context.Background()

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&count); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}
	if count != 1 {
		t.Fatalf("schema_version rows = %d, want 1（重复迁移不得重复记录）", count)
	}
}

func TestMigrate_CreatesExpectedTables(t *testing.T) {
	store := openTestStore(t)

	want := []string{"scans", "files", "tasks", "operations", "settings"}
	for _, table := range want {
		var n int
		err := store.db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&n)
		if err != nil {
			t.Fatalf("query sqlite_master for %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s missing after migrate", table)
		}
	}
}

func TestMigrate_BackfillsWithoutBreakingWrites(t *testing.T) {
	// 迁移后必须能完成一次真实的「建扫描 → 落文件 → 建任务」写入链路，
	// 否则 schema 存在但不可用。
	store := openTestStore(t)
	ctx := context.Background()

	scanID, err := store.CreateScan(ctx, `C:\data`, 1000, 4)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	if err := store.InsertFiles(ctx, scanID, []FileRecord{{
		Path: `C:\data\a.jpg`, Name: "a.jpg", Ext: "jpg", SizeBytes: 10,
		MtimeMS: 2000, Category: "图片", Confidence: 60, Source: "rule", Reason: "扩展名 jpg",
	}}); err != nil {
		t.Fatalf("insert files: %v", err)
	}
	taskID, err := store.CreateTask(ctx, "scan", 1000)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.SetTaskScan(ctx, taskID, scanID); err != nil {
		t.Fatalf("set task scan: %v", err)
	}
}

func TestMigrationVersion(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		want    int64
		wantErr bool
	}{
		{name: "valid", file: "0001_init.sql", want: 1},
		{name: "no underscore", file: "init.sql", wantErr: true},
		{name: "non-numeric", file: "abcd_init.sql", wantErr: true},
		{name: "zero version", file: "0000_init.sql", wantErr: true},
		{name: "negative", file: "-1_init.sql", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := migrationVersion(tt.file)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got version %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("version = %d, want %d", got, tt.want)
			}
		})
	}
}
