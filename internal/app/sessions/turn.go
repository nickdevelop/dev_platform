package sessions

import "devplatform/internal/app/sessions/turns"

// TurnState reports whether a session is processing one user turn. The state
// lives in the turns subpackage; these aliases keep the session API spelling.
type TurnState = turns.State

const (
	// TurnIdle means the session is waiting for the next user message.
	TurnIdle = turns.Idle
	// TurnRunning means the agent is processing a turn of the session.
	TurnRunning = turns.Running
)
