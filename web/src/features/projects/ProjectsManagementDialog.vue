<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useProjectsStore } from '../../stores/projects'
import { unregisterProject, type Project } from '../../api/projects'
import { describeApiError } from '../../api/client'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  'update:visible': [val: boolean]
  'open-settings': [projectId: number]
}>()

const projectsStore = useProjectsStore()
const submitting = ref(false)

const projects = computed(() => projectsStore.orderedProjects)
const totalCount = computed(() => projects.value.length)
const visibleCount = computed(() => projectsStore.orderedVisibleProjects.length)

function isHidden(id: number): boolean {
  return projectsStore.isHidden(id)
}

function toggleVisibility(id: number) {
  projectsStore.toggleProjectVisibility(id)
}

function moveUp(id: number) {
  projectsStore.moveProjectUp(id)
}

function moveDown(id: number) {
  projectsStore.moveProjectDown(id)
}

function handleOpenSettings(projectId: number) {
  emit('open-settings', projectId)
}

async function handleUnregister(project: Project) {
  try {
    await ElMessageBox.confirm(
      `确定将项目「${project.name}」从平台中解除登记吗？磁盘上的完整目录（${project.path}）会完全保留，不会被修改或删除。`,
      '解除项目登记',
      {
        confirmButtonText: '解除登记',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger',
        type: 'warning',
      },
    )
  } catch {
    return
  }

  submitting.value = true
  try {
    await unregisterProject(project.id)
    projectsStore.projects = projectsStore.projects.filter((p) => p.id !== project.id)
    ElMessage.success(`已解除项目「${project.name}」登记`)
  } catch (err) {
    ElMessage.error(describeApiError(err))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="props.visible"
    title="项目列表与显示设置"
    width="820px"
    destroy-on-close
    data-test="projects-management-dialog"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="projects-dialog-body">
      <div class="dialog-desc-bar">
        <span class="dialog-desc-text">
          自定义项目在左侧栏中的显示与排列顺序；被隐藏的项目仍保留在平台中。
        </span>
        <span class="count-chip">{{ visibleCount }} / {{ totalCount }} 已显示</span>
      </div>

      <div v-if="projects.length === 0" class="empty-state">
        <p>当前暂未登记任何项目，请在侧边栏新建项目。</p>
      </div>

      <div v-else class="project-table-card">
        <div class="table-head-row">
          <span class="col-order">顺序</span>
          <span class="col-name">项目名称</span>
          <span class="col-path">工作区路径</span>
          <span class="col-visible">侧边栏显示</span>
          <span class="col-actions">操作</span>
        </div>

        <ul class="project-list" aria-label="项目列表">
          <li
            v-for="(project, index) in projects"
            :key="project.id"
            class="project-item"
            :class="{ 'is-hidden-project': isHidden(project.id) }"
            data-test="project-manage-row"
            :data-project-id="project.id"
          >
            <div class="col-order order-group">
              <button
                type="button"
                class="order-btn"
                :disabled="index === 0"
                title="上移此项目"
                aria-label="上移此项目"
                data-test="btn-move-up"
                @click="moveUp(project.id)"
              >
                <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="4 10 8 6 12 10" />
                </svg>
              </button>
              <button
                type="button"
                class="order-btn"
                :disabled="index === projects.length - 1"
                title="下移此项目"
                aria-label="下移此项目"
                data-test="btn-move-down"
                @click="moveDown(project.id)"
              >
                <svg viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="4 6 8 10 12 6" />
                </svg>
              </button>
              <span class="order-num">{{ index + 1 }}</span>
            </div>

            <div class="col-name">
              <span class="name-text" :title="project.name">{{ project.name }}</span>
            </div>

            <div class="col-path">
              <code class="path-code" :title="project.path">{{ project.path }}</code>
            </div>

            <div class="col-visible">
              <el-switch
                :model-value="!isHidden(project.id)"
                size="small"
                @update:model-value="toggleVisibility(project.id)"
              />
              <span class="switch-state-text">
                {{ isHidden(project.id) ? '已隐藏' : '显示中' }}
              </span>
            </div>

            <div class="col-actions">
              <el-button
                type="primary"
                link
                size="small"
                data-test="open-project-settings"
                @click="handleOpenSettings(project.id)"
              >
                设置
              </el-button>
              <el-button
                type="danger"
                link
                size="small"
                :disabled="submitting"
                @click="handleUnregister(project)"
              >
                解除
              </el-button>
            </div>
          </li>
        </ul>
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">关闭</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.projects-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 65vh;
  overflow-y: auto;
}

.dialog-desc-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--color-border);
}

.dialog-desc-text {
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}

.count-chip {
  font-size: var(--text-size-11);
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  border: 1px solid var(--color-border);
  font-family: var(--font-mono);
  white-space: nowrap;
}

.empty-state {
  padding: 32px 16px;
  text-align: center;
  color: var(--color-text-muted);
  border: 1px dashed var(--color-border);
  border-radius: 4px;
}

.project-table-card {
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  overflow: hidden;
}

.table-head-row {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  background: var(--color-surface-muted);
  border-bottom: 1px solid var(--color-border);
  font-size: var(--text-size-11);
  font-weight: 600;
  color: var(--color-text-muted);
}

.project-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.project-item {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-bottom: 1px solid var(--color-border);
  font-size: var(--text-size-12);
  transition: background 0.12s ease;
}

.project-item:last-child {
  border-bottom: none;
}

.project-item:hover {
  background: var(--color-surface-muted);
}

.project-item.is-hidden-project {
  opacity: 0.6;
}

.col-order {
  width: 72px;
  flex-shrink: 0;
}

.order-group {
  display: flex;
  align-items: center;
  gap: 4px;
}

.order-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 2px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0;
}

.order-btn:hover:not(:disabled) {
  background: var(--color-surface-muted);
  color: var(--color-text);
  border-color: var(--color-border-strong);
}

.order-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.order-num {
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  margin-left: 4px;
}

.col-name {
  width: 140px;
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding-right: 8px;
}

.name-text {
  font-weight: 500;
}

.col-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding-right: 12px;
}

.path-code {
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.col-visible {
  width: 110px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 6px;
}

.switch-state-text {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.col-actions {
  width: 100px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
}
</style>
