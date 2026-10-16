<script lang="ts" setup>
// 状态提示图标（纯展示组件，无任何逻辑）：给成功 / 失败 / 警告 / 信息 /
// 进行中提示配统一图标。用实心（fill=currentColor）而非描边——16px 小尺寸
// 下描边线条过细、不易辨认；颜色仍随父级文字色，随主题自适应。
//
// 状态图标为 Heroicons v2 solid mini（20/solid，专为小尺寸设计）；
// loading 为 Heroicons v2 solid arrow-path + animate-spin。
defineProps<{ kind: 'success' | 'error' | 'warning' | 'info' | 'loading' }>()
</script>

<template>
  <svg
    v-if="kind === 'loading'"
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    fill="currentColor"
    class="h-4 w-4 shrink-0 animate-spin"
    aria-hidden="true"
  >
    <path fill-rule="evenodd" clip-rule="evenodd" d="M4.755 10.059a7.5 7.5 0 0112.548-3.364l1.903 1.903h-3.183a.75.75 0 100 1.5h4.992a.75.75 0 00.75-.75V4.356a.75.75 0 00-1.5 0v3.18l-1.9-1.9A9 9 0 003.306 9.67a.75.75 0 101.45.388zm15.408 3.352a.75.75 0 00-.919.53 7.5 7.5 0 01-12.548 3.364l-1.902-1.903h3.183a.75.75 0 000-1.5H2.984a.75.75 0 00-.75.75v4.992a.75.75 0 001.5 0v-3.18l1.9 1.9a9 9 0 0015.059-4.035.75.75 0 00-.53-.918z" />
  </svg>
  <svg
    v-else-if="kind === 'success'"
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 20 20"
    fill="currentColor"
    class="h-4 w-4 shrink-0"
    aria-hidden="true"
  >
    <path fill-rule="evenodd" clip-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" />
  </svg>
  <svg
    v-else-if="kind === 'error'"
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 20 20"
    fill="currentColor"
    class="h-4 w-4 shrink-0"
    aria-hidden="true"
  >
    <path fill-rule="evenodd" clip-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" />
  </svg>
  <svg
    v-else-if="kind === 'warning'"
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 20 20"
    fill="currentColor"
    class="h-4 w-4 shrink-0"
    aria-hidden="true"
  >
    <path fill-rule="evenodd" clip-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" />
  </svg>
  <svg
    v-else
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 20 20"
    fill="currentColor"
    class="h-4 w-4 shrink-0"
    aria-hidden="true"
  >
    <path fill-rule="evenodd" clip-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z" />
  </svg>
</template>
