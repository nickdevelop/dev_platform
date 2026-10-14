//go:build unix

package unix_test

import (
	"errors"
	"testing"

	"devplatform/internal/platform/policy"
	"devplatform/internal/platform/unix"
)

// TestTranslateWindowsPath pins the WSL translation table: drive paths map
// to /mnt/<drive> while ambiguous forms are rejected outright.
func TestTranslateWindowsPath(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`C:\work\project`, "/mnt/c/work/project"},
		{`c:/work/project`, "/mnt/c/work/project"},
		{`C:\`, "/mnt/c"},
		{`Z:\data`, "/mnt/z/data"},
		{"/mnt/c/work/project", "/mnt/c/work/project"},
		{"/opt/linux/path", "/opt/linux/path"},
	}
	for _, tc := range cases {
		got, err := unix.TranslateWindowsPath(tc.input)
		if err != nil {
			t.Errorf("TranslateWindowsPath(%q) failed: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("TranslateWindowsPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	for _, ambiguous := range []string{`C:relative`, `C:`, `\\server\share\proj`, `/mixed\separator`} {
		if _, err := unix.TranslateWindowsPath(ambiguous); !errors.Is(err, policy.ErrInvalidPath) {
			t.Errorf("TranslateWindowsPath(%q) error = %v, want ErrInvalidPath", ambiguous, err)
		}
	}
}
