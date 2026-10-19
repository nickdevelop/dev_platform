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
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/instances"
	"devplatform/internal/app/sessions/store"
	"devplatform/internal/app/sessions/turns"
	"devplatform/internal/app/database"
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
	// ErrSessionNotFound reports an operation on a session that is not indexed
	// for the authorized project. It is the store's error, re-exported.
	ErrSessionNotFound = store.ErrNotFound
	// ErrServiceNotRunning reports a backend whose machine-level service is not
	// running, so no work can be done on it. It is the same condition the
	// backend layer reports — the session layer adds no new meaning, only the
	// decision to refuse rather than start something.
	ErrServiceNotRunning = agents.ErrServiceNotRunning
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

// resyncFrame is the SSE frame a resumed stream writes when the replay
// buffer cannot cover the client's gap: the client clears its log, reloads
// history and continues with the live events that follow. It carries no id
// because it belongs to no position in the sequence.
const resyncFrame = "event: resync\ndata: {}\n\n"

// turnSnapshotID identifies the synthetic turn frame a stream opens with. It is
// deliberately not a real turn id: the frame restates tracked state rather than
// reporting a boundary, so no consumer ever pairs it with a started/ended pair.
const turnSnapshotID = "stream-join"

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
	Model           string              `json:"model,omitempty"`
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
	// Enabled reports the platform-level enablement: a disabled backend cannot
	// be selected for new sessions. It is independent of per-project settings.
	Enabled bool `json:"enabled"`
	// Label is the backend's display name from the catalog; it falls back to
	// the raw id when the catalog does not describe the backend.
	Label string `json:"label"`
	// Order is the catalog's presentation order.
	Order int `json:"order"`
	// ServiceSupported reports whether the backend runs a machine-level
	// service the operator can start and stop.
	ServiceSupported bool `json:"serviceSupported"`
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
	// ReplaySince returns the instance's buffered events with Sequence past
	// the given one, oldest first, and whether the buffer covers the whole
	// gap. A false coverage answer means the requester must resynchronize
	// from history rather than trust a partial tail.
	ReplaySince(instanceID string, after uint64) ([]agents.NormalizedEvent, bool)
	// Stop terminates the instance (idempotently).
	Stop(ctx context.Context, instanceID string) (instances.Instance, error)
}

// StoreRepository defines the persistence contract for the session index.
// *store.Store satisfies it.
type StoreRepository interface {
	Create(row database.AgentSession) error
	GetByID(id string) (database.AgentSession, error)
	GetByNative(projectID uint, backend, nativeID string) (database.AgentSession, error)
	List(projectID uint) ([]database.AgentSession, error)
	Update(row database.AgentSession) error
	UpdateOnContinue(row database.AgentSession) error
	UpdateStatus(id, status string, at time.Time) error
	Delete(id string) error
}

// Service manages the platform's session index and the operations built on the
// adapter registry.
type Service struct {
	store     StoreRepository
	registry  *agents.Registry
	instances InstanceService
	newID     func() (string, error)
	now       func() time.Time
	// platformEnabled reports whether a backend may be used platform-wide. Nil
	// means every backend is enabled.
	platformEnabled func(agents.BackendID) bool
	// serviceGate refuses an instance start while a backend's machine-level
	// service is down. Nil means no backend needs a service.
	serviceGate ServiceGate

	// streamBuffer bounds one SSE subscriber's backlog; tests may lower it.
	streamBuffer int

	// catalog describes the backends' display facts. Nil leaves the labels as
	// raw ids and reports no service support.
	catalog *agents.Catalog

	// turnTracker records the turn state of each native session.
	turnTracker *turns.Tracker

	// eventBus manages multiplexed client event streams.
	eventBus *EventBusManager
}

// NewService builds the session service over the platform database, the
// adapter registry and the instance lifecycle service.
func NewService(db *gorm.DB, registry *agents.Registry, instanceService InstanceService, opts ...Option) *Service {
	return NewServiceWithStore(store.New(db), registry, instanceService, opts...)
}

