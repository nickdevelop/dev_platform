package sessions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/agents"
	"devplatform/internal/agents/instances"
	"devplatform/internal/database"
)

// SessionStatus is the platform's view of a session's run state. It is derived
// from the owning instance's lifecycle plus an explicit platform stop; it is
// not an agent-native state.
type SessionStatus string

const (
	// SessionActive marks a session whose instance is starting or running.
	SessionActive SessionStatus = "active"
	// SessionStopped marks a session the user explicitly stopped through the
	// platform.
	SessionStopped SessionStatus = "stopped"
	// SessionDisconnected marks a session whose instance stopped, failed or
	// vanished without an explicit session stop. The agent-native session may
	// still be resumable once a capable instance is running again.
	SessionDisconnected SessionStatus = "disconnected"
)

// Valid reports whether the status is one of the defined session states.
func (s SessionStatus) Valid() bool {
	switch s {
	case SessionActive, SessionStopped, SessionDisconnected:
		return true
	default:
		return false
	}
}

// Service errors. Handlers map them to HTTP status codes and callers test them
// with errors.Is.
var (
	// ErrUnsupportedCapability reports an operation the backend did not
	// declare. Unsupported controls must never be presented as available.
	ErrUnsupportedCapability = errors.New("sessions: backend does not support this operation")
	// ErrBackendFailed reports an adapter operation that failed. The wrapped
	// cause is the adapter's (redacted) error.
	ErrBackendFailed = errors.New("sessions: backend operation failed")
	// ErrInvalidSession reports a backend that returned a native session
	// reference inconsistent with the instance it was requested on.
	ErrInvalidSession = errors.New("sessions: invalid native session reference")
	// ErrInvalidRequest reports a malformed request body.
	ErrInvalidRequest = errors.New("sessions: invalid request")
	// ErrAgentDisabled reports an attempt to create a session on a backend the
	// platform has disabled. Existing sessions and instances stay usable.
	ErrAgentDisabled = errors.New("sessions: agent is disabled platform-wide")
)

// errSlowClient reports an SSE client that could not keep up with the event
// stream. The stream is terminated rather than allowed to stall the instance.
var errSlowClient = errors.New("sessions: subscriber too slow to keep up")

// errEventEncode reports a normalized event that could not be encoded for the
// SSE frame. It is a per-event failure: the event is dropped and the stream
// continues, so one malformed payload cannot tear down a session's stream.
var errEventEncode = errors.New("sessions: encode event")

// defaultStreamBuffer bounds how many events one SSE client may fall behind by
// before it is disconnected.
const defaultStreamBuffer = 64

// heartbeatInterval is how often an idle SSE stream emits a comment frame. It
// keeps intermediaries and the browser connection alive when the agent is
// silent for a long time (no data frame would otherwise be sent).
const heartbeatInterval = 15 * time.Second

