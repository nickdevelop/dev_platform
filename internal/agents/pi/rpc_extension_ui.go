package pi

import (
	"sync"

	"devplatform/internal/agents"
)

// rpcExtensionUIRequest is one record from the extension UI subprotocol:
// a request from pi for the client to render a dialog or fire-and-forget
// notification. Dialog methods (select/confirm/input/editor) block the agent
// until the client answers with an extension_ui_response carrying the same id.
type rpcExtensionUIRequest struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Method string `json:"method"`

	Title   string `json:"title"`
	Message string `json:"message"`

	Options     []string `json:"options"`
	Placeholder string   `json:"placeholder"`
	Prefill     string   `json:"prefill"`
	Timeout     int      `json:"timeout"`

	NotifyType string `json:"notifyType"`

	StatusKey  string `json:"statusKey"`
	StatusText string `json:"statusText"`

	WidgetKey       string   `json:"widgetKey"`
	WidgetLines     []string `json:"widgetLines"`
	WidgetPlacement string   `json:"widgetPlacement"`

	Text string `json:"text"`
}

// dialog reports whether the request blocks pi until the client answers. The
// fire-and-forget methods never expect a response.
func (r *rpcExtensionUIRequest) dialog() bool {
	switch r.Method {
	case "select", "confirm", "input", "editor":
		return true
	default:
		return false
	}
}

// rpcExtensionUIResponse is the client's answer to a dialog request. Exactly
// one of value, confirmed or cancelled is meaningful, matching the method.
type rpcExtensionUIResponse struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	Value     *string `json:"value,omitempty"`
	Confirmed *bool   `json:"confirmed,omitempty"`
	Cancelled bool    `json:"cancelled,omitempty"`
}

// piPermission is one pending confirmation the platform can answer. It keeps
// the request id and the original title/message so ListPermissions can render
// a card without asking pi again (pi has no native listing command).
type piPermission struct {
	request agents.PermissionRequest
}

// piPermissions tracks the confirm dialogs one RPC child is waiting on. It is
// owned by the child; a reclaimed child drops its pending set, so an answer
// arriving later is correctly reported as expired.
type piPermissions struct {
	mu      sync.Mutex
	pending map[string]*piPermission
}

func newPiPermissions() *piPermissions {
	return &piPermissions{pending: make(map[string]*piPermission)}
}

// add records a pending confirmation.
func (p *piPermissions) add(request agents.PermissionRequest) {
	p.mu.Lock()
	p.pending[request.ID] = &piPermission{request: request}
	p.mu.Unlock()
}

// list returns the pending confirmations for the session, oldest ids first not
// guaranteed: map iteration is unordered, but the workbench renders all of
// them and ordering is not part of the contract.
func (p *piPermissions) list(sessionID string) []agents.PermissionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	requests := make([]agents.PermissionRequest, 0, len(p.pending))
	for _, entry := range p.pending {
		request := entry.request
		request.SessionID = sessionID
		requests = append(requests, request)
	}
	return requests
}

// take removes and returns the pending confirmation with the given id. It
// reports false when the id is unknown or already answered, which the caller
// turns into agents.ErrReplyExpired.
func (p *piPermissions) take(id string) (*piPermission, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.pending[id]
	if !ok {
		return nil, false
	}
	delete(p.pending, id)
	return entry, true
}

// clear drops every pending confirmation.
func (p *piPermissions) clear() {
	p.mu.Lock()
	p.pending = make(map[string]*piPermission)
	p.mu.Unlock()
}
