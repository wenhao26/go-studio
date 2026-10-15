package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// migrationFS 挂载版本化的 SQL 迁移步（AGENTS.md §22.4：只增不改，禁止修改已执行的迁移）。
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate 按版本步进应用尚未执行的 schema 变更。
//
// 版本记录在 schema_version 表；每个迁移在独立事务中执行并记录版本，
// 任一步失败即整体失败（该步已回滚，应用下次启动可重试）。
func (s *Store) Migrate(ctx context.Context) error {
	// schema_version 自身用 IF NOT EXISTS 幂等创建：它是迁移机制的引导表。
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
)`); err != nil {
		return fmt.Errorf("ensure schema_version: %w", err)
	}

	var current int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		if err := s.applyMigration(ctx, name, version); err != nil {
			return err
		}
	}
	return nil
}

// migrationVersion 从文件名解析版本号（约定 `0001_init.sql` 形态，前缀为数字）。
func migrationVersion(name string) (int64, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q: name must look like 0001_init.sql", name)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("migration %q: invalid version prefix %q", name, prefix)
	}
	return version, nil
}

// applyMigration 在单个事务内执行 SQL 并记录版本。
func (s *Store) applyMigration(ctx context.Context, name string, version int64) error {
	body, err := migrationFS.ReadFile(path.Join("migrations", name))
	if err != nil {
		return fmt.Errorf("read migration %q: %w", name, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %q: %w", name, err)
	}
	// commit 成功后 Rollback 返回 sql.ErrTxDone，属预期路径；commit 失败时
	// 该调用负责回滚。两条路径的错误都不再上报，因为返回值已由 commit 决定。
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("apply migration %q: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`,
		version, time.Now().UTC().UnixMilli()); err != nil {
		return fmt.Errorf("record migration %q: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %q: %w", name, err)
	}
	return nil
}
