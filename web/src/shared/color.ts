// Colour helpers for catalog-declared brand colours. A backend descriptor may
// declare an exact #RRGGBB tag colour; the tag renders it as a solid fill and
// this module picks a readable foreground for it. Kept provider-neutral: it
// knows nothing about which backend declared the colour.

const HEX_COLOR = /^#[0-9a-fA-F]{6}$/

// isHexColor reports whether value is the only colour syntax the catalog
// accepts. The guard matters because the value reaches an inline style.
export function isHexColor(value: string): boolean {
  return HEX_COLOR.test(value)
}

// relativeLuminance implements the WCAG 2.x formula for an sRGB colour.
function relativeLuminance(hex: string): number {
  const channels = [1, 3, 5].map((offset) => parseInt(hex.slice(offset, offset + 2), 16) / 255)
  const [r, g, b] = channels.map((channel) =>
    channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4,
  )
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

// contrastTextColor returns the foreground with the higher contrast ratio
// against the given fill. The luminance threshold is where black (L=0) and
// white (L=1) score equally: (L + 0.05) / 0.05 == 1.05 / (L + 0.05), so any
// fill above it takes the dark ink and anything below it takes white. An
// invalid value never reaches the tag (the store filters it), but returning
// white keeps this function total.
export function contrastTextColor(hex: string): string {
  if (!isHexColor(hex)) return '#ffffff'
  return relativeLuminance(hex) > 0.179 ? '#0b0d11' : '#ffffff'
}
