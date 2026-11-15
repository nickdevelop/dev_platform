package agents

// DeliveryGate is the single admission rule for normalized events. Adapters
// run every event they produce through it at normalization time, so events
// the workbench never shows (thinking, ordinary tool activity, events the
// adapter could not classify) never reach the instance replay buffer, the
// session stream, or history. Human interaction stays: messages, questions
// and their results, confirmations, turn boundaries, lifecycle and errors.
//
// A gate is stateful: it remembers question call ids so the result of a
// question tool is admitted while the results of other tools are not. Each
// independent stream (one live subscription, one history read) owns its gate.
type DeliveryGate struct {
	questionCallIDs map[string]struct{}
}

// NewDeliveryGate returns a gate with no open question calls.
func NewDeliveryGate() *DeliveryGate {
	return &DeliveryGate{questionCallIDs: make(map[string]struct{})}
}

// Admit reports whether ev belongs to the deliverable stream.
func (g *DeliveryGate) Admit(ev NormalizedEvent) bool {
	switch ev.Type {
	case EventMessage, EventInteraction, EventLifecycle, EventError:
		return true
	case EventTurn:
		if ev.Turn != nil && ev.Turn.Phase == TurnEnded {
			g.questionCallIDs = make(map[string]struct{})
		}
		return true
	case EventToolCall:
		if ev.ToolCall == nil || !IsQuestionTool(ev.ToolCall.Name) {
			return false
		}
		if ev.ToolCall.CallID != "" {
			g.questionCallIDs[ev.ToolCall.CallID] = struct{}{}
		}
		return true
	case EventToolResult:
		if ev.ToolResult == nil {
			return false
		}
		_, ok := g.questionCallIDs[ev.ToolResult.CallID]
		return ok
	default:
		return false
	}
}

// AdmitAll returns the admitted subset of events, preserving order. It is the
// batch form for history reads, which materialize a whole transcript at once.
func (g *DeliveryGate) AdmitAll(events []NormalizedEvent) []NormalizedEvent {
	out := make([]NormalizedEvent, 0, len(events))
	for _, ev := range events {
		if g.Admit(ev) {
			out = append(out, ev)
		}
	}
	return out
}
