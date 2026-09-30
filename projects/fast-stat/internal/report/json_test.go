package report

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/wenhao26/go-studio/projects/fast-stat/internal/scan"
)

// wireReport 独立于实现类型镜像 JSON 契约。
//
// 刻意重复定义结构体：这样测试验证的是「对外字段契约」而不是内部字段名，
// 内部结构体重命名不会让测试静默通过。
type wireReport struct {
	TargetPath      string  `json:"target_path"`
	Status          string  `json:"status"`
	DurationSeconds float64 `json:"duration_seconds"`
	Statistics      struct {
		DirectoriesVisited int64  `json:"directories_visited"`
		TotalFiles         int64  `json:"total_files"`
		TotalSizeBytes     int64  `json:"total_size_bytes"`
		TotalSizeHuman     string `json:"total_size_human"`
		SkippedErrors      int64  `json:"skipped_errors"`
	} `json:"statistics"`
	Tool struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"tool"`
	ErrorDetails []struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	} `json:"error_details"`
}

func TestJSON_Contract(t *testing.T) {
	out, err := JSON(sampleInput())
	if err != nil {
		t.Fatalf("JSON() 返回错误: %v", err)
	}
	if !json.Valid(out) {
		t.Fatalf("输出不是合法 JSON: %s", out)
	}
	if !strings.HasSuffix(string(out), "\n") {
		t.Error("JSON 输出应以换行结尾")
	}

	var got wireReport
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if got.TargetPath != "/data/projects" {
		t.Errorf("target_path = %q", got.TargetPath)
	}
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.DurationSeconds != 1.24 {
		t.Errorf("duration_seconds = %v, want 1.24", got.DurationSeconds)
	}
	if got.Statistics.DirectoriesVisited != 12450 {
		t.Errorf("directories_visited = %d", got.Statistics.DirectoriesVisited)
	}
	if got.Statistics.TotalFiles != 148203 {
		t.Errorf("total_files = %d", got.Statistics.TotalFiles)
	}
	if got.Statistics.TotalSizeBytes != 5175123456 {
		t.Errorf("total_size_bytes = %d", got.Statistics.TotalSizeBytes)
	}
	if got.Statistics.TotalSizeHuman != "4.82 GB" {
		t.Errorf("total_size_human = %q", got.Statistics.TotalSizeHuman)
	}
	if got.Statistics.SkippedErrors != 3 {
		t.Errorf("skipped_errors = %d", got.Statistics.SkippedErrors)
	}
	if got.Tool.Name != "faststat" {
		t.Errorf("tool.name = %q", got.Tool.Name)
	}
	if got.Tool.Version != "1.0.0" {
		t.Errorf("tool.version = %q", got.Tool.Version)
	}
	if len(got.ErrorDetails) != 0 {
		t.Errorf("非 verbose 模式不应输出 error_details，实际 %d 条", len(got.ErrorDetails))
	}
}

// TestJSON_DoesNotUseHTTPEnvelope 固化决策 P0-7。
//
// 本地 CLI 的 JSON 报告是「纯数据」，不套用 HTTP 统一响应体；
// 若有人后续加了 code/message/trace_id，该测试会拦住。
func TestJSON_DoesNotUseHTTPEnvelope(t *testing.T) {
	out, err := JSON(sampleInput())
	if err != nil {
		t.Fatalf("JSON() 返回错误: %v", err)
	}

	for _, forbidden := range []string{"trace_id", `"code"`, `"message"`, `"data"`} {
		if strings.Contains(string(out), forbidden) {
			t.Errorf("JSON 报告中不应出现 HTTP 响应体字段 %s", forbidden)
		}
	}
}

func TestJSON_CanceledStatus(t *testing.T) {
	in := sampleInput()
	in.Status = StatusCanceled

	out, err := JSON(in)
	if err != nil {
		t.Fatalf("JSON() 返回错误: %v", err)
	}

	var got wireReport
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if got.Status != "canceled" {
		t.Errorf("status = %q, want canceled", got.Status)
	}
}

func TestJSON_VerboseErrorDetails(t *testing.T) {
	in := sampleInput()
	in.Verbose = true
	in.Snapshot.ErrorDetails = []scan.ScanError{
		{Path: "/data/secret", Err: fs.ErrPermission},
	}

	out, err := JSON(in)
	if err != nil {
		t.Fatalf("JSON() 返回错误: %v", err)
	}

	var got wireReport
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if len(got.ErrorDetails) != 1 {
		t.Fatalf("error_details 条数 = %d, want 1", len(got.ErrorDetails))
	}
	if got.ErrorDetails[0].Path != "/data/secret" {
		t.Errorf("error_details[0].path = %q", got.ErrorDetails[0].Path)
	}
	if got.ErrorDetails[0].Error == "" {
		t.Error("error_details[0].error 不应为空")
	}
}

// TestJSON_VerboseWithoutErrorsOmitsField 覆盖 verbose 但无错误的分支：
// 空切片配合 omitempty 应当整体省略该字段。
func TestJSON_VerboseWithoutErrorsOmitsField(t *testing.T) {
	in := sampleInput()
	in.Verbose = true
	in.Snapshot.ErrorDetails = nil

	out, err := JSON(in)
	if err != nil {
		t.Fatalf("JSON() 返回错误: %v", err)
	}
	if strings.Contains(string(out), "error_details") {
		t.Error("无错误明细时不应输出 error_details 字段")
	}
}
