<script lang="ts" setup>
// P3 扫描结果页（FR-5 统计 + FR-7 分类汇总/详情分页 + FR-8 逐文件调整 + FR-9.1 开始整理）。
//
// scanID 来源：task:status 事件的 scan_id（D-C1）。刷新后无活动任务即无 scanID，
// 展示空态引导重新扫描（v1 无历史入口，属已知范围外）。
//
// 硬规则：
// - 详情分页 100/页（FR-7），禁止全量渲染 DOM；
// - 改类别后置信度显示「手动」、原因「用户手动指定」（后端落库，见 FR-8）；
// - 取消/失败的部分结果不进入预览（T1.6 入口校验）；
// - 「开始整理」必须二次确认，展示「文件数量 N + 目标目录」，取消 = 零文件系统写
//   操作（FR-9.1）；确认后调用 StartOrganize 并跳转 P4 展示进度/报告。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import * as API from '../../wailsjs/go/app/API'
import type { app } from '../../wailsjs/go/models'
import StatusIcon from '../components/StatusIcon.vue'
import { friendlyMessage } from '../lib/errors'
import { formatCount, formatDuration, formatRate, formatSize } from '../lib/format'
import { ensureTask, hasRunningTask, useTaskState } from '../lib/task'

const state = useTaskState()
const router = useRouter()

// ORGANIZE_DIR 与后端 fsx.OrganizeDirName 常量一致（整理目标目录名）。
// 修改任一侧都要同步（contracts §9）。
const ORGANIZE_DIR = 'FilePilot整理'

// CATEGORIES 与后端固定 7 类顺序一致（contracts §3.2 / D-C4）。
const CATEGORIES = ['图片', '视频', '文档', '财务', '工作', '学习', '其他'] as const

// PAGE_SIZE 与后端 defaultPageSize 对齐（contracts §5：默认 100/页）。
const PAGE_SIZE = 100

const summary = ref<app.ScanSummary | null>(null)
const categories = ref<app.CategoryCount[]>([])
const page = ref<app.FilePage | null>(null)

const selectedCategory = ref('') // '' = 全部
const currentPage = ref(1)

const loading = ref(false)
const loadError = ref('')
const rowError = ref('')
const busyFileID = ref<number | null>(null)

// 整理二次确认（FR-9.1）：confirming 打开确认面板；starting 防重复提交。
const confirming = ref(false)
const starting = ref(false)
const organizeError = ref('')

const scanID = computed(() => state.scanId)
const hasScan = computed(() => scanID.value !== null)

// blocked 表示扫描以 canceled/failed 收尾：部分结果不进入预览（T1.6）。
// 状态未知（''，如刷新后）不阻断，交由后端返回的实际数据决定。
const blocked = computed(() => state.status === 'canceled' || state.status === 'failed')

// organizableCount 是「即将整理」的文件数（D-D9：excluded=0，来自后端
// organizable_count）；excludedCount 是其与总数之差（被排除、不参与整理）。
const organizableCount = computed(() => summary.value?.organizable_count ?? 0)
const excludedCount = computed(() =>
  summary.value ? summary.value.file_count - summary.value.organizable_count : 0,
)
const canOrganize = computed(() => !blocked.value && organizableCount.value > 0 && !hasRunningTask())

const items = computed(() => page.value?.items ?? [])
const totalCount = computed(() => page.value?.total_count ?? 0)
const totalPage = computed(() => page.value?.total_page ?? 0)

// categoryCount 返回某类别的汇总计数（未在汇总中出现的类别为 0）。
function categoryCount(category: string): number {
  const found = categories.value.find((c) => c.category === category)
  return found ? found.count : 0
}

// confidenceText 规则来源显示 `NN%`；用户手动（NULL）显示「手动」（FR-8）。
function confidenceText(row: app.FileItem): string {
  if (row.confidence == null) {
    return '手动'
  }
  return `${row.confidence}%`
}

async function loadSummary(): Promise<void> {
  if (scanID.value === null) {
    return
  }
  summary.value = await API.GetScanSummary(scanID.value)
}

async function loadCategories(): Promise<void> {
  if (scanID.value === null) {
    return
  }
  categories.value = await API.GetCategorySummary(scanID.value)
}

async function loadPage(target: number): Promise<void> {
  if (scanID.value === null) {
    return
  }
  const clamped = target < 1 ? 1 : target
  const result = await API.ListFiles(scanID.value, selectedCategory.value, clamped, PAGE_SIZE)
  page.value = result
  currentPage.value = result.page
}

