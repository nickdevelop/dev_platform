// Colour helpers for catalog-declared brand colours. A backend descriptor may
// declare an exact #RRGGBB tag colour; the store filters anything else before it
// reaches an inline style. Kept provider-neutral: it knows nothing about which
// backend declared the colour.

const HEX_COLOR = /^#[0-9a-fA-F]{6}$/

// isHexColor reports whether value is the only colour syntax the catalog
// accepts. The guard matters because the value reaches an inline style.
export function isHexColor(value: string): boolean {
  return HEX_COLOR.test(value)
}
