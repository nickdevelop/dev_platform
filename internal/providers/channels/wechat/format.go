package wechat

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var secretRedactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\b(aws_secret_access_key|secret_key)\s*[:=]\s*[^\s"']+`),
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]{12,}`),
	regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{12,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{16,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bssh-(?:rsa|ed25519|ecdsa)\s+[A-Za-z0-9+/=]{20,}`),
	regexp.MustCompile(`(?i)\b(?:api_key|apikey|bot_token|access_token)\s*[:=]\s*[^\s"']+`),
}

// SanitizeOutput redacts credential-like secrets before sending replies to WeChat.
func SanitizeOutput(text string) string {
	out := strings.TrimSpace(text)
	if out == "" {
		return ""
	}
	for _, re := range secretRedactionPatterns {
		out = re.ReplaceAllString(out, "[REDACTED]")
	}
	return out
}

// SplitMessage splits a long reply into rune-bounded chunks (default 2000)
// preferring paragraph, line, or space boundaries cleanly for CJK text.
func SplitMessage(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = defaultMaxReplyLength
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return []string{text}
	}

	var chunks []string
	remaining := text
	for utf8.RuneCountInString(remaining) > maxRunes {
		cutByteIdx := findSplitIndex(remaining, maxRunes)
		piece := strings.TrimSpace(remaining[:cutByteIdx])
		if piece != "" {
			chunks = append(chunks, piece)
		}
		remaining = strings.TrimSpace(remaining[cutByteIdx:])
	}
	if remaining != "" {
		chunks = append(chunks, remaining)
	}
	return chunks
}

func findSplitIndex(s string, maxRunes int) int {
	// Locate byte offset of the maxRunes-th rune.
	byteLimit := 0
	count := 0
	for i := range s {
		if count == maxRunes {
			byteLimit = i
			break
		}
		count++
	}
	if byteLimit == 0 {
		return len(s)
	}
	window := s[:byteLimit]
	minBoundary := byteLimit / 3

	for _, sep := range []string{"\n\n", "\n", " "} {
		if idx := strings.LastIndex(window, sep); idx >= minBoundary {
			return idx + len(sep)
		}
	}
	return byteLimit
}
