//go:build windows

package windows

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Terminal reads secrets from the Windows console without echoing, via
// x/term's console support.
type Terminal struct{}

// IsInteractive reports whether stdin is a console.
func (Terminal) IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ReadSecret prints prompt and reads a line without echoing. Piped or
// redirected input falls back to a plain stdin read (no echo to disable),
// so harnesses keep working where no console exists.
func (Terminal) ReadSecret(prompt string) (string, error) {
	if !(Terminal{}).IsInteractive() {
		fmt.Fprint(os.Stderr, prompt)
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && len(line) == 0 {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(secret), nil
}
