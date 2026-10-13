package agents

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock the test advances by hand, so freshness is asserted
// without sleeping through real TTLs.
type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{at: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// fetcher is a ModelFetcher that counts calls and answers from a scripted
// result, optionally gating each call on a channel so concurrency is testable.
type fetcher struct {
	mu    sync.Mutex
	calls int

	result  []ModelInfo
	err     error
	entered chan struct{}
	release chan struct{}
}

func newFetcher(models ...ModelInfo) *fetcher {
	return &fetcher{result: models}
}

func (f *fetcher) fn() ModelFetcher {
	return func(ctx context.Context) ([]ModelInfo, error) {
		f.mu.Lock()
		f.calls++
		entered := f.entered
		result, err := f.result, f.err
		release := f.release
		f.mu.Unlock()

		if entered != nil {
			select {
			case entered <- struct{}{}:
			default:
			}
		}
		if release != nil {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return append([]ModelInfo(nil), result...), err
	}
}

func (f *fetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fetcher) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fetcher) setResult(models ...ModelInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.result = models
}

func (f *fetcher) unblock() {
	f.mu.Lock()
	release := f.release
	f.mu.Unlock()
	if release != nil {
		select {
		case <-release:
		default:
			close(release)
		}
	}
}

// gate makes every fetch block until unblock, and reports when one starts.
func (f *fetcher) gate() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entered = make(chan struct{}, 8)
	f.release = make(chan struct{})
	return f.entered
}

func testModels() []ModelInfo {
	return []ModelInfo{
		{ID: "acme/gpt", Name: "GPT", Provider: "acme", Variants: []string{"acme/gpt#fast"}},
		{ID: "acme/small", Name: "Small", Provider: "acme"},
	}
}

// TestModelCatalogCachesUntilTTL checks the first caller pays for the fetch and
// later callers are answered from cache until the window passes; force fetches
// regardless.
func TestModelCatalogCachesUntilTTL(t *testing.T) {
	clock := newFakeClock()
	catalog := NewModelCatalog(WithModelTTL(time.Minute), WithModelClock(clock.now))
	f := newFetcher(testModels()...)

	first, err := catalog.List(context.Background(), "k", false, f.fn())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(first.Models) != 2 || first.Stale || first.UpdatedAt.IsZero() {
		t.Fatalf("first list = %+v, want two fresh models", first)
	}

	second, err := catalog.List(context.Background(), "k", false, f.fn())
	if err != nil {
		t.Fatalf("List cached: %v", err)
	}
	if second.Stale || f.count() != 1 {
		t.Fatalf("cached list = %+v after %d fetches, want no extra fetch", second, f.count())
	}

	if _, err := catalog.List(context.Background(), "k", true, f.fn()); err != nil {
		t.Fatalf("List force: %v", err)
	}
	if f.count() != 2 {
		t.Fatalf("force refresh did not reach the backend: %d fetches, want 2", f.count())
	}

	clock.advance(2 * time.Minute)
	third, err := catalog.List(context.Background(), "k", false, f.fn())
	if err != nil {
		t.Fatalf("List after TTL: %v", err)
	}
	if !third.Stale {
		t.Errorf("list after the TTL is not marked stale: %+v", third)
	}
}

