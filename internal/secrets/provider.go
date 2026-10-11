package secrets

import "context"

// SecretStore is the secret-value contract the resource manager depends on.
// It is deliberately small so a caller that never handles credentials can
// take a nil store, and so tests can substitute an in-memory fake.
type SecretStore interface {
	// Set stores value under the opaque id, replacing any previous value.
	Set(ctx context.Context, id, value string) error
	// Get returns the value stored under id.
	Get(ctx context.Context, id string) (string, error)
	// Delete removes the value stored under id.
	Delete(ctx context.Context, id string) error
}
