// Package units 提供跨阶段共用的「数值 → 展示字符串」格式化。
//
// 单独成包而非塞进 utils（AGENTS.md §56）：这里只放需求已裁决口径的大小格式化，
// 由产生日志文本的后端模块共用（scanner；Phase 3 起含 organizer）。
// 前端展示另行格式化（requirements §2.5），不走本包。
package units

import (
	"strconv"
	"strings"
)

// 大小单位换算基数（requirements §2.5：基数 1024，沿用 fast-stat P0-2/P0-4 口径）。
const (
	kib int64 = 1024
	mib int64 = kib * 1024
	gib int64 = mib * 1024
)

// FormatBytes 把字节数格式化为人类可读字符串（口径与 fast-stat FormatBytes 一致）：
//
//   - < 1 KB：整数 + 千分位 + " Bytes"（D-9）；
//   - < 1 MB：两位小数 + " KB"；
//   - < 1 GB：两位小数 + " MB"；
//   - 其余：两位小数 + " GB"。
//
// 边界行为刻意保留：1048575 字节按档位定义显示为 "1024.00 KB" 而非 "1.00 MB"
// （先换算再比较会引入边界抖动，阈值一律在字节上比较）。
//
// 负值按 0 收敛：调用方不应传入负值，防御性处理避免输出 "-1 Bytes"。
func FormatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	switch {
	case n < kib:
		return formatCount(n) + " Bytes"
	case n < mib:
		return formatScaled(n, kib, "KB")
	case n < gib:
		return formatScaled(n, mib, "MB")
	default:
		return formatScaled(n, gib, "GB")
	}
}

// formatScaled 换算到指定单位并保留两位小数。
func formatScaled(n, unit int64, suffix string) string {
	return strconv.FormatFloat(float64(n)/float64(unit), 'f', 2, 64) + " " + suffix
}

// formatCount 把整数格式化为带千分位的字符串（148203 → "148,203"）。
func formatCount(n int64) string {
	digits := strconv.FormatInt(n, 10)
	if len(digits) <= 3 {
		return digits
	}

	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}

	var b strings.Builder
	b.Grow(len(digits) + (len(digits)-1)/3)
	b.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}
