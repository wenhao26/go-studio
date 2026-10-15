package storage

// 时间列约定：全部为 UTC 毫秒整数（contracts §1），未发生时为 0，
// 数据库中对应可空列为 NULL。

// TaskRow 是 tasks 表的一行。
type TaskRow struct {
	ID           int64
	Type         string // scan / organize
	Status       string // pending / running / completed / failed / canceled
	ScanID       int64  // 0 表示未关联扫描
	ParentTaskID int64  // 0 表示无父任务（撤销任务指向被撤销的整理任务，P1-1）
	StartedAt    int64
	FinishedAt   int64 // 0 表示未结束
	Error        string
	Undone       bool // 原整理任务是否已撤销（FR-11 判重）
}

// ScanRow 是 scans 表的一行（查询结果）。
type ScanRow struct {
	ID             int64
	RootPath       string
	Status         string
	StartedAt      int64
	FinishedAt     int64
	DurationMS     int64
	FileCount      int64
	DirCount       int64
	TotalSizeBytes int64
	SkippedCount   int64
	AvgSpeed       float64
	MaxWorkers     int
}

// ScanOutcome 是扫描记录的终态统计，由 task 层计算后传入（FinishScan）。
type ScanOutcome struct {
	Status         string
	FinishedAt     int64
	DurationMS     int64
	FileCount      int64
	DirCount       int64
	TotalSizeBytes int64
	SkippedCount   int64
	AvgSpeed       float64
}

// FileRecord 是扫描流水线写入 files 表的一行（scanner 构造、storage 落库）。
//
// Hidden/System 不落库：schema 无对应列，它们只用于扫描期的跳过判定
// （requirements FR-2/FR-3 采集它们的目的是过滤，不是展示）。
type FileRecord struct {
	Path       string
	Name       string
	Ext        string // 小写、不含点
	SizeBytes  int64
	MtimeMS    int64
	CtimeMS    int64
	HasCtime   bool // false 时 ctime 写 NULL（best-effort，requirements §十五 R-2）
	Category   string
	Confidence int    // Phase 1 恒为规则来源的 0–100；用户手动为 NULL（Phase 2/FR-8）
	Source     string // rule / user
	Reason     string
}

// FileRow 是分页查询返回的一行（含 NULL 语义）。
type FileRow struct {
	ID         int64
	Path       string
	Name       string
	SizeBytes  int64
	Category   string
	Confidence *int64 // NULL = 用户手动指定（FR-8）
	Source     string
	Reason     string
	Excluded   bool
}

// FilePlanRow 是单文件的整理计划字段（FR-8 读取当前值以判定是否改类别）。
type FilePlanRow struct {
	ID         int64
	Category   string
	Source     string
	Confidence *int64 // NULL = 用户手动指定
	Reason     string
	Excluded   bool
}

// FilePlanUpdate 是一次整理计划写入（FR-8：category/source/confidence/reason/excluded）。
type FilePlanUpdate struct {
	ID         int64
	Category   string
	Source     string // rule / user
	Confidence *int64 // nil 落库为 NULL（用户手动）
	Reason     string
	Excluded   bool
}

// OrganizeCandidate 是一个待整理文件（FR-9：仅 excluded=0）。
//
// 供 organizer 构建整理计划；只取计划所需的字段，不读 confidence/reason 等
// 展示字段（AGENTS.md §22.2：不读无关列）。
type OrganizeCandidate struct {
	ID        int64
	Path      string
	Name      string
	SizeBytes int64
	Category  string
}

// OperationRecord 是一条文件移动/撤销记录（operations 表，FR-9/FR-11）。
//
// Action 为 move（整理）或 undo（撤销）；Status 为 success/failed。
// SizeBytes 仅 move 成功项有意义，用于「已移动数据量」聚合（P1-10）。
type OperationRecord struct {
	TaskID    int64
	Action    string // move / undo
	SrcPath   string
	DstPath   string
	Status    string // success / failed
	Error     string
	SizeBytes int64
	CreatedAt int64 // UTC 毫秒
}
