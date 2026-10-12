// The chat renderer needs to know which parts of an agent reply are PlantUML
// diagrams before marked ever sees them, so a diagram can become a component
// instead of a code block. These tests pin the fence grammar that decision
// depends on, including the streaming case where the fence never closes.
import { describe, expect, it } from 'vitest'
import { splitDiagramBlocks } from '../markdown-segments'

describe('splitDiagramBlocks', () => {
  it('cuts a closed plantuml fence out of the text', () => {
    expect(splitDiagramBlocks('前\n\n```plantuml\n@startuml\nA -> B\n@enduml\n```\n\n后')).toEqual([
      { kind: 'markdown', text: '前\n\n' },
      { kind: 'plantuml', text: '@startuml\nA -> B\n@enduml', closed: true },
      { kind: 'markdown', text: '\n\n后' },
    ])
  })

  it('treats an unterminated fence as an open diagram block', () => {
    expect(splitDiagramBlocks('```puml\n@startuml\nA -> B')).toEqual([
      { kind: 'plantuml', text: '@startuml\nA -> B', closed: false },
    ])
  })

  it('accepts a tilde fence and an uppercase language tag', () => {
    expect(splitDiagramBlocks('~~~PUML\n@startuml\nA -> B\n@enduml\n~~~')).toEqual([
      { kind: 'plantuml', text: '@startuml\nA -> B\n@enduml', closed: true },
    ])
  })

  it('leaves other languages in the markdown stream', () => {
    const source = '```go\nfmt.Println("hi")\n```'
    expect(splitDiagramBlocks(source)).toEqual([{ kind: 'markdown', text: source }])
  })

  it('does not end the diagram on a fence-looking line inside it', () => {
    const source = '```plantuml\n@startuml\nnote right: ```\nnote left: ~~~\n@enduml\n```\n尾巴'
    expect(splitDiagramBlocks(source)).toEqual([
      { kind: 'plantuml', text: '@startuml\nnote right: ```\nnote left: ~~~\n@enduml', closed: true },
      { kind: 'markdown', text: '\n尾巴' },
    ])
  })

  it('cuts every diagram out of one reply', () => {
    expect(splitDiagramBlocks('```plantuml\nA\n```\n中\n```puml\nB\n```')).toEqual([
      { kind: 'plantuml', text: 'A', closed: true },
      { kind: 'markdown', text: '\n中\n' },
      { kind: 'plantuml', text: 'B', closed: true },
    ])
  })

  it('returns the whole reply untouched when it holds no diagram', () => {
    const source = '# 标题\n\n正文\n'
    expect(splitDiagramBlocks(source)).toEqual([{ kind: 'markdown', text: source }])
  })

  it('returns nothing for an empty reply', () => {
    expect(splitDiagramBlocks('')).toEqual([])
  })

  it('loses nothing but the fence lines of diagram blocks', () => {
    const source = '前\n\n```plantuml\n@startuml\nA -> B\n@enduml\n```\n\n后\n```go\nx()\n```\n'
    const withoutDiagramFences = '前\n\n@startuml\nA -> B\n@enduml\n\n后\n```go\nx()\n```\n'
    expect(splitDiagramBlocks(source).map((segment) => segment.text).join('')).toBe(
      withoutDiagramFences,
    )
  })
})
