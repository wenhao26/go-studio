package report

import (
	"strconv"
	"strings"
)

// 大小单位换算基数。
//
// 采用 1024（Kibibyte 语义），但按用户习惯显示为 KB/MB/GB 字样。
// 阈值判断一律在字节上完成，避免「先换算再比较」导致的边界抖动。
const (
	kib int64 = 1024
	mib int64 = kib * 1024
	gib int64 = mib * 1024
)

// FormatBytes 把字节数格式化为人类可读字符串。
//
// 规则（与 PRD §二.4 一致）：
//   - < 1 KB：整数 + 千分位 + " Bytes"；
//   - < 1 MB：两位小数 + " KB"；
//   - < 1 GB：两位小数 + " MB"；
//   - 其余：两位小数 + " GB"。
//
// 边界行为（刻意保留、不做「看起来更漂亮」的修正）：1048575 字节按规则属于
// KB 档，会显示为 "1024.00 KB" 而不是 "1.00 MB"。这与 PRD 的档位定义一致。
//
// 负值按 0 处理：调用方不应传入负值，这里做防御性收敛，避免输出 "-1 Bytes"。
func FormatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	switch {
	case n < kib:
		return FormatCount(n) + " Bytes"
	case n < mib:
		return formatScaled(n, kib, "KB")
	case n < gib:
		return formatScaled(n, mib, "MB")
	default:
		return formatScaled(n, gib, "GB")
	}
}

// formatScaled 把字节数换算到指定单位并保留两位小数。
func formatScaled(n, unit int64, suffix string) string {
	return strconv.FormatFloat(float64(n)/float64(unit), 'f', 2, 64) + " " + suffix
}

// FormatCount 把整数格式化为带千分位的字符串，例如 148203 -> "148,203"。
//
// 负值按 0 处理，理由同 FormatBytes。
func FormatCount(n int64) string {
	if n < 0 {
		n = 0
	}
	return insertThousands(strconv.FormatInt(n, 10))
}

// insertThousands 在纯数字字符串中插入千分位分隔符。
func insertThousands(digits string) string {
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
