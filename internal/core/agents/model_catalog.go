package agents

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// Model list cache defaults. A backend's catalog changes rarely and never
// because the platform asked, so the cache is deliberately long: it exists to
// take the model list off the critical path of opening a session, not to keep
// the workbench perfectly in step with the backend.
const (
	// defaultModelTTL is how long a model list is served without touching the
	// backend at all.
	defaultModelTTL = 60 * time.Second
	// defaultModelNegativeTTL is how long a failed fetch is remembered. It is
	// short on purpose: a backend that just came up should become usable at
	// once, while a burst of requests must not spawn a burst of failing
	// subprocesses or HTTP calls.
	defaultModelNegativeTTL = 5 * time.Second
	// defaultModelFetchTimeout bounds one upstream fetch. It is independent of
	// the caller's context because a background refresh outlives the request
	// that triggered it.
	defaultModelFetchTimeout = 15 * time.Second
)

// ModelList is one (backend, directory) catalog snapshot: the models plus how
// current they are. Stale is the caller's cue to expect newer models shortly —
// the refresh is already running in the background — not a reason to block.
type ModelList struct {
	Models    []ModelInfo `json:"models"`
	UpdatedAt time.Time   `json:"updatedAt"`
	Stale     bool        `json:"stale"`
}

// ModelFetcher loads a model list from the backend. It is called with a context
// the catalog owns, so a background refresh is never canceled by the request
// that triggered it.
type ModelFetcher func(ctx context.Context) ([]ModelInfo, error)

// ErrNilModelFetcher reports a List call with no way to load the list. It is a
// programming error, not a backend failure.
var ErrNilModelFetcher = errors.New("agents: model fetcher is required")

// maxListAttempts bounds List's retry loop. Every pass either answers or is
// caused by a fetch that finished, so a healthy path never needs a third pass;
// the cap keeps a pathological clock from spinning the loop.
const maxListAttempts = 8

// ModelCatalog serves per-(backend, directory) model lists from memory with
// stale-while-revalidate semantics and one in-flight fetch per key.
//
// A request never waits for a slow backend when a snapshot exists: it gets the
// snapshot, marked stale when it has aged out, and the refresh runs behind it.
// Only the very first request for a key — and an explicit force refresh — pays
// for the round trip. Failed fetches are cached briefly so a broken backend is
// not re-probed by every page view.
//
// Safe for concurrent use.
type ModelCatalog struct {
	ttl          time.Duration
	negativeTTL  time.Duration
	fetchTimeout time.Duration
	now          func() time.Time

	mu      sync.Mutex
	entries map[string]*modelEntry
}

// modelEntry is one key's snapshot plus its in-flight fetch. mu guards every
// field; the in-flight call lets concurrent callers join one fetch instead of
// each starting their own.
type modelEntry struct {
	mu sync.Mutex

	models    []ModelInfo
	fetchedAt time.Time
	err       error
	errAt     time.Time
	inflight  *modelCall
}

// modelCall is one in-flight fetch. done closes when it finishes; err carries
// the fetch's failure, so the caller that owned it reports the backend's problem
// rather than a cached state that predates it. ctx-free by design: an
// abandonable waiter never cancels the work.
type modelCall struct {
	done chan struct{}
	err  error
}

