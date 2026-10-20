// Package storage 是 file-pilot 的 SQLite 数据访问层（Repository，AGENTS.md §23）。
//
// 本包只负责打开数据库、迁移、参数化查询与批量写入，不做任何业务决策；
// 行类型（Row/Record）在此定义，上层（scanner/app）负责转换为自己的领域或
// DTO 类型，避免数据库模型直接泄漏到 API 层（AGENTS.md §13.2）。
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（requirements §七，D-2 裁决）
)

// driverName 是 modernc.org/sqlite 注册的驱动名。
const driverName = "sqlite"

// 运行参数（裁决 D-A2，见 development/01-phase-1-scan.md §6）。
const (
	// busyTimeoutMS 是 SQLite 等待锁释放的毫秒数，避免瞬时写冲突直接失败。
	busyTimeoutMS = 5000

	// journalModeWAL 让写入不阻塞读取，并提升崩溃后的恢复能力。
	journalModeWAL = "WAL"
)

// Store 是 SQLite 数据访问入口，一个进程只应打开一个实例。
type Store struct {
	db   *sql.DB
	path string
}

// Path 返回本实例实际打开的数据库文件路径；用于界面向用户展示保存位置，
// 不做任何路径推断（展示值必须与真正打开的库一致）。
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// DefaultPath 返回默认数据库文件路径：`<用户配置目录>/FilePilot/file-pilot.db`
// （Windows 上即 %APPDATA%\FilePilot\file-pilot.db，requirements §七）。
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config dir: %w", err)
	}
	return filepath.Join(base, "FilePilot", "file-pilot.db"), nil
}

// Open 打开（必要时创建）数据库并应用全部运行参数。
//
// 参数策略是「设置 + 复核」：任何一项未生效都直接返回错误，绝不静默降级
// （AGENTS.md §4.1 不臆测），否则「已开启 WAL/外键」会变成未经验证的假设。
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("storage: empty database path")
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database dir %q: %w", dir, err)
		}
	}

	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	// 单连接串行（裁决 D-A2）：在 Ping 之前设置，初始化阶段也不产生并发连接。
	db.SetMaxOpenConns(1)

	store := &Store{db: db, path: path}
	if err := store.applyPragmas(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database %q: %w", path, err)
	}
	return store, nil
}

// applyPragmas 应用并复核运行参数：journal_mode=WAL、busy_timeout、foreign_keys。
func (s *Store) applyPragmas(ctx context.Context) error {
	// journal_mode 的赋值形式会返回生效后的新模式，可直接复核。
	var journal string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode="+journalModeWAL).Scan(&journal); err != nil {
		return fmt.Errorf("set journal_mode=WAL: %w", err)
	}
	if !strings.EqualFold(journal, journalModeWAL) {
		return fmt.Errorf("set journal_mode=WAL: got %q", journal)
	}

	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMS)); err != nil {
		return fmt.Errorf("set busy_timeout=%d: %w", busyTimeoutMS, err)
	}
	var timeout int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil {
		return fmt.Errorf("verify busy_timeout: %w", err)
	}
	if timeout != busyTimeoutMS {
		return fmt.Errorf("verify busy_timeout: got %d", timeout)
	}

	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("set foreign_keys=ON: %w", err)
	}
	var fk int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return fmt.Errorf("verify foreign_keys: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("verify foreign_keys: got %d", fk)
	}
	return nil
}

// Close 关闭数据库连接（可安全重复调用）。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}
