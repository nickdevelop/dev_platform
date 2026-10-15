//go:build unix

package unix

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// Terminal reads secrets from the unix console without echoing.
type Terminal struct{}

// IsInteractive reports whether stdin is a terminal.
func (Terminal) IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ReadSecret prints prompt and reads a line without echoing, followed by a
// newline so subsequent output starts on a fresh line.
func (Terminal) ReadSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(secret), nil
}
