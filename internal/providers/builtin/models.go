package builtin

import (
	"context"

	"devplatform/internal/core/agents"
)

// ModelEndpoint is one resolved model call target: everything the completer
// needs to stream a chat completion. The key stays inside the process.
type ModelEndpoint struct {
	// BaseURL is the OpenAI-compatible service root, without /chat/completions.
	BaseURL string
	// APIKey is the service credential; empty means unauthenticated.
	APIKey string
	// ModelID is the wire model name sent to the service.
	ModelID string
	// SupportsImage reports whether the model accepts image parts.
	SupportsImage bool
}

// ModelResolver answers which model endpoints exist and how a model choice
// maps onto a call target. The app layer implements it over the platform's
// shared OpenAI-compatible model configuration, bound at the composition root.
type ModelResolver interface {
	// ListModels returns every enabled model as ModelInfo. The catalog is
	// platform-wide and ignores the working directory.
	ListModels(ctx context.Context) ([]agents.ModelInfo, error)
	// ResolveModel maps a model reference (the ModelInfo id the platform
	// sent with the turn) onto an endpoint. An unknown or unavailable model
	// is an error.
	ResolveModel(ctx context.Context, modelRef string) (ModelEndpoint, error)
}
