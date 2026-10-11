package secrets

import "strings"

// Redact masks every exact occurrence of the provided secret values. It
// complements shape-based redaction (agents.RedactSecrets): when the platform
// knows the literal value — for example a stored MCP credential being echoed
// into a diagnostic — it can remove that exact string wherever it appears.
//
// Empty and whitespace-only values are ignored, so a missing secret never
// turns ordinary text into a wall of markers.
func Redact(text string, values ...string) string {
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) == "" {
			continue
		}
		text = strings.ReplaceAll(text, value, "[REDACTED]")
	}
	return text
}
