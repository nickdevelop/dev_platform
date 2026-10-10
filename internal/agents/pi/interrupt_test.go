package pi

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"devplatform/internal/agents"
)

func TestParseModelList(t *testing.T) {
	out := "provider           model                            context  max-out  thinking  images\n" +
		"occ-command-code   deepseek/deepseek-v4.1-flash     1M       32K      no        yes\n" +
		"occ-commandcode-a  claude-haiku-5-5                 1M       128K     no        yes\n"
	want := []agents.ModelInfo{
		{ID: "occ-command-code/deepseek/deepseek-v4.1-flash", Name: "deepseek/deepseek-v4.1-flash", Provider: "occ-command-code"},
		{ID: "occ-commandcode-a/claude-haiku-5-5", Name: "claude-haiku-5-5", Provider: "occ-commandcode-a"},
	}
	if got := parseModelList(out); !reflect.DeepEqual(got, want) {
		t.Errorf("parseModelList() = %+v, want %+v", got, want)
	}
}

func TestParseModelListEmpty(t *testing.T) {
	if got := parseModelList("provider model\n"); len(got) != 0 {
		t.Errorf("parseModelList(header only) = %+v, want none", got)
	}
}

func TestAdapterListModelsReadsPiCatalog(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, `if [ "$1" = "--list-models" ]; then printf 'provider model\nacme gpt-1\n'; exit 0; fi
exit 0
`)
	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.ListModels(ctx, inst.ID)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	want := []agents.ModelInfo{{ID: "acme/gpt-1", Name: "gpt-1", Provider: "acme"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListModels() = %+v, want %+v", got, want)
	}
	if _, err := adapter.ListModels(ctx, "missing"); err == nil {
		t.Error("ListModels(unknown instance) = nil, want error")
	}
}

func TestAdapterInterruptEndsOnlyTheTargetTurn(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, sleepingFakePi)
	pidFile := filepath.Join(dir, "pid.txt")
	t.Setenv("FAKE_PI_PID", pidFile)

	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.StopInstance(context.Background(), inst.ID) })
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	events, err := adapter.Subscribe(ctx, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "block"}); err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(waitForFile(t, pidFile))))
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}

	if err := adapter.Interrupt(ctx, *ref); err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}

	// The aborted turn settles on the stream; no error is published for it.
	deadline := time.After(5 * time.Second)
	for settled := false; !settled; {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("subscription closed before the aborted turn settled")
			}
			if ev.Type == agents.EventError {
				t.Fatalf("interrupt published an error event: %+v", ev.Error)
			}
			settled = ev.Type == agents.EventLifecycle && ev.Lifecycle != nil &&
				ev.Lifecycle.State == "agent_settled" && ev.Lifecycle.Detail == "aborted"
		case <-deadline:
			t.Fatal("aborted turn never settled")
		}
	}

	if runtime.GOOS != "windows" {
		if err := syscall.Kill(pid, 0); err == nil {
			t.Errorf("process %d still alive after interrupt", pid)
		}
	}

	// The instance keeps running, so the session accepts the next turn.
	if err := adapter.SendMessage(ctx, *ref, agents.SendMessageRequest{Text: "again"}); err != nil {
		t.Errorf("SendMessage after interrupt = %v, want nil", err)
	}
}

func TestAdapterInterruptIdleSessionIsNoop(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakePi(t, dir, sleepingFakePi)
	adapter := newTestAdapter(t, fake, "")
	ctx := context.Background()
	inst, err := adapter.StartInstance(ctx, agents.StartInstanceRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.StopInstance(context.Background(), inst.ID) })
	ref, err := adapter.CreateSession(ctx, inst.ID, agents.CreateSessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Interrupt(ctx, *ref); err != nil {
		t.Errorf("Interrupt(idle session) = %v, want nil", err)
	}
}
