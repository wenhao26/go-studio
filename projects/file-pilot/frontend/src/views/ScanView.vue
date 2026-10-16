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
      return 'border-emerald-400/40 bg-emerald-400/10 text-emerald-300'
    case 'canceled':
      return 'border-amber-400/40 bg-amber-400/10 text-amber-300'
    case 'failed':
      return 'border-rose-400/40 bg-rose-500/10 text-rose-300'
    default:
      return 'border-slate-600 bg-slate-800/60 text-slate-300'
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

// 日志区：贴在底部时自动滚动，用户上滚查看历史时不打扰。
const logEl = ref<HTMLElement | null>(null)
const stickToBottom = ref(true)

function onLogScroll(): void {
  const el = logEl.value
  if (!el) {
    return
  }
  stickToBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 40
}

watch(
  () => state.logs.length,
  async () => {
    if (!stickToBottom.value) {
      return
    }
    await nextTick()
    const el = logEl.value
    if (el) {
      el.scrollTop = el.scrollHeight
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
  <section class="mx-auto flex h-full max-w-4xl flex-col px-6 py-6">
    <!-- 空态：无进行中任务 -->
    <div v-if="!hasTask" class="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <h2 class="text-xl font-semibold text-slate-200">当前没有进行中的扫描任务</h2>
      <p class="text-sm text-slate-400">请在首页选择目录并开始扫描。</p>
      <RouterLink to="/" class="mt-1 text-sm text-indigo-300 hover:text-indigo-200">← 返回首页</RouterLink>
    </div>

    <template v-else>
      <header class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <h1 class="text-xl font-semibold text-slate-100">扫描中</h1>
          <p class="mt-1 truncate text-xs text-slate-400" :title="state.rootPath">{{ state.rootPath || '—' }}</p>
        </div>
        <div class="flex shrink-0 items-center gap-3">
          <span class="flex items-center gap-1.5 rounded-full border border-slate-600 px-3 py-1 text-xs text-slate-300">
            <StatusIcon v-if="!terminal" kind="loading" />
            {{ statusLabel(state.status) }}
          </span>
          <button
            v-if="!terminal"
            type="button"
            class="rounded-md border border-rose-400/50 bg-rose-500/10 px-4 py-1.5 text-sm text-rose-300 hover:bg-rose-500/20 disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="canceling"
            @click="cancelScan"
          >
            {{ canceling ? '取消中…' : '取消扫描' }}
          </button>
        </div>
      </header>

      <p v-if="cancelError" class="mt-3 flex items-center gap-1.5 text-xs text-rose-300">
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
        <div class="rounded-lg border border-slate-700/60 bg-slate-800/40 px-4 py-3">
          <dt class="text-xs text-slate-500">已扫描文件</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-100">{{ formatCount(scanned) }}</dd>
        </div>
        <div class="rounded-lg border border-slate-700/60 bg-slate-800/40 px-4 py-3">
          <dt class="text-xs text-slate-500">扫描速率</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-100">{{ formatRate(rate) }} <span class="text-xs font-normal text-slate-400">文件/秒</span></dd>
        </div>
        <div class="rounded-lg border border-slate-700/60 bg-slate-800/40 px-4 py-3">
          <dt class="text-xs text-slate-500">已耗时</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-100">{{ formatDuration(elapsedMs) }}</dd>
        </div>
        <div class="rounded-lg border border-slate-700/60 bg-slate-800/40 px-4 py-3">
          <dt class="text-xs text-slate-500">跳过</dt>
          <dd class="mt-1 text-lg font-semibold text-slate-100">{{ formatCount(skipped) }}</dd>
        </div>
        <div class="col-span-2 rounded-lg border border-slate-700/60 bg-slate-800/40 px-4 py-3 sm:col-span-4">
          <dt class="text-xs text-slate-500">当前目录</dt>
          <dd class="mt-1 truncate text-sm text-slate-200" :title="currentDir">{{ currentDir || '—' }}</dd>
        </div>
      </dl>

      <!-- 三类日志流（环形缓冲上限 500 行） -->
      <div class="mt-4 flex min-h-0 flex-1 flex-col rounded-lg border border-slate-700/60 bg-slate-950/60">
        <div class="flex items-center justify-between border-b border-slate-700/60 px-4 py-2">
          <span class="text-xs font-medium text-slate-400">日志</span>
          <span class="text-xs text-slate-500">上限 {{ LOG_LIMIT }} 行</span>
        </div>
        <div ref="logEl" class="min-h-0 flex-1 overflow-y-auto px-4 py-2 font-mono text-xs leading-relaxed" @scroll="onLogScroll">
          <p v-if="!state.logs.length" class="py-4 text-center text-slate-500">暂无日志</p>
          <div v-for="entry in state.logs" :key="entry.id" class="flex gap-3 py-0.5">
            <span class="shrink-0 tabular-nums text-slate-500">{{ formatLogTime(entry.ts) }}</span>
            <span class="shrink-0 rounded px-1.5 text-[10px] leading-5" :class="entry.kind === 'system' ? 'bg-slate-700 text-slate-200' : entry.kind === 'classify' ? 'bg-indigo-500/20 text-indigo-300' : 'bg-slate-700/50 text-slate-400'">
              {{ logKindLabel(entry.kind) }}
            </span>
            <span class="min-w-0 whitespace-pre-wrap break-all text-slate-300">{{ entry.text }}</span>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>
