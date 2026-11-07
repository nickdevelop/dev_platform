// Package agentdata owns the platform-persisted session transcripts of agent
// backends that declare ownership of their session data (the embedded agent).
//
// Regular agent backends keep their agent-native history in their own
// storage; this package is the single home for transcripts the platform
// stores itself. The builtin agent adapter writes through the
// provider-defined TranscriptStore contract; the settings center manages the
// data through the REST routes registered here (list, delete, clear). The
// package is backend-neutral: which backend it serves is bound at the
// composition root and never hardcoded.
package agentdata
