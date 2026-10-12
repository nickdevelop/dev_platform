package agents

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDeriveTitle(t *testing.T) {
	// Collapsed whitespace: a multi-line message becomes one line.
	if got := DeriveTitle("fix the login\n\nbug\tplease"); got != "fix the login bug please" {
		t.Errorf("DeriveTitle(multi-line) = %q, want %q", got, "fix the login bug please")
	}

	// Empty and whitespace-only input yields no title at all.
	if got := DeriveTitle(""); got != "" {
		t.Errorf(`DeriveTitle("") = %q, want ""`, got)
	}
	if got := DeriveTitle(" \n\t "); got != "" {
		t.Errorf("DeriveTitle(whitespace) = %q, want \"\"", got)
	}

	// CJK truncation counts runes, not bytes, and marks the cut.
	long := strings.Repeat("问", 100)
	got := DeriveTitle(long)
	if want := strings.Repeat("问", 60) + "…"; got != want {
		t.Errorf("DeriveTitle(100 CJK runes) = %q, want 60 runes + ellipsis", got)
	}
	if n := utf8.RuneCountInString(got); n != 61 {
		t.Errorf("DeriveTitle(100 CJK runes) has %d runes, want 61 (60 + ellipsis)", n)
	}

	// Exactly at the limit: no truncation, no ellipsis.
	exact := strings.Repeat("问", 60)
	if got := DeriveTitle(exact); got != exact {
		t.Errorf("DeriveTitle(60 runes) = %q, want it unchanged", got)
	}
}
