<script lang="ts" setup>
// P4 整理页（FR-9 进度 + FR-10 报告 + FR-11 撤销）。
//
// 职责划分（requirements §九）：发起整理（含二次确认）在 **P3 结果页**；本页负责
// 展示整理进度/日志/报告，并在此发起**撤销**（同样二次确认）。
//
// 到达方式：P3「开始整理」确认后 StartOrganize 成功 → 跳转本页（任务已登记）。
// 直接访问本页且无进行中的整理任务时，展示引导文案回到结果页。
//
// 硬规则：
// - 撤销必须二次确认（FR-11）；取消 = 零额外文件系统写操作之外的状态（已完成的
//   回移保留，FR-11 中断语义）。
// - 单活动任务：撤销运行中禁用发起按钮（后端另有单活动约束兜底）。
// - scanID 来源同结果页：task:status 的 scan_id（D-C1）。
import { computed, nextTick, ref, watch } from 'vue'
import * as API from '../../wailsjs/go/app/API'
import type { app } from '../../wailsjs/go/models'
import StatusIcon from '../components/StatusIcon.vue'
import { friendlyMessage } from '../lib/errors'
import { formatCount, formatDuration, formatLogTime, formatSize } from '../lib/format'
import { hasRunningTask, isTerminal, LOG_LIMIT, logKindLabel, statusLabel, useTaskState } from '../lib/task'

const state = useTaskState()

const scanID = computed(() => state.scanId)
const hasScan = computed(() => scanID.value !== null)

// 本页只跟踪整理/撤销任务（type=organize）；扫描任务在扫描页展示。
const organizing = computed(
  () => state.type === 'organize' && state.taskId !== null && !isTerminal(state.status),
)
const organizeTerminal = computed(
  () => state.type === 'organize' && state.taskId !== null && isTerminal(state.status),
)
const running = computed(() => hasRunningTask())

const report = ref<app.OrganizeReport | null>(null)

// lastAction 标记当前进行/最近一次动作，用于报告标题与撤销按钮。
// 组件重挂载后回落到默认 'organize'；若上次其实是撤销，再点撤销会被后端以
// ErrAlreadyUndone 安全拒绝（后端为真相来源），不影响正确性。
const lastAction = ref<'organize' | 'undo'>('organize')

const confirmingUndo = ref(false)
const submitting = ref(false)
const actionError = ref('')

const actionLabel = computed(() => (lastAction.value === 'undo' ? '撤销' : '整理'))
const undoing = computed(() => organizing.value && lastAction.value === 'undo')

const done = computed(() => state.progress?.done ?? 0)
const total = computed(() => state.progress?.total ?? 0)
const percent = computed(() => state.progress?.percent ?? 0)
const currentFile = computed(() => state.progress?.current_file ?? '')
const targetCategory = computed(() => state.progress?.target_category ?? '')

// 撤销仅在存在成功移动时有意义（后端会再次判定，这里用于按钮可用性）。
const canUndo = computed(
  () => lastAction.value === 'organize' && report.value !== null && report.value.success_count > 0,
)

// 日志区：最新日志置顶展示，面板钉在顶部（与扫描页一致，判定细节见 ScanView 注释：
// 只有真实用户输入才能进入「暂停跟随」，浏览器自动改写 scrollTop 不得触发）。
const logEl = ref<HTMLElement | null>(null)
const userTookOver = ref(false)

// pointerHeld 记录指针是否按住：按住指针且真的滚动了（拖滚动条）才算用户接管，
// 单纯点一下不算。仅在事件处理里读写，无需响应式。
let pointerHeld = false

// displayLogs 把到达顺序（旧→新）反转成展示顺序（新→旧）以配合钉顶。
const displayLogs = computed(() => [...state.logs].reverse())

