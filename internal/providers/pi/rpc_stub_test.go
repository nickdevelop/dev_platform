package pi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The RPC stub is the test binary re-executed with PI_RPC_STUB=1. It mimics
// `pi --mode rpc` closely enough to drive the adapter's transport: it answers
// commands by id, streams session events, queues follow-up prompts, and can
// block or crash on demand. Keeping it in-process (rather than compiling a
// helper) makes the RPC tests fast and hermetic.

func TestMain(m *testing.M) {
	if os.Getenv("PI_RPC_STUB") == "1" {
		os.Exit(runRPCStub())
	}
	os.Exit(m.Run())
}

// stubState is the scripted pi's session state.
type stubState struct {
	sessionID  string
	sessionDir string
	scenario   string

	streaming bool
	queue     []string
	model     string
	name      string

	confirmID  string
	confirmSeq int
}

func runRPCStub() int {
	args := os.Args[1:]
	if stubArgValue(args, "--list-models") != "" || containsArg(args, "--list-models") {
		fmt.Print("provider model\nstub gpt-1\n")
		return 0
	}
	if containsArg(args, "--version") {
		fmt.Println("1.1.0")
		return 0
	}

	state := &stubState{
		sessionID:  stubArgValue(args, "--session-id"),
		sessionDir: stubArgValue(args, "--session-dir"),
		scenario:   os.Getenv("PI_RPC_STUB_SCENARIO"),
		model:      stubArgValue(args, "--model"),
		name:       stubArgValue(args, "--name"),
	}
	if logPath := os.Getenv("PI_RPC_STUB_ARGS"); logPath != "" {
		if file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintln(file, strings.Join(args, " "))
			file.Close()
		}
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var command map[string]any
		if err := json.Unmarshal(line, &command); err != nil {
			state.write(map[string]any{"type": "response", "command": "parse", "success": false, "error": "invalid json"})
			continue
		}
		state.handle(command)
	}
	return 0
}

func (s *stubState) handle(command map[string]any) {
	commandType, _ := command["type"].(string)
	id, _ := command["id"].(string)
	switch commandType {
	case "get_state":
		sessionID := s.sessionID
		if s.scenario == "getstate-mismatch" {
			sessionID = "different-session"
		}
		s.response(id, "get_state", true, map[string]any{
			"sessionId":   sessionID,
			"sessionFile": s.sessionDir + "/" + sessionID + ".jsonl",
			"isStreaming": s.streaming,
		})
	case "prompt":
		s.handlePrompt(id, command)
	case "abort":
		s.response(id, "abort", true, nil)
		if s.streaming {
			s.finishTurn(true)
		}
	case "set_model":
		provider, _ := command["provider"].(string)
		modelID, _ := command["modelId"].(string)
		s.model = modelID
		if provider != "" {
			s.model = provider + "/" + modelID
		}
		s.response(id, "set_model", true, map[string]any{"id": modelID, "provider": provider, "name": modelID})
	case "set_session_name":
		s.name, _ = command["name"].(string)
		s.response(id, "set_session_name", true, nil)
	case "get_available_models":
		s.response(id, "get_available_models", true, map[string]any{
			"models": []map[string]any{{"id": "gpt-1", "provider": "stub", "name": "gpt-1"}},
		})
	case "extension_ui_response":
		s.handleExtensionUIResponse(id)
	default:
		s.response(id, commandType, false, nil)
	}
}

func (s *stubState) handlePrompt(id string, command map[string]any) {
	message, _ := command["message"].(string)
	behavior, _ := command["streamingBehavior"].(string)

	switch s.scenario {
	case "crash":
		os.Exit(3)
	case "getstate-mismatch":
		// Not reached: binding fails before any prompt.
	}

	if s.streaming {
		if behavior == "" {
			s.write(map[string]any{"id": id, "type": "response", "command": "prompt", "success": false, "error": "Agent is already streaming; provide streamingBehavior"})
			return
		}
		s.queue = append(s.queue, message)
		s.response(id, "prompt", true, map[string]any{"disposition": "queued"})
		return
	}
	s.streaming = true
	s.response(id, "prompt", true, map[string]any{"disposition": "started"})
	s.startTurn(message)
}

func (s *stubState) startTurn(message string) {
	switch s.scenario {
	case "sleep":
		s.write(map[string]any{"type": "agent_start"})
		return
	case "confirm":
		s.confirmSeq++
		s.confirmID = "perm-" + strconv.Itoa(s.confirmSeq)
		s.write(map[string]any{"type": "agent_start"})
		s.write(map[string]any{
			"type":    "extension_ui_request",
			"id":      s.confirmID,
			"method":  "confirm",
			"title":   "Allow tool call?",
			"message": "bash: rm -rf /tmp/x",
		})
		return
	}
	s.emitStream(message)
	s.finishTurn(false)
}

// emitStream writes the representative happy-path event stream.
func (s *stubState) emitStream(message string) {
	s.write(map[string]any{"type": "agent_start"})
	s.write(map[string]any{"type": "turn_start"})
	s.write(map[string]any{"type": "message_start", "message": map[string]any{"role": "user", "content": message}})
	s.write(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "text_delta", "contentIndex": 0, "delta": "Hi "}})
	s.write(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "text_delta", "contentIndex": 0, "delta": "there"}})
	s.write(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "Hi there"}}}})
	s.write(map[string]any{"type": "tool_execution_start", "toolCallId": "call-1", "toolName": "read", "args": map[string]any{"path": "main.go"}})
	s.write(map[string]any{"type": "tool_execution_end", "toolCallId": "call-1", "toolName": "read", "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "package main"}}}, "isError": false})
}

func (s *stubState) handleExtensionUIResponse(id string) {
	if id == s.confirmID {
		s.finishTurn(false)
	}
}

// finishTurn emits agent_settled, clears the streaming flag and drains one
// queued follow-up (which starts a fresh turn).
func (s *stubState) finishTurn(aborted bool) {
	s.write(map[string]any{"type": "agent_settled", "aborted": aborted})
	s.streaming = false
	if len(s.queue) > 0 {
		next := s.queue[0]
		s.queue = s.queue[1:]
		s.streaming = true
		s.startTurn(next)
	}
}

func (s *stubState) response(id, command string, success bool, data any) {
	record := map[string]any{"id": id, "type": "response", "command": command, "success": success}
	if data != nil {
		record["data"] = data
	}
	if !success {
		record["error"] = "stub rejected " + command
	}
	s.write(record)
}

func (s *stubState) write(record any) {
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	os.Stdout.Write(append(data, '\n'))
}

func containsArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

// stubArgValue returns the value of a flag, or "" when absent.
func stubArgValue(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}
