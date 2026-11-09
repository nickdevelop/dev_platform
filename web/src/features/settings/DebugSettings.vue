<script setup lang="ts">
// Debug panel settings: a per-browser switch that only controls whether the
// floating debug launch button is shown. No server round-trip by design — the
// debug panel is a developer aid, its captured messages never persist and the
// switch itself lives in localStorage.
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
          开启后，全站右下角显示调试入口按钮；点击按钮打开浮动调试抽屉并开始捕获。
        </p>
      </div>

      <label class="flex items-start justify-between gap-4 cursor-pointer" data-test="debug-enable-row">
        <span class="min-w-0">
          <span class="block text-sm font-medium text-slate-800 dark:text-slate-200">显示调试面板入口</span>
          <span class="block text-xs text-slate-500 dark:text-slate-400 mt-1 leading-5">
            仅有"开关开启 + 面板打开"两个条件同时满足时才捕获与显示总线消息；关闭面板立即停止捕获并清空数据。
            开关状态仅保存在本浏览器，不随账号同步，也不保存任何历史消息。
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
