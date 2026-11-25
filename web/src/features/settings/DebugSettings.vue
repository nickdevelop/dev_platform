<script setup lang="ts">
// Debug panel settings: a per-browser switch that only controls whether the
// floating debug launch button is shown. Capture itself is started/stopped
// inside the panel and is independent of the drawer's visibility. No server
// round-trip by design — the debug panel is a developer aid, its captured frames
// never persist and the switch itself lives in localStorage.
import { useDebugPanelStore } from '../../stores/debugPanel'

const debug = useDebugPanelStore()

function onToggle(checked: boolean) {
  debug.setEnabled(checked)
}
</script>

<template>
  <section class="debug-settings space-y-4" aria-label="调试面板设置">
    <header class="settings-page-head">
      <h1 class="settings-page-title">调试面板</h1>
      <p class="settings-page-desc">
        实时查看本浏览器收到的 SSE 总线消息（Agent 会话、纯对话、任务协同流），用于联调与排查。
      </p>
    </header>

    <section class="settings-panel" aria-labelledby="debug-entry-title">
      <header class="settings-panel-head">
        <h2 id="debug-entry-title" class="settings-panel-title">启用入口</h2>
        <p class="settings-panel-desc">
          开启后，全站右下角显示调试入口按钮；打开抽屉后点击「开启」开始捕获，关闭抽屉不会停止捕获或清空已捕获帧。
        </p>
      </header>

      <div class="settings-switch-row" data-test="debug-enable-row">
        <div class="min-w-0">
          <div class="settings-switch-title">显示调试面板入口</div>
          <p class="settings-switch-desc">
            捕获由面板内的「开启 / 停止」控制；关闭面板不会停止捕获，也不会清空已捕获帧。
            关闭此入口开关会停止捕获并保留当前缓冲；数据仅保存在本浏览器内存，不落库。
          </p>
        </div>

        <el-switch
          class="shrink-0"
          :model-value="debug.enabled"
          aria-label="显示调试面板入口"
          data-test="debug-enable-toggle"
          @change="onToggle($event as boolean)"
        />
      </div>
    </section>
  </section>
</template>
