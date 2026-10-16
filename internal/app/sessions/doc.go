// Package sessions owns the platform-side session layer of the agent
// workbench: the durable index that maps a platform session id to the
// (project, backend, instance, agent-native session id) it belongs to, the
// discovery/create/continue/send/stop operations built on the adapter
// registry, and the normalized Server-Sent Events stream the workbench
// consumes.
//
// The package persists only the index and ID mappings. Agent-native session
// history stays inside the agent; the platform never stores it.
package sessions
