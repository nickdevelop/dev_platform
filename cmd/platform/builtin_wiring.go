package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"devplatform/internal/app/agentdata"
	"devplatform/internal/app/modelconfig"
	"devplatform/internal/core/agents"
	"devplatform/internal/providers/builtin"
)

// The glue types below bind the app layer's dependencies onto the builtin
// adapter. They live in the composition root because the adapter packages may
// not import the app layer and the app layer may not import an adapter.

// builtinModelResolver maps the platform's shared OpenAI-compatible model
// services onto the builtin agent's resolver contract. A coding agent needs
// function calling, so tool-capable models surface first; when no model
// carries the "supports tools" flag at all, the catalog falls back to every
// usable text model instead of going empty — the studio flag defaults off and
// many OpenAI-compatible upstreams answer tool calls regardless. A model that
// really cannot call tools only fails at turn time with a model_error.
type builtinModelResolver struct {
	service *modelconfig.Service
}

func newBuiltinModelResolver(service *modelconfig.Service) builtinModelResolver {
	return builtinModelResolver{service: service}
}

// catalogEntry couples one platform model with the catalog id the builtin
// agent surfaces.
type catalogEntry struct {
	service modelconfig.ServiceDTO
	model   modelconfig.ModelDTO
	// id is the catalog id: the wire model id when it is unique across the
	// platform's services, otherwise the "provider/model" form.
	id string
}

// entries flattens the platform model catalog into the builtin agent's
// entries in one pass, so listing and resolving agree on ids.
func (r builtinModelResolver) entries() ([]catalogEntry, error) {
	services, err := r.service.List()
	if err != nil {
		return nil, err
	}
	entries := make([]catalogEntry, 0)
	for _, service := range services {
		for _, model := range service.Models {
			if !model.Usable || !model.SupportsText {
				continue
			}
			entries = append(entries, catalogEntry{service: service, model: model, id: model.ModelID})
		}
	}
	// A wire id that two services share becomes ambiguous; qualify those ids
	// with the provider the way the other backends do ("provider/model").
	used := make(map[string]int, len(entries))
	for _, entry := range entries {
		used[entry.model.ModelID]++
	}
	for i := range entries {
		if used[entries[i].model.ModelID] > 1 {
			entries[i].id = entries[i].service.Name + "/" + entries[i].model.ModelID
		}
	}
	return entries, nil
}

// ListModels returns the backend catalog: every usable text model in the
// platform configuration, tool-capable ones first. A model that lacks the
// "supports tools" flag (which defaults off for new platform models) still
// surfaces so the fallback agent stays usable; one that truly cannot call
// tools only fails at turn time with a model_error. Catalog ids are wire
// model ids (qualified with the provider when two services share one),
// matching the platform's "provider/model" selection style everywhere else.
func (r builtinModelResolver) ListModels(ctx context.Context) ([]agents.ModelInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := r.entries()
	if err != nil {
		return nil, err
	}
	toolModels := make([]agents.ModelInfo, 0, len(entries))
	textModels := make([]agents.ModelInfo, 0, len(entries))
	for _, entry := range entries {
		info := builtinModelInfo(entry)
		if entry.model.SupportsTools {
			toolModels = append(toolModels, info)
		} else {
			textModels = append(textModels, info)
		}
	}
	return append(toolModels, textModels...), nil
}

// builtinModelInfo renders one catalog entry into a backend model row.
func builtinModelInfo(entry catalogEntry) agents.ModelInfo {
	name := entry.model.Name
	if name == "" || name == entry.model.ModelID {
		name = entry.service.Name + " / " + entry.model.ModelID
	}
	info := agents.ModelInfo{ID: entry.id, Name: name, Provider: entry.service.Name, SupportsImages: entry.model.SupportsImage}
	if entry.model.ContextTokens > 0 {
		limit := entry.model.ContextTokens
		info.ContextLimit = &limit
	}
	return info
}

