package version

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{
			name:    "injected build info",
			version: "1.0.0",
			commit:  "abc1234",
			want:    "1.0.0 (commit abc1234)",
		},
		{
			name:    "default commit placeholder is omitted",
			version: "dev",
			commit:  "none",
			want:    "dev",
		},
		{
			name:    "empty commit is omitted",
			version: "1.0.0",
			commit:  "",
			want:    "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 保存并恢复包级变量，避免用例之间互相污染。
			origVersion, origCommit := Version, Commit
			t.Cleanup(func() {
				Version, Commit = origVersion, origCommit
			})

			Version, Commit = tt.version, tt.commit

			if got := String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if !strings.Contains(String(), tt.version) {
				t.Errorf("String() 应包含版本号 %q", tt.version)
			}
		})
	}
}

// TestDefaults 固定默认值语义：默认值必须显式表示「非正式构建」，
// 而不是伪造一个看起来真实的版本号。
func TestDefaults(t *testing.T) {
	if Version == "" {
		t.Error("Version 默认值不应为空")
	}
	if Commit != "none" {
		t.Errorf("Commit 默认值 = %q, 期望 none", Commit)
	}
	if Date != "unknown" {
		t.Errorf("Date 默认值 = %q, 期望 unknown", Date)
	}
}
