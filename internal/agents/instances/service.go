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

	"devplatform/internal/agents"
	"devplatform/internal/database"
	"devplatform/internal/projects"
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
	db       *gorm.DB
	registry *agents.Registry
	projects ProjectDirectoryResolver
	newID    func() (string, error)
	now      func() time.Time

	// mu guards streams, the per-instance upstream subscriptions.
	mu      sync.Mutex
	streams map[string]*stream
}

// NewService builds the instance service. registry resolves backend ids to
// adapters; projects resolves project ids to authorized working directories.
func NewService(db *gorm.DB, registry *agents.Registry, projects ProjectDirectoryResolver) *Service {
	return &Service{
		db:       db,
		registry: registry,
		projects: projects,
		newID:    randomID,
		now:      func() time.Time { return time.Now().UTC() },
		streams:  make(map[string]*stream),
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
	row := database.AgentInstance{
		ID:        started.ID,
		ProjectID: projectID,
		Backend:   string(backend),
		// The adapter is never trusted to re-derive the working directory:
		// the platform persists the canonical path it authorized.
		Directory: directory,
		Status:    string(status),
		Endpoint:  started.Endpoint,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error { return tx.Create(&row).Error }); err != nil {
		// The adapter believes an instance is running under started.ID; stop
		// it so a failed record insert does not leak a live instance.
		_ = adapter.StopInstance(ctx, started.ID)
		return Instance{}, fmt.Errorf("instances: record instance %q: %w", started.ID, err)
	}
	return instanceFromModel(row), nil
}

// recordStartFailure persists a failed instance for a start attempt that never
// produced a usable adapter id, then reports ErrStartFailed wrapping the cause.
func (s *Service) recordStartFailure(projectID uint, backend agents.BackendID, directory string, cause error) (Instance, error) {
	id, err := s.newID()
	if err != nil {
		return Instance{}, fmt.Errorf("%w: %v (could not allocate a failure id: %v)", ErrStartFailed, cause, err)
	}
	now := s.now()
	row := database.AgentInstance{
		ID:        id,
		ProjectID: projectID,
		Backend:   string(backend),
		Directory: directory,
		Status:    string(agents.InstanceFailed),
		Error:     cause.Error(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error { return tx.Create(&row).Error }); err != nil {
		return Instance{}, fmt.Errorf("%w: %w (could not record the failure: %v)", ErrStartFailed, cause, err)
	}
	return instanceFromModel(row), fmt.Errorf("%w: %w", ErrStartFailed, cause)
}

// List returns the project's instances in creation order.
func (s *Service) List(projectID uint) ([]Instance, error) {
	var rows []database.AgentInstance
	if err := s.db.Where("project_id = ?", projectID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("instances: list project %d: %w", projectID, err)
	}
	out := make([]Instance, 0, len(rows))
	for _, row := range rows {
		out = append(out, instanceFromModel(row))
	}
	return out, nil
}

// Get returns the instance with the given id.
func (s *Service) Get(instanceID string) (Instance, error) {
	row, err := s.load(instanceID)
	if err != nil {
		return Instance{}, err
	}
	return instanceFromModel(row), nil
}

// Stop terminates an instance and tears down its event fan-out. It is
// idempotent: stopping an already stopped or failed instance returns that
// instance unchanged without calling the adapter again. Stopping one instance
// never affects another. Adapters are expected to treat a backend that is
// already gone as a successful stop; an adapter error is surfaced and the
// instance is marked failed so the failure stays visible.
func (s *Service) Stop(ctx context.Context, instanceID string) (Instance, error) {
	row, err := s.load(instanceID)
	if err != nil {
		return Instance{}, err
	}
	current := agents.InstanceStatus(row.Status)
	if current == agents.InstanceStopped || current == agents.InstanceFailed {
		return instanceFromModel(row), nil
	}

	// Tear down the local subscription first so a stop never leaves a fan-out
	// delivering events for an instance that is going away, even if the adapter
	// stop call is slow or fails.
	s.closeStream(instanceID)

	adapter, ok := s.registry.Get(agents.BackendID(row.Backend))
	var stopErr error
	if !ok {
		stopErr = fmt.Errorf("%w: %q", ErrUnknownBackend, row.Backend)
	} else {
		stopErr = adapter.StopInstance(ctx, instanceID)
	}

	row.Status = string(agents.InstanceStopped)
	row.Error = ""
	if stopErr != nil {
		row.Status = string(agents.InstanceFailed)
		row.Error = stopErr.Error()
	}
	row.UpdatedAt = s.now()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&database.AgentInstance{}).Where("id = ?", instanceID).
			Updates(map[string]any{"status": row.Status, "error": row.Error, "updated_at": row.UpdatedAt}).Error
	}); err != nil {
		return Instance{}, fmt.Errorf("instances: update instance %q: %w", instanceID, err)
	}
	if stopErr != nil {
		return instanceFromModel(row), fmt.Errorf("instances: stop %q: %w", instanceID, stopErr)
	}
	return instanceFromModel(row), nil
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
	return s.addSubscriber(ctx, instanceID, st).out, nil
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
func (s *Service) load(instanceID string) (database.AgentInstance, error) {
	var row database.AgentInstance
	if err := s.db.First(&row, "id = ?", instanceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return database.AgentInstance{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
		}
		return database.AgentInstance{}, fmt.Errorf("instances: load %s: %w", instanceID, err)
	}
	return row, nil
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

// stream is the service's single upstream subscription for one instance. Its
// subscribers each own an output channel; the upstream goroutine is the only
// writer and the only closer of those channels.
type stream struct {
	cancel      context.CancelFunc
	subscribers map[*subscriber]struct{}
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

	st := &stream{cancel: cancel, subscribers: make(map[*subscriber]struct{})}
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

// addSubscriber registers a new subscriber and starts its forwarder.
func (s *Service) addSubscriber(ctx context.Context, instanceID string, st *stream) *subscriber {
	sub := &subscriber{
		stash: make(chan agents.NormalizedEvent, 32),
		out:   make(chan agents.NormalizedEvent, 32),
		done:  make(chan struct{}),
	}

	s.mu.Lock()
	st.subscribers[sub] = struct{}{}
	s.mu.Unlock()

	go sub.forward(ctx)
	go func() {
		<-ctx.Done()
		sub.once.Do(func() { close(sub.done) })
		s.removeSubscriber(instanceID, st, sub)
	}()
	return sub
}

// removeSubscriber drops a canceled subscriber from its stream.
func (s *Service) removeSubscriber(instanceID string, st *stream, sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.streams[instanceID]; ok && current == st && st.subscribers != nil {
		delete(st.subscribers, sub)
	}
}

// forward copies the subscriber's stash to its output channel and closes the
// output when the subscription ends.
func (sub *subscriber) forward(ctx context.Context) {
	defer close(sub.out)
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.done:
			return
		case ev := <-sub.stash:
			select {
			case sub.out <- ev:
			case <-ctx.Done():
				return
			case <-sub.done:
				return
			}
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

	var row database.AgentInstance
	if err := s.db.First(&row, "id = ?", ev.InstanceID).Error; err != nil {
		return
	}
	if !agents.InstanceStatus(row.Status).CanTransitionTo(target) {
		return
	}

	detail := ""
	if target == agents.InstanceFailed {
		detail = ev.Lifecycle.Detail
	}
	// Best-effort mirror: a failure to persist the status must not break the
	// event fan-out, and the record stays readable at its previous status.
	s.db.Model(&database.AgentInstance{}).Where("id = ?", ev.InstanceID).
		Updates(map[string]any{"status": string(target), "error": detail, "updated_at": s.now()})
}
