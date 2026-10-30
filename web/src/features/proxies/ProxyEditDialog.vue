<script setup lang="ts">
import { ref, computed } from 'vue'
import { UiFormDialog, UiInput, UiSelect, UiButton } from '../../shared/ui'
import { createProxy, updateProxy, type ServiceProxy, type ProxyType } from '../../api/proxies'
import { useProjectsStore } from '../../stores/projects'
import ProxyDirectoryPicker from './ProxyDirectoryPicker.vue'

const props = defineProps<{
  proxy?: ServiceProxy | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', proxy: ServiceProxy): void
}>()

const projectsStore = useProjectsStore()

const isEditing = computed(() => !!props.proxy)

const name = ref(props.proxy?.name ?? '')
const alias = ref(props.proxy?.alias ?? '')
const type = ref<ProxyType>(props.proxy?.type ?? 'directory')
const target = ref(props.proxy?.target ?? '')
const enabled = ref(props.proxy?.enabled ?? true)

const selectedProjectId = ref<number | ''>('')
const showDirPicker = ref(false)

const saving = ref(false)
const errorMessage = ref('')

const aliasRegex = /^[a-z0-9][a-z0-9-]{0,31}$/

function onProjectSelected() {
  if (!selectedProjectId.value) return
  const proj = projectsStore.projects.find((p) => p.id === Number(selectedProjectId.value))
  if (proj) {
    target.value = proj.path
    if (!name.value.trim()) {
      name.value = `${proj.name} 预览`
    }
    if (!alias.value.trim()) {
      alias.value = proj.name.toLowerCase().replace(/[^a-z0-9-]/g, '-')
    }
  }
}

function appendSubdir(subdir: string) {
  if (!target.value.trim()) return
  const base = target.value.trim().replace(/\/+$/, '')
  // If base already ends with subdir, avoid duplicate
  if (!base.endsWith(`/${subdir}`)) {
    target.value = `${base}/${subdir}`
  }
}

function onDirectoryChosen(chosenPath: string) {
  target.value = chosenPath
  showDirPicker.value = false
}

