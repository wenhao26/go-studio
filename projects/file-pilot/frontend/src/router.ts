// 应用路由（requirements §九；裁决 D-A5 引入 vue-router）。
//
// 4 条路由与 §九 对齐：P1/P2（首页、扫描中）Phase 1，P3（分类结果）Phase 2，
// P4（整理与撤销）Phase 3。
//
// 使用 hash 历史：生产由 Wails 内嵌资源（embed.FS + assetserver）直接提供
// 静态 index.html，没有服务端重写，hash 模式可避免刷新/深链 404。

import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import HomeView from './views/HomeView.vue'
import ScanView from './views/ScanView.vue'
import ResultsView from './views/ResultsView.vue'
import OrganizeView from './views/OrganizeView.vue'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: HomeView,
    meta: { title: '首页' },
  },
  {
    path: '/scan',
    name: 'scan',
    component: ScanView,
    meta: { title: '扫描中' },
  },
  {
    path: '/results',
    name: 'results',
    component: ResultsView,
    meta: { title: '扫描结果' },
  },
  {
    path: '/organize',
    name: 'organize',
    component: OrganizeView,
    meta: { title: '整理与撤销' },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})
