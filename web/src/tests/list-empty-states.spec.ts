import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import OrgPromptsView from '../features/organization/prompts/OrgPromptsView.vue'
import { usePromptsStore } from '../stores/prompts'
import { useAuthStore } from '../stores/auth'
import { installTestCSRF } from './csrf-fixture'
import type { Prompt } from '../api/prompts'

// 提示词页和另三页一样带 URL 同步（?action=new / ?edit=<id>），所以它需要
// 路由：这里用与 organization-views.spec 相同的最小 mock。
const pushMock = vi.fn()
const replaceMock = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock, replace: replaceMock }),
  useRoute: () => ({ query: {}, params: {}, path: '/org/prompts' }),
}))

// 一个空表格有三种完全不同的成因，页面必须说得出口：数据还在路上、被筛选条件滤空、
// 本来就还没有。三态由 shared/components/ListEmptyState.vue 承担，这里按真实页面走。
const rows: Prompt[] = [
  {
    id: 1,
    name: '需求分析',
    type: 'role',
    description: '资深需求分析人设',
    content: '你是一位拥有10年以上经验的资深需求分析师',
    createdAt: '2026-01-02T03:04:05Z',
    updatedAt: '2026-01-02T03:04:05Z',
  },
  {
    id: 2,
    name: '架构评审',
    type: 'flow',
    description: '架构审查流程',
    content: '按检查清单逐项评审',
    createdAt: '2026-01-03T03:04:05Z',
    updatedAt: '2026-01-03T03:04:05Z',
  },
]

async function mountView() {
  const wrapper = mount(OrgPromptsView)
  await flushPromises()
  return wrapper
}

describe('列表页三态（ListEmptyState）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAuthStore().$patch({ authenticated: true })
    installTestCSRF('csrf-token')
    vi.restoreAllMocks()
  })

  it('在首批数据到达前给骨架，而不是一个空面板', async () => {
    const store = usePromptsStore()
    // 挂起的 loadPrompts：loading 一直为真，正是首屏那半秒的样子。
    vi.spyOn(store, 'loadPrompts').mockImplementation(() => new Promise(() => undefined))
    store.$patch({ loading: true, prompts: [] })

    const wrapper = await mountView()

    expect(wrapper.find('[data-test="list-state-loading"]').exists()).toBe(true)
    expect(wrapper.find('.el-skeleton').exists()).toBe(true)
    expect(wrapper.find('[data-test="list-state-empty"]').exists()).toBe(false)
  })

  it('空库给导航同枚图标作为视觉锚，并把主动作放在原地', async () => {
    const store = usePromptsStore()
    vi.spyOn(store, 'loadPrompts').mockResolvedValue(undefined)
    store.$patch({ loading: false, prompts: [] })

    const wrapper = await mountView()
    const empty = wrapper.find('[data-test="list-state-empty"]')

    expect(empty.exists()).toBe(true)
    // 矢量图标在场（OrgSidebar 的 AI 提示词用的就是 EditPen），且没有 emoji。
    expect(empty.find('svg').exists()).toBe(true)
    expect(empty.text()).toContain('尚未创建任何提示词')
    expect(empty.text()).not.toMatch(/[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]/u)
    expect(empty.find('[data-test="btn-create-prompt-empty"]').exists()).toBe(true)
  })

  it('关键词搜不到时说的是搜不到，并给一条退路', async () => {
    const store = usePromptsStore()
    vi.spyOn(store, 'loadPrompts').mockResolvedValue(undefined)
    store.$patch({ loading: false, prompts: rows })

    const wrapper = await mountView()
    expect(wrapper.findAll('.el-table__row').length).toBe(2)

    await wrapper.find('[data-test="input-list-keyword"]').setValue('不存在的词')
    await wrapper.find('[data-test="btn-search-prompts"]').trigger('submit')
    await flushPromises()

    const filtered = wrapper.find('[data-test="list-state-filtered"]')
    expect(filtered.exists()).toBe(true)
    expect(filtered.text()).toContain('没有匹配的提示词')
    expect(filtered.text()).toContain('不存在的词')

    // 退路：一键把筛选和关键词都清掉，行重新出现。
    await filtered.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.el-table__row').length).toBe(2)
    expect((wrapper.find('[data-test="input-list-keyword"]').element as HTMLInputElement).value).toBe('')
  })

  it('枚举筛选单独命中空档时不算搜不到，退路是新建', async () => {
    const store = usePromptsStore()
    vi.spyOn(store, 'loadPrompts').mockResolvedValue(undefined)
    store.$patch({
      loading: false,
      prompts: [rows[0]], // 只有角色类
    })

    const wrapper = await mountView()
    const select = wrapper.findComponent('[data-test="select-prompt-type"]') as unknown as {
      vm: { $emit: (event: string, ...args: unknown[]) => void }
    }
    // 枚举筛选是即时生效的，不需要点搜索。
    await select.vm.$emit('update:modelValue', 'flow')
    await flushPromises()

    expect(wrapper.find('[data-test="list-state-filtered"]').exists()).toBe(false)
    const empty = wrapper.find('[data-test="list-state-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('暂无流程类提示词')
    expect(empty.text()).not.toContain('没有匹配')
  })
})
