// 哨兵错误 → 中文文案映射（contracts §4 / requirements §8.4）。
//
// Wails 边界约定：绑定方法命中哨兵时返回**裸哨兵**，其 message 即为匹配键；
// 前端按 error.message 精确匹配后展示中文文案，未命中则展示通用文案
// （原始 Go error 不直接展示给用户，详细信息由后端写诊断日志，AGENTS.md §65）。
//
// 注意：key 必须与 Go 侧哨兵 errors.New(...) 的字符串**逐字一致**，
// 修改任一侧都要同步 contracts §4 与本节（contracts §9）。

const FRIENDLY_MESSAGES: Record<string, string> = {
  // task.ErrTaskRunning
  'task already running': '当前有任务进行中',
  // scanner.ErrInvalidWorkers
  'workers must be between 1 and 64': '并发数需在 1–64 之间',
  // app.ErrDirNotFound / app.ErrDirNotDirectory
  'directory not found': '所选目录不可用',
  'path is not a directory': '所选目录不可用',
  // app.ErrConfigInvalid
  'invalid scan config': '请检查扫描配置',
  // app.ErrDirectoryPickCanceled
  'directory pick canceled': '目录选择失败 / 用户取消操作',
  // app.ErrCategoryInvalid（Phase 2 / FR-8）
  'invalid category': '类别不合法',
  // 注：ErrFileNotInScan（`file not in scan`）按 contracts §4 用通用文案，不在此映射。
  // app.ErrScanIncomplete（Phase 3 / FR-9）
  'scan incomplete': '扫描未完成，请重新扫描',
  // organizer.ErrTargetDirCreate（Phase 3 / FR-9）
  'create target dir failed': '无法创建整理目录',
  // app.ErrNothingToUndo（Phase 3 / FR-11）
  'nothing to undo': '没有可撤销的整理',
  // app.ErrAlreadyUndone（Phase 3 / FR-11）
  'already undone': '该整理已撤销',
  // app.ErrStorageUnavailable（v1.2 / 存储位置展示）
  'storage folder unavailable': '无法打开存储目录',
}

// GENERIC_ERROR 是未映射错误的统一文案（原始 error 只进诊断日志）。
export const GENERIC_ERROR = '操作失败，请稍后重试；详细信息已记录到诊断日志'

// NOT_SELECTED_DIR 是前端本地拦截文案（FR-1：未选目录不调后端）。
export const NOT_SELECTED_DIR = '请选择扫描目录后继续'

// errorKey 从各种 reject 形态中取出 message（Wails 以 new Error(message) 拒绝）。
function errorKey(err: unknown): string | null {
  if (err == null) {
    return null
  }
  if (typeof err === 'string') {
    return err
  }
  if (err instanceof Error) {
    return err.message
  }
  if (typeof err === 'object' && 'message' in err) {
    return String((err as { message: unknown }).message)
  }
  return null
}

// friendlyMessage 把绑定方法抛出的错误转为用户可读文案。
export function friendlyMessage(err: unknown): string {
  const key = errorKey(err)
  if (key && FRIENDLY_MESSAGES[key]) {
    return FRIENDLY_MESSAGES[key]
  }
  return GENERIC_ERROR
}
