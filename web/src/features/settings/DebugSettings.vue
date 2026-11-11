<script setup lang="ts">
// Debug panel settings: a per-browser switch that only controls whether the
// floating debug launch button is shown. Capture itself is started/stopped
// inside the panel and is independent of the drawer's visibility. No server
// round-trip by design — the debug panel is a developer aid, its captured frames
// never persist and the switch itself lives in localStorage.
import { useDebugPanelStore } from '../../stores/debugPanel'

const debug = useDebugPanelStore()

function toggleDebugPanel(event: Event) {
  debug.setEnabled((event.target as HTMLInputElement).checked)
}
</script>

<template>
  <section class="debug-settings w-full space-y-6" aria-label="调试面板设置">
    <header class="settings-head pb-4 border-b border-slate-200 dark:border-slate-800">
      <h1 class="text-2xl font-semibold text-slate-900 dark:text-slate-100 tracking-tight">调试面板</h1>
      <p class="text-xs text-slate-500 dark:text-slate-400 mt-1">
        实时查看本浏览器收到的 SSE 总线消息（Agent 会话、纯对话、任务协同流），用于联调与排查。
      </p>
    </header>

    <fieldset class="setting-group p-5 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 space-y-4">
      <div class="group-header">
        <legend class="text-sm font-semibold text-slate-800 dark:text-slate-200 px-0">启用入口</legend>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
          开启后，全站右下角显示调试入口按钮；打开抽屉后点击「开启」开始捕获，关闭抽屉不会停止捕获或清空已捕获帧。
        </p>
      </div>

      <label class="flex items-start justify-between gap-4 cursor-pointer" data-test="debug-enable-row">
        <span class="min-w-0">
          <span class="block text-sm font-medium text-slate-800 dark:text-slate-200">显示调试面板入口</span>
          <span class="block text-xs text-slate-500 dark:text-slate-400 mt-1 leading-5">
            捕获由面板内的「开启 / 停止」控制；关闭面板不会停止捕获，也不会清空已捕获帧。
            关闭此入口开关会停止捕获并保留当前缓冲；数据仅保存在本浏览器内存，不落库。
          </span>
        </span>
        <input
          type="checkbox"
          class="mt-1 h-4 w-4 shrink-0"
          :checked="debug.enabled"
          data-test="debug-enable-toggle"
          aria-label="显示调试面板入口"
          @change="toggleDebugPanel"
        />
      </label>
    </fieldset>
  </section>
</template>
