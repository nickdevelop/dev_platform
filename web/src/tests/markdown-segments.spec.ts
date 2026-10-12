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

  it('leaves a diagram fence shown as an example inside another language alone', () => {
    // Agents document PlantUML by quoting it: a diagram fence inside a
    // ```markdown example is text to read, not a diagram to draw.
    const nested = 'explain:\n````markdown\n```plantuml\nA -> B\n```\n````\ndone'
    expect(splitDiagramBlocks(nested)).toEqual([{ kind: 'markdown', text: nested }])

    // And an unterminated example holds whatever follows it, marked included.
    const unterminated = 'look\n```java\ncode\n```plantuml\nA -> B\n```'
    expect(splitDiagramBlocks(unterminated)).toEqual([{ kind: 'markdown', text: unterminated }])
  })

  it('finds fences in a reply that arrived with CRLF line endings', () => {
    expect(splitDiagramBlocks('看\r\n```plantuml\r\n@startuml\r\nA->B\r\n@enduml\r\n```\r\n好了')).toEqual([
      { kind: 'markdown', text: '看\n' },
      { kind: 'plantuml', text: '@startuml\nA->B\n@enduml', closed: true },
      { kind: 'markdown', text: '\n好了' },
    ])
  })

  it('does not read a sentence about fences as a fence', () => {
    // marked says a backtick fence cannot carry a backtick in its info string,
    // and an agent explaining the syntax writes exactly this sentence.
    const prose = '先说明:\n```puml 与 ```plantuml 是同一种语言\n正文 **粗体**\n'
    expect(splitDiagramBlocks(prose)).toEqual([{ kind: 'markdown', text: prose }])
  })

  it('finds fences in a reply that arrived with lone carriage returns', () => {
    expect(splitDiagramBlocks('看\r```plantuml\r@startuml\rA->B\r@enduml\r```\r好了')).toEqual([
      { kind: 'markdown', text: '看\n' },
      { kind: 'plantuml', text: '@startuml\nA->B\n@enduml', closed: true },
      { kind: 'markdown', text: '\n好了' },
    ])
  })

  it('leaves a diagram fence with nothing in it to markdown', () => {
    const blank = 'a\n```plantuml\n   \n```\nb'
    expect(splitDiagramBlocks(blank)).toEqual([{ kind: 'markdown', text: blank }])
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

    // CRLF is normalized to \n before cutting — marked does the same — so the
    // rule is unchanged by the line endings the reply arrived with.
    const crlf = source.replace(/\n/g, '\r\n')
    expect(splitDiagramBlocks(crlf).map((segment) => segment.text).join('')).toBe(withoutDiagramFences)
  })
})
