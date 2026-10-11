<script setup lang="ts">
// Resource editor for Skill, MCP and prompt definitions. The credential value
// is write-only: the editor never receives a stored value, and only includes
// `secretValue` in the submitted body when the user typed one.
import { ref, watch } from 'vue'
import type { BackendId } from '../../api/agents'
import type {
  PromptScope,
  ResourceDefinition,
  ResourceDefinitionBody,
  ResourceKind,
} from '../../api/resources'
import SecretField from './SecretField.vue'

const props = defineProps<{
  resource?: ResourceDefinition | null
}>()

const emit = defineEmits<{
  submit: [body: ResourceDefinitionBody]
  cancel: []
}>()

const kind = ref<ResourceKind>('skill')
const name = ref('')
const description = ref('')
const content = ref('')
const enabled = ref(true)
const promptScope = ref<PromptScope>('project')
const backends = ref<BackendId[]>([])
const secretValue = ref('')

function reset() {
  const resource = props.resource
  kind.value = resource?.kind ?? 'skill'
  name.value = resource?.name ?? ''
  description.value = resource?.description ?? ''
  content.value = resource?.content ?? ''
  enabled.value = resource?.enabled ?? true
  promptScope.value = resource?.promptScope ?? 'project'
  backends.value = [...(resource?.backends ?? [])]
  secretValue.value = ''
}

watch(() => props.resource, reset, { immediate: true })

// A resource kind is immutable after creation, so editing locks the selector.
const editing = () => props.resource != null

function toggleBackend(backend: BackendId, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  backends.value = checked
    ? [...new Set([...backends.value, backend])]
    : backends.value.filter((item) => item !== backend)
}

function submit() {
  const body: ResourceDefinitionBody = {
    kind: kind.value,
    name: name.value.trim(),
    description: description.value.trim() || undefined,
    content: content.value,
    enabled: enabled.value,
  }
  if (kind.value === 'prompt') {
    body.promptScope = promptScope.value
  }
  if (kind.value === 'mcp') {
    body.backends = [...backends.value]
    // Only send a value the user actually typed; an empty field leaves the
    // stored credential untouched.
    if (secretValue.value !== '') {
      body.secretValue = secretValue.value
    }
    if (props.resource?.secretRef) {
      body.secretRef = props.resource.secretRef
    }
  }
  emit('submit', body)
}
</script>

<template>
  <form class="resource-editor" data-test="resource-editor" @submit.prevent="submit">
    <h3>{{ editing() ? '编辑资源' : '新建资源' }}</h3>

    <label>
      类型
      <select v-model="kind" :disabled="editing()" data-test="resource-kind">
        <option value="skill">Skill</option>
        <option value="mcp">MCP</option>
        <option value="prompt">提示词</option>
      </select>
    </label>

    <label>
      名称
      <input v-model="name" type="text" required data-test="resource-name" />
    </label>

    <label>
      描述
      <input v-model="description" type="text" data-test="resource-description" />
    </label>

    <label>
      内容
      <textarea v-model="content" rows="4" data-test="resource-content"></textarea>
    </label>

    <label v-if="kind === 'prompt'">
      作用范围
      <select v-model="promptScope" data-test="resource-scope">
        <option value="project">项目级</option>
        <option value="session">会话级</option>
      </select>
    </label>

    <fieldset v-if="kind === 'mcp'" class="backends">
      <legend>适用后端（不选表示全部）</legend>
      <label>
        <input
          type="checkbox"
          :checked="backends.includes('opencode-v2')"
          data-test="backend-opencode-v2"
          @change="toggleBackend('opencode-v2', $event)"
        />
        opencode-v2
      </label>
      <label>
        <input
          type="checkbox"
          :checked="backends.includes('pi')"
          data-test="backend-pi"
          @change="toggleBackend('pi', $event)"
        />
        pi
      </label>
    </fieldset>

    <SecretField
      v-if="kind === 'mcp'"
      v-model="secretValue"
      :has-stored-secret="Boolean(resource?.secretRef)"
    />

    <label class="enabled">
      <input v-model="enabled" type="checkbox" data-test="resource-enabled" />
      启用
    </label>

    <div class="actions">
      <button type="submit" data-test="resource-save">保存</button>
      <button type="button" data-test="resource-cancel" @click="emit('cancel')">取消</button>
    </div>
  </form>
</template>

<style scoped>
.resource-editor {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 8px;
  padding: 0.75rem;
}

.resource-editor h3 {
  margin: 0;
  font-size: 1rem;
}

label {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  font-size: 0.9rem;
}

input[type='text'],
select,
textarea {
  padding: 0.35rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  font: inherit;
}

.backends {
  display: flex;
  gap: 0.75rem;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 0.4rem 0.6rem;
}

.backends label,
.enabled {
  flex-direction: row;
  align-items: center;
  gap: 0.35rem;
}

.actions {
  display: flex;
  gap: 0.5rem;
}

.actions button {
  padding: 0.35rem 0.8rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-surface);
  cursor: pointer;
}
</style>
