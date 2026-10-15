// Package classifier 实现文件分类规则引擎（requirements §五）。
//
// Phase 1 仅提供「扩展名占位实现」用于打通扫描流水线的 files.category 数据流；
// 完整规则引擎（关键词 + 路径 + 口径 B 选类，requirements §5.2）在 Phase 2 落地，
// 届时占位实现必须整体替换（development/01-phase-1-scan.md §1、§6）。
package classifier

// 类别常量：固定 7 类，值即目标目录名与 files.category 取值（requirements §5.1）。
const (
	CategoryImages  = "图片"
	CategoryVideos  = "视频"
	CategoryDocs    = "文档"
	CategoryFinance = "财务"
	CategoryWork    = "工作"
	CategoryStudy   = "学习"
	CategoryOther   = "其他"
)

// Categories 是固定 7 类的**枚举 / 展示 / 目录顺序**（requirements §5.1）。
//
// 注意：它与「选类优先级」（§5.2 口径 B：财务 > 工作 > 学习 > 视频 > 图片 > 文档 > 其他）
// 是两个不同顺序，选类优先级必须由 Phase 2 引擎显式定义，禁止用本切片的顺序代替
// （development/contracts.md §3.2）。
var Categories = []string{
	CategoryImages,
	CategoryVideos,
	CategoryDocs,
	CategoryFinance,
	CategoryWork,
	CategoryStudy,
	CategoryOther,
}

// files.source 的取值（requirements §七）。
const (
	SourceRule = "rule"
	SourceUser = "user"
)

// Classification 是一次分类产出，对应 files 表的 category/confidence/source/reason 列。
type Classification struct {
	// Category 是 7 类之一（Categories）。
	Category string

	// Confidence 是 0–100 整数；用户手动指定时落库层写 NULL（Phase 2 / FR-8）。
	Confidence int

	// Source 是 SourceRule 或 SourceUser。
	Source string

	// Reason 是面向用户展示的短语（requirements §5.6），UI 与日志复用同一字符串。
	Reason string
}