// ResolveModel maps a catalog id onto the endpoint the completer calls. It
// accepts the composite form ("provider/model"), a bare unique wire id, and —
// for sessions that still carry them — a numeric database id from older
// snapshots; it then resolves the flagged row exactly.
func (r builtinModelResolver) ResolveModel(ctx context.Context, modelRef string) (builtin.ModelEndpoint, error) {
	if err := ctx.Err(); err != nil {
		return builtin.ModelEndpoint{}, err
	}
	entries, err := r.entries()
	if err != nil {
		return builtin.ModelEndpoint{}, fmt.Errorf("builtin: resolve model %q: %w", modelRef, err)
	}
	dbID := uint(0)
	candidates := 0
	for _, entry := range entries {
		if entry.id != modelRef && entry.model.ModelID != modelRef &&
			strconv.FormatUint(uint64(entry.model.ID), 10) != modelRef {
			continue
		}
		candidates++
		dbID = entry.model.ID
	}
	switch {
	case candidates == 0:
		return builtin.ModelEndpoint{}, fmt.Errorf("builtin: model %q is not available; refresh the model settings", modelRef)
	case candidates > 1:
		return builtin.ModelEndpoint{}, fmt.Errorf("builtin: model %q is ambiguous, pick it again from the model list", modelRef)
	}
	resolved, err := r.service.ResolveModel(dbID)
	if err != nil {
		return builtin.ModelEndpoint{}, fmt.Errorf("builtin: resolve model %q: %w", modelRef, err)
	}
	return builtin.ModelEndpoint{
		BaseURL:       resolved.BaseURL,
		APIKey:        resolved.APIKey,
		ModelID:       resolved.ModelID,
		SupportsImage: resolved.SupportsImage,
	}, nil
}

// newBuiltinTranscriptStore adapts the agentdata transcript service onto the
// builtin adapter's store contract, translating between the two record
// vocabularies (payload bytes are copied both ways).
func newBuiltinTranscriptStore(service *agentdata.Service) *builtinTranscriptStore {
	return &builtinTranscriptStore{service: service}
}

type builtinTranscriptStore struct {
	service *agentdata.Service
}

func (s *builtinTranscriptStore) CreateSession(ctx context.Context, session builtin.TranscriptSession) error {
	return s.service.CreateSession(ctx, session.SessionID, session.Directory, session.Title, session.CreatedAt)
}

func (s *builtinTranscriptStore) SetModel(ctx context.Context, sessionID, model string) error {
	return s.service.SetModel(ctx, sessionID, model)
}

func (s *builtinTranscriptStore) Append(ctx context.Context, sessionID string, records []builtin.TranscriptRecord) error {
	mapped := make([]agentdata.Record, 0, len(records))
	for _, record := range records {
		payload := make([]byte, len(record.Payload))
		copy(payload, record.Payload)
		mapped = append(mapped, agentdata.Record{
			Seq:       record.Seq,
			Kind:      string(record.Kind),
			Content:   record.Content,
			Payload:   payload,
			CreatedAt: record.CreatedAt,
		})
	}
	return s.service.Append(ctx, sessionID, mapped)
}

func (s *builtinTranscriptStore) Load(ctx context.Context, sessionID string) ([]builtin.TranscriptRecord, error) {
	records, err := s.service.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	mapped := make([]builtin.TranscriptRecord, 0, len(records))
	for _, record := range records {
		payload := make([]byte, len(record.Payload))
		copy(payload, record.Payload)
		mapped = append(mapped, builtin.TranscriptRecord{
			Seq:       record.Seq,
			Kind:      builtin.TranscriptKind(record.Kind),
			Content:   record.Content,
			Payload:   payload,
			CreatedAt: record.CreatedAt,
		})
	}
	return mapped, nil
}

func (s *builtinTranscriptStore) Exists(ctx context.Context, sessionID string) (bool, error) {
	return s.service.Exists(ctx, sessionID)
}

// quickSessionBusy adapts the builtin adapter's activity check onto the
// agentdata gate signature.
func quickSessionBusy(adapter *builtin.Adapter) func(sessionID string) bool {
	return func(sessionID string) bool { return adapter.SessionBusy(sessionID) }
}

// compile-time anchor that the glue stays a replay-free translation only.
var _ = time.Time{}
