// Package app 是装配层：把 storage/task/scanner/diag 组合成 Wails 绑定与
// 生命周期宿主（组合根细节在 main.go，本包对象即被 Bind 的对象）。
//
// 职责边界（AGENTS.md §20）：绑定方法只做参数解析、校验、调用业务层、
// DTO 转换与错误映射；扫描流程编排在 run.go，设置持久化在 settings.go。
package app

import "errors"

// 哨兵错误（contracts §4）：Wails 边界返回的稳定错误键。
//
// 绑定方法命中哨兵时一律返回**裸哨兵**（其 message 即前端匹配键），
// 内部包装过的完整错误链只写诊断日志（contracts §4「Wails 边界错误形态」）。
var (
	// ErrDirectoryPickCanceled 用户取消目录选择。
	ErrDirectoryPickCanceled = errors.New("directory pick canceled")

	// ErrDirNotFound 目录不存在。
	ErrDirNotFound = errors.New("directory not found")

	// ErrDirNotDirectory 路径存在但不是目录。
	ErrDirNotDirectory = errors.New("path is not a directory")

	// ErrConfigInvalid 配置非法（如持久化的 scan_config 损坏）。
	ErrConfigInvalid = errors.New("invalid scan config")

	// ErrCategoryInvalid 类别非法（不属于固定 7 类，Phase 2 / FR-8）。
	ErrCategoryInvalid = errors.New("invalid category")

	// ErrFileNotInScan 文件不存在于任一扫描（Phase 2 / FR-8）。
	//
	// 落地口径（D-C2）：SetFilePlan 签名不含 scanID，故只校验 fileID 是否存在，
	// 归属由结果页 UI 保证；此哨兵表示「目标文件不存在」。
	ErrFileNotInScan = errors.New("file not in scan")

	// ErrScanIncomplete 扫描未完成（Phase 3 / FR-9）。
	//
	// StartOrganize 拒绝未完成（completed）的扫描：部分结果不可整理。
	ErrScanIncomplete = errors.New("scan incomplete")

	// ErrNothingToUndo 没有可撤销的整理（Phase 3 / FR-11）。
	//
	// 最近一次整理任务不存在或其成功移动为 0。
	ErrNothingToUndo = errors.New("nothing to undo")

	// ErrAlreadyUndone 整理已撤销（Phase 3 / FR-11）。
	//
	// 最接近的可撤销整理任务已经标记 undone。
	ErrAlreadyUndone = errors.New("already undone")

	// errNotStarted 绑定在应用启动完成前被调用：这不是业务错误，
	// 前端不应有对应入口，故不入 contracts §4 映射表。
	errNotStarted = errors.New("app not started")
)

// bareError 把命中哨兵的错误化为裸哨兵返回（contracts §4：message 即前端
// 匹配键）；未命中的错误原样返回，前端展示通用文案。
//
// 完整错误链由调用点写入诊断日志，此处刻意不再携带上下文。
func bareError(err error, sentinels ...error) error {
	for _, s := range sentinels {
		if errors.Is(err, s) {
			return s
		}
	}
	return err
}
