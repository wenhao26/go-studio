package report

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// JSON 报告的字段命名遵循 docs/development/api.md 的 snake_case 约定，
// 但不套用 HTTP 统一响应体 {code, message, data, trace_id}：code 与 trace_id
// 对本地一次性 CLI 进程没有意义（项目决策 P0-7）。
//
// 大小同时输出「整数字节」与「人类可读字符串」：程序对接方必须能拿到
// 无精度损失的值，而不是去解析 "4.82 GB" 这样的展示字符串。
type jsonReport struct {
	TargetPath   string            `json:"target_path"`
	Status       string            `json:"status"`
	DurationSecs float64           `json:"duration_seconds"`
	Statistics   jsonStatistics    `json:"statistics"`
	Tool         jsonTool          `json:"tool"`
	ErrorDetails []jsonErrorDetail `json:"error_details,omitempty"`
}

// jsonStatistics 是四项统计指标。
type jsonStatistics struct {
	DirectoriesVisited int64  `json:"directories_visited"`
	TotalFiles         int64  `json:"total_files"`
	TotalSizeBytes     int64  `json:"total_size_bytes"`
	TotalSizeHuman     string `json:"total_size_human"`
	SkippedErrors      int64  `json:"skipped_errors"`
}

// jsonTool 标识产生报告的工具与版本，便于下游按版本适配字段。
type jsonTool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// jsonErrorDetail 是一条被跳过错误的明细。
type jsonErrorDetail struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// JSON 把报告输入渲染为缩进 2 空格的 JSON，并以换行结尾。
//
// 错误明细仅在 Verbose 时输出：默认关闭可以让 JSON 体积与目录树的损坏程度无关。
func JSON(in Input) ([]byte, error) {
	rep := jsonReport{
		TargetPath:   in.TargetPath,
		Status:       in.Status.JSONValue(),
		DurationSecs: roundSeconds(in.Snapshot.Duration),
		Statistics: jsonStatistics{
			DirectoriesVisited: in.Snapshot.DirectoriesVisited,
			TotalFiles:         in.Snapshot.FilesFound,
			TotalSizeBytes:     in.Snapshot.TotalSizeBytes,
			TotalSizeHuman:     FormatBytes(in.Snapshot.TotalSizeBytes),
			SkippedErrors:      in.Snapshot.SkippedErrors,
		},
		Tool: jsonTool{Name: toolName, Version: in.ToolVersion},
	}

	if in.Verbose {
		rep.ErrorDetails = make([]jsonErrorDetail, 0, len(in.Snapshot.ErrorDetails))
		for _, detail := range in.Snapshot.ErrorDetails {
			rep.ErrorDetails = append(rep.ErrorDetails, jsonErrorDetail{
				Path:  detail.Path,
				Error: errorText(detail.Err),
			})
		}
	}

	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}
	return append(out, '\n'), nil
}

// roundSeconds 把耗时转换为保留两位小数的秒数。
//
// 先取整再序列化，避免浮点尾数让 JSON 在不同平台上出现无意义的长小数。
func roundSeconds(d time.Duration) float64 {
	secs := d.Seconds()
	if secs < 0 {
		return 0
	}
	return math.Round(secs*100) / 100
}