// TestModelCatalogStaleWhileRevalidate checks an aged snapshot answers at once
// and the refresh runs behind it, so the next caller sees the new catalog.
func TestModelCatalogStaleWhileRevalidate(t *testing.T) {
	catalog := NewModelCatalog(
		WithModelTTL(time.Nanosecond),
		WithModelFetchTimeout(2*time.Second),
	)
	f := newFetcher(testModels()...)

	if _, err := catalog.List(context.Background(), "k", false, f.fn()); err != nil {
		t.Fatalf("List: %v", err)
	}
	// Everything after the first fetch is stale, and each stale answer kicks
	// off one background refresh.
	f.setResult(ModelInfo{ID: "acme/newer", Name: "Newer", Provider: "acme"})
	got, err := catalog.List(context.Background(), "k", false, f.fn())
	if err != nil {
		t.Fatalf("List stale: %v", err)
	}
	if !got.Stale {
		t.Fatalf("stale answer = %+v, want Stale", got)
	}

	// The refresh is asynchronous: wait for it to land, then ask again.
	deadline := time.Now().Add(2 * time.Second)
	for {
		list, err := catalog.List(context.Background(), "k", true, f.fn())
		if err != nil {
			t.Fatalf("List refresh: %v", err)
		}
		if len(list.Models) == 1 && list.Models[0].ID == "acme/newer" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("background refresh did not update the cache: %+v", list.Models)
		}
	}
}

// TestModelCatalogSingleFlight checks concurrent callers join one fetch instead
// of each spawning a subprocess or request of their own.
func TestModelCatalogSingleFlight(t *testing.T) {
	catalog := NewModelCatalog(WithModelTTL(time.Minute))
	f := newFetcher(testModels()...)
	entered := f.gate()

	const callers = 8
	var wg sync.WaitGroup
	results := make([]ModelList, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = catalog.List(context.Background(), "k", false, f.fn())
		}(i)
	}

	// Wait until at least one fetch is in flight, then let everyone through.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("no fetch started")
	}
	f.unblock()
	wg.Wait()

	if got := f.count(); got != 1 {
		t.Fatalf("fetches = %d, want 1 for %d concurrent callers", got, callers)
	}
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if len(results[i].Models) != 2 {
			t.Fatalf("caller %d got %+v, want the two models", i, results[i].Models)
		}
	}
}

// TestModelCatalogNegativeCache checks a failing backend is not re-probed by
// every caller, and recovers once the negative window passes.
func TestModelCatalogNegativeCache(t *testing.T) {
	clock := newFakeClock()
	catalog := NewModelCatalog(
		WithModelTTL(time.Minute),
		WithModelNegativeTTL(time.Second),
		WithModelClock(clock.now),
	)
	boom := errors.New("backend down")
	f := newFetcher()
	f.setErr(boom)

	if _, err := catalog.List(context.Background(), "k", false, f.fn()); !errors.Is(err, boom) {
		t.Fatalf("List error = %v, want the backend error", err)
	}
	if _, err := catalog.List(context.Background(), "k", false, f.fn()); !errors.Is(err, boom) {
		t.Fatalf("cached error = %v, want the backend error", err)
	}
	if f.count() != 1 {
		t.Fatalf("fetches = %d, want the failure to be cached", f.count())
	}

	clock.advance(2 * time.Second)
	f.setErr(nil)
	f.setResult(testModels()...)
	list, err := catalog.List(context.Background(), "k", false, f.fn())
	if err != nil {
		t.Fatalf("List after recovery: %v", err)
	}
	if len(list.Models) != 2 {
		t.Fatalf("recovered list = %+v, want two models", list.Models)
	}
}

// TestModelCatalogKeepsSnapshotOnRefreshFailure checks a refresh that fails does
// not take the last known catalog with it: the caller still gets models, marked
// stale, because an older list beats no list.
func TestModelCatalogKeepsSnapshotOnRefreshFailure(t *testing.T) {
	clock := newFakeClock()
	catalog := NewModelCatalog(
		WithModelTTL(time.Minute),
		WithModelNegativeTTL(time.Second),
		WithModelClock(clock.now),
	)
	f := newFetcher(testModels()...)
	if _, err := catalog.List(context.Background(), "k", false, f.fn()); err != nil {
		t.Fatalf("List: %v", err)
	}

	boom := errors.New("backend down")
	f.setErr(boom)
	if _, err := catalog.List(context.Background(), "k", true, f.fn()); !errors.Is(err, boom) {
		t.Fatalf("forced refresh error = %v, want the backend error", err)
	}

	f.setErr(nil)
	got, err := catalog.List(context.Background(), "k", false, f.fn())
	if err != nil {
		t.Fatalf("List after failed refresh: %v", err)
	}
	if len(got.Models) != 2 {
		t.Fatalf("list = %+v, want the previously known models", got.Models)
	}
}

