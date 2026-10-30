<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useDesktopConnection, type ServerConnection, type TestConnectionResult } from '../../shared/desktop'
import { useAuthStore } from '../../stores/auth'
import { useOpenTabsStore } from '../../stores/openTabs'
import UiDialog from '../../shared/ui/compound/UiDialog.vue'
import UiButton from '../../shared/ui/atomic/UiButton.vue'
import UiInput from '../../shared/ui/atomic/UiInput.vue'
import UiBadge from '../../shared/ui/atomic/UiBadge.vue'
import UiAlert from '../../shared/ui/atomic/UiAlert.vue'
import UiSpinner from '../../shared/ui/atomic/UiSpinner.vue'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const router = useRouter()
const authStore = useAuthStore()
const openTabsStore = useOpenTabsStore()
const {
  isDesktop,
  status,
  connections,
  activeConnection,
  switchTarget,
  save,
  remove,
  test,
  refresh,
} = useDesktopConnection()

// Form state for creating / editing remote connection
const isEditing = ref(false)
const formId = ref<string | undefined>(undefined)
const formName = ref('')
const formUrl = ref('')
const formToken = ref('')
const formError = ref('')
const isTesting = ref(false)
const testResult = ref<TestConnectionResult | null>(null)
const isSwitching = ref(false)
const switchError = ref('')

function openCreateForm() {
  formId.value = undefined
  formName.value = ''
  formUrl.value = 'http://'
  formToken.value = ''
  formError.value = ''
  testResult.value = null
  isEditing.value = true
}

function openEditForm(conn: ServerConnection) {
  formId.value = conn.id
  formName.value = conn.name
  formUrl.value = conn.url || ''
  formToken.value = ''
  formError.value = ''
  testResult.value = null
  isEditing.value = true
}

function cancelEdit() {
  isEditing.value = false
  testResult.value = null
  formError.value = ''
}

async function runTestConnection() {
  if (!formUrl.value || !formUrl.value.startsWith('http')) {
    formError.value = '请输入合法的 HTTP/HTTPS 服务器地址'
    return
  }
  formError.value = ''
  isTesting.value = true
  testResult.value = null
  try {
    const res = await test(formUrl.value.trim(), formToken.value.trim() || undefined)
    testResult.value = res
  } catch (err: any) {
    testResult.value = { ok: false, message: err.message }
  } finally {
    isTesting.value = false
  }
}

async function handleSaveConnection() {
  if (!formName.value.trim()) {
    formError.value = '请输入服务器名称'
    return
  }
  if (!formUrl.value.trim() || !formUrl.value.startsWith('http')) {
    formError.value = '请输入有效的服务器地址（以 http:// 或 https:// 开头）'
    return
  }

  try {
    await save({
      id: formId.value,
      name: formName.value.trim(),
      type: 'remote',
      url: formUrl.value.trim(),
      token: formToken.value.trim() || undefined,
    })
    isEditing.value = false
  } catch (err: any) {
    formError.value = err.message || '保存失败'
  }
}

async function handleRemoveConnection(id: string) {
  if (!confirm('确定删除该远程服务器连接吗？')) return
  await remove(id)
}

async function handleSwitch(conn: ServerConnection) {
  if (conn.id === activeConnection.value?.id) return
  isSwitching.value = true
  switchError.value = ''
  try {
    await switchTarget(conn.id)
    // Clear in-memory tabs and auth session
    openTabsStore.tabs = []
    authStore.reset()
    emit('update:open', false)
    // Force navigate to home/login to reload target environment state
    router.push({ name: 'home' })
  } catch (err: any) {
    switchError.value = err.message || '切换后端连接失败'
  } finally {
    isSwitching.value = false
  }
}
</script>

