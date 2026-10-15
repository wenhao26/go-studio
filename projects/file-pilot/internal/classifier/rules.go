package classifier

import "strings"

// 计分常量（requirements §5.2）。
const (
	// scoreExt 是扩展名命中分；唯一类别，不叠加。
	scoreExt = 60

	// scoreKeyword 是单个关键词命中分；同一类别封顶 scoreKeywordCap。
	scoreKeyword    = 35
	scoreKeywordCap = 70

	// scorePath 是单条路径规则命中分；同一类别封顶 scorePathCap。
	scorePath    = 20
	scorePathCap = 20

	// confidenceCap 是置信度上限（口径 B：跨类别总分封顶 100）。
	confidenceCap = 100
)

// extCategories 是扩展名 → 类别映射（requirements §5.3，唯一类别）。
//
// 财务/工作/学习为纯语义类，无扩展名映射；未列出的扩展名不计分。
var extCategories = map[string]string{
	// 图片
	"jpg": CategoryImages, "jpeg": CategoryImages, "png": CategoryImages,
	"gif": CategoryImages, "bmp": CategoryImages, "webp": CategoryImages,
	"svg": CategoryImages, "ico": CategoryImages, "tif": CategoryImages,
	"tiff": CategoryImages, "heic": CategoryImages, "heif": CategoryImages,

	// 视频
	"mp4": CategoryVideos, "mov": CategoryVideos, "avi": CategoryVideos,
	"mkv": CategoryVideos, "wmv": CategoryVideos, "flv": CategoryVideos,
	"webm": CategoryVideos, "m4v": CategoryVideos, "mpg": CategoryVideos,
	"mpeg": CategoryVideos,

	// 文档
	"pdf": CategoryDocs, "doc": CategoryDocs, "docx": CategoryDocs,
	"xls": CategoryDocs, "xlsx": CategoryDocs, "ppt": CategoryDocs,
	"pptx": CategoryDocs, "txt": CategoryDocs, "md": CategoryDocs,
	"rtf": CategoryDocs, "csv": CategoryDocs, "odt": CategoryDocs,
	"ods": CategoryDocs, "odp": CategoryDocs, "epub": CategoryDocs,
}

// keywordRule 是一个类别的关键词规则（requirements §5.4，对文件名主体匹配）。
//
// keywords 的切片顺序即「原因」并列时的声明顺序（§5.6），勿随意重排。
type keywordRule struct {
	category string
	keywords []string
}

// keywordRules 按 §5.4 表声明顺序排列（财务 → 工作 → 学习）。
var keywordRules = []keywordRule{
	{CategoryFinance, []string{"发票", "账单", "invoice", "工资", "银行", "支付", "报销", "流水", "税"}},
	{CategoryWork, []string{"合同", "项目", "方案", "报告", "会议", "简历", "kpi", "okr"}},
	{CategoryStudy, []string{"课程", "教程", "笔记", "电子书", "习题", "论文"}},
}

// pathRule 是一条路径子串规则（requirements §5.5，对完整路径小写匹配）。
type pathRule struct {
	category string
	token    string
}

// pathRules 按 §5.5 表声明顺序排列（决定「原因」并列取值，§5.6）。
var pathRules = []pathRule{
	{CategoryFinance, "发票"},
	{CategoryFinance, "报销"},
	{CategoryWork, "合同"},
	{CategoryWork, "项目文档"},
	{CategoryStudy, "课程"},
	{CategoryStudy, "教程"},
	{CategoryStudy, "电子书"},
}

// categoryPriority 是「选类优先级」（requirements §5.2 口径 B）。
//
// 与 Categories（枚举/展示/目录顺序）**不同**，禁止互相替代（contracts §3.2）：
// 语义类（财务/工作/学习）天然优先于媒体类型类，故无平局。
var categoryPriority = []string{
	CategoryFinance, CategoryWork, CategoryStudy, CategoryVideos,
	CategoryImages, CategoryDocs, CategoryOther,
}

// hitSource 标识命中项来源，决定「原因」并列时的裁决顺序（§5.6）。
type hitSource int

const (
	sourceExt hitSource = iota
	sourceKeyword
	sourcePath
)

// hit 是一条命中项；order 是同类规则内的声明顺序（保证并列时确定性）。
type hit struct {
	category string
	score    int
	source   hitSource
	token    string // 原因展示用：扩展名 / 关键词 / 路径子串
	order    int
}

// Engine 是完整规则引擎（requirements §5.2 口径 B）。
//
// 规则表为包级常量，Engine 无状态，可并发安全使用。
type Engine struct{}