function onPointerDown(): void {
  pointerHeld = true
  // 松手位置可能在日志区之外（拖滚动条后移出），用一次性全局监听兜底：
  // pointerHeld 若永久为 true，浏览器自动位移会被误判成用户接管。
  window.addEventListener('pointerup', onPointerUp, { once: true })
}

function onPointerUp(): void {
  pointerHeld = false
}

// onWheelIntent：向下滚动即查看更早的日志（最新在顶部），进入阅读模式；
// 向上滚动（面板已在顶部）不改变跟随状态。
function onWheelIntent(event: WheelEvent): void {
  if (event.deltaY > 0) {
    userTookOver.value = true
  }
}

// onTouchIntent：触摸拖动日志区同样视为用户接管。
function onTouchIntent(): void {
  userTookOver.value = true
}

function onLogScroll(): void {
  const el = logEl.value
  if (!el) {
    return
  }
  if (pointerHeld) {
    if (el.scrollTop > 40) {
      userTookOver.value = true
    }
    return
  }
  // 跟随中的非用户位移（滚动锚定等）立即拉回顶部，保证最新日志始终可见。
  if (!userTookOver.value && el.scrollTop !== 0) {
    el.scrollTop = 0
  }
}

// 点悬浮按钮回到顶部并恢复跟随（等价于用户手动滚回顶部）。
function resumeFollow(): void {
  const el = logEl.value
  if (el) {
    el.scrollTop = 0
  }
  userTookOver.value = false
}

watch(
  () => state.logs[state.logs.length - 1]?.id,
  async () => {
    const el = logEl.value
    if (!el) {
      return
    }
    const nearTop = el.scrollTop < 40
    const stick = !userTookOver.value || nearTop
    // 下翻阅读补偿：新行插入列表头部会下推下方内容，按首行位移补回 scrollTop
    // （面板用 [overflow-anchor:none] 关闭了浏览器滚动锚定，改为手动补偿）。
    const anchor = stick ? null : el.firstElementChild
    const anchorTop = anchor ? anchor.getBoundingClientRect().top : 0
    await nextTick()
    const pane = logEl.value
    if (!pane) {
      return
    }
    if (stick) {
      pane.scrollTop = 0
      userTookOver.value = false
      // 浏览器可能在本次布局里再次改写 scrollTop（滚动锚定）：下一帧补钉一次，
      // 让「最新在顶部」不依赖 scroll 事件是否派发（WebView2 实测偶有不派发）。
      requestAnimationFrame(() => {
        const p = logEl.value
        if (p && !userTookOver.value && p.scrollTop !== 0) {
          p.scrollTop = 0
        }
      })
      return
    }
    if (anchor && anchor.isConnected) {
      const shift = anchor.getBoundingClientRect().top - anchorTop
      if (shift > 0) {
        pane.scrollTop += shift
      }
    }
  },
)

// startUndo 在二次确认后撤销上一次整理（FR-11）。
async function startUndo(): Promise<void> {
  if (scanID.value === null || submitting.value || running.value) {
    return
  }
  submitting.value = true
  actionError.value = ''
  report.value = null
  try {
    await API.UndoLastOrganize(scanID.value)
    lastAction.value = 'undo'
  } catch (err) {
    actionError.value = friendlyMessage(err)
  } finally {
    submitting.value = false
    confirmingUndo.value = false
  }
}

// cancelTask 协作式取消当前整理/撤销；终态由后端 runner 收尾后经事件推送。
async function cancelTask(): Promise<void> {
  if (state.taskId === null) {
    return
  }
  actionError.value = ''
  try {
    await API.CancelTask(state.taskId)
  } catch (err) {
    actionError.value = friendlyMessage(err)
  }
}

// 终态后拉取报告（FR-10）。immediate 用于由 P3 跳入/刷新后按全局任务状态恢复报告。
watch(
  () => [state.taskId, state.status, state.type] as const,
  async () => {
    if (!organizeTerminal.value || state.taskId === null) {
      return
    }
    try {
      report.value = await API.GetOrganizeReport(state.taskId)
    } catch (err) {
      actionError.value = friendlyMessage(err)
    }
  },
  { immediate: true },
)

