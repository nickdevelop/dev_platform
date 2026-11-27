<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { describeApiError, UnauthorizedError } from '../../api/client'
import { updateProjectName, type Project } from '../../api/projects'
import { getProjectGitInfo, type GitInfo } from '../../api/git'
import { getGitBranches, getGitCommits, type GitCommitItem } from '../../api/files'
import { useProjectsStore } from '../../stores/projects'
import { useWorktreesStore } from '../../stores/worktrees'

const props = defineProps<{
  projectId: number | null
  visible: boolean
}>()

const emit = defineEmits<{
  'update:visible': [val: boolean]
}>()

const projectsStore = useProjectsStore()
const worktreesStore = useWorktreesStore()

const project = computed<Project | undefined>(() =>
  props.projectId ? projectsStore.byId(props.projectId) : undefined,
)
const worktreeList = computed(() =>
  props.projectId ? worktreesStore.forProject(props.projectId) : [],
)

const nameInput = ref('')
const nameSaving = ref(false)

const gitInfo = ref<GitInfo | null>(null)
const gitLoading = ref(false)
const branches = ref<string[]>([])
const currentBranch = ref('')
const commits = ref<GitCommitItem[]>([])

const baseType = ref<'branch' | 'commit'>('branch')
const branchInput = ref('')
const baseBranch = ref('')
const commitInput = ref('')
const selectedRecentCommit = ref('')
const pathInput = ref('')
const creating = ref(false)
const removeBusy = ref(false)

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

async function loadGitDetails(force = false) {
  if (!props.projectId || !project.value?.isGit) {
    gitInfo.value = null
    branches.value = []
    currentBranch.value = ''
    commits.value = []
    return
  }
  gitLoading.value = true
  try {
    const [info, result, commitList] = await Promise.all([
      getProjectGitInfo(props.projectId).catch(() => null),
      getGitBranches(props.projectId).catch(() => ({ branches: [], current: '' })),
      getGitCommits(props.projectId, 20).catch(() => []),
    ])
    gitInfo.value = info
    branches.value = result.branches ?? []
    currentBranch.value = result.current || info?.branch || ''
    commits.value = commitList
    if (!baseBranch.value || !branches.value.includes(baseBranch.value)) {
      baseBranch.value = currentBranch.value
    }
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    ElMessage.error(describeApiError(error))
  } finally {
    gitLoading.value = false
  }
  await worktreesStore.load(props.projectId, force)
}

watch(
  () => [props.visible, props.projectId],
  ([visible, pId]) => {
    if (visible && pId) {
      const p = projectsStore.byId(Number(pId))
      if (p) nameInput.value = p.name
      void loadGitDetails(true)
    }
  },
  { immediate: true },
)

async function saveName() {
  if (!project.value || nameSaving.value) return
  const name = nameInput.value.trim()
  if (!name || name === project.value.name) return

  nameSaving.value = true
  try {
    const updated = await updateProjectName(project.value.id, name)
    const target = projectsStore.projects.find((entry) => entry.id === updated.id)
    if (target) {
      target.name = updated.name
    }
    ElMessage.success('项目名称已更新')
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    ElMessage.error(describeApiError(error))
  } finally {
    nameSaving.value = false
  }
}

function onSelectRecentCommit(hash: string) {
  if (!hash) return
  commitInput.value = hash
  const short = hash.slice(0, 7)
  if (!branchInput.value.trim() || branchInput.value.startsWith('wt-')) {
    branchInput.value = `wt-${short}`
  }
}

const canCreate = computed(() => {
  if (creating.value) return false
  if (baseType.value === 'commit') {
    return Boolean(commitInput.value.trim() || selectedRecentCommit.value)
  }
  return Boolean(branchInput.value.trim())
})

async function createWorktree() {
  if (!props.projectId || !project.value?.isGit || creating.value) return

  let branch = branchInput.value.trim()
  const isCommitMode = baseType.value === 'commit'
  const commit = commitInput.value.trim()

  if (isCommitMode) {
    if (!commit) {
      ElMessage.warning('请填写 Commit ID 或选择最近提交')
      return
    }
    if (!branch) {
      branch = `wt-${commit.slice(0, 7)}`
    }
  } else {
    if (!branch) {
      ElMessage.warning('请填写分支名称')
      return
    }
  }

  creating.value = true
  try {
    await worktreesStore.create(props.projectId, {
      branch,
      ...(isCommitMode ? { commit } : baseBranch.value ? { baseBranch: baseBranch.value } : {}),
      ...(pathInput.value.trim() ? { path: pathInput.value.trim() } : {}),
    })
    branchInput.value = ''
    commitInput.value = ''
    selectedRecentCommit.value = ''
    pathInput.value = ''
    ElMessage.success(`工作树「${branch}」创建成功`)
    await loadGitDetails(true)
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    ElMessage.error(describeApiError(error))
  } finally {
    creating.value = false
  }
}

