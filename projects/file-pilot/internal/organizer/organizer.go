// Package organizer 实现文件整理的执行与撤销（requirements FR-9/FR-10/FR-11）。
//
// 职责边界（AGENTS.md §23）：本包只负责「计划校验 + 顺序移动 + 反向移动」，
// 不查询扫描/文件表（候选由 app 层读取后传入），不管理任务状态机（由 task
// 包负责），不渲染报告 DTO（由 app 层聚合）。持久化能力以 OperationStore
// 接口声明（contracts §7，消费方定义），由 storage.Store 结构化实现。
//
// 安全约束（SEC-1/SEC-3，详见 development/03-phase-3-organize-undo.md）：
//   - 绝不覆盖：目标名先经存在性检查取第一个空闲变体，且该检查紧邻 rename
//     （Windows 上 os.Rename 会覆盖已存在目标，不能依赖 rename 失败）；
//   - 目标路径必须落在 <扫描根>/FilePilot整理 之下，防止越界写入；
//   - 顺序执行，单条失败不影响后续（FR-9.5）。
package organizer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/classifier"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/fsx"
	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// ErrTargetDirCreate 表示整理目标目录无法创建（contracts §4）。
//
// 由 app 层在整理启动前的同步预检命中并原样返回（裸哨兵），保证「零移动」。
var ErrTargetDirCreate = errors.New("create target dir failed")

// dirPerm 是整理目标目录的创建权限（仅用于本地工具，取常规目录权限）。
const dirPerm fs.FileMode = 0o755

// 操作状态（与 storage 的 OperationSuccess/OperationFailed 一致）。
const (
	StatusSuccess = storage.OperationSuccess
	StatusFailed  = storage.OperationFailed
)

// FS 是 organizer 需要的最小文件系统能力（消费方定义，contracts §7）。
//
// 由 fsx.OSFileSystem 满足；测试可注入假实现以稳定构造权限/占用等错误分支。
type FS interface {
	// Lstat 返回 path 的元数据且不跟随符号链接。
	Lstat(path string) (fs.FileInfo, error)

	// Rename 执行同卷改名。⚠️ Windows 上会覆盖已存在目标，调用方必须先做
	// 存在性检查（见包注释）。
	Rename(oldpath, newpath string) error

	// MkdirAll 递归创建目录（已存在时返回 nil）。
	MkdirAll(path string, perm fs.FileMode) error
}

// OperationStore 是 organizer 需要的操作记录持久化能力（contracts §7）。
//
// 由 storage.Store 结构化实现；参数类型用 storage.OperationRecord，
// 与 scanner.FileSink 使用 storage.FileRecord 同款约定（AGENTS.md §87）。
type OperationStore interface {
	// AppendOperation 追加一条 move/undo 记录（FR-9：逐条同步写）。
	AppendOperation(ctx context.Context, op storage.OperationRecord) error

	// ListSuccessfulMoves 返回某任务全部成功 move（撤销数据源，FR-11）。
	ListSuccessfulMoves(ctx context.Context, taskID int64) ([]storage.OperationRecord, error)
}

// Candidate 已移除：整理候选直接使用 storage.OrganizeCandidate（见 BuildPlan）。

// Item 是整理计划中的一条。
type Item struct {
	// Src 是源文件绝对路径。
	Src string
	// DstDir 是目标类别目录（<扫描根>/FilePilot整理/<类别>）。
	DstDir string
	// Name 是期望文件名（冲突时执行期改写）。
	Name string
	// SizeBytes 是文件大小，用于「已移动数据量」。
	SizeBytes int64
	// Category 是目标类别。
	Category string
}

// Plan 是一次整理的完整计划。
type Plan struct {
	// TargetRoot 是整理根目录 <扫描根>/FilePilot整理。
	TargetRoot string
	// Items 按类别、id 升序（storage.ListOrganizeCandidates 的顺序）。
	Items []Item
}

// ItemResult 是单条移动/撤销的结果（供进度与日志）。
type ItemResult struct {
	// SrcPath/DstPath 是本次物理移动的起止路径。
	// 整理时 Src=原文件、Dst=整理后位置；撤销时 Src=整理后位置、Dst=原位置。
	SrcPath string
	DstPath string
	// Category 仅整理时非空（目标类别）。
	Category string
	// SizeBytes 是文件大小。
	SizeBytes int64
	// Status 是 StatusSuccess 或 StatusFailed。
	Status string
	// Error 是失败原因（成功时为空）。
	Error string
}

