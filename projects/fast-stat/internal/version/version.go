// Package version 暴露构建期注入的版本信息。
//
// 版本号通过 -ldflags "-X .../internal/version.Version=..." 注入（见 Makefile）。
// 源码中的默认值只在 go run、go test 或未注入的构建中出现，用于明确表示
// 「非正式构建」，而不是伪造的版本数据。
package version

// 构建期可覆盖的版本变量。
var (
	// Version 是语义化版本号，例如 "1.0.0"。
	Version = "dev"

	// Commit 是构建时的 Git 提交短哈希。
	Commit = "none"

	// Date 是构建时间，UTC RFC3339 格式。
	Date = "unknown"
)

// String 返回形如 "1.0.0 (commit abc1234)" 的版本描述。
//
// 当提交哈希未注入时只返回版本号，避免输出 "dev (commit none)" 这类噪音。
func String() string {
	if Commit == "" || Commit == "none" {
		return Version
	}
	return Version + " (commit " + Commit + ")"
}
