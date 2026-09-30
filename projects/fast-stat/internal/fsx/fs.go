// Package fsx 提供 fast-stat 所需的最小文件系统抽象与底层写入工具。
//
// 抽象的目的只有一个：可测试性。权限拒绝、符号链接、超大条目数这些场景无法在
// 真实文件系统上稳定构造，必须能被单元测试注入，否则遍历层的错误分支无法被覆盖
// （见 docs/development/testing.md 的覆盖率要求）。
package fsx

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileSystem 抽象本地文件系统的目录读取与元数据查询能力。
//
// 实现约定：
//   - ReadDir 不保证条目顺序，调用方不得依赖顺序；
//   - ReadDir 读取中途失败时，必须返回已成功读取的条目与非 nil 错误，
//     由调用方同时处理部分结果与错误；
//   - 返回的错误必须保留 *fs.PathError 包装，使 errors.Is 能判定
//     fs.ErrNotExist 与 fs.ErrPermission；
//   - Lstat 不跟随符号链接。
type FileSystem interface {
	// ReadDir 读取 dir 下的目录条目。
	ReadDir(dir string) ([]fs.DirEntry, error)

	// Lstat 返回 path 的元数据，且不跟随符号链接。
	Lstat(path string) (fs.FileInfo, error)

	// Stat 返回 path 的元数据，并跟随符号链接与目录联接。
	//
	// 存在的唯一用途：识别 Windows 目录联接。Windows 上的 junction 在 Go 中
	// 既不是 ModeSymlink 也不是目录，而是 ModeIrregular，只有跟随一次才能判断
	// 它指向目录还是文件。调用点被限制在罕见分支上，不会给常规扫描增加开销。
	Stat(path string) (fs.FileInfo, error)
}

// OSFileSystem 是 FileSystem 基于标准库 os 的生产实现。
type OSFileSystem struct{}

// ReadDir 读取目录条目。
//
// 这里刻意不使用 os.ReadDir：标准库实现会对结果额外做一次 O(n log n) 的字典序
// 排序（见 Go 源码 os.ReadDir 中的 slices.SortFunc）。fast-stat 只需要条目集合、
// 不依赖顺序，在百万级条目的目录下这次排序是纯粹的浪费。f.ReadDir(-1) 返回的
// 顺序即底层目录顺序，并且同样不会对每个条目额外发起 stat。
//
// 错误不做二次包装：os.Open 与 f.ReadDir 返回的 *fs.PathError 已包含
// op 与 path 上下文，重复包装只会让日志出现两次路径。
func (OSFileSystem) ReadDir(dir string) ([]fs.DirEntry, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	// 只读目录句柄：Close 仅释放文件描述符，不影响已读出的数据。
	defer func() { _ = f.Close() }()

	// f.ReadDir(-1) 在读取中途失败时会连同已读出的条目一起返回错误，
	// 这里原样向上传递，由调用方决定如何处理部分结果。
	return f.ReadDir(-1)
}

// Lstat 返回 path 的元数据，不跟随符号链接。
func (OSFileSystem) Lstat(path string) (fs.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// Stat 返回 path 的元数据，并跟随符号链接与目录联接。
//
// 注意：这里只读取元数据，不会打开文件，因此不会触发 OneDrive 等云盘的
// 文件按需下载（水合由读取内容触发，而非元数据查询）。
func (OSFileSystem) Stat(path string) (fs.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// DefaultFilePerm 是 --output 生成文件使用的权限位。
const DefaultFilePerm fs.FileMode = 0o644

// WriteFileAtomic 以「同目录临时文件 + rename」的方式原子写入文件。
//
// 直接写目标文件时，进程被中断（例如用户 Ctrl-C）或磁盘写满都会留下半截内容；
// 对 --output 生成的 JSON 而言，半截文件比没有文件更糟——调用方会解析失败，
// 且无法区分「未生成」与「生成失败」。同一文件系统内的 rename 是原子操作。
//
// 返回错误时不会残留临时文件。
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %q: %w", path, err)
	}
	tmpName := tmp.Name()

	// 任何一步失败都必须清理临时文件，避免在用户目录留下垃圾。
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file %q: %w", tmpName, err)
	}
	// os.CreateTemp 以 0600 创建，这里收敛到调用方期望的权限。
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file %q: %w", tmpName, err)
	}
	// 先落盘再 rename：避免掉电后出现「文件名已存在但内容为空」。
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %q to %q: %w", tmpName, path, err)
	}
	committed = true
	return nil
}
