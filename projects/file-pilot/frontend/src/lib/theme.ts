// 主题（浅色 / 深色）切换。
//
// 实现方式：唯一开关是 <html> 上的 `dark` 类——Tailwind 的 dark: 变体据此
// 生效（见 style.css 的 @custom-variant）。组件只声明「浅色基准 + dark: 覆盖」，
// 不需要各自感知当前主题。
//
// 持久化到 localStorage；读取失败或首次启动一律回落到浅色（保持既有观感，
// 不擅自跟随系统，避免与用户预期不一致）。
import { ref } from 'vue'

export type Theme = 'light' | 'dark'

const STORAGE_KEY = 'file-pilot:theme'

// readStored 读取上次选择；除 'dark' 外一律视为浅色。
function readStored(): Theme {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'dark' ? 'dark' : 'light'
  } catch {
    return 'light'
  }
}

// theme 是当前主题；模板据此切换图标与提示文案。
export const theme = ref<Theme>(readStored())

// applyTheme 把当前主题同步到 <html>（启动时与切换后调用）。
export function applyTheme(): void {
  document.documentElement.classList.toggle('dark', theme.value === 'dark')
}

// toggleTheme 在浅色 / 深色之间切换并持久化。
export function toggleTheme(): void {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  applyTheme()
  try {
    localStorage.setItem(STORAGE_KEY, theme.value)
  } catch {
    // 持久化失败（如隐私模式）不影响本次切换生效。
  }
}
