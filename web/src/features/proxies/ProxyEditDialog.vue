<script setup lang="ts">
// ProxyEditDialog: create or edit a ContextPath service proxy.
//
// `EntityFormDialog` + `el-form` own the chrome and the validation: the previous
// version hand-rolled every rule into `submit()` and stacked the failure into an
// `error` prop on the shell, so a typo in the alias and a server-side conflict
// looked identical and both arrived only after a click on 保存. Now each field
// validates on blur/change, and the `/proxy/:alias/` shape is expressed as an
// `el-input` with a prefix and suffix rather than two loose <code> tags that
// were not part of the control.
import { computed, reactive, ref } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { FolderOpened } from '@element-plus/icons-vue'
import { createProxy, updateProxy, type ServiceProxy, type ProxyType } from '../../api/proxies'
import { useProjectsStore } from '../../stores/projects'
import EntityFormDialog from '../../shared/components/EntityFormDialog.vue'
import ProxyDirectoryPicker from './ProxyDirectoryPicker.vue'

const props = defineProps<{
  proxy?: ServiceProxy | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', proxy: ServiceProxy): void
}>()

const projectsStore = useProjectsStore()
const formRef = ref<FormInstance>()
const saving = ref(false)
const showDirPicker = ref(false)
const selectedProjectId = ref<number | ''>('')

const isEditing = computed(() => !!props.proxy)

const form = reactive({
  name: props.proxy?.name ?? '',
  alias: props.proxy?.alias ?? '',
  type: (props.proxy?.type ?? 'directory') as ProxyType,
  target: props.proxy?.target ?? '',
  enabled: props.proxy?.enabled ?? true,
})

// ContextPath alias: lowercase letters, digits and dashes, starting with an
// alphanumeric one. Same expression the server enforces.
const ALIAS_PATTERN = /^[a-z0-9][a-z0-9-]{0,31}$/

// The target label, placeholder and validator all follow the type, so one
// computed pair keeps the field and its hint from drifting apart.
const isDirectory = computed(() => form.type === 'directory')

const rules: FormRules = {
  name: [{ required: true, message: '请输入代理名称', trigger: 'blur' }],
  alias: [
    { required: true, message: '请输入 ContextPath 别名', trigger: 'blur' },
    {
      validator: (_rule, value: string, callback) => {
        if (!value || ALIAS_PATTERN.test(value.trim().toLowerCase())) callback()
        else callback(new Error('别名仅支持小写字母、数字和破折号（1~32位），例如：my-demo'))
      },
      trigger: 'blur',
    },
  ],
  target: [
    {
      validator: (_rule, value: string, callback) => {
        const trimmed = (value || '').trim()
        if (!trimmed) {
          callback(new Error(isDirectory.value ? '请输入或选择目录路径' : '请输入目标端口号'))
          return
        }
        if (!isDirectory.value) {
          const port = Number(trimmed)
          if (Number.isNaN(port) || port < 1024 || port > 65535) {
            callback(new Error('端口号必须在 1024 ~ 65535 范围内'))
            return
          }
        }
        callback()
      },
      trigger: 'blur',
    },
  ],
}

const projectOptions = computed(() =>
  projectsStore.projects.map((p) => ({ value: p.id, label: p.name, path: p.path })),
)

function onProjectSelected() {
  if (!selectedProjectId.value) return
  const proj = projectsStore.projects.find((p) => p.id === Number(selectedProjectId.value))
  if (!proj) return
  form.target = proj.path
  if (!form.name.trim()) form.name = `${proj.name} 预览`
  if (!form.alias.trim()) form.alias = proj.name.toLowerCase().replace(/[^a-z0-9-]/g, '-')
  // Filling the field through the store bypasses the input's blur, so clear any
  // stale "请输入目录路径" the empty value left behind.
  formRef.value?.clearValidate('target')
}

function appendSubdir(subdir: string) {
  if (!form.target.trim()) return
  const base = form.target.trim().replace(/\/+$/, '')
  if (!base.endsWith(`/${subdir}`)) form.target = `${base}/${subdir}`
}

function onDirectoryChosen(chosenPath: string) {
  form.target = chosenPath
  showDirPicker.value = false
  formRef.value?.clearValidate('target')
}

// Server-side rejections that deserve the user's attention, keyed by the code
// the API returns; anything else falls back to the raw message.
const SERVER_ERRORS: Record<string, string> = {
  duplicate_proxy_alias: 'ContextPath 别名已被占用，请更换',
  invalid_proxy_target: '目录不存在或不可访问，或端口无效',
}

