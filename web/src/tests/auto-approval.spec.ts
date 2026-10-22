import { describe, expect, it, beforeEach } from 'vitest'
import {
  findRecommendedOption,
  deriveFieldRecommendedValue,
  resolveAutopilotForm,
  getPersistedApprovalMode,
  setPersistedApprovalMode,
  APPROVAL_MODE_STORAGE_KEY,
} from '../features/chat/interactions/autoApproval'
import type { FormField } from '../features/chat/interactions/types'
import type { FormInfo } from '../api/interactions'

describe('autoApproval logic', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  describe('findRecommendedOption', () => {
    it('returns undefined for empty options', () => {
      expect(findRecommendedOption([])).toBeUndefined()
    })

    it('returns the first option when no option is marked as recommended', () => {
      const options = [
        { value: 'opt1', label: 'Option 1' },
        { value: 'opt2', label: 'Option 2' },
      ]
      expect(findRecommendedOption(options)).toEqual(options[0])
    })

    it('prioritizes options containing (Recommended)', () => {
      const options = [
        { value: 'opt1', label: 'Option 1' },
        { value: 'opt2', label: 'Option 2 (Recommended)' },
        { value: 'opt3', label: 'Option 3' },
      ]
      expect(findRecommendedOption(options)).toEqual(options[1])
    })

    it('prioritizes options containing 推荐 in description', () => {
      const options = [
        { value: 'opt1', label: '方案 A' },
        { value: 'opt2', label: '方案 B', description: '官方推荐模式' },
      ]
      expect(findRecommendedOption(options)).toEqual(options[1])
    })
  })

  describe('deriveFieldRecommendedValue', () => {
    it('selects recommended option for string type with options', () => {
      const field: FormField = {
        key: 'choice',
        type: 'string',
        options: [
          { value: 'yes', label: 'Yes (Recommended)' },
          { value: 'no', label: 'No' },
        ],
      }
      expect(deriveFieldRecommendedValue(field)).toBe('yes')
    })

    it('selects first option as array for multiselect', () => {
      const field: FormField = {
        key: 'items',
        type: 'multiselect',
        options: [
          { value: 'alpha', label: 'Alpha' },
          { value: 'beta', label: 'Beta' },
        ],
      }
      expect(deriveFieldRecommendedValue(field)).toEqual(['alpha'])
    })

    it('returns true for boolean without default, or defaults to default value', () => {
      const booleanField: FormField = { key: 'agree', type: 'boolean' }
      expect(deriveFieldRecommendedValue(booleanField)).toBe(true)

      const booleanFieldFalse: FormField = { key: 'agree', type: 'boolean', default: false }
      expect(deriveFieldRecommendedValue(booleanFieldFalse)).toBe(false)
    })

    it('returns default value if provided for text/number fields', () => {
      const textField: FormField = { key: 'name', type: 'string', default: 'my-service' }
      expect(deriveFieldRecommendedValue(textField)).toBe('my-service')

      const numField: FormField = { key: 'port', type: 'number', default: '8080' }
      expect(deriveFieldRecommendedValue(numField)).toBe(8080)
    })
  })

  describe('resolveAutopilotForm', () => {
    it('resolves all fields and marks canAutoSubmit true if all required fields are satisfied', () => {
      const form: FormInfo = {
        id: 'f1',
        sessionId: 's1',
        title: 'Confirm deploy',
        fields: [
          {
            key: 'env',
            type: 'string',
            required: true,
            options: [
              { value: 'staging', label: 'Staging (Recommended)' },
              { value: 'prod', label: 'Prod' },
            ],
          },
          {
            key: 'confirm',
            type: 'boolean',
            required: true,
          },
        ],
      }

      const res = resolveAutopilotForm(form)
      expect(res.canAutoSubmit).toBe(true)
      expect(res.answers).toEqual({
        env: 'staging',
        confirm: true,
      })
    })

    it('marks canAutoSubmit false if a required text field has no default', () => {
      const form: FormInfo = {
        id: 'f2',
        sessionId: 's1',
        title: 'Enter details',
        fields: [
          {
            key: 'comment',
            type: 'string',
            required: true,
          },
        ],
      }

      const res = resolveAutopilotForm(form)
      expect(res.canAutoSubmit).toBe(false)
      expect(res.answers).toEqual({})
    })
  })

  describe('persistence', () => {
    it('defaults to default mode and persists valid modes', () => {
      expect(getPersistedApprovalMode()).toBe('default')
      setPersistedApprovalMode('auto-approve')
      expect(getPersistedApprovalMode()).toBe('auto-approve')
      setPersistedApprovalMode('autopilot')
      expect(getPersistedApprovalMode()).toBe('autopilot')
      setPersistedApprovalMode('default')
      expect(getPersistedApprovalMode()).toBe('default')
    })
  })
})