// New 返回规则引擎。
func New() *Engine { return &Engine{} }

// Classify 按 §5.2 口径 B 对单个文件分类。
//
// 纯函数：不读文件系统、不感知并发；输入 path/name/ext（ext 由调用方小写去点），
// 输出（category, confidence, reason）。匹配前统一小写化。
func (Engine) Classify(path, name, ext string) Classification {
	hits := collectHits(path, name, ext)

	category := chooseCategory(hits)
	if category == "" {
		// matched 为空 → 类别「其他」、置信度 0，不编造数字（§5.2 步骤 2）。
		return Classification{
			Category:   CategoryOther,
			Confidence: 0,
			Source:     SourceRule,
			Reason:     reasonNoMatch,
		}
	}

	return Classification{
		Category:   category,
		Confidence: confidence(hits),
		Source:     SourceRule,
		Reason:     buildReason(hits, category),
	}
}

// IsValidCategory 报告 c 是否为固定 7 类之一（FR-8 类别校验）。
func IsValidCategory(c string) bool {
	for _, category := range Categories {
		if category == c {
			return true
		}
	}
	return false
}

// collectHits 汇总扩展名 / 关键词 / 路径三类命中项（requirements §5.2）。
func collectHits(path, name, ext string) []hit {
	var hits []hit

	// 扩展名命中（+60，唯一类别）。
	if category, ok := extCategories[ext]; ok {
		hits = append(hits, hit{category: category, score: scoreExt, source: sourceExt, token: ext})
	}

	// 关键词命中（每个 +35）：对不含扩展名的小写文件名做子串匹配。
	base := lowerBaseName(name, ext)
	for _, rule := range keywordRules {
		for i, kw := range rule.keywords {
			if strings.Contains(base, kw) {
				hits = append(hits, hit{
					category: rule.category,
					score:    scoreKeyword,
					source:   sourceKeyword,
					token:    kw,
					order:    i,
				})
			}
		}
	}

	// 路径命中（每条 +20）：对文件**所在目录**（不含文件名）的小写化路径做子串匹配。
	//
	// 只取目录部分：requirements §5.5 的规则名为「所在路径」，且 §5.2 校验示例
	// `合同.pdf → 工作 95%` 要求文件名本身不产生路径命中（否则会同时经关键词与
	// 路径命中，得 100 而非 95）。见 development/02-phase-2-classification.md §6（D-C3）。
	lowerDir := strings.ToLower(parentPath(path))
	for i, rule := range pathRules {
		if strings.Contains(lowerDir, rule.token) {
			hits = append(hits, hit{
				category: rule.category,
				score:    scorePath,
				source:   sourcePath,
				token:    rule.token,
				order:    i,
			})
		}
	}

	return hits
}

// parentPath 返回 path 的所在目录部分（截到最后一个路径分隔符，含 `/` 与 `\`）。
//
// 手动按两种分隔符处理，不依赖 os 相关的 filepath：分类是纯函数，且测试与
// 生产路径可能混用两种分隔符（Windows 两种都合法），需保证跨平台确定性。
func parentPath(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

// lowerBaseName 去掉扩展名后缀并小写化（关键词只匹配文件名主体，§5.2）。
func lowerBaseName(name, ext string) string {
	if ext != "" {
		suffix := "." + ext
		if len(name) >= len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix) {
			name = name[:len(name)-len(suffix)]
		}
	}
	return strings.ToLower(name)
}

// chooseCategory 在命中类别中按优先级取最高（§5.2 步骤 3）；无命中返回空串。
func chooseCategory(hits []hit) string {
	if len(hits) == 0 {
		return ""
	}
	matched := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		matched[h.category] = struct{}{}
	}
	for _, c := range categoryPriority {
		if _, ok := matched[c]; ok {
			return c
		}
	}
	return ""
}

// confidence 汇总所有命中项分数并封顶 100（§5.2 步骤 4）：
// 每类贡献 = 扩展名 + min(关键词合计, +70) + min(路径合计, +20)。
func confidence(hits []hit) int {
	type contribution struct{ ext, keyword, path int }
	per := make(map[string]*contribution, len(hits))
	for _, h := range hits {
		c := per[h.category]
		if c == nil {
			c = &contribution{}
			per[h.category] = c
		}
		switch h.source {
		case sourceExt:
			c.ext = scoreExt
		case sourceKeyword:
			c.keyword += scoreKeyword
		case sourcePath:
			c.path += scorePath
		}
	}

	total := 0
	for _, c := range per {
		total += c.ext + min(c.keyword, scoreKeywordCap) + min(c.path, scorePathCap)
	}
	return min(total, confidenceCap)
}
