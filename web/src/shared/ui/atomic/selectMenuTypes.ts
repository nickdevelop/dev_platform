export interface SelectMenuItem<V = any> {
  value: V
  label: string
  sublabel?: string
  tag?: string
  disabled?: boolean
}
