package agents

import "devplatform/internal/core/textutil"

// DeriveTitle turns free text (a first user message, a session preview)
// into a single-line display title. The implementation lives in
// core/textutil so non-agent domains can derive titles without depending on
// the agent contract.
//
// Backends do not always name their sessions themselves — pi only writes
// a name when the user explicitly sets one — so the platform derives a
// title rather than displaying a session id.
func DeriveTitle(text string) string {
	return textutil.DeriveTitle(text)
}
