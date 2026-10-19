<script lang="ts" setup>
// P2 扫描中（FR-4）：不确定型进度 + 三类日志流 + 取消。
//
// 进度硬规则（§四 FR-4 · §2.4）：目录遍历结束前总数不可知，**禁止**展示
// 总数 / 百分比 / ETA；此处只呈现已扫描数、速率、当前目录、已耗时、跳过数
// （这些字段由 task:progress 载荷提供）。
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import * as API from '../../wailsjs/go/app/API'
import StatusIcon from '../components/StatusIcon.vue'
import { friendlyMessage } from '../lib/errors'
import { formatCount, formatDuration, formatLogTime, formatRate } from '../lib/format'
import { isTerminal, LOG_LIMIT, logKindLabel, reset, statusLabel, useTaskState } from '../lib/task'

const state = useTaskState()
const router = useRouter()

const canceling = ref(false)
const cancelError = ref('')

const hasTask = computed(() => state.taskId !== null)
const terminal = computed(() => isTerminal(state.status))
const showProgress = computed(() => hasTask.value && !terminal.value)

const scanned = computed(() => state.progress?.scanned ?? 0)
const rate = computed(() => state.progress?.rate ?? 0)
const skipped = computed(() => state.progress?.skipped ?? 0)
const elapsedMs = computed(() => state.progress?.elapsed_ms ?? 0)
const currentDir = computed(() => state.progress?.current_dir ?? '')

// terminalMessage 给出终态横幅文案。
//
// 失败一律用通用文案：task:status 的 error 是内部错误链，按 contracts §4/§8.4
// 不直接呈现给用户（失败原因可在日志区查看，那里才是「有原因」的去处）。
function terminalMessage(): string {
  switch (state.status) {
    case 'completed':
      return '扫描完成'
    case 'canceled':
      return '已取消 · 部分结果'
    case 'failed':
      return '扫描失败，请重试；详细信息已记录到诊断日志'
    default:
      return ''
  }
}

const terminalTone = computed(() => {
  switch (state.status) {
    case 'completed':
      return 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-400/40 dark:bg-emerald-400/10 dark:text-emerald-300'
    case 'canceled':
      return 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-300'
    case 'failed':
      return 'border-rose-300 bg-rose-50 text-rose-600 dark:border-rose-400/40 dark:bg-rose-500/10 dark:text-rose-300'
    default:
      return 'border-slate-200 bg-white text-slate-600 dark:border-slate-600 dark:bg-slate-800/60 dark:text-slate-300'
  }
})

// terminalIcon 与 terminalTone 同源：终态横幅的状态图标
// （completed→成功、canceled→警告、failed→失败）。
const terminalIcon = computed((): 'success' | 'warning' | 'error' => {
  switch (state.status) {
    case 'completed':
      return 'success'
    case 'canceled':
      return 'warning'
    default:
      return 'error'
  }
})

// showResults 仅在扫描正常完成且已取到 scanID（D-C1）时提供结果入口；
// 取消/失败的部分结果不进入预览（T1.6）。
const showResults = computed(() => state.status === 'completed' && state.scanId !== null)

// 日志区：最新日志置顶展示——新行从视图顶部出现、旧行往下推，面板钉在顶部；
// 用户不下翻查看历史，就永远不需要手动滚动。
const logEl = ref<HTMLElement | null>(null)

// userTookOver 表示用户主动下翻阅读历史：此后钉顶暂停，回到顶部或点「回到最新」恢复。
//
// 关键约束：**只有真实用户输入才可能进入该状态**。不能仅凭「scrollTop > 40」判定
// 接管：插入新行时浏览器会自行改写 scrollTop（滚动锚定；面板虽已加
// [overflow-anchor:none]，WebView2 实测仍会发生），该位移在用户没有任何操作时
// 也会把状态翻成「已接管」，随后视口被锚死在旧行——表现为「日志不再刷新、
// 当前目录不动，但计数仍在涨」——且不会自愈（见 onLogScroll 的纠正分支）。
const userTookOver = ref(false)

// pointerHeld 记录指针是否按住：按住指针且真的滚动了（拖滚动条）才算用户接管，
// 单纯点一下不算。仅在事件处理里读写，无需响应式。
let pointerHeld = false

// displayLogs 把到达顺序（旧→新）反转成展示顺序（新→旧）以配合钉顶。
// :key 用稳定 id，Vue 只做增量更新（头部插入新行、尾部移除超限旧行），不整表重排。
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

