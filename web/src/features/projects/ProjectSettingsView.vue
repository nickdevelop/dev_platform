<script setup lang="ts">
// One project's settings, opened as a workbench tab. General identity (display
// name and path) lives here, together with the git worktree registry that makes
// parallel sessions possible. The page follows the platform's app-screen rules:
// flat 1px surfaces, one primary action per block, no resting shadows.
import { computed, onMounted, ref, watch } from 'vue'
import { describeApiError, UnauthorizedError } from '../../api/client'
import { updateProjectName, type Project } from '../../api/projects'
import { getProjectGitInfo, type GitInfo } from '../../api/git'
import { getGitBranches } from '../../api/files'
import { useProjectsStore } from '../../stores/projects'
import { useWorktreesStore } from '../../stores/worktrees'
import { UiAlert, UiButton, UiDialog, UiInput, UiSelectMenu, type SelectMenuItem } from '../../shared/ui'

const props = defineProps<{ projectId: number }>()

const projects = useProjectsStore()
const worktrees = useWorktreesStore()

const project = computed<Project | undefined>(() => projects.byId(props.projectId))
const worktreeList = computed(() => worktrees.forProject(props.projectId))

const nameInput = ref('')
const nameSaving = ref(false)
const nameSaved = ref(false)
const nameError = ref('')
let nameSyncedFor = -1

const gitInfo = ref<GitInfo | null>(null)
const gitLoading = ref(false)
const gitError = ref('')
const branches = ref<string[]>([])
const currentBranch = ref('')

const branchInput = ref('')
const baseBranch = ref('')
const pathInput = ref('')
const creating = ref(false)
const createError = ref('')

const removing = ref<{ id: number; branch: string; path: string } | null>(null)
const removeBusy = ref(false)
const removeError = ref('')

const baseBranchOptions = computed<SelectMenuItem<string>[]>(() => {
  const options = branches.value.map((branch) => ({ value: branch, label: branch }))
  if (currentBranch.value && !options.some((entry) => entry.value === currentBranch.value)) {
    options.unshift({ value: currentBranch.value, label: currentBranch.value })
  }
  return options
})

const defaultPathHint = computed(() => {
  const path = project.value?.path ?? ''
  if (!path) return ''
  const isWindows = /\\/.test(path)
  const separator = isWindows ? '\\' : '/'
  const normalized = path.replace(/\\/g, '/').replace(/\/+$/, '')
  const slash = normalized.lastIndexOf('/')
  const parent = slash > 0 ? normalized.slice(0, slash) : ''
  const repo = slash >= 0 ? normalized.slice(slash + 1) : normalized
  return `${parent}${separator}${repo}-worktrees${separator}<分支名>`
})

function syncName() {
  const current = project.value
  if (!current) return
  if (nameSyncedFor === current.id && nameInput.value === current.name) return
  nameInput.value = current.name
  nameSyncedFor = current.id
  nameSaved.value = false
  nameError.value = ''
}

watch(project, syncName, { immediate: true })

async function loadGitDetails(force = false) {
  if (!project.value?.isGit) {
    gitInfo.value = null
    branches.value = []
    currentBranch.value = ''
    return
  }
  gitLoading.value = true
  gitError.value = ''
  try {
    const [info, result] = await Promise.all([
      getProjectGitInfo(props.projectId).catch(() => null),
      getGitBranches(props.projectId).catch(() => ({ branches: [], current: '' })),
    ])
    gitInfo.value = info
    branches.value = result.branches ?? []
    currentBranch.value = result.current || info?.branch || ''
    if (!baseBranch.value || !branches.value.includes(baseBranch.value)) {
      baseBranch.value = currentBranch.value
    }
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    gitError.value = describeApiError(error)
  } finally {
    gitLoading.value = false
  }
  await worktrees.load(props.projectId, force)
}

onMounted(() => {
  if (!projects.byId(props.projectId)) void projects.load()
  void loadGitDetails(true)
})

