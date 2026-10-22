// Package textutil holds small, provider-neutral text helpers shared by the
// platform's domains. It has no knowledge of agents, sessions or storage.
package textutil

import (
	"strings"
	"unicode/utf8"
)

// DefaultMaxTitleRunes bounds a derived title to one readable line.
const DefaultMaxTitleRunes = 60

// DeriveTitle turns free text into a single-line display title: whitespace
// runs collapse to single spaces and over-long titles are cut at
// DefaultMaxTitleRunes runes with a trailing ellipsis. Empty or
// whitespace-only text yields "".
func DeriveTitle(text string) string {
	return DeriveTitleLimit(text, DefaultMaxTitleRunes)
}

// DeriveTitleLimit is DeriveTitle with an explicit rune budget. A limit at or
// below zero falls back to DefaultMaxTitleRunes, so callers cannot
// accidentally produce an empty title by passing a zero value.
func DeriveTitleLimit(text string, limit int) string {
	if limit <= 0 {
		limit = DefaultMaxTitleRunes
	}
	collapsed := strings.Join(strings.Fields(text), " ")
	if collapsed == "" {
		return ""
	}
	if utf8.RuneCountInString(collapsed) <= limit {
		return collapsed
	}
	return string([]rune(collapsed)[:limit]) + "…"
}
