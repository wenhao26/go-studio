package organizer

import (
	"context"
	"errors"
	"io/fs"

	"github.com/wenhao26/go-studio/projects/file-pilot/internal/storage"
)

// Undo 顺序执行撤销：按原整理任务的成功 move 记录逐条反向移动（dst → src）。
//
// 算法（analysis §2.3）：
//  1. 读取原任务的全部成功 move（operation.action='move' AND status='success'），按 id 升序；
//  2. 对每条：检查 dst 存在且 src 不存在 → 执行 Rename(dst, src)；
//     否则记录失败并继续；
//  3. 每条移动结果写 operations(action='undo')，src_path=原 dst，dst_path=原 src；
//  4. 取消时提前结束，返回 Canceled=true（已完成项保留）。
//
// 注意：撤销不改变原整理任务的 undone 状态，由 app 层根据任务终态决定何时将
// 原任务标记为 undone=1（D-D6）。
func (o *Organizer) Undo(ctx context.Context, undoTaskID int64, moves []storage.OperationRecord, hooks Hooks) (Report, error) {
	var rep Report
	if hooks.OnTotal != nil {
		hooks.OnTotal(int64(len(moves)))
	}

	for _, mv := range moves {
		if err := ctx.Err(); err != nil {
			rep.Canceled = true
			return rep, nil
		}

		res := o.undoOne(ctx, undoTaskID, mv)
		if res.Status == StatusSuccess {
			rep.Success++
			rep.MovedSizeBytes += mv.SizeBytes
		} else {
			rep.Failed++
		}
		if hooks.OnItem != nil {
			hooks.OnItem(res)
		}
	}
	return rep, nil
}

// undoOne 处理单条撤销并写 undo 操作记录。永不返回致命错误（写失败时仍记录
// failed 并继续），以保证撤销过程对部分完成仍有效（FR-11）。
func (o *Organizer) undoOne(ctx context.Context, undoTaskID int64, mv storage.OperationRecord) ItemResult {
	// Undo 物理移动：原整理是 mv.SrcPath → mv.DstPath，撤销是 mv.DstPath → mv.SrcPath。
	srcUndo := mv.DstPath // 物理源（已整理位置）
	dstUndo := mv.SrcPath // 物理目标（原位置）

	res := ItemResult{
		SrcPath:   srcUndo,
		DstPath:   dstUndo,
		SizeBytes: mv.SizeBytes,
	}

	// 1. dst（原位置）是否已被占用？
	if _, err := o.fs.Lstat(dstUndo); err == nil {
		res.Status = StatusFailed
		res.Error = "原路径已被占用"
	} else if !errors.Is(err, fs.ErrNotExist) {
		res.Status = StatusFailed
		res.Error = "无法读取原路径"
	} else {
		// 2. src（整理后位置）是否存在？
		if _, err := o.fs.Lstat(srcUndo); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				res.Status = StatusFailed
				res.Error = "目标不存在"
			} else {
				res.Status = StatusFailed
				res.Error = "无法读取目标文件"
			}
		} else {
			// 3. 执行反向移动
			if rerr := o.fs.Rename(srcUndo, dstUndo); rerr != nil {
				res.Status = StatusFailed
				res.Error = moveErrorReason(rerr)
			} else {
				res.Status = StatusSuccess
			}
		}
	}

	// 写 undo 操作记录。此处刻意忽略写失败（AGENTS.md §9.1 例外）：
	// FR-11 要求撤销对「部分完成」仍有效，若因单条写库失败而中止，反而会
	// 留下「已物理回移但任务未收尾」的半途状态；持续的写库故障会在
	// 任务终态落库（persistTerminal）时统一暴露为任务失败。
	_ = o.record(ctx, storage.OperationRecord{
		TaskID:    undoTaskID,
		Action:    storage.ActionUndo,
		SrcPath:   srcUndo,
		DstPath:   dstUndo,
		Status:    res.Status,
		Error:     res.Error,
		SizeBytes: mv.SizeBytes,
	})

	return res
}
