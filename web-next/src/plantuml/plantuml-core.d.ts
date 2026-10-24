// @plantuml/core ships plain JavaScript with no declarations. These are the two
// entry points the renderer uses, written down from the package README and
// verified against the built bundle: `viz-global.js` (the Graphviz layout) is
// loaded for its side effect first, then `plantuml.js` renders.
// See web/src/plantuml/renderer.ts.

declare module '@plantuml/core/viz-global.js' {}

declare module '@plantuml/core/plantuml.js' {
  export function renderToString(
    lines: string[],
    onSuccess: (svg: string) => void,
    onError: (message: unknown) => void,
    options?: { dark?: boolean; maxSvgSize?: number },
  ): void
  export function render(
    lines: string[],
    targetId: string,
    options?: { dark?: boolean; maxSvgSize?: number },
  ): void
}
