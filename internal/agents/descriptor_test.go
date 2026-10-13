package agents

import (
	"errors"
	"testing"
)

func TestCatalogRejectsInvalidEntries(t *testing.T) {
	cases := map[string][]BackendDescriptor{
		"unknown id":   {{ID: BackendID("ghost"), Binary: "ghost"}},
		"empty binary": {{ID: BackendPi}},
		"duplicate id": {{ID: BackendPi, Binary: "pi"}, {ID: BackendPi, Binary: "pi"}},
		"empty id":     {{Binary: "x"}},
	}
	for name, descriptors := range cases {
		if _, err := NewCatalog(descriptors...); !errors.Is(err, ErrCatalogEntry) {
			t.Errorf("%s: NewCatalog err = %v, want ErrCatalogEntry", name, err)
		}
	}
}

func TestCatalogLookupAndOrder(t *testing.T) {
	catalog, err := NewCatalog(
		BackendDescriptor{ID: BackendPi, Label: "Pi Agent", Binary: "pi", Order: 2},
		BackendDescriptor{ID: BackendOpenCodeV2, Label: "OpenCode", Binary: "opencode", Order: 1},
	)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	all := catalog.All()
	if len(all) != 2 || all[0].ID != BackendOpenCodeV2 || all[1].ID != BackendPi {
		t.Fatalf("All() order = %+v, want opencode then pi", all)
	}
	if d, ok := catalog.Get(BackendPi); !ok || d.Binary != "pi" {
		t.Fatalf("Get(pi) = %+v, %v", d, ok)
	}
	if _, ok := catalog.Get(BackendID("ghost")); ok {
		t.Fatal("Get on an unknown id must report false")
	}
}

func TestCatalogAllReturnsCopy(t *testing.T) {
	catalog, err := NewCatalog(BackendDescriptor{ID: BackendPi, Binary: "pi"})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	catalog.All()[0].Binary = "mutated"
	if d, _ := catalog.Get(BackendPi); d.Binary != "pi" {
		t.Fatalf("catalog mutated through All(): binary = %q", d.Binary)
	}
}
