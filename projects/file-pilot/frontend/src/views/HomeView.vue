<script lang="ts" setup>
// P1 首页（FR-1 目录选择、FR-2 扫描配置、最近目录、开始扫描）。
//
// 目录与配置的硬规则：
// - 未选目录点开始：前端拦截（不调后端，FR-1）；
// - 并发数非法：前端即时提示，不静默钳制（FR-2）；
// - 「预计文件」恒为「未知」：不做预扫描/估算（FR-1）。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import * as API from '../../wailsjs/go/app/API'
import type { scanner } from '../../wailsjs/go/models'
import StatusIcon from '../components/StatusIcon.vue'
import { friendlyMessage, NOT_SELECTED_DIR } from '../lib/errors'
import { ensureTask, hasRunningTask } from '../lib/task'

const router = useRouter()

const config = reactive({
  workers: 1,
  scan_hidden: false,
  scan_system: false,
  skip_large: false,
})

const selectedDir = ref('')
const recentDirs = ref<string[]>([])

const loading = ref(true)
const loadError = ref('')
const picking = ref(false)
const starting = ref(false)
const dirNotice = ref('')
const configError = ref('')
const startError = ref('')
const recentError = ref('')

// running 反映后端是否存在进行中任务（含启动时恢复的任务）；响应式推导。
const running = computed(() => hasRunningTask())

// validateWorkers 校验并发数并在非法时给出文案（FR-2：立即报错，不钳制）。
function validateWorkers(): boolean {
  const w = config.workers as unknown
  if (typeof w !== 'number' || !Number.isInteger(w) || w < 1 || w > 64) {
    configError.value = '并发数需在 1–64 之间'
    return false
  }
  configError.value = ''
  return true
}

