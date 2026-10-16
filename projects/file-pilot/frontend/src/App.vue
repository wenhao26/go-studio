<script lang="ts" setup>
// 应用外壳：顶部导航 + 路由出口 + 「当前任务」指示器 + 主题切换。
//
// 交互硬规则（requirements §九 · 2）：任务进行中允许离开页面，任务在后台
// 继续运行，并通过本指示器返回扫描页——因此无需导航拦截。
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import StatusIcon from './components/StatusIcon.vue'
import { dismissRecoveryNotice, hasRunningTask, useRecoveryNotice } from './lib/task'
import { theme, toggleTheme } from './lib/theme'

const route = useRoute()

const running = computed(() => hasRunningTask())

// FR-12：上次异常退出遗留的任务已置 failed，启动时提示用户（可关闭）。
const recovery = useRecoveryNotice()
const recoveryText = computed(() => {
  const n = recovery.notice
  if (!n) {
    return ''
  }
  const parts: string[] = []
  if (n.recovered_tasks > 0) {
    parts.push(`${n.recovered_tasks} 个任务`)
  }
  if (n.recovered_scans > 0) {
    parts.push(`${n.recovered_scans} 次扫描`)
  }
  if (parts.length === 0) {
    return ''
  }
  return `上次运行异常中断，已将 ${parts.join('、')} 标记为失败（原因：应用中断）。可重新发起扫描。`
})
const showRecovery = computed(() => !recovery.dismissed && recoveryText.value !== '')

// 导航链接按当前路由选定样式，而非叠加 active-class：同时出现
// text-slate-500 与 text-slate-900 时，同等特指度下由样式表顺序决定胜负，
// 结果不确定；这里保证任一链接任何时刻只出现一种文字颜色。
const navLinkClass = 'flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs transition-colors'
const navIdleClass = 'border-transparent text-slate-500 hover:text-slate-900 dark:hover:text-white'
const navActiveClass = 'border-indigo-500 text-slate-900 dark:border-indigo-400 dark:text-white'
</script>

<template>
  <div class="relative flex h-full flex-col bg-gradient-to-b from-slate-50 via-slate-50 to-slate-100 text-slate-800 dark:from-slate-900 dark:via-slate-900 dark:to-slate-950 dark:text-slate-100">
    <!-- 氛围光：顶部居中的靛蓝柔光，纯装饰（不接收指针事件），不参与布局。 -->
    <div aria-hidden="true" class="pointer-events-none absolute inset-x-0 top-0 h-96 bg-[radial-gradient(60%_100%_at_50%_0%,rgba(99,102,241,0.10),transparent_72%)] dark:bg-[radial-gradient(60%_100%_at_50%_0%,rgba(99,102,241,0.18),transparent_72%)]" />
    <header class="relative flex items-center gap-4 border-b border-slate-200 bg-white/85 px-6 py-3 dark:border-slate-700/60 dark:bg-slate-900/80">
      <RouterLink to="/" class="flex items-center gap-3">
        <span class="flex h-12 w-12 items-center justify-center">
          <!-- 品牌标志：无背景容器，直接绘制「文件夹 + 归位箭头」图形——
               文件夹即「文件」，向下箭头即「文件归位」，一眼可读；
               本体靛→紫渐变、箭头白色，深浅主题下皆清晰。 -->
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" class="h-9 w-9" aria-hidden="true">
            <defs>
              <linearGradient id="fpLogoGradient" x1="0" y1="0" x2="1" y2="1">
                <stop offset="0%" stop-color="#6366f1" />
                <stop offset="100%" stop-color="#8b5cf6" />
              </linearGradient>
            </defs>
            <path fill="url(#fpLogoGradient)" d="M5.5 4.5h4.1c.53 0 1.04.21 1.41.59l1.4 1.4c.38.38.88.59 1.41.59h4.68A2.5 2.5 0 0 1 21 9.58V17a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17V7a2.5 2.5 0 0 1 2.5-2.5Z" />
            <path fill="none" stroke="#ffffff" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" d="M12 10.2V14.6M9.4 12.1 12 14.6 14.6 12.1" />
          </svg>
        </span>
        <div class="flex flex-col">
          <span class="text-2xl font-bold leading-tight text-slate-900 dark:text-white">FilePilot</span>
          <span class="text-xs leading-tight text-slate-500 dark:text-slate-400">智能文件整理助手</span>
        </div>
      </RouterLink>

      <div class="h-8 w-px bg-slate-200 dark:bg-slate-700/60" />

      <nav class="flex items-center gap-1">
        <RouterLink
          to="/"
          :class="[navLinkClass, route.name === 'home' ? navActiveClass : navIdleClass]"
        >
          <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="h-4 w-4">
            <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12l8.954-8.955a1.126 1.126 0 011.591 0L21.75 12M4.5 9.75v10.125c0 .621.504 1.125 1.125 1.125H9.75v-4.875c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125V21h4.125c.621 0 1.125-.504 1.125-1.125V9.75M8.25 21h8.25" />
          </svg>
          首页
        </RouterLink>
        <RouterLink
          to="/scan"
          :class="[navLinkClass, route.name === 'scan' ? navActiveClass : navIdleClass]"
        >
          <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="h-4 w-4">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          扫描中
        </RouterLink>
      </nav>

      <div class="flex-1" />

      <RouterLink
        v-if="running"
        to="/scan"
        class="flex items-center gap-2 rounded-full border border-emerald-300 bg-emerald-50 px-3 py-1 text-xs text-emerald-700 hover:bg-emerald-100 dark:border-emerald-400/40 dark:bg-emerald-400/10 dark:text-emerald-300 dark:hover:bg-emerald-400/20"
      >
        <span class="h-2 w-2 animate-pulse rounded-full bg-emerald-500 dark:bg-emerald-400" />
        当前任务 · 扫描中
      </RouterLink>

      <!-- 主题切换：浅色 / 深色 -->
      <button
        type="button"
        class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-slate-300 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:border-slate-600 dark:text-slate-400 dark:hover:bg-slate-700/50 dark:hover:text-slate-100"
        :title="theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'"
        :aria-label="theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'"
        @click="toggleTheme"
      >
        <!-- 当前为浅色时显示月亮（点击进入深色） -->
        <svg v-if="theme !== 'dark'" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" class="h-4 w-4" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z" />
        </svg>
        <!-- 当前为深色时显示太阳（点击返回浅色） -->
        <svg v-else xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor" class="h-4 w-4" aria-hidden="true">
          <circle cx="12" cy="12" r="4.25" />
          <path stroke-linecap="round" d="M12 2v2.5M12 19.5V22M2 12h2.5M19.5 12H22M4.93 4.93l1.77 1.77M17.3 17.3l1.77 1.77M19.07 4.93L17.3 6.7M6.7 17.3l-1.77 1.77" />
        </svg>
      </button>
    </header>

    <div
      v-if="showRecovery"
      class="relative flex items-start gap-3 border-b border-amber-300 bg-amber-50 px-6 py-2.5 text-sm text-amber-800 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-200"
      role="status"
    >
      <StatusIcon kind="warning" />
      <span class="min-w-0 flex-1">{{ recoveryText }}</span>
      <button
        type="button"
        class="shrink-0 rounded px-2 py-0.5 text-xs text-amber-700 hover:bg-amber-100 dark:text-amber-200 dark:hover:bg-amber-400/20"
        @click="dismissRecoveryNotice"
      >
        关闭
      </button>
    </div>

    <main class="relative min-h-0 flex-1 overflow-hidden">
      <RouterView />
    </main>
  </div>
</template>
