package database

import (
	"strings"
	"testing"
)

// TestBuildDSNEncodesUnixPath pins the Linux DSN byte-for-byte: the
// percent-encoded file: URI plus the per-connection pragmas.
func TestBuildDSNEncodesUnixPath(t *testing.T) {
	got := buildDSN("/home/u/.config/devplatform/platform.db")
	want := "file:///home/u/.config/devplatform/platform.db" +
		"?_pragma=foreign_keys%281%29&_pragma=busy_timeout%285000%29&_pragma=journal_mode%28WAL%29"
	if got != want {
		t.Fatalf("buildDSN(unix) = %q, want %q", got, want)
	}
}

// TestBuildDSNEncodesWindowsPath is the Windows startup regression test: a
// drive-letter path must form file:///C:/... (empty authority, forward
// slashes). The old shape file://C:%5C... parsed C: as the URI host and
// SQLite refused to open it with "SQL logic error: out of memory (1)".
func TestBuildDSNEncodesWindowsPath(t *testing.T) {
	got := buildDSN(`C:\Users\Administrator\AppData\Roaming\devplatform\platform.db`)
	want := "file:///C:/Users/Administrator/AppData/Roaming/devplatform/platform.db" +
		"?_pragma=foreign_keys%281%29&_pragma=busy_timeout%285000%29&_pragma=journal_mode%28WAL%29"
	if got != want {
		t.Fatalf("buildDSN(windows) = %q, want %q", got, want)
	}
	if strings.Contains(got, "file://C:") {
		t.Fatalf("buildDSN(windows) = %q, contains a URI authority", got)
	}
	if strings.Contains(got, "%5C") {
		t.Fatalf("buildDSN(windows) = %q, contains escaped backslashes", got)
	}
}

// TestBuildDSNEncodesWindowsSpecialChars verifies the anti-collision
// encoding survives the drive-letter branch: spaces and ? must not leak
// into the DSN query separator.
func TestBuildDSNEncodesWindowsSpecialChars(t *testing.T) {
	got := buildDSN(`C:\Users\Some User\we?ird\platform.db`)
	if !strings.Contains(got, "Some%20User") {
		t.Fatalf("buildDSN = %q, want the space escaped", got)
	}
	if !strings.Contains(got, "we%3Fird") {
		t.Fatalf("buildDSN = %q, want the ? escaped", got)
	}
	if strings.Contains(got, "\\") {
		t.Fatalf("buildDSN = %q, want no backslashes", got)
	}
}