// NewModelCatalog builds an empty catalog.
func NewModelCatalog(opts ...ModelCatalogOption) *ModelCatalog {
	c := &ModelCatalog{
		ttl:          defaultModelTTL,
		negativeTTL:  defaultModelNegativeTTL,
		fetchTimeout: defaultModelFetchTimeout,
		now:          func() time.Time { return time.Now().UTC() },
		entries:      make(map[string]*modelEntry),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ModelCatalogOption configures a ModelCatalog.
type ModelCatalogOption func(*ModelCatalog)

// WithModelTTL sets how long a snapshot counts as fresh.
func WithModelTTL(ttl time.Duration) ModelCatalogOption {
	return func(c *ModelCatalog) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// WithModelNegativeTTL sets how long a failed fetch is remembered.
func WithModelNegativeTTL(ttl time.Duration) ModelCatalogOption {
	return func(c *ModelCatalog) {
		if ttl > 0 {
			c.negativeTTL = ttl
		}
	}
}

// WithModelFetchTimeout bounds one upstream fetch, background refreshes included.
func WithModelFetchTimeout(timeout time.Duration) ModelCatalogOption {
	return func(c *ModelCatalog) {
		if timeout > 0 {
			c.fetchTimeout = timeout
		}
	}
}

// WithModelClock replaces the catalog's clock (tests).
func WithModelClock(now func() time.Time) ModelCatalogOption {
	return func(c *ModelCatalog) {
		if now != nil {
			c.now = now
		}
	}
}

// ModelCacheKey builds the catalog key for one backend's catalog as seen from a
// working directory. Backends whose catalog depends on the directory (opencode
// serves a per-directory model list) need the pair; directory-independent
// backends simply get one key per directory they are used in.
func ModelCacheKey(backend BackendID, directory string) string {
	return string(backend) + "\x00" + directory
}

// List returns the model list for a cache key, fetching it with fetch when the
// cache cannot answer. force bypasses freshness and fetches synchronously, so
// an explicit refresh reports the backend's state rather than a snapshot.
func (c *ModelCatalog) List(ctx context.Context, key string, force bool, fetch ModelFetcher) (ModelList, error) {
	if fetch == nil {
		return ModelList{}, ErrNilModelFetcher
	}
	entry := c.entry(key)
	// A forced fetch is spent once: the caller asked for the backend's current
	// state, and the fetch that answered is now that state.
	forceFetch := force
	for attempt := 0; attempt < maxListAttempts; attempt++ {
		list, action, err := c.evaluate(entry, forceFetch)
		switch action {
		case modelActionWait:
			// Another caller is already fetching this key. Joining that fetch
			// is what keeps one slow backend from multiplying subprocesses.
			call, ok := c.current(entry)
			if !ok {
				continue
			}
			if err := wait(ctx, call); err != nil {
				return ModelList{}, err
			}
		case modelActionStale:
			// The snapshot answers now; a fresher one is on its way. A caller
			// that must show the backend's current state asks with force.
			c.startBackground(entry, fetch)
			return list, nil
		case modelActionFetch:
			call, owned := c.start(entry, ctx, fetch)
			if !owned {
				if err := wait(ctx, call); err != nil {
					return ModelList{}, err
				}
				forceFetch = false
				continue
			}
			if call.err != nil {
				// The caller's own fetch failed. An older snapshot is not
				// substituted here: it is what the next, unforced request
				// falls back to, while this one reports what the backend did.
				return ModelList{}, call.err
			}
			forceFetch = false
			// Re-decide: the fetch left a fresh snapshot behind.
		default: // modelActionReturn
			return list, err
		}
	}
	return ModelList{}, errModelCatalogUnsettled
}

// errModelCatalogUnsettled reports that a key's state kept changing for longer
// than maxListAttempts passes. It cannot happen with a healthy clock and is
// reported rather than hidden, because the alternative is an unbounded loop.
var errModelCatalogUnsettled = errors.New("agents: model catalog did not settle")

// wait blocks until a fetch finishes or the caller's context ends. The fetch
// itself keeps running: one client leaving is not a reason to abandon the work
// everybody else is waiting on.
func wait(ctx context.Context, call *modelCall) error {
	select {
	case <-call.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Invalidate drops every snapshot of one backend (all directories), so the next
// request re-reads the backend. It is the hook for the events that really do
// change a catalog's availability: the backend's service coming up or going
// down.
func (c *ModelCatalog) Invalidate(backend BackendID) {
	prefix := string(backend) + "\x00"
	c.mu.Lock()
	entries := make([]*modelEntry, 0, len(c.entries))
	for key, entry := range c.entries {
		if strings.HasPrefix(key, prefix) {
			entries = append(entries, entry)
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
	for _, entry := range entries {
		entry.mu.Lock()
		entry.models, entry.fetchedAt = nil, time.Time{}
		entry.err, entry.errAt = nil, time.Time{}
		entry.mu.Unlock()
	}
}

// modelAction is what List should do with an entry after inspecting it.
type modelAction int

const (
	// modelActionReturn hands the caller the snapshot (or error) evaluate
	// produced, touching nothing.
	modelActionReturn modelAction = iota
	// modelActionWait joins the fetch another caller already started.
	modelActionWait
	// modelActionFetch means this caller must fetch before it can answer.
	modelActionFetch
	// modelActionStale answers from an aged snapshot and refreshes behind it.
	modelActionStale
)

// evaluate decides from the entry's snapshot whether List can answer, must wait
// for an in-flight fetch, must fetch, or may serve a snapshot while refreshing.
func (c *ModelCatalog) evaluate(entry *modelEntry, force bool) (ModelList, modelAction, error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	now := c.now()
	if entry.inflight != nil {
		return ModelList{}, modelActionWait, nil
	}
	// A snapshot survives a later failing refresh on purpose: an older catalog
	// beats no catalog, and the caller can see it is older.
	hasSnapshot := !entry.fetchedAt.IsZero()
	switch {
	case hasSnapshot && force:
		return ModelList{}, modelActionFetch, nil
	case hasSnapshot && now.Sub(entry.fetchedAt) >= c.ttl:
		if entry.err != nil && now.Sub(entry.errAt) < c.negativeTTL {
			// Refreshing already failed inside the negative window. Hand out
			// the snapshot and leave the backend alone until it passes.
			return c.modelList(entry, true), modelActionReturn, nil
		}
		return c.modelList(entry, true), modelActionStale, nil
	case hasSnapshot:
		return c.modelList(entry, false), modelActionReturn, nil
	case entry.err != nil && !force && now.Sub(entry.errAt) < c.negativeTTL:
		return ModelList{}, modelActionReturn, entry.err
	default:
		// Nothing usable yet: the first request for this key pays for the
		// fetch rather than showing an empty list.
		return ModelList{}, modelActionFetch, nil
	}
}

// modelList snapshots the entry's models for one caller. entry.mu must be held.
func (c *ModelCatalog) modelList(entry *modelEntry, stale bool) ModelList {
	models := cloneModels(entry.models)
	return ModelList{Models: models, UpdatedAt: entry.fetchedAt, Stale: stale}
}

// current returns the entry's in-flight call.
func (c *ModelCatalog) current(entry *modelEntry) (*modelCall, bool) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.inflight, entry.inflight != nil
}

// start runs one synchronous fetch for the entry on the caller's context,
// taking care that only one is in flight. It reports the call and whether the
// caller owns it: when it does not, the fetch belongs to another caller and the
// result should be awaited instead of duplicated.
func (c *ModelCatalog) start(entry *modelEntry, ctx context.Context, fetch ModelFetcher) (*modelCall, bool) {
	call, owned := c.claim(entry)
	if !owned {
		return call, false
	}
	c.fetchWith(entry, fetch, call, ctx)
	return call, true
}

// startBackground runs one refresh on a context the catalog owns, so the
// request that noticed a stale snapshot can answer immediately and leave.
func (c *ModelCatalog) startBackground(entry *modelEntry, fetch ModelFetcher) {
	call, owned := c.claim(entry)
	if !owned {
		return
	}
	go func() {
		base, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.fetchWith(entry, fetch, call, base)
	}()
}

// claim takes ownership of the entry's next fetch, or joins the one already
// running.
func (c *ModelCatalog) claim(entry *modelEntry) (*modelCall, bool) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if call := entry.inflight; call != nil {
		return call, false
	}
	call := &modelCall{done: make(chan struct{})}
	entry.inflight = call
	return call, true
}

// fetchWith performs the upstream call and records its outcome, then wakes
// everyone waiting on call.
func (c *ModelCatalog) fetchWith(entry *modelEntry, fetch ModelFetcher, call *modelCall, base context.Context) {
	ctx, cancel := context.WithTimeout(base, c.fetchTimeout)
	defer cancel()
	models, err := fetch(ctx)

	entry.mu.Lock()
	if err == nil {
		entry.models, entry.fetchedAt = cloneModels(models), c.now()
		entry.err, entry.errAt = nil, time.Time{}
	} else {
		entry.err, entry.errAt = err, c.now()
	}
	entry.inflight = nil
	entry.mu.Unlock()
	call.err = err
	close(call.done)
}

// entry returns the key's entry, creating it on first use.
func (c *ModelCatalog) entry(key string) *modelEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &modelEntry{}
		c.entries[key] = entry
	}
	return entry
}

// cloneModels deep-copies a snapshot so a caller cannot mutate the cache
// through the slice it was handed.
func cloneModels(models []ModelInfo) []ModelInfo {
	if len(models) == 0 {
		return []ModelInfo{}
	}
	out := make([]ModelInfo, len(models))
	for i, model := range models {
		if model.Variants != nil {
			variants := make([]string, len(model.Variants))
			copy(variants, model.Variants)
			model.Variants = variants
		}
		out[i] = model
	}
	return out
}
