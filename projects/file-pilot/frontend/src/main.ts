import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { initTaskEvents, syncActiveTask, syncRecoveryNotice } from './lib/task'
import { applyTheme } from './lib/theme'
import './style.css'

// 应用启动即恢复上次主题（浅色 / 深色），先于挂载以避免首帧闪色。
applyTheme()

// 事件订阅必须先于挂载：任务事件可能在任何组件渲染前到达。
initTaskEvents()

const app = createApp(App)
app.use(router)
app.mount('#app')

// 启动即对齐后端活动任务（FR-12：异常恢复后 UI 归位）。
// 失败不阻断 UI：后续事件流入会自然对齐。
void syncActiveTask().catch(() => {
  /* 忽略：无活动任务或后端尚未就绪均属正常。 */
})

// 启动即取回异常恢复提示（FR-12：上次中断任务已置失败，提示用户）。
void syncRecoveryNotice().catch(() => {
  /* 忽略：无恢复或后端尚未就绪均属正常。 */
})