// NewServiceWithStore builds the session service over a custom session index store.
func NewServiceWithStore(st StoreRepository, registry *agents.Registry, instanceService InstanceService, opts ...Option) *Service {
	s := &Service{
		store:        st,
		registry:     registry,
		instances:    instanceService,
		newID:        randomID,
		now:          func() time.Time { return time.Now().UTC() },
		streamBuffer: defaultStreamBuffer,
		turnTracker:  turns.NewTracker(instanceService),
	}
	s.eventBus = NewEventBusManager(s)
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures a Service.
type Option func(*Service)

// WithCatalog supplies the backend catalog that provides display labels,
// presentation order and service support for the backend list.
func WithCatalog(catalog *agents.Catalog) Option {
	return func(s *Service) {
		s.catalog = catalog
	}
}

// ServiceGate reports whether a backend's machine-level service is available.
// agentservices.Service implements it.
type ServiceGate interface {
	// Available returns nil when the backend can start an instance. A backend
	// whose service is down is refused, and the session layer passes the
	// refusal on instead of bringing the service up itself.
	Available(backend agents.BackendID) error
}

// WithServiceGate supplies the machine-level service gate consulted before an
// instance is started.
func WithServiceGate(gate ServiceGate) Option {
	return func(s *Service) {
		s.serviceGate = gate
	}
}

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
		entry := BackendCapabilities{Backend: id, Capabilities: s.capabilities(id), Enabled: !s.agentDisabled(id), Label: string(id)}
		if descriptor, known := s.catalog.Get(id); known {
			entry.Label = descriptor.Label
			entry.Order = descriptor.Order
			entry.ServiceSupported = descriptor.ServiceSupported
		}
		out = append(out, entry)
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
	rows, err := s.store.List(projectID)
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
			s.turnTracker.Ensure(inst.ID)
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
	if err := s.store.Delete(row.ID); err != nil {
		return err
	}
	// The turn tracker is keyed by native session; drop the entry so a later
	// re-index of the same native session starts from a clean state.
	s.turnTracker.Forget(agents.BackendID(row.Backend), row.NativeSessionID)
	return nil
}

// CreateOutcome reports the result of creating a session with an optional
// first message. A failed first message leaves the session usable — the
// MessageError carries the failure code and the user sends again from the
// composer — so the create itself still succeeds.
type CreateOutcome struct {
	Session Session
	// MessageError is the error code of a failed first message; empty when
	// the message was admitted or none was requested.
	MessageError string
}