watch(
  () => project.value?.isGit,
  (isGit, wasGit) => {
    if (isGit === wasGit) return
    void loadGitDetails(true)
  },
)

async function saveName() {
  if (!project.value || nameSaving.value) return
  const name = nameInput.value.trim()
  if (!name || name === project.value.name) {
    nameInput.value = project.value.name
    return
  }
  nameSaving.value = true
  nameError.value = ''
  nameSaved.value = false
  try {
    const updated = await updateProjectName(project.value.id, name)
    const target = projects.projects.find((entry) => entry.id === updated.id)
    if (target) {
      target.name = updated.name
    }
    nameSaved.value = true
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    nameError.value = describeApiError(error)
  } finally {
    nameSaving.value = false
  }
}

async function createWorktree() {
  if (!project.value?.isGit || creating.value) return
  const branch = branchInput.value.trim()
  if (!branch) {
    createError.value = '请填写分支名称'
    return
  }
  creating.value = true
  createError.value = ''
  try {
    await worktrees.create(props.projectId, {
      branch,
      ...(baseBranch.value ? { baseBranch: baseBranch.value } : {}),
      ...(pathInput.value.trim() ? { path: pathInput.value.trim() } : {}),
    })
    branchInput.value = ''
    pathInput.value = ''
    await loadGitDetails(true)
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    createError.value = describeApiError(error)
  } finally {
    creating.value = false
  }
}

function askRemove(id: number, branch: string, path: string) {
  if (removeBusy.value) return
  removeError.value = ''
  removing.value = { id, branch, path }
}

function cancelRemove() {
  if (removeBusy.value) return
  removing.value = null
  removeError.value = ''
}

async function confirmRemove() {
  if (!removing.value || removeBusy.value) return
  removeBusy.value = true
  removeError.value = ''
  try {
    await worktrees.remove(props.projectId, removing.value.id)
    removing.value = null
    await loadGitDetails(true)
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    removeError.value = describeApiError(error)
  } finally {
    removeBusy.value = false
  }
}

function formatDate(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (isNaN(date.getTime())) return ''
  return date.toLocaleDateString()
}
</script>

