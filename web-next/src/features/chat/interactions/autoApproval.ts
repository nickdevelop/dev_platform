import type { FormField, FormOption } from './types'
import type { FormInfo } from '../../../api/interactions'

export type ApprovalMode = 'default' | 'auto-approve' | 'autopilot'

export const APPROVAL_MODE_STORAGE_KEY = 'dev-platform-approval-mode'

export function getPersistedApprovalMode(): ApprovalMode {
  try {
    const val = localStorage.getItem(APPROVAL_MODE_STORAGE_KEY)
    if (val === 'auto-approve' || val === 'autopilot') return val
    return 'default'
  } catch {
    return 'default'
  }
}

export function setPersistedApprovalMode(mode: ApprovalMode): void {
  try {
    localStorage.setItem(APPROVAL_MODE_STORAGE_KEY, mode)
  } catch {
    // LocalStorage failure fallback
  }
}

export function findRecommendedOption(options: FormOption[]): FormOption | undefined {
  if (!options || options.length === 0) return undefined
  const recommended = options.find((opt) => {
    const text = `${opt.label || ''} ${opt.description || ''}`
    return /\(recommended\)|推荐/i.test(text)
  })
  return recommended ?? options[0]
}

export function deriveFieldRecommendedValue(field: FormField): unknown {
  if (field.options && field.options.length > 0) {
    const rec = findRecommendedOption(field.options)
    if (!rec) return undefined
    if (field.type === 'multiselect') return [rec.value]
    if (field.type === 'number' || field.type === 'integer') {
      const num = Number(rec.value)
      return Number.isNaN(num) ? rec.value : num
    }
    return rec.value
  }

  if (field.type === 'boolean') {
    if (field.default !== undefined && field.default !== null) {
      return Boolean(field.default)
    }
    return true
  }

  if (field.default !== undefined && field.default !== null && field.default !== '') {
    if (field.type === 'number' || field.type === 'integer') {
      const num = Number(field.default)
      return Number.isNaN(num) ? field.default : num
    }
    return field.default
  }

  return undefined
}

export interface AutopilotFormResolution {
  answers: Record<string, unknown>
  canAutoSubmit: boolean
}

export function resolveAutopilotForm(form: FormInfo): AutopilotFormResolution {
  const fields = (Array.isArray(form.fields) ? form.fields : []) as FormField[]
  const visible = fields.filter((f) => f && f.key && !f.hidden)

  const answers: Record<string, unknown> = {}
  let canAutoSubmit = visible.length > 0

  for (const field of visible) {
    const rec = deriveFieldRecommendedValue(field)
    if (rec !== undefined && rec !== null && rec !== '') {
      answers[field.key] = rec
    } else {
      if (field.required) {
        canAutoSubmit = false
      }
    }
  }

  return {
    answers,
    canAutoSubmit,
  }
}