async function removeWorktree(id: number, branch: string) {
  if (!props.projectId || removeBusy.value) return

  try {
    await ElMessageBox.confirm(
      `确定移除工作树「${branch}」吗？磁盘上的工作树目录将被清理，但 Git 分支将保留。`,
      '移除 Git 工作树',
      {
        confirmButtonText: '确定移除',
        cancelButtonText: '取消',
        confirmButtonClass: 'el-button--danger',
        type: 'warning',
      },
    )
  } catch {
    return
  }

  removeBusy.value = true
  try {
    await worktreesStore.remove(props.projectId, id)
    ElMessage.success(`工作树「${branch}」已移除`)
    await loadGitDetails(true)
  } catch (error) {
    if (error instanceof UnauthorizedError) return
    ElMessage.error(describeApiError(error))
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
  <el-dialog
    :model-value="props.visible"
    :title="project ? `${project.name} · 项目设置` : '项目设置'"
    width="820px"
    destroy-on-close
    data-test="project-settings-dialog"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="settings-dialog-body" data-test="project-settings-view">
      <!-- Section 1: General Settings -->
      <div class="settings-section">
        <h3 class="section-title">常规信息</h3>
        <div class="section-content">
          <div class="form-row">
            <label class="form-label" for="project-name-input">项目显示名称</label>
            <div class="input-with-action">
              <el-input
                id="project-name-input"
                v-model="nameInput"
                placeholder="项目名称"
                size="default"
              />
              <el-button
                type="primary"
                size="default"
                :loading="nameSaving"
                :disabled="!nameInput.trim() || nameInput.trim() === project?.name"
                @click="saveName"
              >
                保存
              </el-button>
            </div>
          </div>
          <div class="form-row">
            <span class="form-label">磁盘绝对路径</span>
            <code class="path-display">{{ project?.path || '—' }}</code>
          </div>
          <div v-if="project?.isGit" class="git-status-strip" data-test="project-settings-git-status">
            <span class="status-badge git-badge">Git 仓库</span>
            <span class="status-item">
              当前分支: <code>{{ gitLoading ? '…' : (currentBranch || '未检出') }}</code>
            </span>
            <span v-if="gitInfo?.dirty" class="status-item is-dirty">有未提交改动</span>
            <span v-else-if="gitInfo" class="status-item">工作区干净</span>
          </div>
        </div>
      </div>

      <!-- Section 2: Git Worktrees -->
      <div v-if="project?.isGit" class="settings-section">
        <div class="section-head-row">
          <h3 class="section-title">Git 工作树</h3>
          <span class="worktree-count-badge">已创建 {{ worktreeList.length }} 个</span>
        </div>
        <p class="section-desc">
          为并行开发创建独立的检出目录与分支。每个工作树拥有独立的 Agent 会话与文件树，移除工作树不会删除其分支。
        </p>

        <!-- Create Worktree Form -->
        <div class="worktree-create-card">
          <div class="base-type-toggle">
            <span class="form-label">基准模式</span>
            <el-radio-group v-model="baseType" size="small">
              <el-radio-button value="branch" data-test="base-type-branch">基于分支</el-radio-button>
              <el-radio-button value="commit" data-test="base-type-commit">基于提交 (Commit ID)</el-radio-button>
            </el-radio-group>
          </div>

          <div class="create-fields-grid">
            <div class="form-row">
              <label class="form-label" for="wt-branch-input">
                新分支名 {{ baseType === 'commit' ? '(可选)' : '' }}
              </label>
              <el-input
                id="wt-branch-input"
                v-model="branchInput"
                :placeholder="baseType === 'commit' ? '例如: wt-7a3f8c1 (留空自动生成)' : '例如: feature/login'"
                size="default"
              />
            </div>

            <div v-if="baseType === 'branch'" class="form-row">
              <label class="form-label" for="wt-base-branch">基于来源分支</label>
              <el-select
                id="wt-base-branch"
                v-model="baseBranch"
                placeholder="选择来源分支"
                filterable
                size="default"
              >
                <el-option
                  v-for="b in branches"
                  :key="b"
                  :label="b"
                  :value="b"
                />
              </el-select>
            </div>

            <div v-else class="form-row">
              <label class="form-label" for="wt-commit-input">Commit ID</label>
              <div class="commit-input-wrap">
                <el-input
                  id="wt-commit-input"
                  v-model="commitInput"
                  placeholder="输入 Commit Hash"
                  data-test="worktree-commit-input"
                  size="default"
                />
                <select
                  v-if="commits.length > 0"
                  v-model="selectedRecentCommit"
                  class="recent-commit-select"
                  data-test="worktree-recent-commits"
                  @change="onSelectRecentCommit(selectedRecentCommit)"
                >
                  <option value="">从最近提交选择…</option>
                  <option v-for="c in commits" :key="c.hash" :value="c.hash">
                    [{{ c.shortHash }}] {{ c.message }}
                  </option>
                </select>
              </div>
            </div>
          </div>

          <div class="form-row">
            <label class="form-label" for="wt-path-input">检出目录 (可选)</label>
            <el-input
              id="wt-path-input"
              v-model="pathInput"
              :placeholder="`留空则使用默认路径: ${defaultPathHint}`"
              size="default"
            />
          </div>

          <div class="create-action-row">
            <el-button
              type="primary"
              size="default"
              :loading="creating"
              :disabled="!canCreate"
              data-test="worktree-create-btn"
              @click="createWorktree"
            >
              创建工作树
            </el-button>
          </div>
        </div>

        <!-- Worktrees List -->
        <div class="worktree-list-wrapper">
          <div v-if="worktreeList.length === 0" class="empty-state" data-test="worktree-empty">
            <p>还没有工作树。在上方填写分支名即可创建并行工作区。</p>
          </div>

          <ul v-else class="worktree-list" aria-label="已创建的工作树">
            <li
              v-for="wt in worktreeList"
              :key="wt.id"
              class="worktree-item"
              data-test="worktree-row"
              :data-worktree-id="wt.id"
            >
              <div class="wt-info">
                <span class="wt-branch">{{ wt.branch }}</span>
                <span v-if="wt.baseBranch" class="wt-base">基于 {{ wt.baseBranch }}</span>
                <code class="wt-path">{{ wt.path }}</code>
              </div>
              <div class="wt-meta">
                <span v-if="!wt.available" class="badge-missing">路径缺失</span>
                <span class="wt-date">{{ formatDate(wt.createdAt) }}</span>
                <el-button
                  type="danger"
                  link
                  size="small"
                  :disabled="removeBusy"
                  data-test="worktree-remove-btn"
                  @click="removeWorktree(wt.id, wt.branch)"
                >
                  移除
                </el-button>
              </div>
            </li>
          </ul>
        </div>
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
.settings-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 70vh;
  overflow-y: auto;
}

.settings-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 4px;
}

