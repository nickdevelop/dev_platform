// Appearance settings API calls. The Appearance DTO mirrors the server's
// settings.Appearance and docs/api/openapi.yaml; the preset metadata lives in
// theme/presets.ts so the client and the theme application share one type.
import { apiRequest } from './client'
import type { Appearance } from '../theme/presets'

export type { Appearance }

// defaultAppearance mirrors the server's default, used before the first load.
export const defaultAppearance: Appearance = {
  theme: 'light',
  font: 'system',
  customFont: '',
  fontSize: 14,
  density: 'standard',
}

export function getAppearance(): Promise<Appearance> {
  return apiRequest<Appearance>('GET', '/settings/appearance')
}

export function putAppearance(appearance: Appearance, csrfToken: string): Promise<Appearance> {
  return apiRequest<Appearance>('PUT', '/settings/appearance', { body: appearance, csrfToken })
}