async function submit() {
  if (saving.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  const payload = {
    name: form.name.trim(),
    alias: form.alias.trim().toLowerCase(),
    type: form.type,
    target: form.target.trim(),
    enabled: form.enabled,
  }
  try {
    const result = props.proxy
      ? await updateProxy(props.proxy.id, payload)
      : await createProxy(payload)
    emit('saved', result)
  } catch (err: any) {
    ElMessage.error(SERVER_ERRORS[err?.error] || err?.error || err?.message || '保存失败，请检查输入')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <EntityFormDialog
    :title="isEditing ? '编辑服务代理' : '新建服务代理'"
    subtitle="把服务器目录或本地端口暴露为独立的 /proxy/别名/ 访问路径"
    :identifier="isEditing ? props.proxy?.id : null"
    :saving="saving"
    :width="620"
    save-test="btn-save-proxy"
    :data-test="`proxy-edit-dialog-${isEditing ? 'edit' : 'create'}`"
    @save="submit"
    @cancel="emit('close')"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
      <section class="entity-form-section">
        <el-form-item label="代理名称" prop="name">
          <el-input
            v-model="form.name"
            maxlength="60"
            placeholder="例如：前端构建产物预览 / 本地 Vite 开发服务"
            data-test="input-proxy-name"
          />
        </el-form-item>

        <el-form-item label="ContextPath 别名" prop="alias">
          <div class="w-full space-y-1">
            <el-input
              v-model="form.alias"
              maxlength="32"
              placeholder="例如：web-demo"
              class="entity-mono-input"
              data-test="input-proxy-alias"
            >
              <!-- 【/proxy/】与尾部的【/】是路径形状的一部分，得坐在控件里：
                   el-input 的 prefix / suffix 只有槽，没有同名 prop——写成属性会
                   落到原生 <input> 上（它那里 prefix 是只读 getter），既只会喷 Vue
                   警告，前缀也永远不会渲染。 -->
              <template #prefix>
                <span class="path-affix">/proxy/</span>
              </template>
              <template #suffix>
                <span class="path-affix">/</span>
              </template>
            </el-input>
            <p class="entity-field-hint">
              访问路径前缀，仅限小写英文、数字与横线。外部访问链接形如
              <code>http://host:port/proxy/{{ form.alias || 'demo' }}/</code>
            </p>
          </div>
        </el-form-item>
      </section>

      <section class="entity-form-section">
        <el-form-item label="代理类型" prop="type">
          <el-radio-group v-model="form.type" data-test="segment-proxy-type">
            <el-radio value="directory">静态目录</el-radio>
            <el-radio value="port">端口反向代理</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item>
          <el-checkbox v-model="form.enabled" data-test="check-proxy-enabled">立即启用此代理</el-checkbox>
        </el-form-item>

        <el-form-item
          v-if="isDirectory && projectOptions.length"
          label="从已有项目选取（快捷填入）"
        >
          <div class="w-full space-y-2">
            <el-select
              v-model="selectedProjectId"
              placeholder="选择项目…"
              class="w-full"
              data-test="select-proxy-project"
              @change="onProjectSelected"
            >
              <el-option
                v-for="p in projectOptions"
                :key="p.value"
                :label="p.label"
                :value="p.value"
              >
                <span>{{ p.label }}</span>
                <span class="entity-option-sub font-mono">{{ p.path }}</span>
              </el-option>
            </el-select>

            <div v-if="form.target" class="flex items-center gap-1 flex-wrap">
              <span class="entity-field-hint">追加常用构建子目录：</span>
              <el-button
                v-for="sub in ['dist', 'build', 'public', 'out']"
                :key="sub"
                size="small"
                :data-test="`append-subdir-${sub}`"
                @click="appendSubdir(sub)"
              >
                /{{ sub }}
              </el-button>
            </div>
          </div>
        </el-form-item>

        <el-form-item :label="isDirectory ? '服务器目录绝对路径' : '本地监听端口号'" prop="target">
          <div class="w-full space-y-1">
            <div class="flex items-center gap-2">
              <el-input
                v-model="form.target"
                class="flex-1"
                :placeholder="isDirectory ? '例如：/data/projects/my-app/dist' : '例如：5173、3000、8080'"
                data-test="input-proxy-target"
              />
              <el-button
                v-if="isDirectory"
                :icon="FolderOpened"
                class="shrink-0"
                data-test="btn-browse-dir"
                @click="showDirPicker = true"
              >
                浏览目录…
              </el-button>
            </div>
            <p class="entity-field-hint">
              {{ isDirectory
                ? '服务器上生成的静态 HTML/构建产物所在的绝对路径。可点击「浏览目录…」选择服务器文件夹。'
                : '服务器本地回环（127.0.0.1）上运行的 HTTP 端口。支持 WebSocket 双向转发（满足 Vite HMR 热更新）。' }}
            </p>
          </div>
        </el-form-item>
      </section>
    </el-form>

    <!-- Second-level directory picker: the shared server browser over this form. -->
    <ProxyDirectoryPicker
      v-if="showDirPicker"
      :initial-path="form.target"
      @close="showDirPicker = false"
      @select="onDirectoryChosen"
    />
  </EntityFormDialog>
</template>
