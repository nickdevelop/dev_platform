// A PlantUML file in the workspace preview: the drawing by default, the source one
// toggle away, a reason plus the source when the engine refuses it, and a file
// that is only the diagram body is wrapped in its markers before drawing.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import PlantumlArtifactView from '../features/files/PlantumlArtifactView.vue'
import FileViewer from '../features/files/FileViewer.vue'
import * as filesApi from '../api/files'
import { clearDiagramCache, configureEngineLoader, type Engine } from '../plantuml/renderer'

function fakeEngine(
  render: (lines: string[], opts: { dark: boolean }) => string | Promise<string>,
): Engine {
  return (lines, opts, onSuccess, onError) => {
    void Promise.resolve(render(lines, opts)).then(onSuccess, onError)
  }
}

describe('PlantumlArtifactView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    clearDiagramCache()
    configureEngineLoader(null)
  })

  it('defaults to source mode and draws when switched to preview with wrapped markers', async () => {
    const seen: string[][] = []
    configureEngineLoader(
      fakeEngine((lines) => {
        seen.push(lines)
        return '<svg><text>flow</text></svg>'
      }),
    )
    const wrapper = mount(PlantumlArtifactView, {
      props: { filePath: 'flow.puml', content: 'A -> B: hi' },
    })
    await flushPromises()

    expect(seen[0].join('\n')).toContain('@startuml\nA -> B: hi\n@enduml')
    expect(wrapper.find('[data-test="code-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="plantuml-artifact-canvas"]').exists()).toBe(false)

    await wrapper.findAll('.mode-btn')[0].trigger('click')
    expect(wrapper.find('[data-test="plantuml-artifact-canvas"] svg').exists()).toBe(true)
    expect(wrapper.find('[data-test="plantuml-artifact-error"]').exists()).toBe(false)
  })

  it('keeps a file that already has its markers as it is', async () => {
    const seen: string[][] = []
    configureEngineLoader(
      fakeEngine((lines) => {
        seen.push(lines)
        return '<svg><text>seq</text></svg>'
      }),
    )
    mount(PlantumlArtifactView, {
      props: { filePath: 'seq.plantuml', content: '@startuml\nA->B\n@enduml' },
    })
    await flushPromises()
    expect(seen[0].filter((line) => line.startsWith('@startuml'))).toHaveLength(1)
  })

  it('switches to the source view and back to the drawing', async () => {
    configureEngineLoader(fakeEngine(() => '<svg><text>seq</text></svg>'))
    const wrapper = mount(PlantumlArtifactView, {
      props: { filePath: 'seq.uml', content: '@startuml\nA->B\n@enduml' },
    })
    await flushPromises()

    const modeButtons = wrapper.findAll('.mode-btn')
    await modeButtons[1].trigger('click')
    expect(wrapper.find('[data-test="plantuml-artifact-canvas"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('A->B')

    await wrapper.findAll('.mode-btn')[0].trigger('click')
    expect(wrapper.find('[data-test="plantuml-artifact-canvas"] svg').exists()).toBe(true)
  })

  it('shows the reason and the source when the engine refuses the file', async () => {
    configureEngineLoader(fakeEngine(() => Promise.reject(new Error('syntax error'))))
    const wrapper = mount(PlantumlArtifactView, {
      props: { filePath: 'broken.puml', content: '@startuml\nA->\n@enduml' },
    })
    await flushPromises()

    expect(wrapper.find('[data-test="plantuml-artifact-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="plantuml-artifact-canvas"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('A->')
  })

  it('is chosen by the file preview for a plantuml extension', async () => {
    configureEngineLoader(fakeEngine(() => '<svg><text>routed</text></svg>'))
    vi.spyOn(filesApi, 'getFileContent').mockResolvedValue({
      path: 'docs/flow.puml',
      size: 20,
      lines: 3,
      mimeType: 'text/plain',
      isBinary: false,
      content: '@startuml\nA->B\n@enduml',
    })
    const wrapper = mount(FileViewer, {
      props: { projectId: 1, activePath: 'docs/flow.puml', hideTabs: true },
    })
    await flushPromises()

    expect(wrapper.findComponent(PlantumlArtifactView).exists()).toBe(true)
    expect(wrapper.find('[data-test="code-view"]').exists()).toBe(true)

    await wrapper.findAll('.mode-btn')[0].trigger('click')
    expect(wrapper.find('[data-test="plantuml-artifact-canvas"] svg').exists()).toBe(true)

    // Forward add-comment event from PlantumlArtifactView through FileViewer
    const plantumlView = wrapper.findComponent(PlantumlArtifactView)
    const card = {
      id: 'puml-card-1',
      filePath: 'docs/flow.puml',
      fileName: 'flow.puml',
      startLine: 2,
      endLine: 2,
      codeSnippet: 'A->B',
      comment: '修改流转路径',
      createdAt: Date.now(),
    }
    await plantumlView.vm.$emit('add-comment', card)
    expect(wrapper.emitted('add-comment')).toBeTruthy()
    expect(wrapper.emitted('add-comment')![0][0]).toEqual(card)
  })
})