// Session is the JSON-stable API representation of a session index entry. The
// embedded capabilities tell the workbench which controls the backend actually
// supports, so unsupported controls can be hidden.
type Session struct {
	ID              string              `json:"id"`
	ProjectID       uint                `json:"projectId"`
	Backend         agents.BackendID    `json:"backend"`
	InstanceID      string              `json:"instanceId"`
	NativeSessionID string              `json:"nativeSessionId"`
	Title           string              `json:"title,omitempty"`
	Status          SessionStatus       `json:"status"`
	Turn            TurnState           `json:"turn"`
	Capabilities    agents.Capabilities `json:"capabilities"`
	CreatedAt       time.Time           `json:"createdAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
}

// BackendCapabilities pairs a registered backend with its declared
// capabilities, for the workbench's capability-aware controls.
type BackendCapabilities struct {
	Backend      agents.BackendID    `json:"backend"`
	Capabilities agents.Capabilities `json:"capabilities"`
}

// InstanceService is the subset of the instance lifecycle the session layer
// consumes. *instances.Service implements it; tests may substitute a fake.
type InstanceService interface {
	// Get returns the instance record with the given id.
	Get(instanceID string) (instances.Instance, error)
	// List returns a project's instances, so session reconciliation can index
	// them in one query instead of one Get per session row.
	List(projectID uint) ([]instances.Instance, error)
	// Start brings up a new instance of the backend in the project. It is the
	// session layer's auto-start path for creating a session on a backend with
	// no running instance.
	Start(ctx context.Context, projectID uint, backend agents.BackendID) (instances.Instance, error)
	// Subscribe returns the instance's normalized event stream.
	Subscribe(ctx context.Context, instanceID string) (<-chan agents.NormalizedEvent, error)
	// Stop terminates the instance (idempotently).
	Stop(ctx context.Context, instanceID string) (instances.Instance, error)
}

// capabilityReporter is the optional adapter method that publishes a backend's
// capability declaration. The frozen AgentAdapter interface does not include
// it, so the sessions layer asserts it rather than changing the contract.
type capabilityReporter interface {
	Capabilities() agents.Capabilities
}

// Service manages the platform's session index and the operations built on the
// adapter registry.
type Service struct {
	db        *gorm.DB
	repo      *repository
	registry  *agents.Registry
	instances InstanceService
	newID     func() (string, error)
	now       func() time.Time
	// platformEnabled reports whether a backend may be used platform-wide. Nil
	// means every backend is enabled.
	platformEnabled func(agents.BackendID) bool

	// streamBuffer bounds one SSE subscriber's backlog; tests may lower it.
	streamBuffer int

	// turnMu guards trackers and turns. turns maps instance id to native
	// session id to the session's current turn state; trackers records the
	// instances that already have a turn tracker.
	turnMu   sync.Mutex
	trackers map[string]bool
	turns    map[string]map[string]TurnState
}

// NewService builds the session service over the platform database, the
// adapter registry and the instance lifecycle service.
func NewService(db *gorm.DB, registry *agents.Registry, instanceService InstanceService, opts ...Option) *Service {
	s := &Service{
		db:           db,
		repo:         newRepository(db),
		registry:     registry,
		instances:    instanceService,
		newID:        randomID,
		now:          func() time.Time { return time.Now().UTC() },
		streamBuffer: defaultStreamBuffer,
		trackers:     make(map[string]bool),
		turns:        make(map[string]map[string]TurnState),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures a Service.
type Option func(*Service)

// WithPlatformEnabled supplies the platform-level agent enablement lookup. A
// nil lookup leaves every backend enabled (the zero-config default).
func WithPlatformEnabled(fn func(agents.BackendID) bool) Option {
	return func(s *Service) {
		if fn != nil {
			s.platformEnabled = fn
		}
	}
}

// agentDisabled reports whether the platform has disabled a backend.
func (s *Service) agentDisabled(backend agents.BackendID) bool {
	return s.platformEnabled != nil && !s.platformEnabled(backend)
}

// randomID returns a 128-bit random platform session identifier.
func randomID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Backends returns every registered backend with its declared capabilities.
func (s *Service) Backends() []BackendCapabilities {
	ids := s.registry.Backends()
	out := make([]BackendCapabilities, 0, len(ids))
	for _, id := range ids {
		out = append(out, BackendCapabilities{Backend: id, Capabilities: s.capabilities(id)})
	}
	return out
}

// Diagnostics collects the startup diagnostics of every registered backend:
// executable presence, CLI/API version support and API reachability, plus
// each backend's capability declaration. One backend failing to report never
// prevents the others' reports (see agents.CollectDiagnostics).
func (s *Service) Diagnostics(ctx context.Context) []agents.BackendDiagnostics {
	return agents.CollectDiagnostics(ctx, s.registry)
}

// List returns a project's indexed sessions, reconciled against the current
// instance lifecycle so a stale session is reported disconnected rather than
// as if it were still usable.
func (s *Service) List(projectID uint) ([]Session, error) {
	rows, err := s.repo.list(projectID)
	if err != nil {
		return nil, err
	}
	index, err := s.instanceIndex(projectID)
	if err != nil {
		// Preserve the previous best-effort behavior when the instance list
		// cannot be read: fall back to one lookup per row.
		out := make([]Session, 0, len(rows))
		for _, row := range rows {
			row = s.reconcile(row)
			out = append(out, s.sessionFromModel(row))
		}
		return out, nil
	}
	for _, inst := range index {
		if instanceRunnable(inst) {
			s.ensureTracker(inst.ID)
		}
	}
	out := make([]Session, 0, len(rows))
	for _, row := range rows {
		row = s.reconcileIndexed(row, index)
		out = append(out, s.sessionFromModel(row))
	}
	return out, nil
}

// instanceIndex returns a project's instances keyed by id, loaded in one query
// so a session list does not issue one Get per row (finding: reconcile N+1).
func (s *Service) instanceIndex(projectID uint) (map[string]instances.Instance, error) {
	list, err := s.instances.List(projectID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]instances.Instance, len(list))
	for _, inst := range list {
		index[inst.ID] = inst
	}
	return index, nil
}

// Get returns one session, refusing ids outside the authorized project.
func (s *Service) Get(projectID uint, sessionID string) (Session, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return Session{}, err
	}
	row = s.reconcile(row)
	return s.sessionFromModel(row), nil
}

// Delete removes one session from the platform's index. The agent-native
// transcript is the agent's and is never touched, so a later discovery of the
// same native session indexes it again; the delete only forgets it here.
func (s *Service) Delete(projectID uint, sessionID string) error {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return err
	}
	if err := s.repo.delete(row.ID); err != nil {
		return err
	}
	// The turn tracker is keyed by native session; drop the entry so a later
	// re-index of the same native session starts from a clean state.
	s.forgetTurn(row.InstanceID, row.NativeSessionID)
	return nil
}

// Create starts a new agent-native session for the project's backend. It
// reuses the project's most recently created running instance of that backend
// and starts one when none is running, then indexes the new session. A model
// is never chosen here: the workbench sends one with each message.
func (s *Service) Create(ctx context.Context, projectID uint, backend agents.BackendID, title string) (Session, error) {
	if !backend.Valid() {
		return Session{}, fmt.Errorf("%w: backend is required", ErrInvalidRequest)
	}
	if s.agentDisabled(backend) {
		return Session{}, fmt.Errorf("%w: %q", ErrAgentDisabled, backend)
	}
	inst, err := s.instanceForBackend(ctx, projectID, backend)
	if err != nil {
		return Session{}, err
	}
	if !instanceRunnable(inst) {
		return Session{}, fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	adapter, err := s.adapter(inst.Backend)
	if err != nil {
		return Session{}, err
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: title})
	if err != nil {
		return Session{}, fmt.Errorf("%w: create session: %w", ErrBackendFailed, err)
	}
	if ref == nil {
		return Session{}, fmt.Errorf("%w: adapter returned no session", ErrBackendFailed)
	}
	if err := validateRef(ref, inst); err != nil {
		return Session{}, err
	}
	s.ensureTracker(inst.ID)
	return s.indexRef(projectID, inst, *ref, SessionActive)
}

// instanceForBackend returns the project's most recently created running
// instance of the backend, starting one when none is running. The project's
// backend enable/disable configuration is enforced by Start, whose refusals and
// failures the caller surfaces unchanged.
func (s *Service) instanceForBackend(ctx context.Context, projectID uint, backend agents.BackendID) (instances.Instance, error) {
	list, err := s.instances.List(projectID)
	if err != nil {
		return instances.Instance{}, err
	}
	var chosen instances.Instance
	found := false
	for _, inst := range list {
		// List is in creation order, so the last match is the most recent.
		if inst.Backend == backend && inst.Status == agents.InstanceRunning {
			chosen = inst
			found = true
		}
	}
	if found {
		return chosen, nil
	}
	return s.instances.Start(ctx, projectID, backend)
}

// Discover lists an instance's agent-native sessions and indexes each valid one
// exactly once, returning the indexed references. Malformed, misattributed or
// unknown-project metadata is skipped so nothing is attached to the wrong
// project.
// DiscoverProject indexes the agent-native sessions of a project's backend,
// starting an instance when none is running. It is the project-level entry
// point for discovery: the caller names a backend, and which instance serves it
// is the platform's business rather than something the user has to pick.
func (s *Service) DiscoverProject(ctx context.Context, projectID uint, backend agents.BackendID) ([]Session, error) {
	if s.agentDisabled(backend) {
		return nil, fmt.Errorf("%w: %q", ErrAgentDisabled, backend)
	}
	if _, err := s.adapter(backend); err != nil {
		return nil, err
	}
	if !s.capabilities(backend).SessionDiscovery {
		return nil, fmt.Errorf("%w: session discovery", ErrUnsupportedCapability)
	}
	inst, err := s.instanceForBackend(ctx, projectID, backend)
	if err != nil {
		return nil, err
	}
	return s.Discover(ctx, projectID, inst.ID)
}

func (s *Service) Discover(ctx context.Context, projectID uint, instanceID string) ([]Session, error) {
	inst, err := s.scopedInstance(projectID, instanceID)
	if err != nil {
		return nil, err
	}
	if !instanceRunnable(inst) {
		return nil, fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	if !s.capabilities(inst.Backend).SessionDiscovery {
		return nil, fmt.Errorf("%w: session discovery", ErrUnsupportedCapability)
	}
	adapter, err := s.adapter(inst.Backend)
	if err != nil {
		return nil, err
	}
	refs, err := adapter.ListSessions(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: list sessions: %w", ErrBackendFailed, err)
	}
	out := make([]Session, 0, len(refs))
	for _, ref := range refs {
		if err := validateRef(&ref, inst); err != nil {
			// Native metadata that does not belong to this instance/backend
			// is never attached (and never to another project).
			continue
		}
		row, err := s.upsertRef(projectID, inst, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, s.sessionFromModel(row))
	}
	return out, nil
}

// Continue re-attaches to an indexed native session and makes it usable
// again. A session whose instance is gone — the state a platform restart
// leaves behind — is rebound onto a freshly started instance in the same
// project, because the user asked to continue the conversation, not to repair
// the platform. A backend without continue support is refused with a clear
// reason rather than presented as usable.
func (s *Service) Continue(ctx context.Context, projectID uint, sessionID string) (Session, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return Session{}, err
	}
	row = s.reconcile(row)

	backend := agents.BackendID(row.Backend)
	if !s.capabilities(backend).SessionContinue {
		return Session{}, fmt.Errorf("%w: session continue", ErrUnsupportedCapability)
	}

	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil && !errors.Is(err, instances.ErrInstanceNotFound) {
		return Session{}, err
	}
	if err != nil || !instanceRunnable(inst) {
		inst, err = s.instanceForBackend(ctx, projectID, backend)
		if err != nil {
			return Session{}, err
		}
		if err := s.rebind(projectID, &row, inst); err != nil {
			return Session{}, err
		}
	}

	adapter, err := s.adapter(backend)
	if err != nil {
		return Session{}, err
	}
	ref := agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    backend,
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}
	updated, err := adapter.ContinueSession(ctx, ref)
	if err != nil {
		return Session{}, fmt.Errorf("%w: continue session: %w", ErrBackendFailed, err)
	}
	if updated == nil {
		return Session{}, fmt.Errorf("%w: adapter returned no session", ErrBackendFailed)
	}
	if err := validateRef(updated, inst); err != nil {
		return Session{}, err
	}
	if updated.Title != "" {
		row.Title = updated.Title
	}
	row.Status = string(SessionActive)
	row.UpdatedAt = s.now()
	if err := s.repo.update(row); err != nil {
		return Session{}, err
	}
	s.ensureTracker(inst.ID)
	return s.sessionFromModel(row), nil
}

// rebind points a session row at another instance, merging away any other row
// that already indexes the same native session under that instance.
//
// The index's unique key includes the instance, so discovery can legitimately
// create a second row for one native session after a restart. The row the user
// has open wins: keeping it preserves the platform session id in the URL they
// are looking at, and the other row is deleted rather than left to appear as a
// duplicate task.
func (s *Service) rebind(projectID uint, row *database.AgentSession, inst instances.Instance) error {
	existing, err := s.repo.getByNative(inst.ID, row.NativeSessionID)
	switch {
	case err == nil && existing.ID == row.ID:
		// Already bound to this instance; nothing to merge.
	case err == nil:
		if existing.ProjectID != projectID {
			// Defensive: never absorb a row indexed for another project.
			return fmt.Errorf("%w: %s", ErrSessionNotFound, existing.ID)
		}
		// Keep whichever side knows more before dropping the duplicate.
		if row.Title == "" {
			row.Title = existing.Title
		}
		if !existing.CreatedAt.IsZero() && (row.CreatedAt.IsZero() || existing.CreatedAt.Before(row.CreatedAt)) {
			row.CreatedAt = existing.CreatedAt
		}
		if err := s.repo.delete(existing.ID); err != nil {
			return err
		}
	case errors.Is(err, ErrSessionNotFound):
		// No duplicate row to merge.
	default:
		return err
	}
	row.InstanceID = inst.ID
	row.UpdatedAt = s.now()
	return nil
}

// Send delivers one user turn to the session's native session.
// History is the JSON-stable API representation of a session's replayed
// prior turns. Events carry the same envelope as the live stream, stamped with
// the session's instance and native id, and Truncated reports that older
// events exist beyond the backend's read bound.
type History struct {
	Events    []agents.NormalizedEvent `json:"events"`
	Truncated bool                     `json:"truncated"`
}

// History reads a session's agent-native prior turns. History lives in the
// agent's own storage: the platform reads it on demand and keeps no copy, so
// this works for a session whose instance is no longer running.
func (s *Service) History(ctx context.Context, projectID uint, sessionID string) (History, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return History{}, err
	}
	backend := agents.BackendID(row.Backend)
	if !s.capabilities(backend).SessionHistory {
		return History{}, fmt.Errorf("%w: session history", ErrUnsupportedCapability)
	}
	adapter, err := s.adapter(backend)
	if err != nil {
		return History{}, err
	}
	reader, ok := adapter.(agents.HistoryReader)
	if !ok {
		// A backend that declares the capability but cannot serve it is a
		// platform misconfiguration, not a client error; it still must not
		// present as an empty history.
		return History{}, fmt.Errorf("%w: session history", ErrUnsupportedCapability)
	}
	// Instance rows are never deleted, so a stopped or failed instance still
	// carries the working directory the transcript is keyed under.
	inst, err := s.instances.Get(row.InstanceID)
	if err != nil {
		return History{}, err
	}

	result, err := reader.ListHistory(ctx, agents.ListHistoryRequest{
		Directory: inst.Directory,
		SessionID: row.NativeSessionID,
	})
	if err != nil {
		return History{}, fmt.Errorf("%w: read session history: %w", ErrBackendFailed, err)
	}

	events := make([]agents.NormalizedEvent, 0, len(result.Events))
	for _, event := range result.Events {
		event.InstanceID = row.InstanceID
		event.SessionID = row.NativeSessionID
		event.Backend = backend
		if err := event.Validate(); err != nil {
			return History{}, fmt.Errorf("%w: invalid replayed event: %w", ErrBackendFailed, err)
		}
		events = append(events, event)
	}
	return History{Events: events, Truncated: result.Truncated}, nil
}

func (s *Service) Send(ctx context.Context, projectID uint, sessionID, text, model string) (Session, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return Session{}, err
	}
	row = s.reconcile(row)
	if strings.TrimSpace(text) == "" {
		return Session{}, fmt.Errorf("%w: message text is required", ErrInvalidRequest)
	}
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return Session{}, err
	}
	if !instanceRunnable(inst) {
		return Session{}, fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	if model != "" && !s.capabilities(inst.Backend).ModelSelection {
		return Session{}, fmt.Errorf("%w: model selection", ErrUnsupportedCapability)
	}
	adapter, err := s.adapter(inst.Backend)
	if err != nil {
		return Session{}, err
	}
	ref := agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    agents.BackendID(row.Backend),
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}
	if err := adapter.SendMessage(ctx, ref, agents.SendMessageRequest{Text: text, Model: model}); err != nil {
		return Session{}, fmt.Errorf("%w: send message: %w", ErrBackendFailed, err)
	}
	row.Status = string(SessionActive)
	row.UpdatedAt = s.now()
	if err := s.repo.update(row); err != nil {
		return Session{}, err
	}
	s.ensureTracker(inst.ID)
	return s.sessionFromModel(row), nil
}

// ListModels returns the models the instance's backend offers. It is available
// only for backends that declare model selection, so an unsupported control is
// never presented as available.
func (s *Service) ListModels(ctx context.Context, projectID uint, instanceID string) ([]agents.ModelInfo, error) {
	inst, err := s.scopedInstance(projectID, instanceID)
	if err != nil {
		return nil, err
	}
	if !s.capabilities(inst.Backend).ModelSelection {
		return nil, fmt.Errorf("%w: model selection", ErrUnsupportedCapability)
	}
	adapter, err := s.adapter(inst.Backend)
	if err != nil {
		return nil, err
	}
	models, err := adapter.ListModels(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: list models: %w", ErrBackendFailed, err)
	}
	return models, nil
}

// Interrupt ends the session's current turn. It is idempotent: a session whose
// turn is not running is returned untouched without calling the backend. Only
// the session's own turn is interrupted; the instance and its other sessions
// are unaffected.
func (s *Service) Interrupt(ctx context.Context, projectID uint, sessionID string) (Session, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return Session{}, err
	}
	row = s.reconcile(row)
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return Session{}, err
	}
	if !s.capabilities(inst.Backend).Interrupt {
		return Session{}, fmt.Errorf("%w: interrupt", ErrUnsupportedCapability)
	}
	if s.turnOf(row.InstanceID, row.NativeSessionID) != TurnRunning {
		return s.sessionFromModel(row), nil
	}
	adapter, err := s.adapter(inst.Backend)
	if err != nil {
		return Session{}, err
	}
	ref := agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    agents.BackendID(row.Backend),
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}
	if err := adapter.Interrupt(ctx, ref); err != nil {
		return Session{}, fmt.Errorf("%w: interrupt session: %w", ErrBackendFailed, err)
	}
	return s.sessionFromModel(row), nil
}

// Stop stops the instance running the session. The instance stop is
// single-flight and idempotent, so concurrent stops call the adapter once and
// a second stop is a no-op. The target session is marked stopped and the
// instance's other sessions are marked disconnected.
//
// The adapter contract exposes only an instance-level stop, so stopping a
// session stops the instance's run; the workbench surfaces that semantics.
func (s *Service) Stop(ctx context.Context, projectID uint, sessionID string) (Session, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return Session{}, err
	}
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return Session{}, err
	}
	if !s.capabilities(inst.Backend).Stop {
		return Session{}, fmt.Errorf("%w: stop", ErrUnsupportedCapability)
	}

	stopped, stopErr := s.instances.Stop(ctx, row.InstanceID)

	now := s.now()
	target := SessionStopped
	if stopErr != nil || stopped.Status == agents.InstanceFailed {
		target = SessionDisconnected
	}
	// Persist the target first, then its siblings, so a reader never observes
	// a half-updated instance.
	if err := s.repo.updateStatus(row.ID, string(target), now); err != nil {
		return Session{}, err
	}
	row.Status = string(target)
	row.UpdatedAt = now
	s.markSiblingsDisconnected(row.InstanceID, row.ID, now)

	if stopErr != nil {
		// An unknown backend is a platform misconfiguration and stays a 400;
		// every other stop failure is a backend operation failure (502).
		if errors.Is(stopErr, instances.ErrUnknownBackend) {
			return s.sessionFromModel(row), stopErr
		}
		return s.sessionFromModel(row), fmt.Errorf("%w: stop instance: %w", ErrBackendFailed, stopErr)
	}
	return s.sessionFromModel(row), nil
}

// StreamSession writes the session's normalized events to w as Server-Sent
// Events until the stream ends, the caller's context is done, or the client
// falls too far behind. It bounds per-client buffering: a client that cannot
// keep up is disconnected and its subscription released, so a slow or stale
// subscriber can never stall the instance's shared event stream.
func (s *Service) StreamSession(ctx context.Context, projectID uint, sessionID string, w io.Writer, flush func()) error {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return err
	}
	if row.Status == string(SessionStopped) {
		return fmt.Errorf("%w: session is stopped", ErrInvalidRequest)
	}
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return err
	}
	if !instanceRunnable(inst) {
		return fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	if !s.capabilities(inst.Backend).Streaming {
		return fmt.Errorf("%w: streaming", ErrUnsupportedCapability)
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events, err := s.instances.Subscribe(subCtx, row.InstanceID)
	if err != nil {
		return err
	}

	queue := make(chan agents.NormalizedEvent, s.streamBuffer)
	var slow atomic.Bool
	go func() {
		defer close(queue)
		for {
			select {
			case <-subCtx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if !matchesSession(row, ev) {
					continue
				}
				if ev.SessionID != "" {
					ev.SessionID = row.ID
				}
				if err := ev.Validate(); err != nil {
					// Never forward a malformed normalized event.
					continue
				}
				select {
				case queue <- ev:
				case <-subCtx.Done():
					return
				default:
					// The client is not draining the queue: disconnect it
					// and release the instance subscription so the shared
					// stream is never held up.
					slow.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			// A comment frame keeps an idle stream alive without being a
			// data event; SSE clients ignore it.
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				cancel()
				return err
			}
			if flush != nil {
				flush()
			}
		case ev, ok := <-queue:
			if !ok {
				if slow.Load() {
					return errSlowClient
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				s.markDisconnected(row)
				return nil
			}
			if err := writeEvent(w, ev); err != nil {
				if errors.Is(err, errEventEncode) {
					// A single unencodable event (for example a backend raw
					// body that is not valid JSON) is dropped rather than
					// tearing down the stream.
					continue
				}
				cancel()
				return err
			}
			if flush != nil {
				flush()
			}
		}
	}
}

// writeEvent frames one normalized event as an SSE id/event/data block.
func writeEvent(w io.Writer, ev agents.NormalizedEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("%w: %v", errEventEncode, err)
	}
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Sequence, ev.Type, payload); err != nil {
		return fmt.Errorf("sessions: write event: %w", err)
	}
	return nil
}

// matchesSession reports whether an event belongs to the session: a
// session-scoped event for its native id, or an instance-scoped event (empty
// session id) that applies to every session of the instance.
func matchesSession(row database.AgentSession, ev agents.NormalizedEvent) bool {
	return ev.SessionID == "" || ev.SessionID == row.NativeSessionID
}

// markDisconnected marks an active session disconnected after its stream ends.
func (s *Service) markDisconnected(row database.AgentSession) {
	if row.Status != string(SessionActive) {
		return
	}
	if err := s.repo.updateStatus(row.ID, string(SessionDisconnected), s.now()); err != nil {
		return
	}
}

// markSiblingsDisconnected marks every other active session of an instance
// disconnected, without touching sessions that were already stopped.
func (s *Service) markSiblingsDisconnected(instanceID, exceptID string, at time.Time) {
	rows, err := s.repo.listByInstance(instanceID)
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.ID == exceptID || row.Status != string(SessionActive) {
			continue
		}
		_ = s.repo.updateStatus(row.ID, string(SessionDisconnected), at)
	}
}

// reconcile keeps an indexed session's status in step with its instance's
// lifecycle, persisting the change so listings and later reads stay
// consistent. An explicitly stopped session stays stopped; any other session
// becomes active when its instance is runnable and disconnected otherwise.
func (s *Service) reconcile(row database.AgentSession) database.AgentSession {
	if row.Status == string(SessionStopped) {
		return row
	}
	inst, err := s.instances.Get(row.InstanceID)
	if err != nil || inst.ProjectID != row.ProjectID || !instanceRunnable(inst) {
		return s.setStatus(row, SessionDisconnected)
	}
	return s.setStatus(row, SessionActive)
}

// reconcileIndexed is reconcile against an already-loaded instance index, so a
// listing reconciles every session without a per-row instance lookup.
func (s *Service) reconcileIndexed(row database.AgentSession, index map[string]instances.Instance) database.AgentSession {
	if row.Status == string(SessionStopped) {
		return row
	}
	inst, ok := index[row.InstanceID]
	if !ok || inst.ProjectID != row.ProjectID || !instanceRunnable(inst) {
		return s.setStatus(row, SessionDisconnected)
	}
	return s.setStatus(row, SessionActive)
}

// setStatus persists status when it differs and returns the updated row.
func (s *Service) setStatus(row database.AgentSession, status SessionStatus) database.AgentSession {
	if row.Status == string(status) {
		return row
	}
	now := s.now()
	if err := s.repo.updateStatus(row.ID, string(status), now); err != nil {
		// Best effort: the in-memory status is still returned so the caller
		// sees the reconciled state even if the write failed.
	}
	row.Status = string(status)
	row.UpdatedAt = now
	return row
}

// indexRef persists a new index row for a native session. A create that loses a
// concurrent race for the same (instance, native) pair is resolved
// idempotently: the already-indexed row is returned instead of surfacing a 500
// from the unique-index violation.
func (s *Service) indexRef(projectID uint, inst instances.Instance, ref agents.SessionRef, status SessionStatus) (Session, error) {
	row, err := s.newSessionRow(projectID, inst, ref, status)
	if err != nil {
		return Session{}, err
	}
	if err := s.repo.create(row); err != nil {
		if existing, getErr := s.repo.getByNative(inst.ID, ref.SessionID); getErr == nil {
			if existing.ProjectID != projectID {
				return Session{}, fmt.Errorf("%w: %s", ErrSessionNotFound, existing.ID)
			}
			return s.sessionFromModel(existing), nil
		}
		return Session{}, err
	}
	return s.sessionFromModel(row), nil
}

// upsertRef indexes a discovered native session, updating its title in place
// when it already has an index row.
func (s *Service) upsertRef(projectID uint, inst instances.Instance, ref agents.SessionRef) (database.AgentSession, error) {
	existing, err := s.repo.getByNative(inst.ID, ref.SessionID)
	if err == nil {
		if existing.ProjectID != projectID {
			// Defensive: never expose a session indexed for another project.
			return database.AgentSession{}, fmt.Errorf("%w: %s", ErrSessionNotFound, existing.ID)
		}
		if ref.Title != "" && ref.Title != existing.Title {
			existing.Title = ref.Title
			existing.UpdatedAt = s.now()
			if err := s.repo.update(existing); err != nil {
				return database.AgentSession{}, err
			}
		}
		return existing, nil
	}
	if !errors.Is(err, ErrSessionNotFound) {
		return database.AgentSession{}, err
	}
	row, err := s.newSessionRow(projectID, inst, ref, SessionActive)
	if err != nil {
		return database.AgentSession{}, err
	}
	if err := s.repo.create(row); err != nil {
		return database.AgentSession{}, err
	}
	return row, nil
}

// newSessionRow builds a fresh index row for a native session.
func (s *Service) newSessionRow(projectID uint, inst instances.Instance, ref agents.SessionRef, status SessionStatus) (database.AgentSession, error) {
	id, err := s.newID()
	if err != nil {
		return database.AgentSession{}, fmt.Errorf("sessions: allocate id: %w", err)
	}
	now := s.now()
	return database.AgentSession{
		ID:              id,
		ProjectID:       projectID,
		Backend:         string(inst.Backend),
		InstanceID:      inst.ID,
		NativeSessionID: ref.SessionID,
		Title:           ref.Title,
		Status:          string(status),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// sessionFromModel maps the persistence model to the JSON-stable DTO, carrying
// the backend's declared capabilities.
func (s *Service) sessionFromModel(row database.AgentSession) Session {
	backend := agents.BackendID(row.Backend)
	return Session{
		ID:              row.ID,
		ProjectID:       row.ProjectID,
		Backend:         backend,
		InstanceID:      row.InstanceID,
		NativeSessionID: row.NativeSessionID,
		Title:           row.Title,
		Status:          SessionStatus(row.Status),
		Turn:            s.turnOf(row.InstanceID, row.NativeSessionID),
		Capabilities:    s.capabilities(backend),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// loadScoped loads a session and enforces that it belongs to the project.
func (s *Service) loadScoped(projectID uint, sessionID string) (database.AgentSession, error) {
	row, err := s.repo.getByID(sessionID)
	if err != nil {
		return database.AgentSession{}, err
	}
	if row.ProjectID != projectID {
		return database.AgentSession{}, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	return row, nil
}

// scopedInstance loads an instance and enforces that it belongs to the project.
func (s *Service) scopedInstance(projectID uint, instanceID string) (instances.Instance, error) {
	inst, err := s.instances.Get(instanceID)
	if err != nil {
		return instances.Instance{}, err
	}
	if inst.ProjectID != projectID {
		return instances.Instance{}, fmt.Errorf("%w: %s", instances.ErrInstanceNotFound, instanceID)
	}
	return inst, nil
}

// adapter resolves a backend to its registered adapter.
func (s *Service) adapter(backend agents.BackendID) (agents.AgentAdapter, error) {
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return nil, fmt.Errorf("%w: %q", instances.ErrUnknownBackend, backend)
	}
	return adapter, nil
}

// capabilities returns a backend's declared capabilities, or the zero value
// when the adapter does not report any.
func (s *Service) capabilities(backend agents.BackendID) agents.Capabilities {
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return agents.Capabilities{}
	}
	if reporter, ok := adapter.(capabilityReporter); ok {
		return reporter.Capabilities()
	}
	return agents.Capabilities{}
}

// instanceRunnable reports whether an instance can accept session work.
func instanceRunnable(inst instances.Instance) bool {
	return inst.Status == agents.InstanceStarting || inst.Status == agents.InstanceRunning
}

// validateRef reports whether an adapter's native session reference is
// well-formed and consistent with the instance it was requested on.
func validateRef(ref *agents.SessionRef, inst instances.Instance) error {
	if ref == nil {
		return fmt.Errorf("%w: nil reference", ErrInvalidSession)
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSession, err)
	}
	if ref.InstanceID != inst.ID || ref.Backend != inst.Backend {
		return fmt.Errorf("%w: reference for %s/%s", ErrInvalidSession, ref.InstanceID, ref.Backend)
	}
	return nil
}