<template>
  <UiDialog
    v-if="open"
    title="服务连接管理器"
    width-class="w-full max-w-xl"
    @close="emit('update:open', false)"
  >
    <div class="space-y-4">
      <UiAlert v-if="switchError" variant="error">
        {{ switchError }}
      </UiAlert>

      <!-- Web environment notice if not inside Electron -->
      <UiAlert v-if="!isDesktop" variant="info">
        当前处于浏览器模式，已直连当前宿主服务。如需在本地自带服务与远程服务间自由切换，请使用桌面客户端。
      </UiAlert>

      <!-- Switching overlay -->
      <div v-if="isSwitching" class="py-8 flex flex-col items-center justify-center space-y-3">
        <UiSpinner class="w-8 h-8 text-blue-500" />
        <p class="text-sm text-slate-500 dark:text-slate-400">正在切换目标后端，请稍候...</p>
      </div>

      <!-- Main view: Connection list -->
      <div v-else-if="!isEditing" class="space-y-3">
        <div class="flex items-center justify-between pb-1">
          <span class="text-xs font-medium text-slate-500 dark:text-slate-400">
            可用后端连接
          </span>
          <UiButton
            v-if="isDesktop"
            variant="secondary"
            size="sm"
            @click="openCreateForm"
          >
            + 添加远程服务
          </UiButton>
        </div>

        <div class="space-y-2">
          <!-- Connection items -->
          <div
            v-for="conn in connections"
            :key="conn.id"
            class="flex items-center justify-between p-3 rounded-md border transition-colors"
            :class="[
              conn.id === activeConnection?.id
                ? 'border-blue-500 bg-blue-50/50 dark:bg-blue-950/20'
                : 'border-slate-200 dark:border-slate-800 hover:border-slate-300 dark:hover:border-slate-700 bg-white dark:bg-slate-900',
            ]"
          >
            <div class="flex items-center space-x-3 min-w-0">
              <div
                class="w-8 h-8 rounded flex items-center justify-center shrink-0"
                :class="conn.type === 'local' ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400' : 'bg-sky-100 text-sky-700 dark:bg-sky-950 dark:text-sky-400'"
              >
                <svg v-if="conn.type === 'local'" viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6">
                  <rect x="2" y="3" width="12" height="8" rx="1" />
                  <line x1="5" y1="14" x2="11" y2="14" />
                  <line x1="8" y1="11" x2="8" y2="14" />
                </svg>
                <svg v-else viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6">
                  <circle cx="8" cy="8" r="6.5" />
                  <ellipse cx="8" cy="8" rx="3" ry="6.5" />
                  <line x1="1.5" y1="8" x2="14.5" y2="8" />
                </svg>
              </div>
              <div class="min-w-0">
                <div class="flex items-center space-x-2">
                  <span class="font-medium text-sm text-slate-900 dark:text-slate-100 truncate">
                    {{ conn.name }}
                  </span>
                  <UiBadge
                    v-if="conn.id === activeConnection?.id"
                    variant="info"
                  >
                    当前激活
                  </UiBadge>
                  <UiBadge
                    v-else-if="conn.type === 'local' && status?.localService.running"
                    variant="success"
                  >
                    本地运行中
                  </UiBadge>
                </div>
                <div class="text-xs text-slate-500 dark:text-slate-400 truncate mt-0.5">
                  <span v-if="conn.type === 'local'">
                    本地单机服务（127.0.0.1:{{ status?.localService.port || '动态' }}）
                  </span>
                  <span v-else>
                    {{ conn.url }}
                  </span>
                </div>
              </div>
            </div>

            <div class="flex items-center space-x-2 shrink-0">
              <UiButton
                v-if="conn.id !== activeConnection?.id"
                variant="primary"
                size="sm"
                @click="handleSwitch(conn)"
              >
                连接
              </UiButton>
              <template v-if="conn.type === 'remote'">
                <button
                  type="button"
                  title="编辑"
                  class="p-1.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 rounded hover:bg-slate-100 dark:hover:bg-slate-800"
                  @click="openEditForm(conn)"
                >
                  <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                    <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                  </svg>
                </button>
                <button
                  type="button"
                  title="删除"
                  class="p-1.5 text-slate-400 hover:text-red-600 rounded hover:bg-slate-100 dark:hover:bg-slate-800"
                  @click="handleRemoveConnection(conn.id)"
                >
                  <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="3 6 5 6 21 6" />
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                  </svg>
                </button>
              </template>
            </div>
          </div>
        </div>
      </div>

      <!-- Edit / Create Form -->
      <div v-else class="space-y-4">
        <div class="text-sm font-medium text-slate-900 dark:text-slate-100 pb-1 border-b border-slate-100 dark:border-slate-800">
          {{ formId ? '编辑远程服务器' : '添加远程服务器' }}
        </div>

        <UiAlert v-if="formError" variant="error">
          {{ formError }}
        </UiAlert>

        <div class="space-y-3">
          <div>
            <label class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
              服务器名称 *
            </label>
            <UiInput
              v-model="formName"
              placeholder="例如：开发测试服 / 团队工作站"
            />
          </div>

          <div>
            <label class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
              服务地址 (URL) *
            </label>
            <div class="flex space-x-2">
              <UiInput
                v-model="formUrl"
                placeholder="http://192.168.1.100:8080"
                class="flex-1"
              />
              <UiButton
                variant="secondary"
                type="button"
                :loading="isTesting"
                @click="runTestConnection"
              >
                测试连通性
              </UiButton>
            </div>
          </div>

          <div v-if="testResult" class="text-xs p-2 rounded border" :class="testResult.ok ? 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-300' : 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300'">
            <div class="flex items-center space-x-1.5 font-medium">
              <svg v-if="testResult.ok" viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="3 8 6 11 13 4" />
              </svg>
              <svg v-else viewBox="0 0 16 16" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="3" y1="3" x2="13" y2="13" />
                <line x1="13" y1="3" x2="3" y2="13" />
              </svg>
              <span>{{ testResult.ok ? '连接成功' : '连接失败' }}</span>
              <span v-if="testResult.latencyMs !== undefined" class="text-slate-400">({{ testResult.latencyMs }}ms)</span>
            </div>
            <div v-if="testResult.message" class="mt-0.5">{{ testResult.message }}</div>
          </div>

          <div>
            <label class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
              访问密码 / 凭据（可选，将安全加密存储）
            </label>
            <UiInput
              v-model="formToken"
              type="password"
              placeholder="留空则保持原凭据或免密"
            />
          </div>
        </div>

        <div class="pt-3 flex items-center justify-end space-x-2.5 border-t border-slate-100 dark:border-slate-800">
          <UiButton
            variant="secondary"
            type="button"
            @click="cancelEdit"
          >
            取消
          </UiButton>
          <UiButton
            variant="primary"
            type="button"
            @click="handleSaveConnection"
          >
            保存
          </UiButton>
        </div>
      </div>
    </div>
  </UiDialog>
</template>
