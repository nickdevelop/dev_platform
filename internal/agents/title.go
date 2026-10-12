package agents

import (
	"strings"
	"unicode/utf8"
)

// maxTitleRunes bounds a derived title to one readable line.
const maxTitleRunes = 60

// DeriveTitle turns free text (a first user message, a session preview)
// into a single-line display title: whitespace runs collapse to single
// spaces and over-long titles are cut at maxTitleRunes runes with a
// trailing ellipsis. Empty or whitespace-only text yields "".
//
// Backends do not always name their sessions themselves — pi only writes
// a name when the user explicitly sets one — so the platform derives a
// title rather than displaying a session id.
func DeriveTitle(text string) string {
	collapsed := strings.Join(strings.Fields(text), " ")
	if collapsed == "" {
		return ""
	}
	if utf8.RuneCountInString(collapsed) <= maxTitleRunes {
		return collapsed
	}
	return string([]rune(collapsed)[:maxTitleRunes]) + "…"
}
