package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDeriveTitle(t *testing.T) {
	if got := DeriveTitle("fix the login\n\nbug\tplease"); got != "fix the login bug please" {
		t.Errorf("DeriveTitle(multi-line) = %q, want %q", got, "fix the login bug please")
	}
	if got := DeriveTitle(""); got != "" {
		t.Errorf(`DeriveTitle("") = %q, want ""`, got)
	}
	if got := DeriveTitle(" \n\t "); got != "" {
		t.Errorf("DeriveTitle(whitespace) = %q, want \"\"", got)
	}

	long := strings.Repeat("问", 100)
	got := DeriveTitle(long)
	if want := strings.Repeat("问", 60) + "…"; got != want {
		t.Errorf("DeriveTitle(100 CJK runes) = %q, want 60 runes + ellipsis", got)
	}
	if n := utf8.RuneCountInString(got); n != 61 {
		t.Errorf("DeriveTitle(100 CJK runes) has %d runes, want 61", n)
	}
}

func TestDeriveTitleLimit(t *testing.T) {
	got := DeriveTitleLimit("one two three", 7)
	if got != "one two…" {
		t.Errorf("DeriveTitleLimit(7) = %q, want %q", got, "one two…")
	}
	if got := DeriveTitleLimit("short", 0); got != "short" {
		t.Errorf("DeriveTitleLimit(0) = %q, want the default limit", got)
	}
}