// 触发源用「最后一条日志的 id」：环形缓冲打满后 length 恒为 LOG_LIMIT，
// 监听 length 会让钉顶在打满后不再触发。
watch(
  () => state.logs[state.logs.length - 1]?.id,
  async () => {
    const el = logEl.value
    if (!el) {
      return
    }
    // 在 DOM 更新前判定（watch 默认 pre flush），避免读到自己刚插入的行。
    const nearTop = el.scrollTop < 40
    const stick = !userTookOver.value || nearTop
    // 下翻阅读补偿：新行插入列表头部会把下方内容整体下推，先记下现有首行的
    // 位置，插入后按位移把 scrollTop 补回去，让阅读位置保持不动。
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

async function cancelScan(): Promise<void> {
  if (state.taskId === null || canceling.value) {
    return
  }
  canceling.value = true
  cancelError.value = ''
  try {
    await API.CancelTask(state.taskId)
    // 真正的终态由后端 runner 收尾后经 task:status 推送，此处只进入「取消中」。
  } catch (err) {
    cancelError.value = friendlyMessage(err)
  } finally {
    canceling.value = false
  }
}

// scanAgain 清除本次任务状态并回到首页，供终态后重新发起扫描。
function scanAgain(): void {
  reset()
  void router.push('/')
}
</script>

<template>
  <section class="mx-auto flex h-full max-w-5xl flex-col px-6 py-6">
    <!-- 空态：无进行中任务 -->
    <div v-if="!hasTask" class="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <h2 class="text-xl font-semibold text-slate-800 dark:text-slate-200">当前没有进行中的扫描任务</h2>
      <p class="text-sm text-slate-500 dark:text-slate-400">请在首页选择目录并开始扫描。</p>
      <RouterLink to="/" class="mt-1 text-sm text-indigo-600 hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200">← 返回首页</RouterLink>
    </div>

    <template v-else>
      <header class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <h1 class="text-xl font-semibold text-slate-900 dark:text-slate-100">扫描中</h1>
          <!-- 目录说明：路径本身不自明，前面补「目录：」标签与文件夹图标 -->
          <p class="mt-1 flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" class="h-3.5 w-3.5 shrink-0" aria-hidden="true">
              <path d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z" />
            </svg>
            <span class="shrink-0">目录：</span>
            <span class="min-w-0 truncate" :title="state.rootPath">{{ state.rootPath || '—' }}</span>
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-3">
          <span class="flex items-center gap-1.5 rounded-full border border-slate-300 px-3 py-1 text-xs text-slate-700 dark:border-slate-600 dark:text-slate-300">
            <StatusIcon v-if="!terminal" kind="loading" />
            {{ statusLabel(state.status) }}
          </span>
          <button
            v-if="!terminal"
            type="button"
            class="rounded-md border border-rose-300 bg-rose-50 px-4 py-1.5 text-sm text-rose-600 hover:bg-rose-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-rose-400/50 dark:bg-rose-500/10 dark:text-rose-300 dark:hover:bg-rose-500/20"
            :disabled="canceling"
            @click="cancelScan"
          >
            {{ canceling ? '取消中…' : '取消扫描' }}
          </button>
        </div>
      </header>

      <p v-if="cancelError" class="mt-3 flex items-center gap-1.5 text-xs text-rose-600 dark:text-rose-300">
        <StatusIcon kind="error" />
        {{ cancelError }}
      </p>

      <!-- 终态横幅 -->
      <div v-if="terminal" class="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-md border px-4 py-3 text-sm" :class="terminalTone">
        <span class="flex items-center gap-2">
          <StatusIcon :kind="terminalIcon" />
          {{ terminalMessage() }}
        </span>
        <span class="flex items-center gap-3 text-xs opacity-80">
          <template v-if="showResults">
            <RouterLink to="/results" class="underline">查看分类结果</RouterLink>
          </template>
          <span v-else>本次扫描未正常完成，部分结果不可整理。</span>
          <button type="button" class="underline disabled:opacity-50" @click="scanAgain">重新扫描</button>
          <RouterLink to="/" class="underline">返回首页</RouterLink>
        </span>
      </div>

      <!-- 不确定型进度（无总数 / 百分比 / ETA） -->
      <dl v-if="showProgress" class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
          <dt class="text-xs text-slate-400 dark:text-slate-500">已扫描文件</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatCount(scanned) }}</dd>
        </div>
        <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
          <dt class="text-xs text-slate-400 dark:text-slate-500">扫描速率</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatRate(rate) }} <span class="text-xs font-normal text-slate-500 dark:text-slate-400">文件/秒</span></dd>
        </div>
        <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
          <dt class="text-xs text-slate-400 dark:text-slate-500">已耗时</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatDuration(elapsedMs) }}</dd>
        </div>
        <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
          <dt class="text-xs text-slate-400 dark:text-slate-500">跳过</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatCount(skipped) }}</dd>
        </div>
        <div class="col-span-2 rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm sm:col-span-4 dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
          <dt class="text-xs text-slate-400 dark:text-slate-500">当前目录</dt>
          <dd class="mt-1 truncate text-sm text-slate-800 dark:text-slate-200" :title="currentDir">{{ currentDir || '—' }}</dd>
        </div>
      </dl>

      <!-- 三类日志流（环形缓冲上限 500 行） -->
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
            <span class="shrink-0 rounded px-1.5 text-[10px] leading-5" :class="entry.kind === 'system' ? 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-200' : entry.kind === 'classify' ? 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/20 dark:text-indigo-300' : 'bg-slate-100 text-slate-500 dark:bg-slate-700/50 dark:text-slate-400'">
              {{ logKindLabel(entry.kind) }}
            </span>
            <span class="min-w-0 whitespace-pre-wrap break-all text-slate-700 dark:text-slate-300">{{ entry.text }}</span>
          </div>
        </div>
        <button v-if="userTookOver" type="button" class="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full border border-amber-300 bg-white/95 px-3 py-1 text-xs text-amber-700 shadow-sm hover:bg-white dark:border-amber-400/40 dark:bg-slate-800/95 dark:text-amber-300 dark:hover:bg-slate-800" @click="resumeFollow">已暂停跟随 · 回到最新</button>
      </div>
    </template>
  </section>
</template>
