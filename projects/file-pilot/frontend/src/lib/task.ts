// 任务状态与事件桥接（contracts §6）。
//
// 单例响应式仓库，不用 Pinia：Phase 1 只有「扫描任务」一种活动任务，
// 事件载荷带 task_id 可自证归属，模块级 reactive 状态足够（AGENTS.md §31：
// 不为一处共享状态引入状态管理依赖）。组件通过 useTaskState() 读取。
//
// 事件不持久化（contracts §6），UI 恢复靠 GetActiveTask 查询——因此刷新后
// 已产生的日志会丢失，只有后续事件继续流入。

import { reactive } from 'vue'
import * as Runtime from '../../wailsjs/runtime/runtime'
import * as API from '../../wailsjs/go/app/API'

// LOG_LIMIT 是日志环形缓冲上限（requirements §8.3：前端 500 行）。
export const LOG_LIMIT = 500

// 事件名（contracts §6）。
const EVENT_STATUS = 'task:status'
const EVENT_PROGRESS = 'task:progress'
const EVENT_LOG = 'task:log'

// StatusPayload 是 task:status 的载荷（后端 events.go statusPayload）。
// scan_id 关联扫描记录（D-C1，结果页据此查询汇总/详情；非扫描任务为 0）。
export interface StatusPayload {
  task_id: number
  type: string
  status: string
  scan_id: number
  error: string
}

// ProgressPayload 是 task:progress 的载荷。
//
// 扫描（不确定型）：scanned/skipped/rate/current_dir，无 total/percent。
// 整理/撤销（确定型）：done/total/percent/current_file/target_category（contracts §3.8）。
// 两分支字段同结构发送（未用分支为零值），UI 按 state.type 取舍展示。
export interface ProgressPayload {
  task_id: number
  scanned: number
  skipped: number
  rate: number
  current_dir: string
  elapsed_ms: number
  // 整理/撤销专属
  done: number
  total: number
  percent: number
  current_file: string
  target_category: string
}

// LogEntry 是一条日志；ts 为 UTC 毫秒（contracts §3.9）。
export interface LogEntry {
  ts: number
  kind: string
  text: string
}

// StoredLogEntry 是入库到前端缓冲的日志：附加自增 id 作为稳定的渲染 key。
// 环形缓冲会从头部丢弃条目，若以数组下标为 key 会导致整列表重渲染。
export interface StoredLogEntry extends LogEntry {
  id: number
}

// LogPayload 是 task:log 的批量载荷。
export interface LogPayload {
  task_id: number
  entries: LogEntry[]
}

// TaskState 是当前跟踪任务的完整 UI 状态。
export interface TaskState {
  taskId: number | null
  type: string
  status: string
  scanId: number | null
  rootPath: string
  error: string
  progress: ProgressPayload | null
  logs: StoredLogEntry[]
}

const TERMINAL_STATUSES = new Set(['completed', 'failed', 'canceled'])

const state = reactive<TaskState>({
  taskId: null,
  type: '',
  status: '',
  scanId: null,
  rootPath: '',
  error: '',
  progress: null,
  logs: [],
})

// nextLogID 为日志条目分配稳定的渲染 key（模块内自增）。
let nextLogID = 1

// useTaskState 返回全局任务状态（只读使用；写入走本模块导出的动作）。
export function useTaskState(): TaskState {
  return state
}

// isTerminal 判断任务状态是否为终态（contracts §3.3）。
export function isTerminal(status: string): boolean {
  return TERMINAL_STATUSES.has(status)
}

// hasRunningTask 表示存在进行中（非终态）的任务，用于「当前任务」指示器。
export function hasRunningTask(): boolean {
  return state.taskId !== null && !isTerminal(state.status)
}

// statusLabel 把任务状态转为中文标签（contracts §3.3）。
export function statusLabel(status: string): string {
  switch (status) {
    case 'pending':
      return '等待中'
    case 'running':
      return '执行中'
    case 'completed':
      return '已完成'
    case 'failed':
      return '失败'
    case 'canceled':
      return '已取消'
    default:
      return status
  }
}

// logKindLabel 把日志类别转为中文标签（contracts §3.9）。
export function logKindLabel(kind: string): string {
  switch (kind) {
    case 'scan':
      return '扫描'
    case 'classify':
      return '分类'
    case 'system':
      return '系统'
    default:
      return kind
  }
}

// hasWailsRuntime 判断运行在 Wails 宿主中（纯 vite 浏览器预览下无 runtime）。
function hasWailsRuntime(): boolean {
  return typeof window !== 'undefined' && typeof (window as { runtime?: unknown }).runtime !== 'undefined'
}

let initialized = false