// Create starts a new agent-native session for the project's backend. It
// reuses the project's most recently created running instance of that backend
// and starts one when none is running, then indexes the new session. When a
// model is supplied it is chosen at creation — backends create the session on
// that model directly, so no switch call is needed — independent of whether a
// first message rides along; a supplied message is admitted right after: the
// turn runs while the workbench opens the session. The message failing to be
// admitted does not fail the create — the session is already live and the caller
// surfaces MessageError instead.
func (s *Service) Create(ctx context.Context, projectID uint, backend agents.BackendID, title, message, model string, images ...[]agents.ImageAttachment) (CreateOutcome, error) {
	if !backend.Valid() {
		return CreateOutcome{}, fmt.Errorf("%w: backend is required", ErrInvalidRequest)
	}
	if s.agentDisabled(backend) {
		return CreateOutcome{}, fmt.Errorf("%w: %q", ErrAgentDisabled, backend)
	}
	inst, err := s.instanceForBackend(ctx, projectID, backend)
	if err != nil {
		return CreateOutcome{}, err
	}
	if !instanceRunnable(inst) {
		return CreateOutcome{}, fmt.Errorf("%w: %s is %s", instances.ErrInstanceNotRunning, inst.ID, inst.Status)
	}
	if model != "" && !s.capabilities(inst.Backend).ModelSelection {
		return CreateOutcome{}, fmt.Errorf("%w: model selection", ErrUnsupportedCapability)
	}
	adapter, err := s.adapter(inst.Backend)
	if err != nil {
		return CreateOutcome{}, err
	}
	// Backends do not name new sessions themselves (pi only records a name
	// when one is given), so an untitled first message names the session the
	// same way a first composer message would.
	message = strings.TrimSpace(message)
	if title == "" && message != "" {
		title = agents.DeriveTitle(message)
	}
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{Title: title, Model: model})
	if err != nil {
		return CreateOutcome{}, fmt.Errorf("%w: create session: %w", ErrBackendFailed, err)
	}
	if ref == nil {
		return CreateOutcome{}, fmt.Errorf("%w: adapter returned no session", ErrBackendFailed)
	}
	if err := validateRef(ref, inst); err != nil {
		return CreateOutcome{}, err
	}
	// The tracker subscribes before the message is admitted so the turn's
	// first lifecycle events are never ahead of the subscription.
	s.turnTracker.Ensure(inst.ID)
	row, err := s.indexRef(projectID, inst, *ref, SessionActive)
	if err != nil {
		return CreateOutcome{}, err
	}
	if model != "" {
		if dbRow, getErr := s.store.GetByID(row.ID); getErr == nil {
			dbRow.Model = model
			_ = s.store.Update(dbRow)
			row.Model = model
		}
	}
	outcome := CreateOutcome{Session: row}
	var imgs []agents.ImageAttachment
	if len(images) > 0 {
		imgs = images[0]
	}
	if message != "" || len(imgs) > 0 {
		if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: message, Images: imgs}); err != nil {
			// The session stays; the workbench pre-fills the composer and
			// reports the failure so the user can send again.
			outcome.MessageError = "agent_operation_failed"
		}
	}
	return outcome, nil
}

