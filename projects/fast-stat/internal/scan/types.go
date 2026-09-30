package scan

import (
	"errors"
	"fmt"
	"time"
)

// ErrNotDirectory 表示传入的根路径存在但不是目录。
//
// 复用标准库哨兵无法表达这一语义（fs.ErrNotExist 描述的是不存在），因此本包
// 定义唯一的哨兵错误；调用方用 errors.Is 判定，不要比较错误字符串。
var ErrNotDirectory = errors.New("root path is not a directory")

// Options 定义一次扫描的输入参数，构造后视为只读。
type Options struct {
	// Root 是统计的根目录路径，必须解析为目录。
	Root string

	// Workers 是并发遍历的 worker 数量。
	//
	// 取值范围被内部收敛到 [1, maxWorkers]；<=0 时使用 DefaultWorkers。
	Workers int
}

// ProgressInfo 是一次扫描的实时进度快照。
//
// 这里刻意不包含百分比与 ETA：目录遍历结束前无法获知总工作量，
// 任何百分比都只能是被编造出来的数字。
type ProgressInfo struct {
	// DirectoriesVisited 是已成功完整读取的目录数。
	DirectoriesVisited int64

	// FilesFound 是已计数的文件数。
	FilesFound int64

	// TotalSizeBytes 是已累计的文件逻辑大小（字节）。
	TotalSizeBytes int64

	// SkippedErrors 是因读取失败被跳过的路径数。
	SkippedErrors int64

	// CurrentDir 是最近一次开始处理的目录路径；可能为空。
	CurrentDir string
}

// ScanError 记录一次被跳过但未中断扫描的文件系统错误。
type ScanError struct {
	// Path 是出错的目标路径，未经终端净化处理。
	Path string

	// Err 是原始错误，保留底层包装以便 errors.Is 溯源。
	Err error
}

// Error 实现 error 接口，输出 "路径: 原因" 形式。
func (e ScanError) Error() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

// Unwrap 暴露底层错误，使 errors.Is / errors.As 能穿透到根因。
func (e ScanError) Unwrap() error { return e.Err }

// Snapshot 是一次扫描完成后的不可变结果快照。
type Snapshot struct {
	// Root 是实际扫描的根路径（已做 filepath.Clean）。
	Root string

	// DirectoriesVisited 是成功完整读取的目录节点数，含根目录。
	//
	// 读取失败的目录不计入该值，而是计入 SkippedErrors。
	DirectoriesVisited int64

	// FilesFound 是目录树中的普通文件与特殊文件（FIFO/socket/设备）数量。
	//
	// 不含目录本身，不含符号链接。
	FilesFound int64

	// TotalSizeBytes 是文件逻辑大小（st_size）之和，按字节计。
	//
	// 采用逻辑大小而非磁盘占用（st_blocks）：前者跨平台语义一致，且与
	// os.FileInfo.Size() 对应；后者需要平台特定代码，且与 du 的默认口径
	// 一致但与用户对「路径总大小」的直觉不同。
	TotalSizeBytes int64

	// SkippedErrors 是被跳过的错误总数；可能大于 ErrorDetails 的长度。
	SkippedErrors int64

	// Duration 是扫描实际耗时。
	Duration time.Duration

	// ErrorDetails 是被跳过错误的前 maxErrorDetails 条明细，用于诊断展示。
	//
	// 明细被有意限流：错误数量与目录规模相关，若全量保留会让内存占用
	// 随目录树的损坏程度线性增长，违背流式低内存的目标。
	ErrorDetails []ScanError
}
