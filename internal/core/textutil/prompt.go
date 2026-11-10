package textutil

import (
	"html"
	"regexp"
	"strings"
)

// promptBlockPattern matches the platform's leading prompt block. The block is
// emitted by the new-session form as:
//
//	<Role name="name">
//	prompt body
//	</Role>
//
// followed by the user's own message. Only a leading block is recognised; a
// tag that appears later in the message is ordinary text. The trailing \s*
// consumes the separator between the block and the user's message so Rest is
// the user's own input.
var promptBlockPattern = regexp.MustCompile(`(?s)^\s*<(Role|Flow)\s+name="([^"]*)"\s*>(.*?)</(Role|Flow)>\s*`)

// PromptBlock is the parsed form of one leading Role/Flow prompt block.
type PromptBlock struct {
	// Kind is "role" or "flow" (lower case).
	Kind string
	// Name is the prompt's display name, with attribute escaping undone.
	Name string
	// Body is the prompt content between the tags.
	Body string
	// Rest is the user's own text after the block.
	Rest string
}

// ParsePromptBlock parses a leading Role/Flow prompt block. It reports false
// when the text does not start with a well-formed block, leaving the caller to
// treat the text as an ordinary message.
func ParsePromptBlock(text string) (PromptBlock, bool) {
	match := promptBlockPattern.FindStringSubmatch(text)
	if match == nil {
		return PromptBlock{}, false
	}
	openTag, closeTag := match[1], match[4]
	if !strings.EqualFold(openTag, closeTag) {
		return PromptBlock{}, false
	}
	return PromptBlock{
		Kind: strings.ToLower(openTag),
		Name: html.UnescapeString(match[2]),
		Body: strings.TrimSpace(match[3]),
		Rest: strings.TrimSpace(text[len(match[0]):]),
	}, true
}

// DeriveSessionTitle derives a session title from the message a user sent.
// When the message carries a leading prompt block, the title comes from the
// user's own input after the block — not from the prompt body, which would
// make every session using the same prompt share one title. A prompt-only
// message falls back to the prompt's name.
func DeriveSessionTitle(text string) string {
	if block, ok := ParsePromptBlock(text); ok {
		if title := DeriveTitle(block.Rest); title != "" {
			return title
		}
		return DeriveTitle(block.Name)
	}
	return DeriveTitle(text)
}
