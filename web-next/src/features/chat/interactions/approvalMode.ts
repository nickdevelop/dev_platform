import type { FormInfo } from '../../../api/interactions'

interface AutoField {
  key?: string
  hidden?: boolean
  type?: string
  required?: boolean
  default?: unknown
  options?: Array<{ value: unknown; label?: string; description?: string }>
}

export type ApprovalMode = 'default' | 'auto-approve' | 'autopilot'

export const APPROVAL_MODE_STORAGE_KEY = 'dev-platform-approval-mode'

export function getPersistedApprovalMode(): ApprovalMode {
  try {
    const value = localStorage.getItem(APPROVAL_MODE_STORAGE_KEY)
    return value === 'auto-approve' || value === 'autopilot' ? value : 'default'
  } catch {
    return 'default'
  }
}

export function setPersistedApprovalMode(mode: ApprovalMode): void {
  try {
    localStorage.setItem(APPROVAL_MODE_STORAGE_KEY, mode)
  } catch {
    // Storage may be unavailable; the in-memory selection still works.
  }
}

// Recommended answers for autopilot: prefer an option labelled recommended,
// otherwise the first option, otherwise the field default. Required fields
// without any usable value keep the form manual.
export function resolveAutopilotForm(form: FormInfo): {
  answers: Record<string, unknown>
  canAutoSubmit: boolean
} {
  const answers: Record<string, unknown> = {}
  let canAutoSubmit = true
  const fields = (Array.isArray(form.fields) ? form.fields : []) as AutoField[]
  for (const field of fields) {
    if (!field.key || field.hidden) continue
    if (field.options && field.options.length > 0) {
      const recommended =
        field.options.find((option) => /recommended|推荐/i.test(`${option.label} ${option.description ?? ''}`)) ??
        field.options[0]
      if (field.type === 'multiselect') answers[field.key] = [recommended.value]
      else if (field.type === 'number' || field.type === 'integer') {
        const numeric = Number(recommended.value)
        answers[field.key] = Number.isNaN(numeric) ? recommended.value : numeric
      } else answers[field.key] = recommended.value
      continue
    }
    if (field.type === 'boolean') {
      answers[field.key] = field.default !== undefined ? Boolean(field.default) : true
      continue
    }
    if (field.default !== undefined && field.default !== null && field.default !== '') {
      answers[field.key] = field.default
      continue
    }
    if (field.required) canAutoSubmit = false
  }
  return { answers, canAutoSubmit }
}
