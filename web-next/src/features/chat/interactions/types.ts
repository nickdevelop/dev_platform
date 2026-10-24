export interface FormOption {
  value: string
  label: string
  description?: string
}

export interface FormField {
  key: string
  type: 'string' | 'number' | 'integer' | 'boolean' | 'multiselect' | 'external'
  title?: string
  description?: string
  required?: boolean
  hidden?: boolean
  options?: FormOption[]
  custom?: boolean
  url?: string
  default?: unknown
}
