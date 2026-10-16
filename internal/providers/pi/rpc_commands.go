package pi

import (
	"encoding/json"
	"fmt"
	"strings"

	"devplatform/internal/core/agents"
)

// RPC command names accepted on stdin in `pi --mode rpc`. Only the commands
// the adapter actually uses are listed; the protocol is much larger and its
// reference is docs/rpc-commands.md in the pi installation. Keeping the names
// in one file makes a protocol drift easy to audit.
const (
	rpcCommandPrompt             = "prompt"
	rpcCommandAbort              = "abort"
	rpcCommandGetState           = "get_state"
	rpcCommandSetSessionName     = "set_session_name"
	rpcCommandGetAvailableModels = "get_available_models"
)

// rpcCommand is the wire shape of every command the adapter sends. The
// protocol is a discriminated union on "type"; unused fields are omitted so a
// record carries only the command it names. Every command accepts an optional
// string id, echoed by its response, so concurrent commands are correlated by
// id rather than by order.
type rpcCommand struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type"`

	// prompt: the user text and, when a run is already streaming, how pi
	// should treat the message ("steer" or "followUp").
	Message           string            `json:"message,omitempty"`
	StreamingBehavior string            `json:"streamingBehavior,omitempty"`
	Images            []rpcImageContent `json:"images,omitempty"`

	// set_session_name: the new display name.
	Name string `json:"name,omitempty"`

	// set_model: the provider and model id to switch to.
	Provider string `json:"provider,omitempty"`
	ModelID  string `json:"modelId,omitempty"`
}

// rpcResponse is one stdout response record. A successful response repeats the
// command's id and carries command-specific data; a failed one carries an
// error string.
type rpcResponse struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// rpcStateData is the get_state response body (the subset the adapter uses).
type rpcStateData struct {
	Model               json.RawMessage `json:"model"`
	IsStreaming         bool            `json:"isStreaming"`
	IsCompacting        bool            `json:"isCompacting"`
	SessionFile         string          `json:"sessionFile"`
	SessionID           string          `json:"sessionId"`
	SessionName         string          `json:"sessionName"`
	PendingMessageCount int             `json:"pendingMessageCount"`
}

// rpcModelData is one model in a get_available_models response.
type rpcModelData struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type rpcImageContent struct {
	Type     string `json:"type"` // "image"
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// newPromptCommand builds a prompt command. behavior is empty for a normal
// prompt and "followUp" when the session is already streaming, which queues
// the message instead of failing (pi requires an explicit streaming behavior
// mid-run).
func newPromptCommand(id, message, behavior string, images []agents.ImageAttachment) rpcCommand {
	var rpcImages []rpcImageContent
	for _, img := range images {
		if img.Data != "" {
			rpcImages = append(rpcImages, rpcImageContent{
				Type:     "image",
				Data:     img.Data,
				MimeType: img.MediaType,
			})
		}
	}
	return rpcCommand{
		ID:                id,
		Type:              rpcCommandPrompt,
		Message:           message,
		StreamingBehavior: behavior,
		Images:            rpcImages,
	}
}

// newAbortCommand builds an abort command. abort waits for the session to
// become idle before its response, so callers bound the wait themselves.
func newAbortCommand(id string) rpcCommand {
	return rpcCommand{ID: id, Type: rpcCommandAbort}
}

// newGetStateCommand builds a get_state command, used to bind a freshly
// started child to its session and to learn the on-disk session file.
func newGetStateCommand(id string) rpcCommand {
	return rpcCommand{ID: id, Type: rpcCommandGetState}
}

// newSetSessionNameCommand builds a set_session_name command.
func newSetSessionNameCommand(id, name string) rpcCommand {
	return rpcCommand{ID: id, Type: rpcCommandSetSessionName, Name: name}
}

// newGetAvailableModelsCommand builds a get_available_models command.
func newGetAvailableModelsCommand(id string) rpcCommand {
	return rpcCommand{ID: id, Type: rpcCommandGetAvailableModels}
}

// decodeResponse unmarshals a response record's typed data body.
func decodeResponseData[T any](response *rpcResponse) (T, error) {
	var data T
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return data, nil
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return data, fmt.Errorf("pi: decode %s response: %w", response.Command, err)
	}
	return data, nil
}

// parseAvailableModels maps a get_available_models response body onto the
// platform's ModelInfo. The wire model id is the provider-scoped value
// SendMessageRequest.Model accepts, matching the id parseModelList derives
// from `--list-models`.
func parseAvailableModels(raw json.RawMessage) ([]agents.ModelInfo, error) {
	if len(raw) == 0 {
		return []agents.ModelInfo{}, nil
	}
	var payload struct {
		Models []rpcModelData `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("pi: decode available models: %w", err)
	}
	models := make([]agents.ModelInfo, 0, len(payload.Models))
	for _, model := range payload.Models {
		id := model.ID
		if model.Provider != "" && !strings.Contains(id, "/") {
			id = model.Provider + "/" + id
		}
		if id == "" {
			continue
		}
		models = append(models, agents.ModelInfo{
			ID:       id,
			Name:     firstNonEmpty(model.Name, model.ID),
			Provider: model.Provider,
		})
	}
	return models, nil
}

// rpcModelID extracts a model's provider-scoped id from a get_state response's
// model object. It returns an empty string when no model is selected.
func rpcModelID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var model rpcModelData
	if err := json.Unmarshal(raw, &model); err != nil {
		return ""
	}
	if model.ID == "" {
		return ""
	}
	if model.Provider != "" && !strings.Contains(model.ID, "/") {
		return model.Provider + "/" + model.ID
	}
	return model.ID
}
