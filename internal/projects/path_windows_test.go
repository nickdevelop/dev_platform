//go:build windows

package projects

import "testing"

// TestPathWithinIgnoresCase verifies containment is case-insensitive on
// Windows: a differently-cased child is still inside its parent, while
// siblings and name-prefix lookalikes are not.
func TestPathWithinIgnoresCase(t *testing.T) {
	if !pathWithin(`C:\work\proj`, `c:\WORK\PROJ\sub`) {
		t.Error(`pathWithin(C:\work\proj, c:\WORK\PROJ\sub) = false, want true`)
	}
	if pathWithin(`C:\work\proj`, `C:\work\other`) {
		t.Error(`pathWithin(C:\work\proj, C:\work\other) = true, want false`)
	}
	if pathWithin(`C:\work\proj`, `C:\work\proj-clone\sub`) {
		t.Error(`pathWithin prefix lookalike = true, want false`)
	}
	if pathWithin(`C:\work\proj`, `C:\work\proj`) {
		t.Error(`pathWithin(equal) = true, want strictly-inside only`)
	}
	if !pathWithin(`\\server\share`, `\\SERVER\share\dir`) {
		t.Error(`pathWithin(UNC, differently-cased child) = false, want true`)
	}
}
