// 展示层格式化（requirements-analysis §2.5；口径与后端 internal/units 对齐）。
//
// 后端只在日志文本里做格式化；统计面板/进度面板的字符串由前端生成，
// 故此处独立实现同一口径，避免两端显示不一致。

const KIB = 1024
const MIB = KIB * 1024
const GIB = MIB * 1024

// withThousands 给整数加千分位（148203 → "148,203"）。
function withThousands(value: number): string {
  return Math.trunc(value).toLocaleString('en-US')
}

// formatSize 把字节数格式化为人类可读字符串（§2.5：基数 1024）。
//
// 阈值比较在字节上做，与 internal/units 一致：1048575 字节显示为
// "1024.00 KB" 而非 "1.00 MB"，避免换算后的边界抖动。
export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) {
    return '0 Bytes'
  }
  if (bytes < KIB) {
    return `${withThousands(bytes)} Bytes`
  }
  if (bytes < MIB) {
    return `${(bytes / KIB).toFixed(2)} KB`
  }
  if (bytes < GIB) {
    return `${(bytes / MIB).toFixed(2)} MB`
  }
  return `${(bytes / GIB).toFixed(2)} GB`
}

// formatDuration 格式化耗时（§2.5）：≥1s 两位小数秒；<1s 一位小数毫秒。
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) {
    return '0.0 毫秒'
  }
  if (ms < 1000) {
    return `${ms.toFixed(1)} 毫秒`
  }
  return `${(ms / 1000).toFixed(2)} 秒`
}

// formatCount 给计数加千分位（已扫描文件数、跳过数等）。
export function formatCount(value: number): string {
  if (!Number.isFinite(value)) {
    return '0'
  }
  return withThousands(value)
}

// formatRate 格式化扫描速率：files/s 取整（§2.5 建议取整）。
export function formatRate(filesPerSecond: number): string {
  if (!Number.isFinite(filesPerSecond) || filesPerSecond < 0) {
    return '0'
  }
  return withThousands(Math.round(filesPerSecond))
}

// formatLogTime 格式化日志时间戳为本地 HH:mm:ss。
//
// 入参是 UTC 毫秒整数（contracts §1），展示层按本地时区格式化；用 tabular-nums
// 让数字等宽、行间对齐。无效戳（<= 0）显示占位符，不假设 ts 一定有效。
export function formatLogTime(ts: number): string {
  if (!Number.isFinite(ts) || ts <= 0) {
    return '--:--:--'
  }
  const d = new Date(ts)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
}
