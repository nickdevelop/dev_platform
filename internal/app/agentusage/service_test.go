package agentusage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"devplatform/internal/core/agents"
)

type fakeRegistry struct {
	adapters map[agents.BackendID]agents.AgentAdapter
}

func (r *fakeRegistry) Get(id agents.BackendID) (agents.AgentAdapter, bool) {
	a, ok := r.adapters[id]
	return a, ok
}

type fakeAdapterWithUsage struct {
	agents.AgentAdapter
	report *agents.UsageReport
	err    error
}

func (f *fakeAdapterWithUsage) GetUsage(ctx context.Context) (*agents.UsageReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.report, nil
}

type fakeAdapterPlain struct {
	agents.AgentAdapter
}

func TestGetUsage(t *testing.T) {
	ctx := context.Background()
	testBackend := agents.BackendID("builtin")

	t.Run("unknown backend", func(t *testing.T) {
		svc := NewService(&fakeRegistry{adapters: map[agents.BackendID]agents.AgentAdapter{}})
		_, err := svc.GetUsage(ctx, testBackend)
		if !errors.Is(err, ErrUnknownBackend) {
			t.Errorf("got error %v, want %v", err, ErrUnknownBackend)
		}
	})

	t.Run("unsupported capability", func(t *testing.T) {
		svc := NewService(&fakeRegistry{
			adapters: map[agents.BackendID]agents.AgentAdapter{
				testBackend: &fakeAdapterPlain{},
			},
		})
		_, err := svc.GetUsage(ctx, testBackend)
		if !errors.Is(err, ErrUnsupportedCapability) {
			t.Errorf("got error %v, want %v", err, ErrUnsupportedCapability)
		}
	})

	t.Run("success", func(t *testing.T) {
		expected := &agents.UsageReport{
			Backend:     testBackend,
			Description: "test quota",
			UpdatedAt:   time.Now().UTC(),
		}
		svc := NewService(&fakeRegistry{
			adapters: map[agents.BackendID]agents.AgentAdapter{
				testBackend: &fakeAdapterWithUsage{report: expected},
			},
		})
		report, err := svc.GetUsage(ctx, testBackend)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if report.Description != expected.Description {
			t.Errorf("got %q, want %q", report.Description, expected.Description)
		}
	})
}

func TestRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testBackend := agents.BackendID("builtin")
	expected := &agents.UsageReport{
		Backend:     testBackend,
		Description: "routed quota",
		UpdatedAt:   time.Now().UTC(),
	}

	svc := NewService(&fakeRegistry{
		adapters: map[agents.BackendID]agents.AgentAdapter{
			testBackend: &fakeAdapterWithUsage{report: expected},
		},
	})

	router := gin.New()
	svc.RegisterRoutes(router)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/agent-backends/"+string(testBackend)+"/usage", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
}
