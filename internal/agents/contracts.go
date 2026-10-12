// Package agents defines the backend-neutral contracts of the agent session
// workbench: backend identifiers, the capability declaration every adapter
// must make, the instance and session reference models, the normalized event
// union, and the AgentAdapter interface that the opencode and pi backends
// implement.
//
// The package is deliberately implementation-free. It is the vocabulary the
// registry, the lifecycle owner and the per-backend adapters share; nothing
// here talks to an agent process or a database. Every type is JSON-stable and
// free of persistence concerns: the platform keeps only indexes and ID
// mappings, while agent-native session data stays inside the agent.
package agents

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors reported by the contract validation helpers. Callers can
// test for them with errors.Is; implementations wrap them with context.
var (
	// ErrUnknownBackend reports a backend identifier the platform does not
	// know.
	ErrUnknownBackend = errors.New("agents: unknown backend")
	// ErrInvalidInstance reports an instance record that violates the
	// contract (empty identity, unknown backend, unknown status or missing
	// timestamps).
	ErrInvalidInstance = errors.New("agents: invalid instance")
	// ErrInvalidSessionRef reports a session reference that does not name a
	// backend, instance and agent-native session id.
	ErrInvalidSessionRef = errors.New("agents: invalid session reference")
	// ErrInvalidEvent reports a normalized event that violates the contract
	// (missing envelope field, unknown type or inconsistent payload).
	ErrInvalidEvent = errors.New("agents: invalid normalized event")
	// ErrRawEventTooLarge reports a backend JSON blob beyond the contract
	// size bound.
	ErrRawEventTooLarge = errors.New("agents: raw event exceeds size bound")
	// ErrInvalidHistoryRequest reports a history read that does not name both
	// a working directory and an agent-native session id.
	ErrInvalidHistoryRequest = errors.New("agents: invalid history request")
)

// BackendID identifies an agent backend supported by the workbench. The
// constants are the stable wire values shared by the registry, the adapters
// and the platform indexes.
type BackendID string

const (
	// BackendOpenCodeV2 is the opencode backend, driven through its v2 API.
	BackendOpenCodeV2 BackendID = "opencode-v2"
	// BackendPi is the pi backend, driven through its CLI.
	BackendPi BackendID = "pi"
)

// Valid reports whether the backend id is one the platform knows.
func (b BackendID) Valid() bool {
	switch b {
	case BackendOpenCodeV2, BackendPi:
		return true
	default:
		return false
	}
}

// Capabilities declares what a backend adapter actually supports. Every field
// is an explicit boolean so that a capability the backend lacks is visibly
// false: callers must never expose a control — a stop button, a model picker,
// a streaming view — for a capability the backend did not declare.
//
// The zero value declares nothing and is the correct starting point for a
// backend that must opt in to each feature.
type Capabilities struct {
	// Streaming reports whether assistant messages arrive incrementally
	// while they are being produced.
	Streaming bool `json:"streaming"`
	// SessionDiscovery reports whether existing agent-native sessions can be
	// listed (ListSessions).
	SessionDiscovery bool `json:"sessionDiscovery"`
	// SessionContinue reports whether an existing agent-native session can
	// be resumed (ContinueSession).
	SessionContinue bool `json:"sessionContinue"`
	// Stop reports whether a running instance can be stopped through the
	// adapter (StopInstance).
	Stop bool `json:"stop"`
	// ModelSelection reports whether a model can be chosen for sessions or
	// individual messages (CreateSessionOptions.Model and
	// SendMessageRequest.Model).
	ModelSelection bool `json:"modelSelection"`
	// ToolCallEvents reports whether the backend surfaces tool call and tool
	// result events on the event stream.
	ToolCallEvents bool `json:"toolCallEvents"`
	// ToolConfirmation reports whether the backend can pause a tool call and
	// wait for the user to confirm it.
	ToolConfirmation bool `json:"toolConfirmation"`
	// Interrupt reports whether the current turn of one session can be
	// interrupted through the adapter (Interrupt) without stopping the
	// instance or its other sessions.
	Interrupt bool `json:"interrupt"`
	// SessionHistory reports whether an existing agent-native session's prior
	// turns can be read (HistoryReader). A backend that does not declare it
	// must not be asked for history, and the workbench must not offer a
	// history view for it.
	SessionHistory bool `json:"sessionHistory"`
}

// ModelInfo is one model an agent can run, as the backend reports it. ID is
// the value SendMessageRequest.Model and CreateSessionOptions.Model accept:
// "provider/model", or "provider/model#variant" for a model variant.
type ModelInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Provider string   `json:"provider"`
	Variants []string `json:"variants,omitempty"`
}

// InstanceStatus is the lifecycle state of an adapter-managed instance.
type InstanceStatus string

const (
	// InstanceStarting marks an instance whose backend is being launched or
	// attached but is not usable yet.
	InstanceStarting InstanceStatus = "starting"
	// InstanceRunning marks a usable instance.
	InstanceRunning InstanceStatus = "running"
	// InstanceStopped marks an instance that ended cleanly, or that was
	// never started.
	InstanceStopped InstanceStatus = "stopped"
	// InstanceFailed marks an instance that ended or failed to start with an
	// error.
	InstanceFailed InstanceStatus = "failed"
)