async function loadAll(): Promise<void> {
  if (scanID.value === null) {
    return
  }
  loading.value = true
  loadError.value = ''
  try {
    await Promise.all([loadSummary(), loadCategories(), loadPage(1)])
  } catch (err) {
    loadError.value = friendlyMessage(err)
  } finally {
    loading.value = false
  }
}

async function selectCategory(category: string): Promise<void> {
  selectedCategory.value = category
  loadError.value = ''
  try {
    await loadPage(1)
  } catch (err) {
    loadError.value = friendlyMessage(err)
  }
}

// applyPlan 提交单文件整理计划并刷新「受影响」的数据：类别计数与当前页。
// 汇总（文件/目录/大小）不受类别调整影响，无需重取。
async function applyPlan(row: app.FileItem, category: string, excluded: boolean): Promise<void> {
  if (scanID.value === null) {
    return
  }
  busyFileID.value = row.id
  rowError.value = ''
  try {
    await API.SetFilePlan(row.id, category, excluded)
    await Promise.all([loadCategories(), loadPage(currentPage.value)])
  } catch (err) {
    rowError.value = friendlyMessage(err)
  } finally {
    busyFileID.value = null
  }
}

function changeCategory(row: app.FileItem, event: Event): void {
  const category = (event.target as HTMLSelectElement).value
  if (category === row.category) {
    return
  }
  void applyPlan(row, category, row.excluded)
}

function toggleExcluded(row: app.FileItem): void {
  void applyPlan(row, row.category, !row.excluded)
}

async function gotoPage(target: number): Promise<void> {
  if (target < 1 || (totalPage.value > 0 && target > totalPage.value)) {
    return
  }
  try {
    await loadPage(target)
  } catch (err) {
    loadError.value = friendlyMessage(err)
  }
}

// startOrganize 在二次确认后发起整理（FR-9.1），并跳转 P4 展示进度/报告。
async function startOrganize(): Promise<void> {
  if (scanID.value === null || starting.value) {
    return
  }
  starting.value = true
  organizeError.value = ''
  try {
    const taskId = await API.StartOrganize(scanID.value)
    ensureTask(taskId, state.rootPath, 'organize')
    confirming.value = false
    await router.push('/organize')
  } catch (err) {
    organizeError.value = friendlyMessage(err)
  } finally {
    starting.value = false
  }
}

onMounted(loadAll)
</script>