// TestModelCatalogInvalidate checks a backend's lists are dropped for every
// directory while another backend's stay.
func TestModelCatalogInvalidate(t *testing.T) {
	catalog := NewModelCatalog(WithModelTTL(time.Minute))
	pi := newFetcher(testModels()...)
	opencode := newFetcher(testModels()...)
	piKey := ModelCacheKey(BackendPi, "/work/alpha")
	piOther := ModelCacheKey(BackendPi, "/work/beta")
	openKey := ModelCacheKey(BackendOpenCodeV2, "/work/alpha")

	for _, key := range []string{piKey, piOther, openKey} {
		if _, err := catalog.List(context.Background(), key, false, pi.fn()); err != nil {
			t.Fatalf("List %s: %v", key, err)
		}
	}
	catalog.Invalidate(BackendPi)

	if _, err := catalog.List(context.Background(), piKey, false, pi.fn()); err != nil {
		t.Fatalf("List after invalidate: %v", err)
	}
	if pi.count() != 4 {
		t.Fatalf("pi fetches = %d, want 3 cached plus 1 after invalidation", pi.count())
	}
	if _, err := catalog.List(context.Background(), openKey, false, opencode.fn()); err != nil {
		t.Fatalf("List other backend: %v", err)
	}
	if opencode.count() != 0 {
		t.Fatalf("opencode was fetched %d times, want its own key left untouched", opencode.count())
	}
}

// TestModelCatalogSnapshotIsolation checks a caller cannot mutate the cache
// through the slice it was handed.
func TestModelCatalogSnapshotIsolation(t *testing.T) {
	catalog := NewModelCatalog(WithModelTTL(time.Minute))
	if _, err := catalog.List(context.Background(), "k", false, newFetcher(testModels()...).fn()); err != nil {
		t.Fatalf("List: %v", err)
	}
	first, err := catalog.List(context.Background(), "k", false, newFetcher().fn())
	if err != nil {
		t.Fatalf("List cached: %v", err)
	}
	first.Models[0].ID = "tampered"
	first.Models[0].Variants[0] = "tampered"

	second, err := catalog.List(context.Background(), "k", false, newFetcher().fn())
	if err != nil {
		t.Fatalf("List cached again: %v", err)
	}
	if second.Models[0].ID != "acme/gpt" || second.Models[0].Variants[0] != "acme/gpt#fast" {
		t.Fatalf("cache was mutated through a returned snapshot: %+v", second.Models[0])
	}
}

// TestModelCatalogContextCancellation checks a caller that gives up is released
// even while another caller's fetch is still running.
func TestModelCatalogContextCancellation(t *testing.T) {
	catalog := NewModelCatalog(WithModelTTL(time.Minute))
	f := newFetcher(testModels()...)
	entered := f.gate()

	errs := make(chan error, 2)
	go func() {
		_, err := catalog.List(context.Background(), "k", false, f.fn())
		errs <- err
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	go func() {
		_, err := catalog.List(ctx, "k", false, f.fn())
		got <- err
	}()
	cancel()
	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled List error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled caller was not released")
	}

	f.unblock()
	if err := <-errs; err != nil {
		t.Fatalf("fetching caller: %v", err)
	}
}

// TestModelCatalogRequiresFetcher checks the programming error is reported
// rather than answered with an empty list that looks like an empty catalog.
func TestModelCatalogRequiresFetcher(t *testing.T) {
	catalog := NewModelCatalog()
	if _, err := catalog.List(context.Background(), "k", false, nil); !errors.Is(err, ErrNilModelFetcher) {
		t.Fatalf("List without a fetcher = %v, want ErrNilModelFetcher", err)
	}
}