// instanceTransitions lists the status changes the contract allows. Anything
// not reachable from here must go through an intermediate state: a stopped
// instance becomes running only by starting again, and a failed instance must
// be acknowledged as stopped before it can restart.
var instanceTransitions = map[InstanceStatus][]InstanceStatus{
	InstanceStarting: {InstanceRunning, InstanceFailed, InstanceStopped},
	InstanceRunning:  {InstanceStopped, InstanceFailed},
	InstanceStopped:  {InstanceStarting},
	InstanceFailed:   {InstanceStopped},
}

// Valid reports whether the status is one of the defined instance states.
func (s InstanceStatus) Valid() bool {
	_, ok := instanceTransitions[s]
	return ok
}

// CanTransitionTo reports whether moving from s to target is a valid
// instance lifecycle transition.
func (s InstanceStatus) CanTransitionTo(target InstanceStatus) bool {
	for _, next := range instanceTransitions[s] {
		if next == target {
			return true
		}
	}
	return false
}

// Instance is one adapter-managed agent backend occurrence: a running (or
// startable) opencode server session or pi CLI process, scoped to a working
// directory. The platform persists only the identity mapping; the record
// itself lives with the registry and the adapter that produced it.
type Instance struct {
	// ID is the platform-wide unique instance identifier.
	ID string `json:"id"`
	// Backend names the agent backend this instance runs.
	Backend BackendID `json:"backend"`
	// Directory is the working directory the agent operates in. It is an
	// already-authorized platform path; adapters must not re-derive it from
	// user input.
	Directory string `json:"directory"`
	// Status is the current lifecycle state.
	Status InstanceStatus `json:"status"`
	// Endpoint is the base URL of a server-based backend, when one exists.
	// CLI backends leave it empty.
	Endpoint string `json:"endpoint,omitempty"`
	// CreatedAt and UpdatedAt are the record's lifecycle timestamps.
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Validate reports whether the instance record satisfies the contract.
func (i Instance) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("%w: instance id is empty", ErrInvalidInstance)
	}
	if !i.Backend.Valid() {
		return fmt.Errorf("%w: unknown backend %q", ErrUnknownBackend, i.Backend)
	}
	if i.Directory == "" {
		return fmt.Errorf("%w: directory is empty", ErrInvalidInstance)
	}
	if !i.Status.Valid() {
		return fmt.Errorf("%w: unknown status %q", ErrInvalidInstance, i.Status)
	}
	if i.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created timestamp is zero", ErrInvalidInstance)
	}
	if i.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: updated timestamp is zero", ErrInvalidInstance)
	}
	return nil
}

// SessionRef names an agent-native session. It is the only thing the platform
// needs to keep about a conversation: which backend, which instance, and the
// agent's own session id. Title is agent-reported display data carried
// through for the UI — the platform persists just the ID mapping and never
// treats agent-native fields as its own state.
type SessionRef struct {
	// InstanceID is the instance the session belongs to.
	InstanceID string `json:"instanceId"`
	// Backend names the agent backend owning the session; it must match the
	// instance's backend.
	Backend BackendID `json:"backend"`
	// SessionID is the agent-native session identifier.
	SessionID string `json:"sessionId"`
	// Title is the agent-reported display title, when it has one.
	Title string `json:"title,omitempty"`
	// Preview is agent-derived fallback display text (the first user
	// message), not an explicit name: consumers must never let it
	// overwrite an explicit title.
	Preview string `json:"preview,omitempty"`
}

// Validate reports whether the session reference satisfies the contract.
func (s SessionRef) Validate() error {
	if s.InstanceID == "" {
		return fmt.Errorf("%w: instance id is empty", ErrInvalidSessionRef)
	}
	if !s.Backend.Valid() {
		return fmt.Errorf("%w: unknown backend %q", ErrUnknownBackend, s.Backend)
	}
	if s.SessionID == "" {
		return fmt.Errorf("%w: session id is empty", ErrInvalidSessionRef)
	}
	return nil
}

// ListHistoryRequest scopes a read of an agent-native session's prior turns.
// Directory is an already-authorized platform path and SessionID is the
// agent's own session identifier; the pair is what a transcript is stored
// under, so neither may be empty.
type ListHistoryRequest struct {
	// Directory is the working directory the agent operated in.
	Directory string `json:"directory"`
	// SessionID is the agent-native session identifier.
	SessionID string `json:"sessionId"`
}

// Validate reports whether the history request names both the directory and
// the session it reads.
func (r ListHistoryRequest) Validate() error {
	if r.Directory == "" {
		return fmt.Errorf("%w: directory is empty", ErrInvalidHistoryRequest)
	}
	if r.SessionID == "" {
		return fmt.Errorf("%w: session id is empty", ErrInvalidHistoryRequest)
	}
	return nil
}

