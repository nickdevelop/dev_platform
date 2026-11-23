import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  getEnginesSettings,
  putEnginesSettings,
  type EnginesSetting,
} from '../api/engines'

export const useEnginesStore = defineStore('engines', () => {
  const settings = ref<EnginesSetting | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  async function loadSettings() {
    loading.value = true
    error.value = null
    try {
      settings.value = await getEnginesSettings()
    } catch (err: unknown) {
      error.value = err instanceof Error ? err.message : String(err)
    } finally {
      loading.value = false
    }
  }

  async function saveSettings(payload?: EnginesSetting): Promise<boolean> {
    const toSave = payload || settings.value
    if (!toSave) return false

    saving.value = true
    error.value = null
    try {
      settings.value = await putEnginesSettings(toSave)
      return true
    } catch (err: unknown) {
      error.value = err instanceof Error ? err.message : String(err)
      return false
    } finally {
      saving.value = false
    }
  }

  return {
    settings,
    loading,
    saving,
    error,
    loadSettings,
    saveSettings,
  }
})