<template>
  <div class="project-settings-view" data-test="project-settings-view">
    <div class="settings-canvas">
      <header class="settings-head">
        <div class="title-row">
          <h1 class="page-title">{{ project?.name || '项目设置' }}</h1>
          <span v-if="project?.isGit" class="git-chip" title="Git 项目">Git</span>
        </div>
        <p class="page-path">{{ project?.path || '加载中…' }}</p>
        <div v-if="project?.isGit" class="git-status" data-test="project-settings-git-status">
          <span class="status-item">
            当前分支
            <code>{{ gitLoading ? '…' : (currentBranch || '未检出') }}</code>
          </span>
          <span v-if="gitInfo?.dirty" class="status-item is-dirty">有未提交改动</span>
          <span v-else-if="gitInfo" class="status-item">工作区干净</span>
        </div>
      </header>

      <UiAlert v-if="projects.error" variant="error" class="block-alert">{{ projects.error }}</UiAlert>
      <UiAlert v-if="gitError" variant="error" class="block-alert">{{ gitError }}</UiAlert>

      <section class="settings-section" aria-labelledby="settings-general-title">
        <div class="section-aside">
          <h2 id="settings-general-title">常规</h2>
          <p>项目在平台中展示的名称。修改名称不会移动或重命名磁盘上的目录。</p>
        </div>
        <div class="section-panel">
          <form class="field-stack" @submit.prevent="saveName">
            <label for="project-display-name" class="field-label">项目显示名称</label>
            <div class="field-row">
              <UiInput
                id="project-display-name"
                v-model="nameInput"
                autocomplete="off"
                spellcheck="false"
                block
              />
              <UiButton
                type="submit"
                variant="primary"
                class="save-name-btn"
                :loading="nameSaving"
                :disabled="!nameInput.trim() || nameSaving"
              >
                保存
              </UiButton>
            </div>
            <p v-if="nameError" class="field-error" role="alert">{{ nameError }}</p>
            <p v-else-if="nameSaved" class="field-saved" role="status">已保存</p>
          </form>

          <div class="field-stack">
            <span class="field-label">项目路径</span>
            <code class="path-value">{{ project?.path || '—' }}</code>
            <p class="field-hint">平台只记录该路径，不会移动或重命名磁盘目录。</p>
          </div>
        </div>
      </section>

      <section
        v-if="project?.isGit"
        class="settings-section"
        aria-labelledby="settings-worktrees-title"
      >
        <div class="section-aside">
          <h2 id="settings-worktrees-title">Git 工作树</h2>
          <p>
            为并行开发创建独立的检出目录与分支。每个工作树拥有自己的 Agent 实例和会话，
            文件树与 Git 面板会跟随会话所在的工作树。
          </p>
          <p class="side-note">删除工作树不会删除其分支。</p>
        </div>

        <div class="section-panel">
          <form class="worktree-create" data-test="worktree-create-form" @submit.prevent="createWorktree">
            <div class="field-grid">
              <div class="field-stack">
                <label for="worktree-branch" class="field-label">分支名称</label>
                <UiInput
                  id="worktree-branch"
                  v-model="branchInput"
                  placeholder="例如：feature/login"
                  autocomplete="off"
                  spellcheck="false"
                  block
                />
              </div>
              <div class="field-stack">
                <label for="worktree-base" class="field-label">基于分支</label>
                <UiSelectMenu
                  id="worktree-base"
                  v-model="baseBranch"
                  :options="baseBranchOptions"
                  :placeholder="currentBranch || '当前分支'"
                  :disabled="gitLoading"
                  block
                />
              </div>
            </div>

            <div class="field-stack">
              <label for="worktree-path" class="field-label">目录路径</label>
              <UiInput
                id="worktree-path"
                v-model="pathInput"
                :placeholder="defaultPathHint"
                autocomplete="off"
                spellcheck="false"
                block
              />
              <p class="field-hint">留空则创建到与仓库同级的 {{ defaultPathHint }}</p>
            </div>

            <div class="create-actions">
              <UiButton
                type="submit"
                variant="primary"
                :loading="creating"
                :disabled="!branchInput.trim() || creating"
                data-test="worktree-create-btn"
              >
                创建工作树
              </UiButton>
            </div>
            <p v-if="createError" class="field-error" role="alert" data-test="worktree-create-error">
              {{ createError }}
            </p>
          </form>

          <div class="worktree-list-head">
            <span>已创建工作树</span>
            <span class="count">{{ worktreeList.length }}</span>
          </div>

          <div v-if="worktrees.isLoading(props.projectId) && worktreeList.length === 0" class="list-loading">
            加载工作树…
          </div>
          <div v-else-if="worktreeList.length === 0" class="empty-state" data-test="worktree-empty">
            <p>还没有工作树。</p>
            <p class="empty-sub">在上方填写分支名称即可创建第一个并行工作区。</p>
          </div>
          <ul v-else class="worktree-list" aria-label="已创建的工作树">
            <li
              v-for="entry in worktreeList"
              :key="entry.id"
              class="worktree-row"
              data-test="worktree-row"
              :data-worktree-id="entry.id"
            >
              <div class="worktree-identity">
                <span class="worktree-branch" :title="entry.branch">{{ entry.branch }}</span>
                <span v-if="entry.baseBranch" class="worktree-base">基于 {{ entry.baseBranch }}</span>
              </div>
              <code class="worktree-path" :title="entry.path">{{ entry.path }}</code>
              <div class="worktree-meta">
                <span v-if="!entry.available" class="state-chip is-missing">路径缺失</span>
                <span class="created-at">{{ formatDate(entry.createdAt) }}</span>
                <button
                  type="button"
                  class="remove-btn"
                  :aria-label="`移除工作树：${entry.branch}`"
                  :disabled="removeBusy"
                  data-test="worktree-remove-btn"
                  @click="askRemove(entry.id, entry.branch, entry.path)"
                >
                  移除
                </button>
              </div>
            </li>
          </ul>
        </div>
      </section>

      <section v-else-if="project" class="settings-section" aria-labelledby="settings-worktrees-title">
        <div class="section-aside">
          <h2 id="settings-worktrees-title">Git 工作树</h2>
          <p>当前项目目录没有 Git 元数据，无法创建 worktree。</p>
        </div>
        <div class="section-panel">
          <div class="empty-state" data-test="worktree-not-git">
            <p>此项目不是 Git 仓库。</p>
            <p class="empty-sub">在项目目录执行 git init 并提交初始版本后，重新打开此页面。</p>
          </div>
        </div>
      </section>
    </div>

    <UiDialog
      v-if="removing"
      title="移除 Git 工作树"
      width-class="w-full max-w-md"
      @close="cancelRemove"
    >
      <div class="remove-confirm">
        <p>
          确定移除工作树 <code>{{ removing.branch }}</code> 吗？
        </p>
        <p class="remove-path">{{ removing.path }}</p>
        <p class="remove-warning">
          该工作树下运行中的 Agent 实例会被停止；工作树目录会被删除，但分支
          <code>{{ removing.branch }}</code> 会保留。
        </p>
        <p v-if="removeError" class="field-error" role="alert">{{ removeError }}</p>
        <div class="remove-actions">
          <UiButton variant="secondary" :disabled="removeBusy" @click="cancelRemove">取消</UiButton>
          <UiButton
            variant="danger"
            :loading="removeBusy"
            :disabled="removeBusy"
            data-test="worktree-remove-confirm"
            @click="confirmRemove"
          >
            移除工作树
          </UiButton>
        </div>
      </div>
    </UiDialog>
  </div>