.section-head-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  margin: 0;
  font-size: var(--text-size-13);
  font-weight: 600;
  color: var(--color-text);
}

.section-desc {
  margin: 0;
  font-size: var(--text-size-12);
  color: var(--color-text-muted);
  line-height: 1.5;
}

.worktree-count-badge {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  font-family: var(--font-mono);
}

.form-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-label {
  font-size: var(--text-size-11);
  font-weight: 500;
  color: var(--color-text-muted);
}

.input-with-action {
  display: flex;
  gap: 8px;
}

.path-display {
  display: block;
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  padding: 6px 8px;
  border-radius: 3px;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  color: var(--color-text-muted);
  word-break: break-all;
}

.git-status-strip {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  padding-top: 4px;
}

.git-badge {
  padding: 1px 6px;
  border-radius: 2px;
  background: var(--color-primary-soft, rgba(59, 130, 246, 0.1));
  color: var(--color-primary);
  font-weight: 600;
}

.status-item code {
  font-family: var(--font-mono);
  font-weight: 600;
  color: var(--color-text);
}

.status-item.is-dirty {
  color: var(--color-warning);
}

.worktree-create-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 10px 12px;
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: 4px;
}

.base-type-toggle {
  display: flex;
  align-items: center;
  gap: 10px;
}

.create-fields-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.commit-input-wrap {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.recent-commit-select {
  height: 26px;
  padding: 2px 6px;
  border-radius: 3px;
  border: 1px solid var(--color-border);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: var(--text-size-11);
  font-family: var(--font-mono);
  outline: none;
}

.create-action-row {
  display: flex;
  justify-content: flex-end;
}

.worktree-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.worktree-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border: 1px solid var(--color-border);
  border-radius: 3px;
  background: var(--color-surface);
}

.wt-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.wt-branch {
  font-size: var(--text-size-12);
  font-weight: 600;
  color: var(--color-text);
}

.wt-base {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
}

.wt-path {
  font-family: var(--font-mono);
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wt-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.badge-missing {
  font-size: var(--text-size-11);
  padding: 1px 4px;
  border-radius: 2px;
  background: rgba(239, 68, 68, 0.1);
  color: var(--color-danger);
}

.wt-date {
  font-size: var(--text-size-11);
  color: var(--color-text-muted);
  font-family: var(--font-mono);
}

.empty-state {
  padding: 20px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: var(--text-size-12);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
}
</style>