function reportTone(status: string): string {
  switch (status) {
    case 'completed':
      return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-400/40 dark:bg-emerald-400/10 dark:text-emerald-300'
    case 'canceled':
      return 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-300'
    case 'failed':
      return 'border-rose-300 bg-rose-50 text-rose-600 dark:border-rose-400/40 dark:bg-rose-500/10 dark:text-rose-300'
    default:
      return 'border-slate-200 bg-white text-slate-600 dark:border-slate-600 dark:bg-slate-800/60 dark:text-slate-300'
  }
}

// reportIcon 与 reportTone 同源：终态报告的状态图标
// （completed→成功、canceled→警告、failed→失败）。
const reportIcon = computed((): 'success' | 'warning' | 'error' | 'info' => {
  switch (report.value?.status) {
    case 'completed':
      return 'success'
    case 'canceled':
      return 'warning'
    case 'failed':
      return 'error'
    default:
      return 'info'
  }
})
</script>

<template>
  <section class="mx-auto flex h-full max-w-5xl flex-col px-6 py-6">
    <!-- 空态：无 scanID（未扫描或刷新后无历史入口） -->
    <div v-if="!hasScan" class="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <h2 class="text-xl font-semibold text-slate-800 dark:text-slate-200">暂无可整理的内容</h2>
      <p class="text-sm text-slate-500 dark:text-slate-400">请先在首页选择目录并完成一次扫描。</p>
      <RouterLink to="/" class="mt-1 text-sm text-indigo-600 hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200">← 返回首页</RouterLink>
    </div>

    <template v-else>
      <header class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <h1 class="text-xl font-semibold text-slate-900 dark:text-slate-100">整理与撤销</h1>
          <!-- 目录说明：路径本身不自明，前面补「目录：」标签与文件夹图标 -->
          <p class="mt-1 flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" class="h-3.5 w-3.5 shrink-0" aria-hidden="true">
              <path d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z" />
            </svg>
            <span class="shrink-0">目录：</span>
            <span class="min-w-0 truncate" :title="state.rootPath">{{ state.rootPath || '—' }}</span>
          </p>
        </div>
        <RouterLink to="/results" class="shrink-0 text-sm text-indigo-600 hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200">← 返回结果</RouterLink>
      </header>

      <p v-if="actionError" class="mt-3 flex items-center gap-1.5 text-xs text-rose-600 dark:text-rose-300">
        <StatusIcon kind="error" />
        {{ actionError }}
      </p>

      <!-- 运行中：确定型进度（可展示总数 / 百分比） + 日志 + 取消 -->
      <template v-if="organizing">
        <div class="mt-4 flex items-center justify-between">
          <span class="flex items-center gap-1.5 rounded-full border border-slate-300 px-3 py-1 text-xs text-slate-700 dark:border-slate-600 dark:text-slate-300">
            <StatusIcon kind="loading" />
            {{ statusLabel(state.status) }} · {{ undoing ? '撤销中' : '整理中' }}
          </span>
          <button
            type="button"
            class="rounded-md border border-rose-300 bg-rose-50 px-4 py-1.5 text-sm text-rose-600 hover:bg-rose-100 dark:border-rose-400/50 dark:bg-rose-500/10 dark:text-rose-300 dark:hover:bg-rose-500/20"
            @click="cancelTask"
          >
            取消
          </button>
        </div>

        <dl class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">已处理</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatCount(done) }} / {{ formatCount(total) }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">进度</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ percent }}%</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">已耗时</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatDuration(state.progress?.elapsed_ms ?? 0) }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">目标类别</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ targetCategory || '—' }}</dd>
          </div>
          <div class="col-span-2 rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm sm:col-span-4 dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">当前文件</dt>
            <dd class="mt-1 truncate text-sm text-slate-800 dark:text-slate-200" :title="currentFile">{{ currentFile || '—' }}</dd>
          </div>
        </dl>

        <div class="mt-3 h-2 w-full overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
          <div class="h-full rounded-full bg-indigo-600 transition-all dark:bg-indigo-500" :style="{ width: percent + '%' }" />
        </div>

        <div class="relative mt-4 flex min-h-0 flex-1 flex-col rounded-lg border border-slate-200 bg-slate-50 dark:border-slate-700/60 dark:bg-slate-950/60">
          <div class="flex items-center justify-between border-b border-slate-200 px-4 py-2 dark:border-slate-700/60">
            <span class="text-xs font-medium text-slate-500 dark:text-slate-400">日志</span>
            <span class="flex items-center gap-2">
              <span v-if="userTookOver" class="rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-400/15 dark:text-amber-300">已暂停跟随</span>
              <span class="text-xs text-slate-400 dark:text-slate-500">上限 {{ LOG_LIMIT }} 行</span>
            </span>
          </div>
          <div ref="logEl" class="min-h-0 flex-1 [overflow-anchor:none] overflow-y-auto px-4 py-2 font-mono text-xs leading-relaxed" @wheel.passive="onWheelIntent" @touchmove.passive="onTouchIntent" @pointerdown="onPointerDown" @scroll="onLogScroll">
            <p v-if="!state.logs.length" class="py-4 text-center text-slate-400 dark:text-slate-500">暂无日志</p>
            <div v-for="entry in displayLogs" :key="entry.id" class="flex gap-3 py-0.5">
              <span class="shrink-0 tabular-nums text-slate-400 dark:text-slate-500">{{ formatLogTime(entry.ts) }}</span>
              <span class="shrink-0 rounded px-1.5 text-[10px] leading-5" :class="entry.kind === 'system' ? 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-200' : 'bg-slate-100 text-slate-500 dark:bg-slate-700/50 dark:text-slate-400'">
                {{ logKindLabel(entry.kind) }}
              </span>
              <span class="min-w-0 whitespace-pre-wrap break-all text-slate-700 dark:text-slate-300">{{ entry.text }}</span>
            </div>
          </div>
          <button v-if="userTookOver" type="button" class="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full border border-amber-300 bg-white/95 px-3 py-1 text-xs text-amber-700 shadow-sm hover:bg-white dark:border-amber-400/40 dark:bg-slate-800/95 dark:text-amber-300 dark:hover:bg-slate-800" @click="resumeFollow">已暂停跟随 · 回到最新</button>
        </div>
      </template>

      <!-- 终态报告（FR-10） -->
      <template v-else-if="organizeTerminal">
        <template v-if="report">
          <div class="mt-4 flex items-center gap-2 rounded-md border px-4 py-3 text-sm" :class="reportTone(report.status)">
            <StatusIcon :kind="reportIcon" />
            <span>{{ actionLabel }}{{ report.status === 'completed' ? '完成' : report.status === 'canceled' ? '已取消（已完成部分保留）' : '失败' }}</span>
          </div>

          <dl class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
              <dt class="text-xs text-slate-400 dark:text-slate-500">成功</dt>
              <dd class="mt-1 text-lg font-semibold text-emerald-600 dark:text-emerald-300">{{ formatCount(report.success_count) }}</dd>
            </div>
            <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
              <dt class="text-xs text-slate-400 dark:text-slate-500">失败</dt>
              <dd class="mt-1 text-lg font-semibold" :class="report.failed_count > 0 ? 'text-rose-600 dark:text-rose-300' : 'text-slate-900 dark:text-slate-100'">{{ formatCount(report.failed_count) }}</dd>
            </div>
            <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
              <dt class="text-xs text-slate-400 dark:text-slate-500">{{ lastAction === 'undo' ? '已回移数据量' : '已移动数据量' }}</dt>
              <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatSize(report.moved_size_bytes) }}</dd>
            </div>
            <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
              <dt class="text-xs text-slate-400 dark:text-slate-500">耗时</dt>
              <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatDuration(report.duration_ms) }}</dd>
            </div>
          </dl>

          <!-- 失败明细 -->
          <div v-if="report.failures.length" class="mt-4 flex min-h-0 flex-1 flex-col rounded-lg border border-slate-200 bg-white dark:border-slate-700/60 dark:bg-slate-950/40">
            <div class="border-b border-slate-200 px-4 py-2 text-xs font-medium text-slate-500 dark:border-slate-700/60 dark:text-slate-400">
              失败明细 · {{ formatCount(report.failures.length) }} 项
            </div>
            <div class="min-h-0 flex-1 overflow-auto">
              <table class="w-full border-collapse text-left text-xs">
                <thead class="sticky top-0 bg-slate-50 text-slate-500 dark:bg-slate-900/95 dark:text-slate-400">
                  <tr>
                    <th class="px-4 py-2 font-medium">文件</th>
                    <th class="px-4 py-2 font-medium">原因</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(f, i) in report.failures" :key="i" class="border-t border-slate-100 dark:border-slate-800">
                    <td class="max-w-md px-4 py-2">
                      <p class="truncate text-slate-800 dark:text-slate-200" :title="f.path">{{ f.path }}</p>
                    </td>
                    <td class="px-4 py-2 text-rose-600 dark:text-rose-300">{{ f.reason }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- 后续动作 -->
          <div class="mt-4 flex flex-wrap items-center gap-3">
            <button
              v-if="canUndo && !confirmingUndo"
              type="button"
              class="rounded-md border border-amber-300 bg-amber-50 px-4 py-1.5 text-sm text-amber-800 hover:bg-amber-100 dark:border-amber-400/50 dark:bg-amber-500/10 dark:text-amber-200 dark:hover:bg-amber-500/20"
              @click="confirmingUndo = true"
            >
              撤销本次整理
            </button>
            <RouterLink to="/results" class="text-sm text-indigo-600 hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200">查看结果</RouterLink>
          </div>

          <div v-if="confirmingUndo" class="mt-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-200">
            <p class="flex items-start gap-2">
              <StatusIcon kind="warning" />
              <span class="min-w-0">将把本次整理移动的文件移回原位置；原位置若已被占用则跳过（绝不覆盖）。是否继续？</span>
            </p>
            <div class="mt-2 flex items-center gap-3">
              <button type="button" class="rounded border border-amber-300 px-3 py-1 text-amber-800 hover:bg-amber-100 disabled:opacity-50 dark:border-amber-400/60 dark:text-amber-100 dark:hover:bg-amber-400/20" :disabled="submitting" @click="startUndo">
                确认撤销
              </button>
              <button type="button" class="rounded border border-slate-300 px-3 py-1 text-slate-700 hover:bg-slate-100 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-700/50" @click="confirmingUndo = false">
                取消
              </button>
            </div>
          </div>
        </template>
        <p v-else class="mt-6 flex items-center gap-2 text-sm text-slate-500 dark:text-slate-400">
          <StatusIcon kind="loading" />
          正在加载报告…
        </p>
      </template>

      <!-- 空闲：尚无进行中的整理任务（发起入口在 P3 结果页） -->
      <template v-else>
        <div class="mt-6 flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <h2 class="text-lg font-semibold text-slate-800 dark:text-slate-200">尚无进行中的整理任务</h2>
          <p class="text-sm text-slate-500 dark:text-slate-400">请在「扫描结果」页点击「开始整理」，确认后自动跳转到本页展示进度与报告。</p>
          <RouterLink to="/results" class="mt-1 text-sm text-indigo-600 hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200">← 前往扫描结果</RouterLink>
        </div>
      </template>
    </template>
  </section>
</template>
