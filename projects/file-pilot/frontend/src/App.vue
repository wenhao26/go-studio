<script lang="ts" setup>
// 应用外壳：顶部导航 + 路由出口 + 「当前任务」指示器。
//
// 交互硬规则（requirements §九 · 2）：任务进行中允许离开页面，任务在后台
// 继续运行，并通过本指示器返回扫描页——因此无需导航拦截。
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import StatusIcon from './components/StatusIcon.vue'
import { dismissRecoveryNotice, hasRunningTask, useRecoveryNotice } from './lib/task'

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
// text-slate-300 与 text-white 时，同等特指度下由样式表顺序决定胜负，结果不确定；
// 这里保证任一链接任何时刻只出现一种文字颜色。
const navLinkClass = 'flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs transition-colors'
const navIdleClass = 'border-transparent text-slate-500 hover:text-slate-300'
const navActiveClass = 'border-indigo-400 text-white'
</script>

<template>
  <div class="flex h-full flex-col bg-slate-900 text-slate-100">
    <header class="flex items-center gap-4 border-b border-slate-700/60 bg-slate-900/80 px-6 py-3">
      <RouterLink to="/" class="flex items-center gap-3">
        <span class="flex h-12 w-12 items-center justify-center rounded-2xl bg-indigo-500/20 text-indigo-300 ring-1 ring-inset ring-indigo-400/30">
          <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="h-7 w-7">
            <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
          </svg>
        </span>
        <div class="flex flex-col">
          <span class="text-2xl font-bold leading-tight text-white">FilePilot</span>
          <span class="text-[10px] leading-tight text-slate-500">智能文件整理助手</span>
        </div>
      </RouterLink>

      <div class="h-8 w-px bg-slate-700/60" />

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
        class="flex items-center gap-2 rounded-full border border-emerald-400/40 bg-emerald-400/10 px-3 py-1 text-xs text-emerald-300 hover:bg-emerald-400/20"
      >
        <span class="h-2 w-2 animate-pulse rounded-full bg-emerald-400" />
        当前任务 · 扫描中
      </RouterLink>
    </header>

    <div
      v-if="showRecovery"
      class="flex items-start gap-3 border-b border-amber-400/40 bg-amber-400/10 px-6 py-2.5 text-sm text-amber-200"
      role="status"
    >
      <StatusIcon kind="warning" />
      <span class="min-w-0 flex-1">{{ recoveryText }}</span>
      <button
        type="button"
        class="shrink-0 rounded px-2 py-0.5 text-xs text-amber-200 hover:bg-amber-400/20"
        @click="dismissRecoveryNotice"
      >
        关闭
      </button>
    </div>

    <main class="min-h-0 flex-1 overflow-hidden">
      <RouterView />
    </main>
  </div>
</template>
