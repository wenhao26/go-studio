package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/scanner"
)

// settings 表键名（storage.settings）。
const (
	settingScanConfig = "scan_config"
	settingRecentDirs = "recent_dirs"
)

// recentDirsLimit 是最近目录的最大保留条数（FR-13：保留 20 条）。
const recentDirsLimit = 20

// Settings 是 GetSettings/SaveSettings 的边界 DTO（contracts §3.1 + FR-13）。
//
// ScanConfig 内联展开，其 json 标签即 Wails 边界名；RecentDirs 由服务端维护，
// SaveSettings 不接受该字段（前端不可写，避免把便利性数据变成可篡改输入）。
type Settings struct {
	scanner.Config
	RecentDirs []string `json:"recent_dirs"`
}

// GetSettings 读取持久化配置与最近目录；键缺失时返回默认值（FR-1/FU-2）。
func (a *App) GetSettings() (Settings, error) {
	ctx, err := a.requireLifecycle()
	if err != nil {
		return Settings{}, err
	}

	cfg, err := a.loadConfig(ctx)
	if err != nil {
		// 配置损坏命中 ErrConfigInvalid：裸哨兵上边界（message 即匹配键）。
		return Settings{}, bareError(err, ErrConfigInvalid)
	}
	dirs, err := a.loadRecentDirs(ctx)
	if err != nil {
		return Settings{}, err
	}
	return Settings{Config: cfg, RecentDirs: dirs}, nil
}

// SaveSettings 保存扫描配置；只写 scan_config，不接受 recent_dirs。
func (a *App) SaveSettings(cfg scanner.Config) error {
	if err := cfg.Validate(); err != nil {
		// 命中 ErrInvalidWorkers：裸返回（message 即前端匹配键）。
		return bareError(err, scanner.ErrInvalidWorkers)
	}

	ctx, err := a.requireLifecycle()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		// Config 全是标量，理论上不可失败；防御性处理而非假设（AGENTS.md §4.1）。
		return fmt.Errorf("encode scan config: %w", err)
	}
	if err := a.store.SetSetting(ctx, settingScanConfig, string(raw)); err != nil {
		a.diag.Error("save scan config", zap.Error(err))
		return fmt.Errorf("save scan config: %w", err)
	}
	return nil
}

// loadConfig 读取 scan_config：键缺失返回默认值；JSON 损坏或越界一律报
// ErrConfigInvalid（不静默回退默认值——那会掩盖损坏的持久化数据）。
//
// 可恢复性：用户仍可调用 SaveSettings 覆盖损坏的值。
func (a *App) loadConfig(ctx context.Context) (scanner.Config, error) {
	raw, err := a.store.GetSetting(ctx, settingScanConfig)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scanner.DefaultConfig(), nil
		}
		a.diag.Error("load scan config", zap.Error(err))
		return scanner.Config{}, fmt.Errorf("load scan config: %w", err)
	}

	var cfg scanner.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		a.diag.Error("decode scan config", zap.String("raw", raw), zap.Error(err))
		return scanner.Config{}, fmt.Errorf("decode scan config: %w", ErrConfigInvalid)
	}
	if err := cfg.Validate(); err != nil {
		a.diag.Error("validate scan config", zap.String("raw", raw), zap.Error(err))
		return scanner.Config{}, fmt.Errorf("validate scan config: %w", ErrConfigInvalid)
	}
	return cfg, nil
}

// loadRecentDirs 读取最近目录；键缺失返回空切片（序列化为 [] 而非 null）。
//
// 历史数据损坏时记诊断日志并按空列表处理：它是便利性数据，不应让设置页
// 整体不可用，且下一次扫描会写入新列表完成自愈。
func (a *App) loadRecentDirs(ctx context.Context) ([]string, error) {
	raw, err := a.store.GetSetting(ctx, settingRecentDirs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		a.diag.Error("load recent dirs", zap.Error(err))
		return nil, fmt.Errorf("load recent dirs: %w", err)
	}

	var dirs []string
	if err := json.Unmarshal([]byte(raw), &dirs); err != nil {
		a.diag.Warn("decode recent dirs", zap.String("raw", raw), zap.Error(err))
		return []string{}, nil
	}
	if dirs == nil {
		dirs = []string{}
	}
	return dirs, nil
}

// RemoveRecentDir 从 recent_dirs 移除指定目录（幂等：不存在时返回 nil）。
//
// 这是前端移除最近目录的唯一入口：SaveSettings 不接受 recent_dirs（§3.1），
// 受维护数据只能经此受控方法变更。保留其余条目顺序不变。
func (a *App) RemoveRecentDir(dir string) error {
	if dir == "" {
		return nil
	}

	ctx, err := a.requireLifecycle()
	if err != nil {
		return err
	}

	dirs, err := a.loadRecentDirs(ctx)
	if err != nil {
		return err
	}

	out := make([]string, 0, len(dirs))
	removed := false
	for _, d := range dirs {
		if d == dir {
			removed = true
			continue
		}
		out = append(out, d)
	}
	if !removed {
		return nil
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("encode recent dirs: %w", err)
	}
	if err := a.store.SetSetting(ctx, settingRecentDirs, string(raw)); err != nil {
		a.diag.Error("remove recent dir", zap.String("dir", dir), zap.Error(err))
		return fmt.Errorf("remove recent dir: %w", err)
	}
	return nil
}

// rememberDir 把本次扫描目录写到 recent_dirs 头部：去重、保留最近
// recentDirsLimit 条（FR-13）。
//
// 写入失败只由调用方记录诊断日志：它是 UI 便利性数据，不应影响已启动的扫描。
func (a *App) rememberDir(ctx context.Context, root string) error {
	dirs, err := a.loadRecentDirs(ctx)
	if err != nil {
		return err
	}

	out := make([]string, 0, recentDirsLimit)
	out = append(out, root)
	for _, d := range dirs {
		if d == root || len(out) >= recentDirsLimit {
			continue
		}
		out = append(out, d)
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("encode recent dirs: %w", err)
	}
	return a.store.SetSetting(ctx, settingRecentDirs, string(raw))
}
