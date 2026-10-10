<template>
  <section class="empty-workspace" aria-label="未选择内容">
    <p>从左侧选择一个项目或任务。</p>
    <p class="hint">项目设置与任务对话会显示在这里。</p>
  </section>
</template>

<style scoped>
.empty-workspace {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #64748b;
  gap: 0.25rem;
}

.empty-workspace p {
  margin: 0;
}

.hint {
  font-size: 0.85rem;
  color: #94a3b8;
}
</style>