// instanceForBackend returns the project's most recently created running
// instance of the backend, starting one when none is running. The project's
// backend enable/disable configuration is enforced by Start, whose refusals and
// failures the caller surfaces unchanged. When no instance runs and the backend's
// machine-level service is down, the call is refused with ErrServiceNotRunning:
// starting the service is an operator's decision, not this request's to make.
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
	// Starting an instance of a service-backed backend would need that service.
	// The platform never starts one itself, so the request is refused with the
	// reason instead of quietly launching a process or failing opaquely.
	if s.serviceGate != nil {
		if err := s.serviceGate.Available(backend); err != nil {
			return instances.Instance{}, fmt.Errorf("%w: %q (%w)", ErrServiceNotRunning, backend, err)
		}
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
	if s.agentDisabled(backend) {
		return Session{}, fmt.Errorf("%w: %q", ErrAgentDisabled, backend)
	}
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
		row.InstanceID = inst.ID
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
	// Opening a session is not an interaction: the re-attach must not refresh the
	// row's updated_at, the last-interaction time the session list sorts by.
	if err := s.store.UpdateOnContinue(row); err != nil {
		return Session{}, err
	}
	s.turnTracker.Ensure(inst.ID)
	s.alignTurnState(ctx, backend, ref)
	return s.sessionFromModel(row), nil
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

func (s *Service) Send(ctx context.Context, projectID uint, sessionID, text, model string, images ...[]agents.ImageAttachment) (Session, error) {
	var attachedImages []agents.ImageAttachment
	if len(images) > 0 {
		attachedImages = images[0]
	}
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return Session{}, err
	}
	row = s.reconcile(row)
	if s.agentDisabled(agents.BackendID(row.Backend)) {
		return Session{}, fmt.Errorf("%w: %q", ErrAgentDisabled, row.Backend)
	}
	if strings.TrimSpace(text) == "" && len(attachedImages) == 0 {
		return Session{}, fmt.Errorf("%w: message text or images required", ErrInvalidRequest)
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
	// Backends do not name new sessions themselves (pi only records a name
	// when one is given), so the first message becomes the title while it
	// travels down: the adapter carries it to the backend for persistence.
	if row.Title == "" {
		if strings.TrimSpace(text) != "" {
			row.Title = agents.DeriveTitle(text)
		} else if len(attachedImages) > 0 {
			row.Title = "图片分析"
		}
	}
	ref := agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    agents.BackendID(row.Backend),
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}
	if err := adapter.SendMessage(ctx, ref, agents.SendMessageRequest{Text: text, Model: model, Images: attachedImages}); err != nil {
		return Session{}, fmt.Errorf("%w: send message: %w", ErrBackendFailed, err)
	}
	if model != "" {
		row.Model = model
	}
	row.Status = string(SessionActive)
	row.UpdatedAt = s.now()
	if err := s.store.Update(row); err != nil {
		return Session{}, err
	}
	s.turnTracker.Ensure(inst.ID)
	return s.sessionFromModel(row), nil
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
	if s.turnTracker.Of(agents.BackendID(row.Backend), row.NativeSessionID) != TurnRunning {
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

// Stop ends the session's own run and marks the session stopped. It is
// session-scoped: the instance keeps running and its other sessions are not
// touched, because each session has its own native run and cancellation. A
// session with a run in flight is interrupted through the adapter first; if
// that fails the session is left as it was. Stopping an instance is a separate
// operation on the instance lifecycle.
func (s *Service) Stop(ctx context.Context, projectID uint, sessionID string) (Session, error) {
	row, err := s.loadScoped(projectID, sessionID)
	if err != nil {
		return Session{}, err
	}
	inst, err := s.scopedInstance(projectID, row.InstanceID)
	if err != nil {
		return Session{}, err
	}
	if !s.capabilities(inst.Backend).Interrupt {
		return Session{}, fmt.Errorf("%w: stop", ErrUnsupportedCapability)
	}
	if s.turnTracker.Of(agents.BackendID(row.Backend), row.NativeSessionID) == TurnRunning && instanceRunnable(inst) {
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
			return Session{}, fmt.Errorf("%w: stop session: %w", ErrBackendFailed, err)
		}
	}
	now := s.now()
	if err := s.store.UpdateStatus(row.ID, string(SessionStopped), now); err != nil {
		return Session{}, err
	}
	row.Status = string(SessionStopped)
	row.UpdatedAt = now
	return s.sessionFromModel(row), nil
}

// StreamSession writes the session's normalized events to w as Server-Sent
// Events until the stream ends, the caller's context is done, or the client
// falls too far behind. It bounds per-client buffering: a client that cannot
// keep up is disconnected and its subscription released, so a slow or stale
// subscriber can never stall the instance's shared event stream.
//
// resumeFrom is the sequence the client saw last (the SSE Last-Event-ID):
// when the instance's replay buffer covers the gap, the events the client
// missed are replayed before the live ones; when it does not, the stream
// first emits a resync frame so the client can rebuild its log from history.
// A zero resumeFrom means the client is joining, not resuming.
func (s *Service) StreamSession(ctx context.Context, projectID uint, sessionID string, resumeFrom uint64, w io.Writer, flush func()) error {
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

	// Replaying from the buffer after the live subscription is established is
	// what makes the resume gapless: an event published concurrently with this
	// stream is either already buffered (and written below) or arrives on the
	// channel, and the watermark drops the ones that land in both.
	watermark := uint64(0)
	if resumeFrom > 0 {
		replay, covered := s.instances.ReplaySince(row.InstanceID, resumeFrom)
		if !covered {
			// The client's position was evicted or predates the current
			// stream: a partial tail would render as a silently truncated
			// transcript, so the client rebuilds from history instead, and
			// live events flow unfiltered from here on.
			if _, err := io.WriteString(w, resyncFrame); err != nil {
				cancel()
				return err
			}
			if flush != nil {
				flush()
			}
		} else {
			watermark = resumeFrom
			for _, ev := range replay {
				if !matchesSession(row, ev) {
					continue
				}
				if ev.SessionID != "" {
					ev.SessionID = row.ID
				}
				if err := ev.Validate(); err != nil {
					// A buffered event cannot be newer than its live
					// original, but it went through the same gate then, so
					// a malformed one never belonged on the wire either.
					continue
				}
				if err := writeEvent(w, ev); err != nil {
					if errors.Is(err, errEventEncode) {
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

	// The snapshot is written after the replay and before the live queue
	// drains, so it is the authoritative pending set at the join point: a
	// request that settled inside the replayed gap is already gone from it.
	ref := agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    agents.BackendID(row.Backend),
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}
	if err := s.writeInteractionSnapshot(subCtx, ref, w, flush); err != nil {
		cancel()
		return err
	}
	s.alignTurnState(subCtx, agents.BackendID(row.Backend), ref)

	// The stream opens with the session's turn state, seeded as the queue's first
	// item. A turn boundary is published on the instance stream while the agent
	// runs, so a client that subscribes after the turn ended would never see the
	// closing frame: the state it last read (the REST DTO at open time) would
	// keep saying "running" with nothing left to correct it. Restating the
	// tracked state here gives the joining client the same answer the DTO would
	// report now, in stream order. It goes through the queue rather than being
	// written here so a wedged client meets the same policy as any other event.
	type streamItem struct {
		event agents.NormalizedEvent
		// join marks the state frame a stream opens with: it belongs to no
		// position in the sequence, so it is written without an SSE id.
		join bool
	}
	queue := make(chan streamItem, s.streamBuffer)
	queue <- streamItem{event: s.turnSnapshotEvent(row), join: true}
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
				if watermark > 0 && ev.Sequence <= watermark {
					// Already written from the replay buffer.
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
				case queue <- streamItem{event: ev}:
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
		case item, ok := <-queue:
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
			var err error
			if item.join {
				err = writeFrame(w, item.event)
			} else {
				err = writeEvent(w, item.event)
			}
			if err != nil {
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

// writeFrame writes an event as an SSE event/data block with no id line: the
// frame belongs to no position in the event sequence. The frames written at a
// stream's join point use it, so a client never adopts a resume position it did
// not actually reach.
func writeFrame(w io.Writer, ev agents.NormalizedEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("%w: %v", errEventEncode, err)
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
		return fmt.Errorf("sessions: write frame: %w", err)
	}
	return nil
}

// turnSnapshotEvent is the session's turn state at a join point, framed as a
// turn boundary so the client's existing turn handling consumes it: "started"
// while the agent is working, "ended" once it is not. The ending's outcome is
// not knowable here and is not read from this frame, but a valid turn payload
// names one.
func (s *Service) turnSnapshotEvent(row database.AgentSession) agents.NormalizedEvent {
	phase := agents.TurnEnded
	outcome := agents.TurnCompleted
	if s.turnTracker.Of(agents.BackendID(row.Backend), row.NativeSessionID) == TurnRunning {
		phase = agents.TurnStarted
		outcome = ""
	}
	return agents.NormalizedEvent{
		InstanceID: row.InstanceID,
		SessionID:  row.ID,
		Backend:    agents.BackendID(row.Backend),
		Timestamp:  s.now(),
		Type:       agents.EventTurn,
		Turn: &agents.TurnPayload{
			ID:      turnSnapshotID,
			Phase:   phase,
			Outcome: outcome,
		},
	}
}

// alignTurnState ensures the tracker aligns with the live agent backend state when opened.
func (s *Service) alignTurnState(ctx context.Context, backend agents.BackendID, ref agents.SessionRef) {
	adapter, err := s.adapter(backend)
	if err != nil {
		return
	}
	if detector, ok := adapter.(agents.SessionTurnDetector); ok {
		if running, err := detector.DetectTurn(ctx, ref); err == nil && running {
			s.turnTracker.Set(backend, ref.SessionID, TurnRunning)
		}
	}
}


// matchesSession reports whether an event belongs to the session: a
// session-scoped event for its native id, or an instance-scoped event (empty
// session id) that applies to every session of the instance. Strict session-scoped
// event types (interactions, messages, turns, tool activity) are never broadcast
// to other sessions even if their session id happens to be empty.
func matchesSession(row database.AgentSession, ev agents.NormalizedEvent) bool {
	if isSessionStrictEvent(ev.Type) {
		return ev.SessionID == row.NativeSessionID
	}
	return ev.SessionID == "" || ev.SessionID == row.NativeSessionID
}

func isSessionStrictEvent(eventType agents.EventType) bool {
	switch eventType {
	case agents.EventInteraction, agents.EventTurn, agents.EventMessage,
		agents.EventThinking, agents.EventToolCall, agents.EventToolResult:
		return true
	default:
		return false
	}
}

// markDisconnected marks an active session disconnected after its stream ends.
func (s *Service) markDisconnected(row database.AgentSession) {
	if row.Status != string(SessionActive) {
		return
	}
	if err := s.store.UpdateStatus(row.ID, string(SessionDisconnected), s.now()); err != nil {
		return
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
	if err := s.store.UpdateStatus(row.ID, string(status), now); err != nil {
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
	if err := s.store.Create(row); err != nil {
		if existing, getErr := s.store.GetByNative(projectID, string(inst.Backend), ref.SessionID); getErr == nil {
			return s.sessionFromModel(existing), nil
		}
		return Session{}, err
	}
	return s.sessionFromModel(row), nil
}

// upsertRef indexes a discovered native session, updating its title in place
// when it already has an index row. Title precedence: an explicit agent name
// wins over whatever the platform holds; a preview never overwrites a title,
// it only fills an empty one.
func (s *Service) upsertRef(projectID uint, inst instances.Instance, ref agents.SessionRef) (database.AgentSession, error) {
	existing, err := s.store.GetByNative(projectID, string(inst.Backend), ref.SessionID)
	if err == nil {
		if existing.InstanceID != inst.ID {
			existing.InstanceID = inst.ID
			existing.Status = string(SessionActive)
		}
		if ref.Title != "" && ref.Title != existing.Title {
			existing.Title = ref.Title
			existing.UpdatedAt = s.now()
		} else if existing.Title == "" && ref.Preview != "" {
			existing.Title = ref.Preview
			existing.UpdatedAt = s.now()
		}
		if err := s.store.Update(existing); err != nil {
			return database.AgentSession{}, err
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
	if err := s.store.Create(row); err != nil {
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
	title := ref.Title
	if title == "" {
		// A discovered session pi never named still deserves its first
		// message as a display title.
		title = ref.Preview
	}
	now := s.now()
	return database.AgentSession{
		ID:              id,
		ProjectID:       projectID,
		Backend:         string(inst.Backend),
		InstanceID:      inst.ID,
		NativeSessionID: ref.SessionID,
		Title:           title,
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
		Model:           row.Model,
		Status:          SessionStatus(row.Status),
		Turn:            s.turnTracker.Of(backend, row.NativeSessionID),
		Capabilities:    s.capabilities(backend),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// loadScoped loads a session and enforces that it belongs to the project.
func (s *Service) loadScoped(projectID uint, sessionID string) (database.AgentSession, error) {
	row, err := s.store.GetByID(sessionID)
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
	return adapter.Capabilities()
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

// ListCommands returns the commands and skills supported by the session's agent backend.
// If the backend adapter does not implement agents.CommandDiscoverer, it returns an empty list.
func (s *Service) ListCommands(ctx context.Context, projectID uint, sessionID string) ([]agents.AgentCommand, error) {
	row, err := s.store.GetByID(sessionID)
	if err != nil {
		return nil, err
	}
	if row.ProjectID != projectID {
		return nil, ErrSessionNotFound
	}

	backendID := agents.BackendID(row.Backend)
	adapter, err := s.adapter(backendID)
	if err != nil {
		return nil, err
	}

	discoverer, ok := adapter.(agents.CommandDiscoverer)
	if !ok {
		return []agents.AgentCommand{}, nil
	}

	ref := agents.SessionRef{
		InstanceID: row.InstanceID,
		Backend:    backendID,
		SessionID:  row.NativeSessionID,
		Title:      row.Title,
	}

	return discoverer.ListCommands(ctx, ref)
}
