// Package fsx 提供 file-pilot 所需的最小文件系统抽象与平台属性采集。
//
// 抽象的目的只有一个：可测试性——权限拒绝、符号链接、目录联接（junction）
// 这些错误分支无法在真实文件系统上稳定构造，必须能被单元测试注入
// （docs/development/testing.md 覆盖率要求）。设计沿用 fast-stat/internal/fsx
// 的已验证思路（AGENTS.md §87：已有模式优先复用）。
package fsx

import (
	"io/fs"
	"os"
	"strings"
	"time"
)

// OrganizeDirName 是整理目标根目录名（requirements FR-9）。
//
// v1 固定常量（requirements-analysis §7 P1-3）。放在本包供 scanner（扫描期
// 排除输出目录，SEC-4）与 organizer（创建目录与前缀校验，SEC-3）共用，
// 避免两处各自维护同名字符串而分裂。
const OrganizeDirName = "FilePilot整理"

// FileSystem 抽象目录遍历所需的最小能力。
//
// 实现约定：
//   - ReadDir 不保证条目顺序，调用方不得依赖顺序；
//   - ReadDir 中途失败时返回已读条目与非 nil 错误，由调用方同时处理部分结果；
//   - 返回错误保留 *fs.PathError 包装，使 errors.Is 能判定
//     fs.ErrNotExist 与 fs.ErrPermission；
//   - Lstat 不跟随链接；Stat 跟随链接，仅用于识别 Windows 目录联接。
type FileSystem interface {
	// ReadDir 读取 dir 下的目录条目。
	ReadDir(dir string) ([]fs.DirEntry, error)

	// Lstat 返回 path 的元数据，且不跟随符号链接。
	Lstat(path string) (fs.FileInfo, error)

	// Stat 返回 path 的元数据并跟随链接。
	//
	// 存在的唯一用途：Windows 目录联接在 Go 中既不是 ModeSymlink 也不是目录，
	// 而是 ModeIrregular（Phase 0 V5 实测），只有跟随一次才能判断它指向目录。
	// 调用点被限制在罕见分支上，不会给常规扫描增加 syscall。
	Stat(path string) (fs.FileInfo, error)
}

// OSFileSystem 是 FileSystem 基于标准库 os 的生产实现。
type OSFileSystem struct{}

// ReadDir 读取目录条目。
//
// 刻意不使用 os.Open + Readdirnames：调用方需要条目类型信息（DirEntry），
// 且不希望在中间层持有文件句柄（句柄泄漏风险与显式 Close 义务）。
func (OSFileSystem) ReadDir(dir string) ([]fs.DirEntry, error) {
	return os.ReadDir(dir)
}

// Lstat 返回 path 的元数据，不跟随符号链接。
func (OSFileSystem) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

// Stat 返回 path 的元数据，跟随符号链接与目录联接。
func (OSFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

// Rename 执行同卷改名。
//
// ⚠️ 安全提示（SEC-1，Phase 0 V1 已核实）：Windows 上 os.Rename 底层为
// MoveFileEx(..., MOVEFILE_REPLACE_EXISTING)，**会覆盖已存在的目标文件**。
// 因此调用方（organizer）必须先用 Lstat 确认目标不存在，绝不依赖
// 「目标存在则 rename 失败」这一错误假设。
func (OSFileSystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// MkdirAll 递归创建目录（已存在时返回 nil）。
func (OSFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Attributes 是一次元数据查询中获得的平台相关文件属性（best-effort）。
//
// 取不到的字段保持零值，绝不猜测（AGENTS.md §4.1）。
type Attributes struct {
	// Hidden 是 Windows FILE_ATTRIBUTE_HIDDEN 属性。
	Hidden bool

	// System 是 Windows FILE_ATTRIBUTE_SYSTEM 属性。
	System bool

	// CreatedAt 是文件创建时间；未知时为零值，落库层据此写 NULL
	//（requirements-analysis §3.1：ctime best-effort）。
	CreatedAt time.Time
}

// AttributesOf 提取 fi 的平台属性，fi 为 nil 时返回零值。
// 各字段的平台差异见 attrs_windows.go / attrs_other.go。
func AttributesOf(fi fs.FileInfo) Attributes {
	if fi == nil {
		return Attributes{}
	}
	return attributesOf(fi)
}

// IsHiddenName 判断条目名是否属于「点前缀隐藏」。
//
// requirements FR-2 的隐藏文件口径是「`.` 前缀 / Windows 隐藏属性」二者取或，
// 点前缀部分与平台无关，故在此统一判定，与 attrs.Hidden 由调用方取或。
func IsHiddenName(name string) bool {
	return strings.HasPrefix(name, ".")
}