<template>
  <section class="mx-auto flex h-full max-w-5xl flex-col px-6 py-6">
    <!-- 空态：无 scanID（未扫描或刷新后无历史入口） -->
    <div v-if="!hasScan" class="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <h2 class="text-xl font-semibold text-slate-800 dark:text-slate-200">暂无可查看的扫描结果</h2>
      <p class="text-sm text-slate-500 dark:text-slate-400">请先在首页选择目录并完成一次扫描。</p>
      <RouterLink to="/" class="mt-1 text-sm text-indigo-600 hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200">← 返回首页</RouterLink>
    </div>

    <template v-else>
      <header class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <h1 class="text-xl font-semibold text-slate-900 dark:text-slate-100">扫描结果</h1>
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
          <button
            v-if="!blocked"
            type="button"
            class="rounded-md border border-indigo-300 bg-indigo-50 px-4 py-1.5 text-sm text-indigo-700 hover:bg-indigo-100 disabled:opacity-50 dark:border-indigo-400/60 dark:bg-indigo-500/20 dark:text-indigo-200 dark:hover:bg-indigo-500/30"
            :disabled="!canOrganize"
            :title="organizableCount === 0 ? '没有可整理的文件（可能已全部排除）' : ''"
            @click="confirming = true"
          >
            开始整理
          </button>
          <RouterLink to="/" class="text-sm text-indigo-600 hover:text-indigo-500 dark:text-indigo-300 dark:hover:text-indigo-200">← 返回首页</RouterLink>
        </div>
      </header>

      <!-- FR-9.1 二次确认：数量 + 目标目录；取消 = 零文件系统写操作 -->
      <div
        v-if="confirming"
        class="mt-4 rounded-lg border border-indigo-200 bg-indigo-50 px-4 py-3 text-sm text-indigo-900 dark:border-indigo-400/40 dark:bg-indigo-500/10 dark:text-indigo-100"
      >
        <p class="flex items-start gap-2">
          <StatusIcon kind="info" />
          <span class="min-w-0">
            即将整理: 文件数量 <span class="font-semibold">{{ formatCount(organizableCount) }}</span>
            · 目标目录 <span class="font-mono text-xs">&lt;扫描根&gt;/{{ ORGANIZE_DIR }}</span>
            · 是否继续？
          </span>
        </p>
        <p v-if="excludedCount > 0" class="mt-1 text-xs text-indigo-700/80 dark:text-indigo-200/80">
          已排除 {{ formatCount(excludedCount) }} 个文件，不会被整理。
        </p>
        <p class="mt-1 text-xs text-indigo-700/80 dark:text-indigo-200/80">同名文件将自动改名，绝不覆盖已存在的文件。</p>
        <div class="mt-2 flex items-center gap-3">
          <button
            type="button"
            class="rounded border border-indigo-300 px-3 py-1 text-indigo-800 hover:bg-indigo-100 disabled:opacity-50 dark:border-indigo-400/60 dark:text-indigo-100 dark:hover:bg-indigo-500/20"
            :disabled="starting"
            @click="startOrganize"
          >
            确认
          </button>
          <button
            type="button"
            class="rounded border border-slate-300 px-3 py-1 text-slate-700 hover:bg-slate-100 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-700/50"
            @click="confirming = false"
          >
            取消
          </button>
        </div>
      </div>
      <p v-if="organizeError" class="mt-3 flex items-center gap-1.5 text-xs text-rose-600 dark:text-rose-300">
        <StatusIcon kind="error" />
        {{ organizeError }}
      </p>

      <p v-if="blocked" class="mt-4 flex items-center gap-2 rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-300">
        <StatusIcon kind="warning" />
        本次扫描未正常完成，部分结果不进入预览与整理。请重新扫描。
      </p>

      <p v-if="loadError" class="mt-3 flex items-center gap-1.5 text-xs text-rose-600 dark:text-rose-300">
        <StatusIcon kind="error" />
        {{ loadError }}
      </p>

      <p v-if="loading" class="mt-3 flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400">
        <StatusIcon kind="loading" />
        加载中…
      </p>

      <template v-if="!blocked && !loading">
        <!-- FR-5 扫描统计 -->
        <dl v-if="summary" class="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">文件数</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatCount(summary.file_count) }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">目录数</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatCount(summary.dir_count) }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">总大小</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatSize(summary.total_size_bytes) }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">跳过</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatCount(summary.skipped_count) }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">耗时</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatDuration(summary.duration_ms) }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">平均速率</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ formatRate(summary.avg_speed) }} <span class="text-xs font-normal text-slate-500 dark:text-slate-400">文件/秒</span></dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">最大并发</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ summary.max_workers }}</dd>
          </div>
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-3 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
            <dt class="text-xs text-slate-400 dark:text-slate-500">状态</dt>
            <dd class="mt-1 text-lg font-semibold text-slate-900 dark:text-slate-100">{{ summary.status }}</dd>
          </div>
        </dl>

        <!-- FR-7 分类汇总（点击筛选） -->
        <div class="mt-5 flex flex-wrap gap-2">
          <button
            type="button"
            class="rounded-full border px-3 py-1 text-xs"
            :class="selectedCategory === '' ? 'border-indigo-300 bg-indigo-50 text-indigo-700 dark:border-indigo-400/60 dark:bg-indigo-500/20 dark:text-indigo-200' : 'border-slate-300 text-slate-700 hover:border-slate-400 dark:border-slate-600 dark:text-slate-300 dark:hover:border-slate-500'"
            @click="selectCategory('')"
          >
            全部 · {{ formatCount(summary?.file_count ?? 0) }}
          </button>
          <button
            v-for="category in CATEGORIES"
            :key="category"
            type="button"
            class="rounded-full border px-3 py-1 text-xs"
            :class="selectedCategory === category ? 'border-indigo-300 bg-indigo-50 text-indigo-700 dark:border-indigo-400/60 dark:bg-indigo-500/20 dark:text-indigo-200' : 'border-slate-300 text-slate-700 hover:border-slate-400 dark:border-slate-600 dark:text-slate-300 dark:hover:border-slate-500'"
            @click="selectCategory(category)"
          >
            {{ category }} · {{ formatCount(categoryCount(category)) }}
          </button>
        </div>

        <!-- FR-7 详情分页列表 -->
        <div class="mt-4 flex min-h-0 flex-1 flex-col rounded-lg border border-slate-200 bg-white dark:border-slate-700/60 dark:bg-slate-950/40">
          <div class="flex items-center justify-between border-b border-slate-200 px-4 py-2 dark:border-slate-700/60">
            <span class="text-xs font-medium text-slate-500 dark:text-slate-400">
              {{ selectedCategory === '' ? '全部文件' : selectedCategory }} · 共 {{ formatCount(totalCount) }} 项
            </span>
            <span class="text-xs text-slate-400 dark:text-slate-500">{{ PAGE_SIZE }} 项/页</span>
          </div>

          <div class="min-h-0 flex-1 overflow-auto">
            <p v-if="!items.length" class="py-8 text-center text-sm text-slate-400 dark:text-slate-500">当前筛选下暂无文件</p>
            <table v-else class="w-full border-collapse text-left text-xs">
              <thead class="sticky top-0 bg-slate-50 text-slate-500 dark:bg-slate-900/95 dark:text-slate-400">
                <tr>
                  <th class="px-4 py-2 font-medium">文件名 / 路径</th>
                  <th class="px-3 py-2 font-medium">大小</th>
                  <th class="px-3 py-2 font-medium">类别</th>
                  <th class="px-3 py-2 font-medium">置信度</th>
                  <th class="px-3 py-2 font-medium">原因</th>
                  <th class="px-3 py-2 font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in items"
                  :key="row.id"
                  class="border-t border-slate-100 dark:border-slate-800"
                  :class="row.excluded ? 'opacity-50' : ''"
                >
                  <td class="max-w-xs px-4 py-2">
                    <p class="truncate text-slate-800 dark:text-slate-200" :title="row.path">{{ row.name }}</p>
                    <p class="truncate text-[10px] text-slate-400 dark:text-slate-500" :title="row.path">{{ row.path }}</p>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 text-slate-700 dark:text-slate-300">{{ formatSize(row.size_bytes) }}</td>
                  <td class="px-3 py-2">
                    <select
                      class="rounded border border-slate-300 bg-white px-2 py-1 text-slate-800 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-200"
                      :value="row.category"
                      :disabled="busyFileID === row.id"
                      @change="changeCategory(row, $event)"
                    >
                      <option v-for="category in CATEGORIES" :key="category" :value="category">{{ category }}</option>
                    </select>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 text-slate-700 dark:text-slate-300">{{ confidenceText(row) }}</td>
                  <td class="max-w-xs px-3 py-2 text-slate-500 dark:text-slate-400">
                    <span class="line-clamp-2" :title="row.reason">{{ row.reason }}</span>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2">
                    <button
                      type="button"
                      class="rounded border px-2 py-1 disabled:opacity-50"
                      :class="row.excluded ? 'border-emerald-300 text-emerald-700 hover:bg-emerald-50 dark:border-emerald-400/50 dark:text-emerald-300 dark:hover:bg-emerald-500/10' : 'border-rose-300 text-rose-600 hover:bg-rose-50 dark:border-rose-400/50 dark:text-rose-300 dark:hover:bg-rose-500/10'"
                      :disabled="busyFileID === row.id"
                      @click="toggleExcluded(row)"
                    >
                      {{ row.excluded ? '恢复' : '排除' }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="flex items-center justify-between border-t border-slate-200 px-4 py-2 text-xs text-slate-500 dark:border-slate-700/60 dark:text-slate-400">
            <button
              type="button"
              class="rounded border border-slate-300 px-3 py-1 disabled:opacity-40 dark:border-slate-600"
              :disabled="currentPage <= 1"
              @click="gotoPage(currentPage - 1)"
            >
              上一页
            </button>
            <span>第 {{ currentPage }} / {{ Math.max(totalPage, 1) }} 页</span>
            <button
              type="button"
              class="rounded border border-slate-300 px-3 py-1 disabled:opacity-40 dark:border-slate-600"
              :disabled="totalPage === 0 || currentPage >= totalPage"
              @click="gotoPage(currentPage + 1)"
            >
              下一页
            </button>
          </div>
        </div>

        <p v-if="rowError" class="mt-2 flex items-center gap-1.5 text-xs text-rose-600 dark:text-rose-300">
          <StatusIcon kind="error" />
          {{ rowError }}
        </p>
      </template>
    </template>
  </section>
</template>
