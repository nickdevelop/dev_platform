// Package instances owns the lifecycle of the platform's agent instances. It
// turns a registered project and a backend id into a durable instance record,
// delegates the backend-specific work to the registered adapter, and fans the
// adapter's per-instance event stream out to the platform's subscribers.
//
// Isolation is the core invariant: every instance has its own durable id, its
// own status row, its own upstream subscription and its own set of subscriber
// channels. Stopping or failing one instance never touches another, and an
// event produced for one instance is never delivered to a subscriber of
// another.
package instances

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/database"
	"devplatform/internal/app/projects"
)

// Service errors. Handlers map them to HTTP status codes; callers test them
// with errors.Is.
var (
	// ErrProjectNotFound reports an instance request for a project that is not
	// registered with the platform.
	ErrProjectNotFound = errors.New("instances: project not found")
	// ErrInstanceNotFound reports an operation on an unknown instance id.
	ErrInstanceNotFound = errors.New("instances: instance not found")
	// ErrUnknownBackend reports an unknown backend id, or a valid one with no
	// registered adapter.
	ErrUnknownBackend = errors.New("instances: unknown or unregistered backend")
	// ErrStartFailed reports that the adapter failed to start the instance.
	// The failed attempt is still recorded so its diagnostics stay visible.
	ErrStartFailed = errors.New("instances: adapter failed to start instance")
	// ErrInstanceNotRunning reports a subscription to an instance that is not
	// starting or running.
	ErrInstanceNotRunning = errors.New("instances: instance is not running")
	// ErrAgentDisabled reports an attempt to start a backend the platform has
	// disabled. Disabling only blocks new work; existing instances keep running.
	ErrAgentDisabled = errors.New("instances: agent is disabled platform-wide")
)

// ProjectDirectoryResolver turns a project id into its canonical, authorized
// working directory. *projects.Service implements it; the interface keeps the
// instance lifecycle decoupled from the project repository's other operations.
type ProjectDirectoryResolver interface {
	// ResolveDirectory returns the canonical working directory of the project
	// with the given id, re-authorizing the stored path against the current
	// workspace roots. It returns projects.ErrProjectNotFound when no project
	// exists.
	ResolveDirectory(id uint) (string, error)
}