async function submit() {
  errorMessage.value = ''

  const trimmedName = name.value.trim()
  if (!trimmedName) {
    errorMessage.value = '请输入代理名称'
    return
  }

  const trimmedAlias = alias.value.trim().toLowerCase()
  if (!trimmedAlias || !aliasRegex.test(trimmedAlias)) {
    errorMessage.value = 'ContextPath 别名仅支持小写字母、数字和破折号（1~32位），例如：my-demo'
    return
  }

  const trimmedTarget = target.value.trim()
  if (!trimmedTarget) {
    errorMessage.value = type.value === 'directory' ? '请输入或选择目录路径' : '请输入目标端口号'
    return
  }

  if (type.value === 'port') {
    const port = Number(trimmedTarget)
    if (isNaN(port) || port < 1024 || port > 65535) {
      errorMessage.value = '端口号必须在 1024 ~ 65535 范围内'
      return
    }
  }

  saving.value = true
  try {
    let result: ServiceProxy
    if (isEditing.value && props.proxy) {
      result = await updateProxy(props.proxy.id, {
        name: trimmedName,
        alias: trimmedAlias,
        type: type.value,
        target: trimmedTarget,
        enabled: enabled.value,
      })
    } else {
      result = await createProxy({
        name: trimmedName,
        alias: trimmedAlias,
        type: type.value,
        target: trimmedTarget,
        enabled: enabled.value,
      })
    }
    emit('saved', result)
  } catch (err: any) {
    if (err?.error === 'duplicate_proxy_alias') {
      errorMessage.value = `ContextPath 别名 "${trimmedAlias}" 已被占用，请更换`
    } else if (err?.error === 'invalid_proxy_target') {
      errorMessage.value = type.value === 'directory' ? '目录不存在或不可访问' : '无效的端口号'
    } else {
      errorMessage.value = err?.error || err?.message || '保存失败，请检查输入'
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiFormDialog
    :title="isEditing ? '编辑服务代理' : '新建服务代理'"
    width-class="w-full max-w-lg"
    :confirm-label="saving ? '保存中…' : '保存'"
    :confirm-loading="saving"
    :error="errorMessage"
    @close="emit('close')"
    @submit="submit"
  >
    <div class="field flex flex-col gap-1.5">
      <label for="dlg-proxy-name" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        代理名称 <span class="text-red-500">*</span>
      </label>
      <UiInput
        id="dlg-proxy-name"
        v-model="name"
        placeholder="例如：前端构建产物预览 / 本地 Vite 开发服务"
        required
        block
      />
    </div>

    <div class="field flex flex-col gap-1.5">
      <label for="dlg-proxy-alias" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        ContextPath 别名 <span class="text-red-500">*</span>
      </label>
      <div class="flex items-center gap-1.5">
        <code class="text-xs text-slate-400 select-none">/proxy/</code>
        <UiInput
          id="dlg-proxy-alias"
          v-model="alias"
          placeholder="例如：web-demo"
          required
          block
        />
        <code class="text-xs text-slate-400 select-none">/</code>
      </div>
      <p class="text-[11px] text-slate-400">
        访问路径前缀，仅限小写英文、数字与横线。外部访问链接将形如 <code>http://host:port/proxy/{{ alias || 'demo' }}/</code>
      </p>
    </div>

    <div class="field-row grid grid-cols-1 sm:grid-cols-2 gap-3">
      <div class="field flex flex-col gap-1.5">
        <label for="dlg-proxy-type" class="text-xs font-medium text-slate-700 dark:text-slate-300">
          代理类型 <span class="text-red-500">*</span>
        </label>
        <UiSelect id="dlg-proxy-type" v-model="type" block required>
          <option value="directory">静态目录 (Directory)</option>
          <option value="port">端口反向代理 (Port)</option>
        </UiSelect>
      </div>

      <div class="field flex flex-col gap-1.5">
        <label class="text-xs font-medium text-slate-700 dark:text-slate-300">
          启用状态
        </label>
        <div class="flex items-center h-[34px]">
          <label class="inline-flex items-center cursor-pointer select-none text-xs text-slate-700 dark:text-slate-300">
            <input
              type="checkbox"
              v-model="enabled"
              class="w-4 h-4 rounded text-blue-600 focus:ring-blue-500 mr-2 border-slate-300 dark:border-slate-600 dark:bg-slate-700"
            />
            立即启用此代理
          </label>
        </div>
      </div>
    </div>

    <!-- Quick select from existing projects (when type === 'directory') -->
    <div v-if="type === 'directory' && projectsStore.projects.length > 0" class="field flex flex-col gap-1.5 bg-slate-50 dark:bg-slate-800/60 p-2.5 rounded border border-slate-200 dark:border-slate-700">
      <div class="flex items-center justify-between">
        <label for="dlg-proxy-project" class="text-xs font-medium text-slate-700 dark:text-slate-300">
          从已有项目选取（快捷填入）
        </label>
        <span class="text-[11px] text-slate-400">选择后自动带出项目目录</span>
      </div>
      <UiSelect id="dlg-proxy-project" v-model="selectedProjectId" block @change="onProjectSelected">
        <option value="">选择项目…</option>
        <option v-for="p in projectsStore.projects" :key="p.id" :value="p.id">
          {{ p.name }} ({{ p.path }})
        </option>
      </UiSelect>
      <div v-if="target" class="flex items-center gap-1 mt-1 flex-wrap">
        <span class="text-[11px] text-slate-400">追加常用构建子目录：</span>
        <button
          v-for="sub in ['dist', 'build', 'public', 'out']"
          :key="sub"
          type="button"
          class="text-[11px] px-1.5 py-0.5 rounded bg-white dark:bg-slate-700 border border-slate-200 dark:border-slate-600 hover:border-blue-400 text-slate-600 dark:text-slate-300 transition-colors"
          @click="appendSubdir(sub)"
        >
          + /{{ sub }}
        </button>
      </div>
    </div>

    <!-- Target Field -->
    <div class="field flex flex-col gap-1.5">
      <label for="dlg-proxy-target" class="text-xs font-medium text-slate-700 dark:text-slate-300">
        {{ type === 'directory' ? '服务器目录绝对路径' : '本地监听端口号' }} <span class="text-red-500">*</span>
      </label>
      <div class="flex items-center gap-2">
        <UiInput
          id="dlg-proxy-target"
          v-model="target"
          :placeholder="type === 'directory' ? '例如：/data/projects/my-app/dist' : '例如：5173、3000、8080'"
          required
          block
          class="flex-1"
        />
        <UiButton
          v-if="type === 'directory'"
          variant="secondary"
          size="sm"
          type="button"
          class="shrink-0"
          @click="showDirPicker = true"
        >
          浏览目录…
        </UiButton>
      </div>
      <p class="text-[11px] text-slate-400">
        {{ type === 'directory'
          ? '服务器上生成的静态 HTML/构建产物所在的绝对路径。可点击“浏览目录…”选择服务器文件夹。'
          : '服务器本地回环（127.0.0.1）上运行的 HTTP 端口。支持 WebSocket 双向转发（满足 Vite HMR 热更新）。'
        }}
      </p>
    </div>

    <!-- Directory Picker Dialog -->
    <ProxyDirectoryPicker
      v-if="showDirPicker"
      :initial-path="target"
      @close="showDirPicker = false"
      @select="onDirectoryChosen"
    />
  </UiFormDialog>
</template>
