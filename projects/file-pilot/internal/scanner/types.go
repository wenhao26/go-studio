package scanner

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/classifier"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

const (
	// MaxWorkers 是扫描并发度的硬上限（requirements FR-2：允许 1–64）。
	MaxWorkers = 64

	// defaultWorkerCap 是默认并发度的上限。
	//
	// 上限由 I/O 特征而非 CPU 决定：机械硬盘上并发遍历会因磁头反复寻道
	// 使吞吐低于单线程顺序遍历，因此默认值不随核数无限放大（沿用 fast-stat 结论）。
	defaultWorkerCap = 16

	// defaultBatchSize 是 files 落库批量阈值（requirements §七：每 500 条一个事务）。
	defaultBatchSize = 500

	// maxSkipDetails 限制跳过明细条数（FR-5：明细最多 20 条，展示于日志区）。
	maxSkipDetails = 20

	// largeFileThreshold 是「跳过大文件」开关的固定阈值 1 GiB（FR-2：v1 不可配置）。
	largeFileThreshold = 1 << 30
)

// ErrNotDirectory 表示传入的根路径存在但不是目录。
var ErrNotDirectory = errors.New("root path is not a directory")

// ErrInvalidWorkers 表示并发数非法（contracts §4，前端文案「并发数需在 1–64 之间」）。
var ErrInvalidWorkers = errors.New("workers must be between 1 and 64")

// Config 是一次扫描的配置（contracts §3.1）；json 字段名即 Wails 边界名。
type Config struct {
	// Workers 是扫描并发数，允许 [1,64]，非法值报错不静默钳制（FR-2）。
	Workers int `json:"workers"`

	// ScanHidden 为 true 时纳入隐藏文件与隐藏目录（. 前缀 / Windows 隐藏属性）。
	ScanHidden bool `json:"scan_hidden"`

	// ScanSystem 为 true 时纳入带 Windows system 属性的文件与目录。
	ScanSystem bool `json:"scan_system"`

	// SkipLarge 为 true 时跳过 > 1 GiB 的文件。
	SkipLarge bool `json:"skip_large"`
}

// DefaultConfig 返回默认配置：workers = clamp(NumCPU, 1, 64)，三个开关均为 false。
func DefaultConfig() Config {
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	if n > MaxWorkers {
		n = MaxWorkers
	}
	return Config{Workers: n}
}

// Validate 校验配置边界；非法值立即报错，不静默钳制（FR-2）。
func (c Config) Validate() error {
	if c.Workers < 1 || c.Workers > MaxWorkers {
		return fmt.Errorf("scan config workers=%d: %w", c.Workers, ErrInvalidWorkers)
	}
	return nil
}

// LogKind 是三类日志的分类（requirements FR-4 / contracts §3.9）。
type LogKind string

const (
	// LogScan 是扫描日志（会被采样）。
	LogScan LogKind = "scan"
	// LogClassify 是分类日志（会被采样）。
	LogClassify LogKind = "classify"
	// LogSystem 是系统日志，**不采样**（contracts §3.9）。
	LogSystem LogKind = "system"
)

// LogEntry 是一条待推送日志；TS 为 UTC 毫秒（contracts §3.9）。
type LogEntry struct {
	TS   int64   `json:"ts"`
	Kind LogKind `json:"kind"`
	Text string  `json:"text"`
}

// LogSink 接收扫描期日志。接口由消费方 scanner 定义（contracts §2），
// 生产实现是 task.Throttler：它负责 200ms 窗口批量与采样丢弃（§8.3）。
// 调用方允许为 nil（无日志场景）。
type LogSink interface {
	Log(kind LogKind, text string)
}

// FileSink 批量落库 files。
//
// 接口由消费方 scanner 定义；参数使用 storage.FileRecord（更下层的行类型），
// 因此 storage.Store 结构化实现本接口时无需 import scanner（避免反向依赖）。
type FileSink interface {
	InsertFiles(ctx context.Context, scanID int64, rows []storage.FileRecord) error
}

// Classifier 对单个文件产出分类。
//
// 接口由消费方 scanner 定义（contracts §2），实现见 internal/classifier；
// 分类在扫描流水线中即时完成（requirements FR-3、§十四 D-5）。
type Classifier interface {
	Classify(path, name, ext string) classifier.Classification
}

// Options 是一次扫描的输入参数，构造后视为只读。
type Options struct {
	// Root 是待扫描的绝对根目录。
	Root string

	// ScanID 是本次扫描在 scans 表中的主键，用于 files 落库归属。
	ScanID int64

	// Config 是扫描配置（已通过 Validate）。
	Config Config

	// Files 是落库出口，必填。
	Files FileSink

	// Logs 是日志出口，可为 nil。
	Logs LogSink
}

// SkipDetail 是一条被跳过项的明细（FR-5：最多 maxSkipDetails 条）。
type SkipDetail struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Result 是一次扫描的汇总统计（FR-5 的数据来源）。
type Result struct {
	Root           string
	FileCount      int64
	DirCount       int64
	TotalSizeBytes int64
	Skipped        int64
	Duration       time.Duration
	Workers        int
	Details        []SkipDetail
}

// Progress 是一次扫描的实时进度快照（不确定型，FR-4）。
//
// 刻意不包含总数、百分比与 ETA：目录遍历结束前总文件数不可知，
// 任何百分比都只能是被编造出来的数字（fast-stat P0-5 同源裁决）。
type Progress struct {
	Files      int64
	Dirs       int64
	SizeBytes  int64
	Skipped    int64
	CurrentDir string
}