</template>

<style scoped>
.project-settings-view {
  width: 100%;
  min-height: 100%;
  overflow-y: auto;
  padding: 24px 32px 64px;
  background: var(--color-bg);
  color: var(--color-text);
}

.settings-canvas {
  max-width: 1080px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.settings-head {
  padding-bottom: 16px;
  border-bottom: 1px solid var(--color-border);
}

.title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.page-title {
  margin: 0;
  font-size: var(--text-size-22);
  line-height: 1.25;
  font-weight: 600;
  letter-spacing: -0.02em;
  color: var(--color-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.git-chip {
  flex-shrink: 0;
  padding: 1px 6px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 3px);
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  font-size: var(--text-size-11);
  font-weight: 500;
}

.page-path {
  margin: 6px 0 0;
  color: var(--color-text-muted);
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
  overflow-wrap: anywhere;
}

.git-status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 14px;
  margin-top: 10px;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}

.status-item code {
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
  color: var(--color-text);
}

.status-item.is-dirty {
  color: var(--color-warning);
}

.block-alert {
  margin: 0;
}

.settings-section {
  display: grid;
  grid-template-columns: minmax(200px, 300px) minmax(0, 1fr);
  gap: 32px;
  padding: 24px 0;
  border-top: 1px solid var(--color-border);
}

.settings-section:first-of-type {
  border-top: none;
  padding-top: 0;
}

.section-aside h2 {
  margin: 0;
  font-size: var(--text-size-14);
  font-weight: 600;
  color: var(--color-text);
}

.section-aside p {
  margin: 8px 0 0;
  font-size: var(--text-size-12);
  line-height: 1.55;
  color: var(--color-text-muted);
}

.section-aside .side-note {
  color: var(--color-text-muted);
  opacity: 0.85;
}

.section-panel {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 16px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 3px);
  background: var(--color-surface);
}

.field-stack {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.field-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 16px;
}

.field-label {
  font-size: var(--text-size-12);
  font-weight: 500;
  color: var(--color-text);
}

.field-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.save-name-btn {
  flex-shrink: 0;
}