async function loadSettings(): Promise<void> {
  try {
    const settings = await API.GetSettings()
    config.workers = settings.workers
    config.scan_hidden = settings.scan_hidden
    config.scan_system = settings.scan_system
    config.skip_large = settings.skip_large
    recentDirs.value = settings.recent_dirs ?? []
  } catch (err) {
    loadError.value = friendlyMessage(err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadSettings()
})

async function chooseDirectory(): Promise<void> {
  if (picking.value) {
    return
  }
  picking.value = true
  dirNotice.value = ''
  startError.value = ''
  try {
    const dir = await API.ChooseDirectory()
    if (dir) {
      selectedDir.value = dir
    }
  } catch (err) {
    // 取消目录选择命中 ErrDirectoryPickCanceled，映射为友好文案（FR-1）。
    dirNotice.value = friendlyMessage(err)
  } finally {
    picking.value = false
  }
}

function selectRecent(dir: string): void {
  selectedDir.value = dir
  dirNotice.value = ''
  startError.value = ''
}

// removeRecent 移除单条最近目录（contracts §3.1：recent_dirs 由服务端维护，
// 前端只能经 RemoveRecentDir 受控变更）。成功后即时更新本地列表；失败给出提示。
async function removeRecent(dir: string): Promise<void> {
  recentError.value = ''
  try {
    await API.RemoveRecentDir(dir)
    recentDirs.value = recentDirs.value.filter((d) => d !== dir)
  } catch (err) {
    recentError.value = friendlyMessage(err)
  }
}

function onWorkersInput(): void {
  // 输入即校验：非法并发数立即提示（不等待提交）。
  validateWorkers()
}

async function onConfigChange(): Promise<void> {
  if (!validateWorkers()) {
    return
  }
  try {
    await API.SaveSettings({ ...config } as scanner.Config)
    configError.value = ''
  } catch (err) {
    configError.value = friendlyMessage(err)
  }
}

async function startScan(): Promise<void> {
  startError.value = ''
  if (starting.value) {
    return
  }
  if (!selectedDir.value) {
    startError.value = NOT_SELECTED_DIR
    return
  }
  if (!validateWorkers()) {
    return
  }

  starting.value = true
  try {
    const taskId = await API.StartScan(selectedDir.value, { ...config } as scanner.Config)
    ensureTask(taskId, selectedDir.value)
    await router.push('/scan')
  } catch (err) {
    // 单活动冲突命中 ErrTaskRunning → 「当前有任务进行中」。
    startError.value = friendlyMessage(err)
  } finally {
    starting.value = false
  }
}
</script>

<template>
  <section class="mx-auto h-full max-w-3xl overflow-y-auto px-6 py-8">
    <h1 class="text-2xl font-semibold text-slate-900 dark:text-slate-100">开始扫描</h1>
    <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">选择一个目录，配置扫描参数后开始。</p>

    <p v-if="loading" class="mt-6 flex items-center gap-2 text-sm text-slate-500 dark:text-slate-400">
      <StatusIcon kind="loading" />
      正在加载设置…
    </p>
    <p v-else-if="loadError" class="mt-6 flex items-center gap-2 rounded-md border border-rose-300 bg-rose-50 px-4 py-3 text-sm text-rose-600 dark:border-rose-400/40 dark:bg-rose-500/10 dark:text-rose-300">
      <StatusIcon kind="error" />
      {{ loadError }}
    </p>

    <template v-else>
      <div class="mt-6 rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-200">扫描目录</h2>
          <span class="text-xs text-slate-400 dark:text-slate-500">预计文件：未知</span>
        </div>

        <div class="mt-3 flex items-center gap-3">
          <div class="min-w-0 flex-1 truncate rounded-md border border-slate-300 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900/60" :class="selectedDir ? 'text-slate-800 dark:text-slate-200' : 'text-slate-400 dark:text-slate-500'">
            {{ selectedDir || '尚未选择目录' }}
          </div>
          <button
            type="button"
            class="shrink-0 rounded-md border border-slate-300 bg-slate-100 px-4 py-2 text-sm text-slate-700 hover:bg-slate-200 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-600 dark:bg-slate-700/60 dark:text-slate-100 dark:hover:bg-slate-700"
            :disabled="picking"
            @click="chooseDirectory"
          >
            {{ picking ? '选择中…' : '选择目录' }}
          </button>
        </div>

        <p v-if="dirNotice" class="mt-2 flex items-center gap-1.5 text-xs text-amber-700 dark:text-amber-300">
          <StatusIcon kind="warning" />
          {{ dirNotice }}
        </p>

        <div class="mt-4">
          <div class="text-xs text-slate-400 dark:text-slate-500">最近目录</div>
          <div v-if="recentDirs.length" class="mt-2 flex flex-wrap gap-2">
            <div
              v-for="dir in recentDirs"
              :key="dir"
              class="flex max-w-full items-center rounded-full border border-slate-300 text-xs text-slate-700 hover:border-slate-400 dark:border-slate-600 dark:text-slate-300 dark:hover:border-slate-500"
            >
              <button
                type="button"
                class="truncate py-1 pl-3 pr-1 hover:text-slate-900 dark:hover:text-white"
                :title="dir"
                @click="selectRecent(dir)"
              >
                {{ dir }}
              </button>
              <button
                type="button"
                class="shrink-0 py-1 pl-1 pr-2 text-slate-400 transition-colors hover:text-rose-500 dark:text-slate-500 dark:hover:text-rose-300"
                :title="`移除 ${dir}`"
                :aria-label="`从最近目录移除 ${dir}`"
                @click="removeRecent(dir)"
              >
                ×
              </button>
            </div>
          </div>
          <p v-else class="mt-2 text-xs text-slate-400 dark:text-slate-500">暂无最近目录</p>
          <p v-if="recentError" class="mt-2 flex items-center gap-1.5 text-xs text-rose-600 dark:text-rose-300">
            <StatusIcon kind="error" />
            {{ recentError }}
          </p>
        </div>
      </div>

      <div class="mt-5 rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-700/60 dark:bg-slate-800/40 dark:shadow-none">
        <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-200">扫描配置</h2>

        <div class="mt-3 flex items-center gap-3">
          <label for="workers" class="w-28 shrink-0 text-sm text-slate-700 dark:text-slate-300">扫描并发数</label>
          <input
            id="workers"
            v-model.number="config.workers"
            type="number"
            min="1"
            max="64"
            class="w-28 rounded-md border border-slate-300 bg-white px-3 py-1.5 text-sm text-slate-900 focus:border-indigo-500 focus:outline-none dark:border-slate-700 dark:bg-slate-900/60 dark:text-slate-100"
            @input="onWorkersInput"
            @change="onConfigChange"
          />
          <span class="text-xs text-slate-400 dark:text-slate-500">允许 1–64</span>
        </div>
        <p v-if="configError" class="mt-2 flex items-center gap-1.5 text-xs text-rose-600 dark:text-rose-300">
          <StatusIcon kind="error" />
          {{ configError }}
        </p>

        <div class="mt-4 space-y-3">
          <label class="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
            <input v-model="config.scan_hidden" type="checkbox" class="h-4 w-4 rounded border-slate-300 bg-white dark:border-slate-600 dark:bg-slate-900" @change="onConfigChange" />
            扫描隐藏文件
          </label>
          <label class="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
            <input v-model="config.scan_system" type="checkbox" class="h-4 w-4 rounded border-slate-300 bg-white dark:border-slate-600 dark:bg-slate-900" @change="onConfigChange" />
            扫描系统文件
          </label>
          <label class="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
            <input v-model="config.skip_large" type="checkbox" class="h-4 w-4 rounded border-slate-300 bg-white dark:border-slate-600 dark:bg-slate-900" @change="onConfigChange" />
            跳过大文件（&gt; 1 GiB）
          </label>
        </div>
      </div>

      <div v-if="running" class="mt-5 flex items-center gap-2 rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-300">
        <StatusIcon kind="loading" />
        <span>当前有任务进行中。</span>
        <RouterLink to="/scan" class="underline">前往查看</RouterLink>
      </div>

      <div class="mt-6 flex items-center gap-4">
        <button
          type="button"
          class="rounded-md bg-indigo-600 px-6 py-2.5 text-sm font-medium text-white hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-500 dark:hover:bg-indigo-400"
          :disabled="picking || starting || running"
          @click="startScan"
        >
          {{ starting ? '启动中…' : '开始扫描' }}
        </button>
        <span v-if="startError" class="flex items-center gap-1.5 text-sm text-rose-600 dark:text-rose-300">
          <StatusIcon kind="error" />
          {{ startError }}
        </span>
      </div>
    </template>
  </section>
</template>
