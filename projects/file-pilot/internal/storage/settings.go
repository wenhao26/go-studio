package storage

import (
	"context"
	"fmt"
)

// GetSetting 读取 settings 键；键不存在时返回 sql.ErrNoRows。
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("get setting: empty key")
	}
	var value string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value); err != nil {
		return "", fmt.Errorf("get setting %q: %w", key, err)
	}
	return value, nil
}

// SetSetting 写入（或覆盖）settings 键。
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	if key == "" {
		return fmt.Errorf("set setting: empty key")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