.field-hint {
  margin: 0;
  font-size: var(--text-size-11);
  line-height: 1.5;
  color: var(--color-text-muted);
}

.field-error {
  margin: 0;
  font-size: var(--text-size-12);
  color: var(--color-danger);
}

.field-saved {
  margin: 0;
  font-size: var(--text-size-11);
  color: var(--color-success);
}

.path-value {
  display: block;
  padding: 6px 8px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 3px);
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
  overflow-wrap: anywhere;
}

.worktree-create {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-bottom: 18px;
  border-bottom: 1px solid var(--color-border);
}

.create-actions {
  display: flex;
  justify-content: flex-end;
}

.worktree-list-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--text-size-12);
  font-weight: 500;
  color: var(--color-text-muted);
}

.worktree-list-head .count {
  min-width: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  font-size: var(--text-size-11);
  text-align: center;
  font-variant-numeric: tabular-nums;
}

.list-loading {
  padding: 12px 0;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
}

.empty-state {
  padding: 20px 16px;
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius, 3px);
  text-align: center;
}

.empty-state p {
  margin: 0;
  font-size: var(--text-size-13);
  color: var(--color-text);
}

.empty-state .empty-sub {
  margin-top: 4px;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.worktree-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}

.worktree-row {
  display: grid;
  grid-template-columns: minmax(140px, 220px) minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  min-height: 48px;
  padding: 8px 0;
  border-top: 1px solid var(--color-border);
}

.worktree-row:first-child {
  border-top: none;
}

.worktree-identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.worktree-branch {
  font-size: var(--text-size-13);
  font-weight: 500;
  color: var(--color-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.worktree-base {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.worktree-path {
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.worktree-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.state-chip {
  padding: 1px 6px;
  border-radius: var(--radius, 3px);
  font-size: var(--text-size-11);
  white-space: nowrap;
}

.state-chip.is-missing {
  background: var(--color-warning-soft);
  border: 1px solid var(--color-warning-border);
  color: var(--color-warning);
}

.created-at {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.remove-btn {
  border: 1px solid transparent;
  border-radius: var(--radius, 3px);
  background: transparent;
  color: var(--color-text-muted);
  font: inherit;
  font-size: var(--text-size-11);
  padding: 3px 6px;
  cursor: pointer;
  transition: color 0.12s ease-out, background-color 0.12s ease-out,
    border-color 0.12s ease-out;
}

.remove-btn:hover:not(:disabled) {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
  background: var(--color-danger-soft);
}

.remove-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.remove-confirm {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.remove-confirm p {
  margin: 0;
  font-size: var(--text-size-13);
  line-height: 1.55;
  color: var(--color-text);
}

.remove-confirm code {
  font-family: var(--font-mono);
  font-size: var(--text-size-12);
}

.remove-confirm .remove-path {
  padding: 6px 8px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius, 3px);
  background: var(--color-surface-muted);
  color: var(--color-text-muted);
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  overflow-wrap: anywhere;
}

.remove-confirm .remove-warning {
  color: var(--color-text-muted);
  font-size: var(--text-size-12);
}

.remove-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--color-border);
}

@media (max-width: 900px) {
  .project-settings-view {
    padding: 20px 20px 48px;
  }

  .settings-section {
    grid-template-columns: minmax(0, 1fr);
    gap: 14px;
  }

  .field-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .worktree-row {
    grid-template-columns: minmax(0, 1fr) auto;
    row-gap: 4px;
  }

  .worktree-path {
    grid-column: 1 / -1;
    order: 3;
  }
}

@media (max-width: 480px) {
  .project-settings-view {
    padding: 16px 16px 40px;
  }

  .page-title {
    font-size: var(--text-size-20);
  }

  .field-row {
    align-items: stretch;
    flex-direction: column;
  }

  .worktree-row {
    grid-template-columns: minmax(0, 1fr);
    align-items: flex-start;
  }

  .worktree-meta {
    flex-wrap: wrap;
  }
}
</style>
