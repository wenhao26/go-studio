//go:build !windows

package fsx

import (
	"io/fs"
	"time"
)

// attributesOf 在非 Windows 平台没有可读的隐藏/系统属性位：
// 隐藏判定退化为点前缀（见 IsHiddenName），System 恒为 false。
//
// 创建时间按 requirements §十五 R-2 回退为 mtime（非 Windows 平台 best-effort）。
func attributesOf(fi fs.FileInfo) Attributes {
	return Attributes{CreatedAt: fi.ModTime().UTC()}
}