// Report 是一次整理/撤销的汇总（供 app 层组装 OrganizeReport）。
type Report struct {
	Success        int64
	Failed         int64
	MovedSizeBytes int64
	// Canceled 表示执行因 context 取消而提前结束（已完成项保留）。
	Canceled bool
}

// Hooks 是执行期回调（均可为 nil）。
type Hooks struct {
	// OnTotal 在执行开始前报告待处理总数（撤销任务在读取记录后才知道）。
	OnTotal func(total int64)
	// OnItem 在每条处理完成后调用。
	OnItem func(r ItemResult)
}

// Organizer 执行整理与撤销。并发安全：单活动任务约束保证同一实例串行使用。
type Organizer struct {
	fs          FS
	ops         OperationStore
	now         func() time.Time
	maxConflict int
}

// New 构造 Organizer。
//
// fs/ops 为 nil 时使用生产默认（fsx.OSFileSystem / 由 app 注入 storage）。
func New(fs FS, ops OperationStore) *Organizer {
	if fs == nil {
		fs = fsx.OSFileSystem{}
	}
	return &Organizer{
		fs:          fs,
		ops:         ops,
		now:         time.Now,
		maxConflict: maxConflictAttempts,
	}
}

// BuildPlan 由扫描根与候选文件构建整理计划，并做路径安全校验（SEC-3）。
//
// 校验失败（非法类别、文件名含路径分隔符、目标越界）返回错误；调用方应拒绝
// 启动整理（零文件系统写）。
//
// candidates 直接用 storage.OrganizeCandidate（候选由 app 从 files 表读取）：
// 该类型已只含计划所需字段，再定义一层同构 DTO 属于无意义抽象（AGENTS.md §71）。
func BuildPlan(root string, candidates []storage.OrganizeCandidate) (Plan, error) {
	if strings.TrimSpace(root) == "" {
		return Plan{}, fmt.Errorf("build plan: empty scan root")
	}
	targetRoot := filepath.Join(root, fsx.OrganizeDirName)

	items := make([]Item, 0, len(candidates))
	for _, c := range candidates {
		if !classifier.IsValidCategory(c.Category) {
			return Plan{}, fmt.Errorf("build plan: invalid category %q for %q", c.Category, c.Name)
		}
		if err := checkBaseName(c.Name); err != nil {
			return Plan{}, fmt.Errorf("build plan: %q: %w", c.Name, err)
		}
		dstDir := filepath.Clean(filepath.Join(targetRoot, c.Category))
		if !within(targetRoot, dstDir) {
			return Plan{}, fmt.Errorf("build plan: target %q escapes %q", dstDir, targetRoot)
		}
		if strings.TrimSpace(c.Path) == "" {
			return Plan{}, fmt.Errorf("build plan: empty source path for %q", c.Name)
		}
		items = append(items, Item{
			Src:       c.Path,
			DstDir:    dstDir,
			Name:      c.Name,
			SizeBytes: c.SizeBytes,
			Category:  c.Category,
		})
	}
	return Plan{TargetRoot: targetRoot, Items: items}, nil
}

// EnsureTargetDirs 创建计划涉及的所有目标类别目录（FR-9 预检）。
//
// 任一目录创建失败即返回 ErrTargetDirCreate；调用方据此在任何移动发生前
// 中止（零移动）。与 Move 分离，使预检可在绑定方法内同步执行。
func (o *Organizer) EnsureTargetDirs(plan Plan) error {
	seen := make(map[string]struct{}, len(classifier.Categories))
	for _, it := range plan.Items {
		if _, ok := seen[it.DstDir]; ok {
			continue
		}
		seen[it.DstDir] = struct{}{}
		if err := o.fs.MkdirAll(it.DstDir, dirPerm); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrTargetDirCreate, it.DstDir, err)
		}
	}
	return nil
}