// initTaskEvents 订阅三个任务事件；应在应用挂载前调用一次（幂等）。
export function initTaskEvents(): void {
  if (initialized) {
    return
  }
  initialized = true
  if (!hasWailsRuntime()) {
    // 非 Wails 环境（浏览器直开 vite）：事件通道不存在，静默跳过。
    return
  }
  Runtime.EventsOn(EVENT_STATUS, (payload: StatusPayload) => onStatus(payload))
  Runtime.EventsOn(EVENT_PROGRESS, (payload: ProgressPayload) => onProgress(payload))
  Runtime.EventsOn(EVENT_LOG, (payload: LogPayload) => onLog(payload))
}

// syncActiveTask 在启动时对齐后端活动任务（FR-12：恢复 UI）。
//
// 刷新期间已推送的日志不会补发（事件不持久化），此处只恢复任务身份与根目录。
export async function syncActiveTask(): Promise<void> {
  if (!hasWailsRuntime()) {
    return
  }
  const active = (await API.GetActiveTask()) as ActiveTaskLike | null
  if (!active) {
    return
  }
  state.taskId = active.task_id
  state.type = active.type
  state.status = active.status
  state.scanId = active.scan_id > 0 ? active.scan_id : null
  state.rootPath = active.root_path
  state.error = active.error ?? ''
}

// ActiveTaskLike 描述 GetActiveTask 的非空返回（Go 侧返回指针，可能为 null）。
interface ActiveTaskLike {
  task_id: number
  type: string
  status: string
  scan_id: number
  root_path: string
  error: string
  elapsed_ms: number
}

// RecoveryNotice 是 FR-12 启动恢复提示的载荷（Go app.RecoveryNotice，contracts §3.10）。
export interface RecoveryNotice {
  recovered_tasks: number
  recovered_scans: number
}

// RecoveryState 是恢复提示的 UI 状态；dismissed 仅前端生效（恢复只发生一次，
// 后端不重复提示）。
export interface RecoveryState {
  notice: RecoveryNotice | null
  dismissed: boolean
}

const recovery = reactive<RecoveryState>({ notice: null, dismissed: false })

// useRecoveryNotice 返回恢复提示状态（只读使用）。
export function useRecoveryNotice(): RecoveryState {
  return recovery
}

// dismissRecoveryNotice 关闭恢复提示。
export function dismissRecoveryNotice(): void {
  recovery.dismissed = true
}

// syncRecoveryNotice 在启动时取回 FR-12 恢复提示；无恢复时后端返回 null。
export async function syncRecoveryNotice(): Promise<void> {
  if (!hasWailsRuntime()) {
    return
  }
  const notice = (await API.GetRecoveryNotice()) as RecoveryNotice | null
  if (notice) {
    recovery.notice = notice
  }
}

// ensureTask 在本地登记一个新启动的任务（type 为 'scan' 或 'organize'）。
//
// 后端在 StartScan/StartOrganize 内同步推送 pending→running 事件，该事件可能
// 先于 Promise 返回到达并已登记 task_id（含正确 type）；此函数只在尚未登记时
// 初始化，避免清空已流入的状态，并总是补写调用方已知的根目录（事件载荷不含
// root_path）。type 仅在初始化时使用，已有登记的 type 以事件为准。
export function ensureTask(taskId: number, rootPath: string, type = 'scan'): void {
  if (state.taskId !== taskId) {
    reset()
    state.taskId = taskId
    state.type = type
    state.status = 'running'
  }
  if (rootPath) {
    state.rootPath = rootPath
  }
}

// reset 清空任务状态，回到「无任务」空态。
export function reset(): void {
  state.taskId = null
  state.type = ''
  state.status = ''
  state.scanId = null
  state.rootPath = ''
  state.error = ''
  state.progress = null
  state.logs = []
}

function onStatus(payload: StatusPayload): void {
  if (state.taskId !== null && state.taskId !== payload.task_id && !isTerminal(state.status)) {
    // 单活动约束下不应出现其它任务的事件；保护性忽略。
    return
  }
  if (state.taskId !== payload.task_id) {
    // 新任务接管（旧任务已终态或从未有任务）：清空上一任务残留。
    state.progress = null
    state.logs = []
    state.scanId = null
    state.rootPath = ''
  }
  state.taskId = payload.task_id
  state.type = payload.type || 'scan'
  state.status = payload.status
  if (payload.scan_id > 0) {
    state.scanId = payload.scan_id
  }
  state.error = payload.error ?? ''
}

function onProgress(payload: ProgressPayload): void {
  if (state.taskId === null || state.taskId !== payload.task_id) {
    return
  }
  state.progress = payload
}

function onLog(payload: LogPayload): void {
  if (state.taskId === null || state.taskId !== payload.task_id) {
    return
  }
  if (!payload.entries || payload.entries.length === 0) {
    return
  }
  const appended = payload.entries.map((entry) => ({ ...entry, id: nextLogID++ }))
  const merged = state.logs.concat(appended)
  state.logs = merged.length > LOG_LIMIT ? merged.slice(merged.length - LOG_LIMIT) : merged
}