// Instance is the JSON-stable API representation of an agent instance. GORM
// models are never serialized directly.
type Instance struct {
	// ID is the durable platform instance identifier.
	ID string `json:"id"`
	// ProjectID is the project the instance belongs to.
	ProjectID uint `json:"projectId"`
	// Backend names the agent backend this instance runs.
	Backend agents.BackendID `json:"backend"`
	// Directory is the instance's authorized working directory.
	Directory string `json:"directory"`
	// Status is the instance's current lifecycle state.
	Status agents.InstanceStatus `json:"status"`
	// Endpoint is the base URL of a server-based backend, when one exists.
	Endpoint string `json:"endpoint,omitempty"`
	// Error carries the adapter's redacted failure text for a failed instance.
	Error string `json:"error,omitempty"`
	// CreatedAt and UpdatedAt are the record's lifecycle timestamps.
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Service manages agent instance records and their event fan-out over the
// platform database and a backend registry.
type Service struct {
	repo     Repository
	registry *agents.Registry
	projects ProjectDirectoryResolver
	newID    func() (string, error)
	now      func() time.Time
	// platformEnabled reports whether a backend may be used platform-wide. Nil
	// means every backend is enabled.
	platformEnabled func(agents.BackendID) bool

	// mu guards streams, the per-instance upstream subscriptions.
	mu      sync.Mutex
	streams map[string]*stream

	// stopMu guards stopGates. stopGates gives every instance its own stop
	// mutex so concurrent Stop calls on one instance are single-flight while
	// stops of different instances still run concurrently.
	stopMu    sync.Mutex
	stopGates map[string]*stopGate

	// cleanup tracks subscriber fan-out cleanup goroutines. finishStream
	// closes the subscriber's done channel, which wakes its cleanup goroutine
	// even when the subscriber's own context never ends; cleanup makes that
	// exit observable (and lets a future Service shutdown await it).
	cleanup sync.WaitGroup
}

// stopGate is a per-instance stop mutex with a reference count so the gate map
// does not grow without bound as instances come and go.
type stopGate struct {
	mu   sync.Mutex
	refs int
}

// NewService builds the instance service. registry resolves backend ids to
// adapters; projects resolves project ids to authorized working directories.
func NewService(db *gorm.DB, registry *agents.Registry, projects ProjectDirectoryResolver, opts ...Option) *Service {
	return NewServiceWithRepository(NewGORMRepository(db), registry, projects, opts...)
}

// NewServiceWithRepository builds the instance service over a custom repository.
func NewServiceWithRepository(repo Repository, registry *agents.Registry, projects ProjectDirectoryResolver, opts ...Option) *Service {
	s := &Service{
		repo:         repo,
		registry:     registry,
		projects:     projects,
		newID:     randomID,
		now:       func() time.Time { return time.Now().UTC() },
		streams:   make(map[string]*stream),
		stopGates: make(map[string]*stopGate),
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

// randomID returns a 128-bit random identifier, used for a failed start
// attempt that has no adapter-assigned id.
func randomID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Start resolves the project's authorized directory, asks the registered
// adapter to bring up an instance of the backend there, and persists the
// result under the adapter-assigned id. A start failure is recorded as a failed
// instance (with a generated id) so the failure stays visible; it never affects
// other instances.
func (s *Service) Start(ctx context.Context, projectID uint, backend agents.BackendID) (Instance, error) {
	if !backend.Valid() {
		return Instance{}, fmt.Errorf("%w: %q", ErrUnknownBackend, backend)
	}
	adapter, ok := s.registry.Get(backend)
	if !ok {
		return Instance{}, fmt.Errorf("%w: %q (%w)", ErrUnknownBackend, backend, agents.ErrBackendNotRegistered)
	}

	directory, err := s.resolveProject(projectID)
	if err != nil {
		return Instance{}, err
	}

	// Platform-level enablement is the only switch: it gates new work for every
	// project and is checked before anything starts.
	if s.platformEnabled != nil && !s.platformEnabled(backend) {
		return Instance{}, fmt.Errorf("%w: %q", ErrAgentDisabled, backend)
	}

	started, startErr := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: directory})
	if startErr != nil {
		return s.recordStartFailure(projectID, backend, directory, startErr)
	}
	if started == nil || started.ID == "" {
		return s.recordStartFailure(projectID, backend, directory, errors.New("adapter returned no usable instance"))
	}

	status := started.Status
	if !status.Valid() {
		status = agents.InstanceStarting
	}
	now := s.now()
	inst := Instance{
		ID:        started.ID,
		ProjectID: projectID,
		Backend:   backend,
		Directory: directory,
		Status:    status,
		Endpoint:  started.Endpoint,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(inst); err != nil {
		// The adapter believes an instance is running under started.ID; stop
		// it so a failed record insert does not leak a live instance.
		_ = adapter.StopInstance(ctx, started.ID)
		return Instance{}, fmt.Errorf("instances: record instance %q: %w", started.ID, err)
	}
	return inst, nil
}

// recordStartFailure persists a failed instance for a start attempt that never
// produced a usable adapter id, then reports ErrStartFailed wrapping the cause.
func (s *Service) recordStartFailure(projectID uint, backend agents.BackendID, directory string, cause error) (Instance, error) {
	id, err := s.newID()
	if err != nil {
		return Instance{}, fmt.Errorf("%w: %v (could not allocate a failure id: %v)", ErrStartFailed, cause, err)
	}
	now := s.now()
	inst := Instance{
		ID:        id,
		ProjectID: projectID,
		Backend:   backend,
		Directory: directory,
		Status:    agents.InstanceFailed,
		Error:     cause.Error(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(inst); err != nil {
		return Instance{}, fmt.Errorf("%w: %w (could not record the failure: %v)", ErrStartFailed, cause, err)
	}
	return inst, fmt.Errorf("%w: %w", ErrStartFailed, cause)
}

// List returns the project's instances in creation order, pruning old dead instances.
func (s *Service) List(projectID uint) ([]Instance, error) {
	_ = s.repo.PruneDead(projectID, 5)
	return s.repo.List(projectID)
}

// ListFiltered returns the project's instances matching the filter (e.g. "live", "running").
func (s *Service) ListFiltered(projectID uint, filter string) ([]Instance, error) {
	_ = s.repo.PruneDead(projectID, 5)
	return s.repo.ListFiltered(projectID, filter)
}

// PruneDead removes old stopped or failed instances for the project, keeping only the most recent keepLast records.
func (s *Service) PruneDead(projectID uint, keepLast int) error {
	return s.repo.PruneDead(projectID, keepLast)
}

// Get returns the instance with the given id.
func (s *Service) Get(instanceID string) (Instance, error) {
	return s.load(instanceID)
}

// Stop terminates an instance and tears down its event fan-out. It is
// idempotent, including under concurrency: a per-instance stop gate serializes
// callers so only the first observes the running instance and invokes the
// adapter, while the rest observe the terminal state it wrote. Stopping an
// already stopped or failed instance returns that instance unchanged without
// calling the adapter again. Stopping one instance never affects another.
// Adapters are expected to treat a backend that is already gone as a
// successful stop; an adapter error is surfaced and the instance is marked
// failed so the failure stays visible.
func (s *Service) Stop(ctx context.Context, instanceID string) (Instance, error) {
	gate := s.acquireStopGate(instanceID)
	gate.mu.Lock()
	defer func() {
		gate.mu.Unlock()
		s.releaseStopGate(instanceID, gate)
	}()

	// Re-read under the gate: a caller that arrives second must observe the
	// terminal state the first caller wrote rather than re-running the stop.
	row, err := s.load(instanceID)
	if err != nil {
		return Instance{}, err
	}
	if row.Status == agents.InstanceStopped || row.Status == agents.InstanceFailed {
		return row, nil
	}

	// Tear down the local subscription first so a stop never leaves a fan-out
	// delivering events for an instance that is going away, even if the adapter
	// stop call is slow or fails.
	s.closeStream(instanceID)

	adapter, ok := s.registry.Get(row.Backend)
	var stopErr error
	if !ok {
		stopErr = fmt.Errorf("%w: %q", ErrUnknownBackend, row.Backend)
	} else {
		stopErr = adapter.StopInstance(ctx, instanceID)
	}

	row.Status = agents.InstanceStopped
	row.Error = ""
	if stopErr != nil {
		row.Status = agents.InstanceFailed
		row.Error = stopErr.Error()
	}
	row.UpdatedAt = s.now()
	if err := s.repo.UpdateStatus(instanceID, row.Status, row.Error, row.UpdatedAt); err != nil {
		return Instance{}, fmt.Errorf("instances: update instance %q: %w", instanceID, err)
	}
	if stopErr != nil {
		return row, fmt.Errorf("instances: stop %q: %w", instanceID, stopErr)
	}
	return row, nil
}

// StopProjectInstances stops every live instance of one project. It is the
// instance side of unregistering a project (projects.InstanceStopper). Each
// live instance goes through the same idempotent Stop path; instances that are
// already stopped or failed are skipped. One failure never prevents the others
// from being stopped, and the failures are joined and returned so the caller
// can refuse to remove the project.
func (s *Service) StopProjectInstances(ctx context.Context, projectID uint) error {
	list, err := s.List(projectID)
	if err != nil {
		return err
	}
	var errs []error
	for _, inst := range list {
		if inst.Status == agents.InstanceStopped || inst.Status == agents.InstanceFailed {
			continue
		}
		if _, err := s.Stop(ctx, inst.ID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Close stops every live instance and tears down its event fan-out. It is the
// platform shutdown seam: without it, `opencode serve` servers and pi child
// processes would outlive the platform and could never be reaped after a
// restart. Each stop goes through the same single-flight, idempotent path as an
// explicit Stop, so one instance's failure never prevents the others from being
// stopped; the failures are joined and returned for the caller to log.
func (s *Service) Close(ctx context.Context) error {
	live, err := s.repo.ListLive()
	if err != nil {
		return fmt.Errorf("instances: close: %w", err)
	}
	var errs []error
	for _, inst := range live {
		if _, err := s.Stop(ctx, inst.ID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// acquireStopGate returns the instance's stop gate, creating it on first use
// and taking a reference so it is not reclaimed while a caller holds it.
func (s *Service) acquireStopGate(instanceID string) *stopGate {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()

	gate, ok := s.stopGates[instanceID]
	if !ok {
		gate = &stopGate{}
		s.stopGates[instanceID] = gate
	}
	gate.refs++
	return gate
}

// releaseStopGate drops a caller's reference and removes the gate once no
// caller holds it, so the gate map stays bounded.
func (s *Service) releaseStopGate(instanceID string, gate *stopGate) {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()

	gate.refs--
	if gate.refs == 0 && s.stopGates[instanceID] == gate {
		delete(s.stopGates, instanceID)
	}
}

// SubscriberCount reports how many live subscribers are attached to the
// instance's event stream, or zero when no stream exists. A caller that needs
// one specific subscriber to be registered (rather than merely the stream to
// exist) can wait for the count to reach the number it expects.
func (s *Service) SubscriberCount(instanceID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.streams[instanceID]
	if !ok || st.subscribers == nil {
		return 0
	}
	return len(st.subscribers)
}

// Subscribe returns a channel of the instance's normalized events. All
// subscribers of one instance share a single upstream adapter subscription and
// each gets an independent channel; events for other instances are never
// delivered. The returned channel closes when the passed context is done or
// when the instance's upstream stream ends.
func (s *Service) Subscribe(ctx context.Context, instanceID string) (<-chan agents.NormalizedEvent, error) {
	row, err := s.load(instanceID)
	if err != nil {
		return nil, err
	}
	status := agents.InstanceStatus(row.Status)
	if status != agents.InstanceStarting && status != agents.InstanceRunning {
		return nil, fmt.Errorf("%w: %s is %s", ErrInstanceNotRunning, instanceID, status)
	}
	adapter, ok := s.registry.Get(agents.BackendID(row.Backend))
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownBackend, row.Backend)
	}

	st, err := s.streamFor(instanceID, agents.BackendID(row.Backend), adapter)
	if err != nil {
		return nil, err
	}
	sub, _ := s.addSubscriber(ctx, instanceID, st)
	return sub.out, nil
}

// ReplaySince returns the instance's buffered events with Sequence greater
// than after, oldest first, alongside whether the buffer covers the whole
// gap: false means events were lost to eviction or the sequence belongs to a
// stream that no longer exists, so the caller must resynchronize from
// history instead of trusting a partial tail. A caller that needs no events
// lost between the live subscription and this snapshot must Subscribe first:
// events published after the subscription are delivered on the channel, and
// events published before it are in the buffer.
func (s *Service) ReplaySince(instanceID string, after uint64) ([]agents.NormalizedEvent, bool) {
	s.mu.Lock()
	st, ok := s.streams[instanceID]
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	return st.ring.since(after)
}

// resolveProject maps a project id to its authorized directory, translating
// the project repository's not-found sentinel.
func (s *Service) resolveProject(projectID uint) (string, error) {
	directory, err := s.projects.ResolveDirectory(projectID)
	if err != nil {
		if errors.Is(err, projects.ErrProjectNotFound) {
			return "", fmt.Errorf("%w: %d", ErrProjectNotFound, projectID)
		}
		return "", err
	}
	return directory, nil
}

// load reads the instance record with the given id.
func (s *Service) load(instanceID string) (Instance, error) {
	return s.repo.Get(instanceID)
}

// instanceFromModel maps the persistence model to the JSON-stable DTO.
func instanceFromModel(row database.AgentInstance) Instance {
	return Instance{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Backend:   agents.BackendID(row.Backend),
		Directory: row.Directory,
		Status:    agents.InstanceStatus(row.Status),
		Endpoint:  row.Endpoint,
		Error:     row.Error,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// ReconcileOrphans marks any lingering 'starting' or 'running' instances from a
// previous platform process as 'stopped'. It is intended to run once during
// platform startup after database migrations.
func (s *Service) ReconcileOrphans(ctx context.Context) (int64, error) {
	return s.repo.ReconcileOrphaned(s.now())
}

// stream is the service's single upstream subscription for one instance. Its
// subscribers each own an output channel; the upstream goroutine is the only
// writer and the only closer of those channels. The replay ring records every
// event the stream has published so a reconnecting SSE client can be caught
// up on the events it missed while it was away.
type stream struct {
	cancel      context.CancelFunc
	subscribers map[*subscriber]struct{}
	ring        *replayRing
}

// subscriber is one consumer of an instance's event stream. stash is written by
// the broadcaster and never closed; out is owned and closed by forward.
type subscriber struct {
	stash chan agents.NormalizedEvent
	out   chan agents.NormalizedEvent
	done  chan struct{}
	once  sync.Once
}

// streamFor returns the instance's existing stream, or creates one by
// subscribing to the adapter. A concurrent first subscriber that loses the race
// discards its redundant upstream subscription.
func (s *Service) streamFor(instanceID string, backend agents.BackendID, adapter agents.AgentAdapter) (*stream, error) {
	s.mu.Lock()
	if st, ok := s.streams[instanceID]; ok {
		s.mu.Unlock()
		return st, nil
	}
	s.mu.Unlock()

	// The upstream subscription is owned by the service, not by any caller's
	// context, so it survives one SSE client disconnecting while others stay.
	upstreamCtx, cancel := context.WithCancel(context.Background())
	upstream, err := adapter.Subscribe(upstreamCtx, instanceID)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("instances: subscribe %s: %w", instanceID, err)
	}

	st := &stream{cancel: cancel, subscribers: make(map[*subscriber]struct{}), ring: newReplayRing()}
	s.mu.Lock()
	if existing, ok := s.streams[instanceID]; ok {
		s.mu.Unlock()
		cancel()
		return existing, nil
	}
	s.streams[instanceID] = st
	s.mu.Unlock()

	go s.runStream(instanceID, backend, st, upstream)
	return st, nil
}

// runStream forwards the adapter's events to the stream's subscribers until the
// upstream channel closes, then tears the stream down.
func (s *Service) runStream(instanceID string, backend agents.BackendID, st *stream, upstream <-chan agents.NormalizedEvent) {
	for ev := range upstream {
		// The subscription is per instance; stamp the envelope so a
		// mislabelled adapter event can never surface under another instance.
		ev.InstanceID = instanceID
		ev.Backend = backend
		s.applyLifecycle(ev)
		// Retain the event for replay before handing it to current
		// subscribers: a client subscribing (and then snapshotting the ring)
		// concurrently with this loop must never find the event in neither
		// place, while seeing it in both is resolved by the replay watermark.
		st.ring.add(ev)
		s.broadcast(st, ev)
	}
	s.finishStream(instanceID, st)
}

// broadcast delivers ev to every current subscriber, blocking on a full
// subscriber buffer until it drains or the subscriber goes away. Ordering is
// preserved per subscriber.
func (s *Service) broadcast(st *stream, ev agents.NormalizedEvent) {
	s.mu.Lock()
	subs := make([]*subscriber, 0, len(st.subscribers))
	for sub := range st.subscribers {
		subs = append(subs, sub)
	}
	s.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.stash <- ev:
		case <-sub.done:
		}
	}
}

// finishStream removes the stream and closes every subscriber once the upstream
// subscription has ended.
func (s *Service) finishStream(instanceID string, st *stream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if current, ok := s.streams[instanceID]; ok && current == st {
		delete(s.streams, instanceID)
	}
	for sub := range st.subscribers {
		sub.once.Do(func() { close(sub.done) })
	}
	st.subscribers = nil
}

// closeStream cancels an instance's upstream subscription. The upstream
// goroutine then closes the subscriber channels.
func (s *Service) closeStream(instanceID string) {
	s.mu.Lock()
	st, ok := s.streams[instanceID]
	if ok {
		delete(s.streams, instanceID)
	}
	s.mu.Unlock()
	if ok {
		st.cancel()
	}
}

// addSubscriber registers a new subscriber and starts its forwarder. It
// returns false when the stream has already finished (finishStream nil-ed the
// subscriber set), in which case the caller must not register: the returned
// subscriber's output is already closed rather than risking a write to a nil
// map. Registration and the finished check happen under the same lock that
// finishStream holds while nil-ing the set, so the two are ordered safely.
func (s *Service) addSubscriber(ctx context.Context, instanceID string, st *stream) (*subscriber, bool) {
	sub := &subscriber{
		stash: make(chan agents.NormalizedEvent, 32),
		out:   make(chan agents.NormalizedEvent, 32),
		done:  make(chan struct{}),
	}

	s.mu.Lock()
	if st.subscribers == nil {
		// The stream completed between streamFor and here; hand back a
		// subscriber whose output is already closed (the documented contract
		// is that the channel closes when the stream ends).
		s.mu.Unlock()
		close(sub.out)
		return sub, false
	}
	st.subscribers[sub] = struct{}{}
	s.mu.Unlock()

	go sub.forward(ctx)
	s.cleanup.Add(1)
	go func() {
		defer s.cleanup.Done()
		// Wake on either the caller's context or the subscriber's own done
		// channel, so stream completion / unsubscribe (which closes done)
		// always exits this goroutine even if the context never ends.
		select {
		case <-ctx.Done():
		case <-sub.done:
		}
		sub.once.Do(func() { close(sub.done) })
		s.removeSubscriber(instanceID, st, sub)
	}()
	return sub, true
}

// removeSubscriber drops a canceled subscriber from its stream.
func (s *Service) removeSubscriber(instanceID string, st *stream, sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.streams[instanceID]; ok && current == st && st.subscribers != nil {
		delete(st.subscribers, sub)
	}
}

// terminalDeliverTimeout bounds how long a torn-down subscriber waits to hand
// an already-queued terminal event to its consumer before giving up.
const terminalDeliverTimeout = 100 * time.Millisecond

// forward copies the subscriber's stash to its output channel and closes the
// output when the subscription ends. On stream completion it flushes events
// already queued, so a terminal lifecycle event (for example the `failed` event
// published just before the upstream ended) is delivered rather than dropped.
func (sub *subscriber) forward(ctx context.Context) {
	defer close(sub.out)
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.done:
			sub.flush()
			return
		case ev := <-sub.stash:
			select {
			case sub.out <- ev:
			case <-ctx.Done():
				return
			case <-sub.done:
				sub.deliver(ev)
				sub.flush()
				return
			}
		}
	}
}

// deliver sends one event to the output, giving up after a short bound so a
// gone consumer cannot wedge the goroutine.
func (sub *subscriber) deliver(ev agents.NormalizedEvent) {
	select {
	case sub.out <- ev:
	case <-time.After(terminalDeliverTimeout):
	}
}

// flush delivers the subscriber's already-queued events to its output.
func (sub *subscriber) flush() {
	for {
		select {
		case ev := <-sub.stash:
			sub.deliver(ev)
		default:
			return
		}
	}
}

// applyLifecycle mirrors a lifecycle event onto the instance's stored status so
// a backend that reports state asynchronously (running, stopped, failed) keeps
// the platform record in step. Only valid, reachable transitions are applied;
// the update never touches another instance.
func (s *Service) applyLifecycle(ev agents.NormalizedEvent) {
	if ev.Type != agents.EventLifecycle || ev.Lifecycle == nil {
		return
	}
	target := agents.InstanceStatus(ev.Lifecycle.State)
	if !target.Valid() {
		return
	}

	inst, err := s.repo.Get(ev.InstanceID)
	if err != nil {
		return
	}
	if !inst.Status.CanTransitionTo(target) {
		return
	}

	detail := ""
	if target == agents.InstanceFailed {
		detail = ev.Lifecycle.Detail
	}
	// Best-effort mirror: a failure to persist the status must not break the
	// event fan-out, and the record stays readable at its previous status.
	_ = s.repo.UpdateStatus(ev.InstanceID, target, detail, s.now())
}