// Move 顺序执行整理计划（FR-9）。
//
// 每条结果立即写 operations（action=move），保证可撤销/可审计；单条失败记录
// failed 并继续下一条。context 取消时停止后续移动、保留已完成项，返回
// Report.Canceled=true 且 error 为 nil。
//
// 唯一的致命错误是操作记录写入失败：此时已移动的文件会被尽力回移，避免
// 「已移动但不可撤销」的不一致状态（AGENTS.md §22.3 数据一致性）。
func (o *Organizer) Move(ctx context.Context, taskID int64, plan Plan, hooks Hooks) (Report, error) {
	var rep Report
	if hooks.OnTotal != nil {
		hooks.OnTotal(int64(len(plan.Items)))
	}

	for _, it := range plan.Items {
		if err := ctx.Err(); err != nil {
			rep.Canceled = true
			return rep, nil
		}

		res, fatal := o.moveOne(ctx, taskID, it)
		if fatal != nil {
			// 操作记录写入失败：已尽力回移，直接中止。
			return rep, fatal.err
		}
		if res.Status == StatusSuccess {
			rep.Success++
			rep.MovedSizeBytes += it.SizeBytes
		} else {
			rep.Failed++
		}
		if hooks.OnItem != nil {
			hooks.OnItem(res)
		}
	}
	return rep, nil
}

// moveOutcome 承载 moveOne 的结果或致命错误。
type moveOutcome struct {
	err error
}

// moveOne 处理单条移动并写操作记录。返回 (结果, nil) 表示已处理；
// 返回 (零值, 非 nil) 表示操作记录写入失败（致命，调用方应中止）。
func (o *Organizer) moveOne(ctx context.Context, taskID int64, it Item) (ItemResult, *moveOutcome) {
	res := ItemResult{SrcPath: it.Src, Category: it.Category, SizeBytes: it.SizeBytes}

	dst, err := resolveTarget(o.fs, it.DstDir, it.Name, o.maxConflict)
	if err != nil {
		res.Status = StatusFailed
		res.Error = resolveReason(err)
		return res, o.record(ctx, storage.OperationRecord{
			TaskID: taskID, Action: storage.ActionMove, SrcPath: it.Src,
			Status: StatusFailed, Error: res.Error,
		})
	}
	res.DstPath = dst

	if err := o.fs.Rename(it.Src, dst); err != nil {
		res.Status = StatusFailed
		res.Error = moveErrorReason(err)
		return res, o.record(ctx, storage.OperationRecord{
			TaskID: taskID, Action: storage.ActionMove, SrcPath: it.Src, DstPath: dst,
			Status: StatusFailed, Error: res.Error,
		})
	}

	// rename 成功：必须在写记录前先记成功态；写失败则尽力回移（D-D8）。
	if out := o.record(ctx, storage.OperationRecord{
		TaskID: taskID, Action: storage.ActionMove, SrcPath: it.Src, DstPath: dst,
		Status: StatusSuccess, SizeBytes: it.SizeBytes,
	}); out != nil {
		// 尽力回移，失败也不覆盖原始错误（Join 保留两条链）。
		revertErr := o.fs.Rename(dst, it.Src)
		return res, &moveOutcome{err: errors.Join(out.err, revertErr)}
	}

	res.Status = StatusSuccess
	return res, nil
}

// record 写一条操作记录，成功返回 nil，失败返回致命错误。
//
// 使用 context.WithoutCancel（D-D8）：即使任务已被取消，已经发生的移动也
// 必须落库，否则撤销将无法回移该文件（FR-11 正确性优先于取消即时性）。
func (o *Organizer) record(ctx context.Context, op storage.OperationRecord) *moveOutcome {
	op.CreatedAt = o.now().UTC().UnixMilli()
	if err := o.ops.AppendOperation(context.WithoutCancel(ctx), op); err != nil {
		return &moveOutcome{err: fmt.Errorf("organizer: append %s operation: %w", op.Action, err)}
	}
	return nil
}

// within 报告 p 是否位于 root 之内（含 root 本身）。
func within(root, p string) bool {
	root = filepath.Clean(root)
	p = filepath.Clean(p)
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(filepath.Separator))
}

// checkBaseName 校验 name 是一个纯文件名（不含路径分隔符、非空、非 . / ..）。
func checkBaseName(name string) error {
	if name == "" {
		return fmt.Errorf("empty file name")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("reserved file name %q", name)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("file name contains path separator: %q", name)
	}
	return nil
}
