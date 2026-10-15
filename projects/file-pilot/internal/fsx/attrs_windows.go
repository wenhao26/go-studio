//go:build windows

package fsx

import (
	"io/fs"
	"syscall"
	"time"
)

// Windows 文件属性位（winnt.h FILE_ATTRIBUTE_*）。
const (
	fileAttributeHidden = 0x00000002
	fileAttributeSystem = 0x00000004
)

// attributesOf 在 Windows 上读取隐藏/系统属性与创建时间。
//
// Phase 0 V2/V3 实测：os 的 FileInfo.Sys() 返回 *syscall.Win32FileAttributeData，
// 纯标准库即可读取，因此不引入 golang.org/x/sys/windows（见
// development/00-phase-0-prerequisites.md）。读取失败（类型断言不成立、
// FILETIME 无效）时保持零值，绝不猜测。
func attributesOf(fi fs.FileInfo) Attributes {
	data, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return Attributes{}
	}

	var attrs Attributes
	attrs.Hidden = data.FileAttributes&fileAttributeHidden != 0
	attrs.System = data.FileAttributes&fileAttributeSystem != 0

	// FILETIME 是自 1601-01-01 起的 100ns 计数；Nanoseconds() 对无效值返回 0。
	if ns := data.CreationTime.Nanoseconds(); ns > 0 {
		attrs.CreatedAt = time.Unix(0, ns).UTC()
	}
	return attrs
}
