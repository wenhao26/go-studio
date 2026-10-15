package classifier

import "fmt"

// 面向用户展示的原因短语（requirements §5.6）。
const (
	// reasonNoMatch 是零命中时的原因；分类日志与详情列表复用同一字符串。
	reasonNoMatch = "未命中任何规则"

	// ReasonManual 是用户手动指定类别时的原因（FR-8）。
	// 由落库层写入，保证与 §5.6 格式一致、前后端同一字符串。
	ReasonManual = "用户手动指定"
)

// buildReason 取「决定该类别的命中项」中分值最高者作为原因（§5.6）。
//
// 注意是「决定类别」的命中项（即属于 category 的命中项），而非全局最高分：
// 例如 `合同.pdf` 的扩展名分（60 → 文档）高于关键词分（35 → 工作），但决定
// 类别的是关键词（工作优先级更高），故原因为 `命中关键词: 合同`。
//
// 并列时先按「扩展名 > 关键词 > 路径」，仍并列则按规则表声明顺序取第一个。
func buildReason(hits []hit, category string) string {
	best := hit{score: -1}
	for _, h := range hits {
		if h.category != category {
			continue
		}
		if betterHit(h, best) {
			best = h
		}
	}
	return reasonText(best)
}

// betterHit 报告 a 是否比 b 更适合作为原因（分值 → 来源 → 声明顺序）。
func betterHit(a, b hit) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if a.source != b.source {
		return a.source < b.source
	}
	return a.order < b.order
}

// reasonText 把命中项格式化为展示短语（§5.6）。
func reasonText(h hit) string {
	if h.score < 0 {
		// 理论不可达（调用方只在有命中时进入），防御性兜底避免编造原因。
		return reasonNoMatch
	}
	switch h.source {
	case sourceExt:
		return fmt.Sprintf("扩展名 .%s → %s", h.token, h.category)
	case sourceKeyword:
		return fmt.Sprintf("命中关键词: %s", h.token)
	case sourcePath:
		return fmt.Sprintf("路径命中: %s", h.token)
	default:
		return reasonNoMatch
	}
}
