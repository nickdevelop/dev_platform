package agents

import (
	"errors"
	"fmt"
	"sort"
)

// ErrCatalogEntry reports a descriptor the catalog refuses: an unknown backend
// id, a duplicate id, or a missing executable name.
var ErrCatalogEntry = errors.New("agents: invalid backend descriptor")

// BackendDescriptor is the static, presentation-level description of one
// backend: what it is called, which executable it runs, whether it is on by
// default, and the version range it was tested against. Adapter packages export
// their own descriptor, so the platform's settings, diagnostics and UI read
// these facts from one place instead of keeping parallel tables.
type BackendDescriptor struct {
	// ID is the backend identifier the descriptor describes.
	ID BackendID
	// Label is the display name, such as "OpenCode" or "Pi Agent".
	Label string
	// Binary is the default executable name resolved from PATH.
	Binary string
	// DefaultEnabled is the platform-wide enablement before an operator saves
	// a choice.
	DefaultEnabled bool
	// Order is the presentation order; lower values come first.
	Order int
	// Tested is the version the adapter was verified against.
	Tested string
	// Minimum and Maximum bound the supported version range as [Minimum,
	// Maximum).
	Minimum, Maximum string
	// ServiceSupported reports whether the backend runs a machine-level
	// service that the operator can start and stop.
	ServiceSupported bool
}

// Catalog is the immutable set of backend descriptors the platform knows. It
// is built once at the composition root and shared read-only.
type Catalog struct {
	byID    map[BackendID]BackendDescriptor
	ordered []BackendDescriptor
}

// NewCatalog validates the descriptors and returns the catalog. It refuses an
// invalid backend id, a duplicate id, and a descriptor without an executable
// name, so a wiring mistake fails at startup.
func NewCatalog(descriptors ...BackendDescriptor) (*Catalog, error) {
	c := &Catalog{byID: make(map[BackendID]BackendDescriptor, len(descriptors))}
	for _, d := range descriptors {
		if !d.ID.Valid() {
			return nil, fmt.Errorf("%w: unknown backend %q", ErrCatalogEntry, d.ID)
		}
		if d.Binary == "" {
			return nil, fmt.Errorf("%w: backend %q has no executable name", ErrCatalogEntry, d.ID)
		}
		if _, exists := c.byID[d.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate backend %q", ErrCatalogEntry, d.ID)
		}
		c.byID[d.ID] = d
		c.ordered = append(c.ordered, d)
	}
	sort.SliceStable(c.ordered, func(i, j int) bool { return c.ordered[i].Order < c.ordered[j].Order })
	return c, nil
}

// Get returns the descriptor for id, and whether the catalog knows it.
func (c *Catalog) Get(id BackendID) (BackendDescriptor, bool) {
	if c == nil {
		return BackendDescriptor{}, false
	}
	d, ok := c.byID[id]
	return d, ok
}

// All returns every descriptor in presentation order.
func (c *Catalog) All() []BackendDescriptor {
	if c == nil {
		return nil
	}
	out := make([]BackendDescriptor, len(c.ordered))
	copy(out, c.ordered)
	return out
}

// SupportedVersion returns the version range the descriptor declares, in the
// shape the diagnostics checks classify against.
func (d BackendDescriptor) SupportedVersion() SupportedVersion {
	return SupportedVersion{Tested: d.Tested, Minimum: d.Minimum, Maximum: d.Maximum}
}
