<script setup lang="ts">
import { computed, ref } from 'vue'
import { useProjectsStore } from '../../stores/projects'
import { updateProjectName, unregisterProject, type Project } from '../../api/projects'
import { describeApiError } from '../../api/client'
import { UiButton, UiInput, UiAlert, UiDialog } from '../../shared/ui'

const projectsStore = useProjectsStore()

const editingProject = ref<Project | null>(null)
const editNameInput = ref('')
const unregisteringProject = ref<Project | null>(null)
const actionError = ref('')
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

function startEditName(project: Project) {
  editingProject.value = project
  editNameInput.value = project.name
  actionError.value = ''
}

async function saveEditName() {
  if (!editingProject.value || !editNameInput.value.trim() || submitting.value) return
  submitting.value = true
  actionError.value = ''
  try {
    const updated = await updateProjectName(editingProject.value.id, editNameInput.value.trim())
    const target = projectsStore.projects.find((p) => p.id === updated.id)
    if (target) {
      target.name = updated.name
    }
    editingProject.value = null
  } catch (err) {
    actionError.value = describeApiError(err)
  } finally {
    submitting.value = false
  }
}

function startUnregister(project: Project) {
  unregisteringProject.value = project
  actionError.value = ''
}

async function confirmUnregister() {
  if (!unregisteringProject.value || submitting.value) return
  submitting.value = true
  actionError.value = ''
  try {
    const id = unregisteringProject.value.id
    await unregisterProject(id)
    projectsStore.projects = projectsStore.projects.filter((p) => p.id !== id)
    unregisteringProject.value = null
  } catch (err) {
    actionError.value = describeApiError(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="projects-management-view" data-test="projects-management-view">
    <div class="management-container">
      <!-- Header bar following design-taste-frontend app screen rules -->
      <header class="management-head">
        <div class="head-info">
          <div class="title-row">
            <h1 class="page-title">项目列表与显示设置</h1>
            <span class="count-tag">{{ visibleCount }} / {{ totalCount }} 已显示</span>
          </div>
          <p class="page-subtitle">
            自定义左侧项目树中的项目显示状态与排列顺序；被隐藏的项目仍保留在系统中，可随时恢复显示。
          </p>
        </div>
      </header>

      <UiAlert v-if="actionError" variant="error" class="mb-4">
        {{ actionError }}
      </UiAlert>

      <!-- Projects List Table/Card -->
      <div v-if="projects.length === 0" class="empty-state">
        <p class="empty-text">当前暂未登记任何项目，请在侧边栏新建项目。</p>
      </div>

      <div v-else class="project-list-card">
        <div class="list-head-row">
          <span class="col-order">顺序</span>
          <span class="col-name">项目名称</span>
          <span class="col-path">工作区路径</span>
          <span class="col-visible">侧边栏显示</span>
          <span class="col-actions">操作</span>
        </div>

        <ul class="project-items" aria-label="项目列表">
          <li
            v-for="(project, index) in projects"
            :key="project.id"
            class="project-item-row"
            :class="{ 'is-hidden-project': isHidden(project.id) }"
            data-test="project-manage-row"
            :data-project-id="project.id"
          >
            <!-- Order Adjustment Buttons -->
            <div class="col-order order-control-group">
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

            <!-- Project Name -->
            <div class="col-name name-wrap">
              <span class="name-text" :title="project.name">{{ project.name }}</span>
              <button
                type="button"
                class="edit-name-btn"
                title="修改项目显示名称"
                aria-label="修改项目显示名称"
                @click="startEditName(project)"
              >
                <svg viewBox="0 0 16 16" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M11 2l3 3-9 9H2v-3l9-9z" />
                </svg>
              </button>
            </div>

            <!-- Project Absolute Path -->
            <div class="col-path path-wrap">
              <code class="path-code" :title="project.path">{{ project.path }}</code>
            </div>

            <!-- Visibility Toggle -->
            <div class="col-visible visible-toggle-cell">
              <label class="toggle-switch" :title="isHidden(project.id) ? '点击恢复在侧边栏显示' : '点击在侧边栏隐藏此项目'">
                <input
                  type="checkbox"
                  class="toggle-checkbox"
                  :checked="!isHidden(project.id)"
                  data-test="project-visibility-toggle"
                  @change="toggleVisibility(project.id)"
                />
                <span class="toggle-slider"></span>
                <span class="toggle-label text-xs">
                  {{ isHidden(project.id) ? '已隐藏' : '显示中' }}
                </span>
              </label>
            </div>

            <!-- Actions -->
            <div class="col-actions actions-cell">
              <button
                type="button"
                class="action-link text-danger"
                title="从平台解除登记（保留磁盘目录）"
                @click="startUnregister(project)"
              >
                解除
              </button>
            </div>
          </li>
        </ul>
      </div>
    </div>

    <!-- Edit Name Dialog -->
    <UiDialog
      v-if="editingProject"
      title="修改项目名称"
      width-class="w-full max-w-sm"
      @close="editingProject = null"
    >
      <form class="space-y-4" @submit.prevent="saveEditName">
        <div class="flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-slate-700 dark:text-slate-300">项目显示名称</label>
          <UiInput v-model="editNameInput" autocomplete="off" block />
          <p class="text-xs text-slate-400">仅修改在平台中展示的友好名称，磁盘物理目录不受影响。</p>
        </div>
        <div class="flex justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
          <UiButton type="button" variant="secondary" size="sm" @click="editingProject = null">取消</UiButton>
          <UiButton type="submit" variant="primary" size="sm" :loading="submitting" :disabled="!editNameInput.trim()">保存</UiButton>
        </div>
      </form>
    </UiDialog>

    <!-- Unregister Confirm Dialog -->
    <UiDialog
      v-if="unregisteringProject"
      title="解除项目登记"
      width-class="w-full max-w-md"
      @close="unregisteringProject = null"
    >
      <div class="space-y-3">
        <p class="text-xs text-slate-700 dark:text-slate-300">
          确定将项目“<strong>{{ unregisteringProject.name }}</strong>”从平台中移除吗？
        </p>
        <p class="text-xs text-emerald-600 dark:text-emerald-400 font-medium">
          磁盘上的完整目录（{{ unregisteringProject.path }}）会完全保留，不会被修改或删除。
        </p>
        <div class="flex justify-end gap-2 pt-3 border-t border-slate-100 dark:border-slate-800">
          <UiButton type="button" variant="secondary" size="sm" @click="unregisteringProject = null">取消</UiButton>
          <UiButton type="button" variant="danger" size="sm" :loading="submitting" @click="confirmUnregister">解除登记</UiButton>
        </div>
      </div>
    </UiDialog>
  </div>
</template>

<style scoped>
.projects-management-view {
  width: 100%;
  height: 100%;
  overflow-y: auto;
  padding: 2rem 2.5rem;
  background: var(--color-surface, #ffffff);
  color: var(--color-text, #0f172a);
}

.management-container {
  max-width: 58rem;
  margin: 0 auto;
}

.management-head {
  margin-bottom: 1.5rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--color-border, #e2e8f0);
}

.title-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.35rem;
}

.page-title {
  font-size: 1.125rem;
  font-weight: 600;
  margin: 0;
  color: var(--color-text, #0f172a);
}

.count-tag {
  font-size: 0.75rem;
  padding: 0.15rem 0.5rem;
  border-radius: 9999px;
  background: var(--color-surface-muted, #f1f5f9);
  color: var(--color-text-muted, #64748b);
  border: 1px solid var(--color-border, #e2e8f0);
  font-family: var(--font-mono, monospace);
}

.page-subtitle {
  font-size: 0.8125rem;
  color: var(--color-text-muted, #64748b);
  margin: 0;
  line-height: 1.5;
}

.empty-state {
  padding: 3rem 1rem;
  text-align: center;
  border: 1px dashed var(--color-border, #cbd5e1);
  border-radius: 6px;
}

.empty-text {
  font-size: 0.875rem;
  color: var(--color-text-muted, #94a3b8);
  margin: 0;
}

.project-list-card {
  border: 1px solid var(--color-border, #e2e8f0);
  border-radius: 6px;
  background: var(--color-surface, #ffffff);
  overflow: hidden;
}

.list-head-row {
  display: flex;
  align-items: center;
  padding: 0.625rem 1rem;
  background: var(--color-surface-muted, #f8fafc);
  border-bottom: 1px solid var(--color-border, #e2e8f0);
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--color-text-muted, #64748b);
}

.project-items {
  list-style: none;
  margin: 0;
  padding: 0;
  divide-y: 1px;
}

.project-item-row {
  display: flex;
  align-items: center;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--color-border, #f1f5f9);
  font-size: 0.8125rem;
  transition: background 0.15s ease;
}

.project-item-row:last-child {
  border-bottom: none;
}

.project-item-row:hover {
  background: var(--color-surface-muted, #f8fafc);
}

.project-item-row.is-hidden-project {
  opacity: 0.65;
  background: rgba(148, 163, 184, 0.04);
}

.col-order {
  width: 5rem;
  flex-shrink: 0;
}

.order-control-group {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.order-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  border-radius: 3px;
  border: 1px solid var(--color-border, #e2e8f0);
  background: var(--color-surface, #ffffff);
  color: var(--color-text-muted, #64748b);
  cursor: pointer;
}

.order-btn:hover:not(:disabled) {
  background: var(--color-surface-muted, #e2e8f0);
  color: var(--color-text, #0f172a);
}

.order-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.order-num {
  font-size: 0.75rem;
  font-family: var(--font-mono, monospace);
  color: var(--color-text-muted, #94a3b8);
  margin-left: 0.25rem;
}

.col-name {
  width: 14rem;
  flex-shrink: 0;
}

.name-wrap {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.name-text {
  font-weight: 500;
  color: var(--color-text, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.edit-name-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.2rem;
  border-radius: 3px;
  color: var(--color-text-muted, #94a3b8);
  border: none;
  background: transparent;
  cursor: pointer;
  opacity: 0.6;
}

.edit-name-btn:hover {
  opacity: 1;
  color: var(--color-text, #0f172a);
  background: var(--color-surface-muted, #f1f5f9);
}

.col-path {
  flex: 1;
  min-width: 0;
  padding-right: 1rem;
}

.path-code {
  font-family: var(--font-mono, monospace);
  font-size: 0.75rem;
  color: var(--color-text-muted, #64748b);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}

.col-visible {
  width: 7.5rem;
  flex-shrink: 0;
}

.toggle-switch {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
  user-select: none;
}

.toggle-checkbox {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-slider {
  position: relative;
  width: 1.875rem;
  height: 1.125rem;
  background: var(--color-border, #cbd5e1);
  border-radius: 9999px;
  transition: background 0.2s ease;
}

.toggle-slider::after {
  content: '';
  position: absolute;
  top: 2px;
  left: 2px;
  width: 0.875rem;
  height: 0.875rem;
  background: #ffffff;
  border-radius: 50%;
  transition: transform 0.2s ease;
}

.toggle-checkbox:checked + .toggle-slider {
  background: #2563eb;
}

.toggle-checkbox:checked + .toggle-slider::after {
  transform: translateX(0.75rem);
}

.toggle-label {
  color: var(--color-text-muted, #64748b);
}

.col-actions {
  width: 7.5rem;
  flex-shrink: 0;
  text-align: right;
}

.actions-cell {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.35rem;
}

.action-link {
  font-size: 0.75rem;
  color: #2563eb;
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  text-decoration: none;
}

.action-link:hover {
  text-decoration: underline;
}

.action-link.text-danger {
  color: #dc2626;
}

.action-divider {
  color: var(--color-border, #cbd5e1);
}
</style>
