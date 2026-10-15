package resources

import (
	"strings"

	"devplatform/internal/core/agents"
	"devplatform/internal/app/database"
)

// definitionFromModel maps the persistence row to the JSON-stable contract
// DTO. GORM models are never serialized directly.
func definitionFromModel(row database.Resource) ResourceDefinition {
	return ResourceDefinition{
		ID:          row.ID,
		Kind:        ResourceKind(row.Kind),
		Name:        row.Name,
		Description: row.Description,
		Content:     row.Content,
		Enabled:     row.Enabled,
		PromptScope: PromptScope(row.PromptScope),
		SecretRef:   row.SecretRef,
		Backends:    parseBackends(row.Backends),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

// parseBackends splits the comma-separated backend column into backend ids.
func parseBackends(value string) []agents.BackendID {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	backends := make([]agents.BackendID, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		backends = append(backends, agents.BackendID(id))
	}
	if len(backends) == 0 {
		return nil
	}
	return backends
}

// joinBackends renders backend ids into the comma-separated column form,
// preserving order and dropping duplicates.
func joinBackends(backends []agents.BackendID) string {
	if len(backends) == 0 {
		return ""
	}
	seen := make(map[agents.BackendID]bool, len(backends))
	parts := make([]string, 0, len(backends))
	for _, backend := range backends {
		if backend == "" || seen[backend] {
			continue
		}
		seen[backend] = true
		parts = append(parts, string(backend))
	}
	return strings.Join(parts, ",")
}