// StartInstanceRequest asks an adapter to bring up one instance of its
// backend in a working directory.
type StartInstanceRequest struct {
	// Directory is the working directory for the agent. It must be an
	// already-authorized platform path.
	Directory string `json:"directory"`
}

// CreateSessionOptions carries the optional parameters of CreateSession.
// Fields a backend cannot honor must be rejected or ignored according to its
// declared Capabilities — never silently pretended to work.
type CreateSessionOptions struct {
	// Title is a display title for the new session, when the backend
	// supports naming sessions.
	Title string `json:"title,omitempty"`
	// Model selects the agent model, when the backend declared
	// Capabilities.ModelSelection.
	Model string `json:"model,omitempty"`
}

// SendMessageRequest is one user turn sent to an agent session. The adapter
// returns once the backend accepted the message; the assistant's reply, tool
// activity and errors arrive through the event stream.
type SendMessageRequest struct {
	// Text is the message body sent to the agent.
	Text string `json:"text"`
	// Model overrides the model for this message, when the backend declared
	// Capabilities.ModelSelection.
	Model string `json:"model,omitempty"`
}

// AgentAdapter is the contract every agent backend implements so the
// workbench can treat opencode and pi uniformly. Implementations own the
// backend-specific mechanics; they must not build shell command strings from
// user input, log secrets, or block indefinitely: every method honors the
// context, returning its cancellation or deadline error when the context is
// done.
type AgentAdapter interface {
	// Probe reports whether the backend is usable on this machine right now
	// (installed binary, reachable server). It must not start anything.
	Probe(ctx context.Context) error

	// StartInstance launches a new instance of the backend in the requested
	// directory and returns it in the InstanceStarting or InstanceRunning
	// state. Implementations report the endpoint for server-based backends.
	StartInstance(ctx context.Context, request StartInstanceRequest) (*Instance, error)

	// StopInstance terminates a running instance. Implementations whose
	// backend cannot stop an instance (Capabilities.Stop is false) must
	// reject the call instead of pretending it worked.
	StopInstance(ctx context.Context, instanceID string) error

	// ListSessions returns the agent-native sessions the backend knows for
	// the instance. Requires Capabilities.SessionDiscovery.
	ListSessions(ctx context.Context, instanceID string) ([]SessionRef, error)

	// CreateSession starts a new agent-native session on the instance and
	// returns its reference.
	CreateSession(ctx context.Context, instanceID string, options CreateSessionOptions) (*SessionRef, error)

	// ContinueSession re-attaches to an existing agent-native session so
	// subsequent messages extend it, and returns the refreshed reference.
	// Requires Capabilities.SessionContinue.
	ContinueSession(ctx context.Context, ref SessionRef) (*SessionRef, error)

	// SendMessage delivers one user turn to the session. Replies and tool
	// activity are observed through Subscribe, never through the return
	// value.
	SendMessage(ctx context.Context, ref SessionRef, message SendMessageRequest) error

	// Subscribe returns the instance's normalized event stream. Events on
	// the channel carry per-instance increasing Sequence values and are
	// delivered in sequence order. The adapter closes the channel when the
	// subscription ends — because the context is canceled, the instance is
	// gone, or the backend stream terminated — and any error terminates the
	// stream through an EventError event before closing.
	Subscribe(ctx context.Context, instanceID string) (<-chan NormalizedEvent, error)

	// ListModels returns the models the instance's backend can run. Requires
	// Capabilities.ModelSelection.
	ListModels(ctx context.Context, instanceID string) ([]ModelInfo, error)

	// Interrupt ends the current turn of one session and leaves the instance
	// and its other sessions running. Requires Capabilities.Interrupt.
	Interrupt(ctx context.Context, ref SessionRef) error
}

// HistoryResult is the outcome of a history read: the session's prior events,
// oldest first, and whether an older part of the transcript was left out. The
// reader that bounded the read reports it, because only it knows whether
// anything was dropped.
type HistoryResult struct {
	// Events are the replayed events, oldest first, bounded by
	// MaxHistoryEvents and renumbered from one.
	Events []NormalizedEvent
	// Truncated reports that older events exist beyond the bound.
	Truncated bool
}

// HistoryReader is the optional half of the adapter contract: an adapter
// implements it when Capabilities.SessionHistory is true, so the workbench can
// read an existing agent-native session's prior turns.
//
// It is deliberately separate from AgentAdapter and deliberately carries no
// instance id. History lives in the agent's own storage, so reading it needs
// only the working directory and the agent-native session id — which is also
// what lets a caller replay a session whose original instance is gone.
type HistoryReader interface {
	// ListHistory returns the session's prior turns, oldest first, bounded
	// by MaxHistoryEvents.
	//
	// The returned events carry Backend, Sequence, Timestamp and exactly one
	// typed payload; InstanceID and SessionID are left empty because the
	// request names neither. Callers that need a complete envelope (for
	// Validate) stamp the instance and session identity themselves.
	ListHistory(ctx context.Context, request ListHistoryRequest) (HistoryResult, error)
}
