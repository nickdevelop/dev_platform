// The composer's context for one open session: which model is in force, the
// backend model catalog's context limit and the usage meter built on it, plus
// the workdir row (directory, git branch and the copy button). The pickers own
// catalog fetching, so this composable only reads the shared cache; it never
// issues its own catalog request.

import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { AgentSession } from '../../../api/sessions'
import type { ChatEntry } from '../state/sessionLog'
import { useAgentsStore } from '../../../stores/agents'
import { useSessionsStore } from '../../../stores/sessions'
import { getInstanceGitInfo, type GitInfo } from '../../../api/git'
import { cachedModels } from '../../../shared/modelCache'
import { computeContextUsage } from '../../../shared/contextUsage'
import { ellipsisMiddle, formatTokens } from '../../../shared/format'

export interface ComposerContextOptions {
  projectId: Ref<number>
  sessionId: Ref<string>
  activeSession: ComputedRef<AgentSession | null>
  activeMessages: ComputedRef<ChatEntry[]>
}

export function useComposerContext(options: ComposerContextOptions) {
  const agents = useAgentsStore()
  const sessions = useSessionsStore()

  // The composer's model for this session: the store resolves it from the
  // user's explicit choice, the model the session actually used, or the
  // picker's own first-model fallback. An update:modelValue from the picker is
  // that automatic fallback, so it is recorded as such; a deliberate pick is
  // recorded separately via the picker's `picked` event so it is never
  // mistaken for a fallback.
  const selectedModel = computed({
    get: () => sessions.modelFor(options.sessionId.value),
    set: (value: string) => sessions.setAutoModel(options.sessionId.value, value),
  })

  function onModelPicked(id: string) {
    sessions.setModel(options.sessionId.value, id)
  }

  // Whether the project's model list for the session's backend loaded. A
  // failed list disables sending rather than falling back to a free-text model.
  const modelOk = ref(true)
  // The picker ref is bound by the template; the composable only needs it to
  // force a catalog reload after a refused send, and the panel passes its own
  // element in, so nothing is created here.

  const instance = computed(() => {
    const session = options.activeSession.value
    if (!session) return null
    return agents.instances.find((entry) => entry.id === session.instanceId) ?? null
  })

  // The workdir row shows the session's real working directory on its own line:
  // middle-ellipsized with the full path on hover and a copy button, plus the
  // git branch (with a dirty marker) when the directory is a git work tree.
  const shortDirectory = computed(() => ellipsisMiddle(instance.value?.directory ?? ''))
  const gitInfo = ref<GitInfo | null>(null)
  const gitCache = new Map<string, GitInfo>()
  const copiedPath = ref(false)
  async function loadGit(force = false) {
    const session = options.activeSession.value
    if (!session) {
      gitInfo.value = null
      return
    }
    const key = session.instanceId
    if (!force && gitCache.has(key)) {
      gitInfo.value = gitCache.get(key) ?? null
      return
    }
    try {
      const info = await getInstanceGitInfo(options.projectId.value, session.instanceId)
      gitCache.set(key, info)
      if (options.activeSession.value?.instanceId === key) gitInfo.value = info
    } catch {
      // Branch display is best-effort: a failed probe leaves the row without
      // the branch chip rather than breaking the session.
      if (options.activeSession.value?.instanceId === key) gitInfo.value = null
    }
  }
  const branchLabel = computed(() => {
    const info = gitInfo.value
    if (!info?.isGit || !info.branch) return ''
    return `⎇ ${info.branch}${info.dirty ? '*' : ''}`
  })
  const branchTitle = computed(() => {
    const info = gitInfo.value
    if (!info?.isGit) return ''
    const state = info.dirty ? '有未提交改动' : '干净'
    return `分支 ${info.branch ?? ''} · ${state}${info.root ? ` · ${info.root}` : ''}`
  })
  async function copyDirectory() {
    const directory = instance.value?.directory
    if (!directory) return
    try {
      await navigator.clipboard.writeText(directory)
    } catch {
      // Non-secure contexts have no async clipboard: fall back to execCommand.
      const area = document.createElement('textarea')
      area.value = directory
      document.body.appendChild(area)
      area.select()
      try {
        document.execCommand('copy')
      } catch {
        // Copy is convenience only; failure stays silent.
      }
      area.remove()
    }
    copiedPath.value = true
    setTimeout(() => {
      copiedPath.value = false
    }, 1500)
  }

  // The composer context line anchors on exact backend usage and estimates the
  // turn in flight. The limit is read from the model picker's cached catalog
  // for the composer's current model — the picker owns fetching, so this panel
  // never issues its own catalog request; without a known limit no percentage
  // is derived.
  const modelLimit = ref<number | undefined>(undefined)
  function refreshModelLimit() {
    const session = options.activeSession.value
    const model = selectedModel.value
    if (!session || !model) {
      modelLimit.value = undefined
      return
    }
    const cached = cachedModels(options.projectId.value, session.backend)
    const base = model.split('#')[0]
    const found = cached?.find((entry) => entry.id === model || entry.id === base)
    const limit = found?.contextLimit
    modelLimit.value = typeof limit === 'number' && limit > 0 ? limit : undefined
  }
  function onModelStatus(ok: boolean) {
    modelOk.value = ok
    // The picker just (re)loaded its catalog: the cached limit may have
    // appeared or changed.
    refreshModelLimit()
  }
  const contextUsage = computed(() =>
    computeContextUsage(
      options.activeMessages.value,
      sessions.lifecycleUsage[options.sessionId.value],
      modelLimit.value,
    ),
  )
  const showContextLine = computed(
    () => contextUsage.value.used > 0 || contextUsage.value.limit !== undefined,
  )
  const contextMain = computed(() => {
    const usage = contextUsage.value
    const used = `${usage.estimated ? '~' : ''}${formatTokens(usage.used)}`
    if (usage.limit === undefined || usage.percent === undefined) return `上下文 ${used}`
    return `上下文 ${used}/${formatTokens(usage.limit)}（${usage.percent}%）`
  })
  const contextDetail = computed(() => {
    const parts = contextUsage.value.parts
    const segments = [`↑${formatTokens(parts.input)}`, `↓${formatTokens(parts.output)}`]
    if (parts.cacheRead > 0 || parts.cacheWrite > 0) {
      segments.push(`缓存读${formatTokens(parts.cacheRead)}/写${formatTokens(parts.cacheWrite)}`)
    }
    if (parts.reasoning > 0) segments.push(`推理${formatTokens(parts.reasoning)}`)
    return segments.join(' ')
  })
  const contextBarClass = computed(() => {
    const percent = contextUsage.value.percent ?? 0
    if (percent >= 95) return 'context-bar-fill danger'
    if (percent >= 80) return 'context-bar-fill warn'
    return 'context-bar-fill'
  })
  const contextBarWidth = computed(() => `${Math.min(contextUsage.value.percent ?? 0, 100)}%`)

  return {
    selectedModel,
    onModelPicked,
    modelOk,
    instance,
    shortDirectory,
    gitInfo,
    copiedPath,
    loadGit,
    branchLabel,
    branchTitle,
    copyDirectory,
    modelLimit,
    refreshModelLimit,
    onModelStatus,
    contextUsage,
    showContextLine,
    contextMain,
    contextDetail,
    contextBarClass,
    contextBarWidth,
  }
}
