package events

import (
	"context"
	"encoding/json"
	"time"
)

// Standard channel names for multiplexed events.
const (
	ChannelSessions = "sessions"
	ChannelChat     = "chat"
	ChannelTasks    = "tasks"
	ChannelSystem   = "system"
)

// EventEnvelope is the universal wire format for all events streamed from the platform.
type EventEnvelope struct {
	ID        uint64          `json:"id,omitempty"`
	BusID     string          `json:"busId,omitempty"`
	Channel   string          `json:"channel"`
	TargetID  string          `json:"targetId"`
	EventType string          `json:"eventType"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// SubscribeRequest is the incoming payload for attaching a channel/target to a client bus.
type SubscribeRequest struct {
	Channel     string          `json:"channel"`
	TargetID    string          `json:"targetId"`
	LastEventID uint64          `json:"lastEventId,omitempty"`
	ProjectID   uint            `json:"projectId,omitempty"`
	Extra       json.RawMessage `json:"extra,omitempty"`
}

// SubscribeResponse is returned when a subscription is established, optionally carrying an initial snapshot.
type SubscribeResponse struct {
	Channel  string `json:"channel"`
	TargetID string `json:"targetId"`
	Snapshot any    `json:"snapshot,omitempty"`
}

// ChannelHandler allows specific domain services to hook into subscription lifecycles
// (e.g. creating instance listeners or returning initial state snapshots).
type ChannelHandler interface {
	OnSubscribe(ctx context.Context, busID string, req SubscribeRequest) (snapshot any, err error)
	OnUnsubscribe(ctx context.Context, busID string, channel string, targetID string) error
}

func NowMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
