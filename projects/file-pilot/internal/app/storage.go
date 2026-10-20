package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"

	"go.uber.org/zap"
)

// 存储位置的种类（OpenStorageFolder 的唯一入参）。
//
// 前端只传枚举值，路径一律由后端解析：不把「用资源管理器打开任意目录」
// 变成对外能力（AGENTS.md §43）。
const (
	storageKindData = "data"
	storageKindLog  = "log"
)

// StorageInfo 是「存储位置」的只读展示 DTO（requirements §8.1）。
//
// LogPath 为空表示诊断日志降级为 stderr（未落文件），前端据此禁用打开入口。
type StorageInfo struct {
	DataPath string `json:"data_path"`
	LogPath  string `json:"log_path"`
}

// GetStorageInfo 返回数据库与诊断日志文件的真实路径。
//
// 路径取自进程启动时真正打开的文件（storage.Store.Path / diag.Log.FilePath），
// 不在此处重新推断，避免展示值与实际写入位置不一致。
func (a *App) GetStorageInfo() (StorageInfo, error) {
	return StorageInfo{
		DataPath: a.store.Path(),
		LogPath:  a.diag.FilePath(),
	}, nil
}

// OpenStorageFolder 在系统文件管理器中打开指定存储文件所在目录。
//
// 只读操作：不修改任何配置、不写数据库。失败统一返回 ErrStorageUnavailable
// （裸哨兵，前端映射为「无法打开存储目录」），详细错误链只进诊断日志。
func (a *App) OpenStorageFolder(kind string) error {
	path, err := a.storageFilePath(kind)
	if err != nil {
		// 非法 kind 属调用方错误：记录后原样返回（未命中哨兵 → 通用文案）。
		a.diag.Warn("open storage folder: unresolved kind", zap.String("kind", kind), zap.Error(err))
		return err
	}

	dir := filepath.Dir(path)
	if err := ensureDirectory(dir); err != nil {
		a.diag.Error("open storage folder",
			zap.String("kind", kind), zap.String("dir", dir), zap.Error(err))
		return ErrStorageUnavailable
	}
	if err := a.openFolder(dir); err != nil {
		a.diag.Error("open storage folder",
			zap.String("kind", kind), zap.String("dir", dir), zap.Error(err))
		return ErrStorageUnavailable
	}
	return nil
}

// storageFilePath 把 kind 解析为对应的存储文件路径；路径缺失时返回
// ErrStorageUnavailable（例如日志降级为 stderr）。
func (a *App) storageFilePath(kind string) (string, error) {
	switch kind {
	case storageKindData:
		if path := a.store.Path(); path != "" {
			return path, nil
		}
	case storageKindLog:
		if path := a.diag.FilePath(); path != "" {
			return path, nil
		}
	default:
		return "", fmt.Errorf("unknown storage kind %q", kind)
	}
	return "", ErrStorageUnavailable
}

// ensureDirectory 校验目录存在且确实是目录（不跟随不可用的路径）。
func ensureDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}
	return nil
}

// openFolderNative 用系统文件管理器打开目录（Windows：explorer.exe）。
//
// 只检查启动是否成功：不等进程退出（explorer 会把请求转交给已运行的实例），
// 也不用退出码判定成败（系统行为上成功时也可能返回非 0）。
func openFolderNative(dir string) error {
	if goruntime.GOOS != "windows" {
		return fmt.Errorf("open folder: unsupported platform %s", goruntime.GOOS)
	}
	cmd := exec.Command("explorer.exe", dir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start explorer: %w", err)
	}
	// 不调用 Wait（explorer 可能长期驻留）；Release 释放进程句柄。
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release explorer process: %w", err)
	}
	return nil
}
