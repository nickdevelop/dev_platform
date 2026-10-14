package policy

import "errors"

// ErrInvalidPath reports a malformed, relative, ambiguous, or unresolvable
// input path that cannot be canonicalized safely.
//
// It lives here rather than in internal/paths because the unix path policy
// wraps it while internal/paths delegates to the Dirs port: keeping the
// value in this dependency-free package avoids a platform -> unix -> paths
// -> platform import cycle. internal/paths aliases it, so existing
// references and the message text are unchanged.
var ErrInvalidPath = errors.New("paths: invalid path")
