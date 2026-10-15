package organizer

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// maxConflictAttempts 是冲突改名的上限：`原名 (1)..(10000)`（analysis §2.2）。
// 超出仍未取到空缺名视为该条失败，不做无限枚举。
const maxConflictAttempts = 10000

// errNoUniqueName 表示在尝试上限内未能生成不冲突的目标名。
var errNoUniqueName = errors.New("无法生成唯一文件名")

// resolveTarget 返回 dir 下 name 的第一个不冲突变体（**绝不覆盖**，SEC-1，D-D4）。
//
// 算法（analysis §2.2）：
//  1. 先试原名；
//  2. 冲突则试 `base (n)ext`，n = 1..maxConflictAttempts，取第一个不存在者；
//  3. 返回前的存在性检查即「紧邻 rename 前的复检」，缩小 TOCTOU 窗口。
//
// ⚠️ 不能用「rename 目标存在会失败」来保证不覆盖：Windows 上 os.Rename 会
// 覆盖已存在的目标（Phase 0 V1 实测），因此存在性检查是唯一保障。
func resolveTarget(fsys FS, dir, name string, maxAttempts int) (string, error) {
	if err := checkBaseName(name); err != nil {
		return "", err
	}
	if maxAttempts <= 0 {
		maxAttempts = maxConflictAttempts
	}

	target := filepath.Join(dir, name)
	exists, err := pathExists(fsys, target)
	if err != nil {
		return "", err
	}
	if !exists {
		return target, nil
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for n := 1; n <= maxAttempts; n++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, n, ext))
		exists, err := pathExists(fsys, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", errNoUniqueName
}

// pathExists 报告 path 是否存在。
//
// 关键区分：只有 fs.ErrNotExist 才算「不存在」；其他错误（如权限不足）必须
// 上抛而非当作「不存在」——否则可能在无法确认时贸然 rename 覆盖目标。
func pathExists(fsys FS, path string) (bool, error) {
	_, err := fsys.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// resolveReason 把生成目标名阶段的错误转为面向用户的失败原因。
func resolveReason(err error) string {
	if errors.Is(err, errNoUniqueName) {
		return fmt.Sprintf("同名文件过多（超过 %d 个）", maxConflictAttempts)
	}
	if errors.Is(err, fs.ErrPermission) {
		return "无法访问目标目录（权限不足）"
	}
	return "无法确定目标文件名"
}

// moveErrorReason 把 rename 错误转为面向用户的失败原因。
//
// 已知类别给出中文短语；其余保留 OS 原始说明（本地单机工具，路径与系统错误
// 不涉及敏感信息，保留原文有助于用户定位「被占用 / 跨卷」等原因，AGENTS.md §28）。
func moveErrorReason(err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "权限不足"
	case errors.Is(err, fs.ErrExist):
		return "目标已存在"
	case errors.Is(err, fs.ErrNotExist):
		return "源文件或目标目录不存在"
	default:
		return "移动失败: " + err.Error()
	}
}
