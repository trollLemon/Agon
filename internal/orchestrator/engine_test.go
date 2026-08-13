package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/trollLemon/agon/internal/tools"
)

// scriptStep describes one canned model reply for the fake client, keyed by
// the sequence in which a role is called.
type scriptStep struct {
	content   string
	toolCalls []ToolCallRequest
	err       error
}

// fakeClient is a deterministic, in-memory ChatClient for orchestrator unit
// tests. Each role has its own queue of scripted steps consumed in order;
// calling a role more times than it has steps repeats its last step (or, if
// none, returns an empty final message).
type fakeClient struct {
	mu    sync.Mutex
	steps map[Role][]scriptStep
	calls map[Role]int

	// lastToolSpecs records the tool specs offered on the most recent call
	// per role, so tests can assert tools were (or weren't) exposed.
	lastToolSpecs map[Role][]tools.Spec
	lastHistory   map[Role][]ChatMessage
}

func newFakeClient(steps map[Role][]scriptStep) *fakeClient {
	return &fakeClient{
		steps:         steps,
		calls:         make(map[Role]int),
		lastToolSpecs: make(map[Role][]tools.Spec),
		lastHistory:   make(map[Role][]ChatMessage),
	}
}

func (f *fakeClient) ChatStreaming(ctx context.Context, role Role, messages []ChatMessage, toolSpecs []tools.Spec) (<-chan StreamEvent, error) {
	f.mu.Lock()
	f.lastToolSpecs[role] = toolSpecs
	f.lastHistory[role] = append([]ChatMessage(nil), messages...)
	idx := f.calls[role]
	f.calls[role]++
	steps := f.steps[role]
	f.mu.Unlock()

	var step scriptStep
	switch {
	case idx < len(steps):
		step = steps[idx]
	case len(steps) > 0:
		step = steps[len(steps)-1]
	default:
		step = scriptStep{content: ""}
	}

	ch := make(chan StreamEvent, 4)
	go func() {
		defer close(ch)
		if step.err != nil {
			ch <- StreamEvent{Err: step.err}
			return
		}
		if step.content != "" {
			ch <- StreamEvent{ContentDelta: step.content}
		}
		if len(step.toolCalls) > 0 {
			ch <- StreamEvent{ToolCalls: step.toolCalls}
		}
	}()
	return ch, nil
}

func (f *fakeClient) callCount(role Role) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[role]
}

func (f *fakeClient) toolSpecsSeen(role Role) []tools.Spec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastToolSpecs[role]
}

// alwaysToolClient loops forever requesting the same tool call, used to
// exercise the max-tool-iterations bound.
type alwaysToolClient struct{ calls int }

func (c *alwaysToolClient) ChatStreaming(ctx context.Context, role Role, messages []ChatMessage, toolSpecs []tools.Spec) (<-chan StreamEvent, error) {
	c.calls++
	ch := make(chan StreamEvent, 1)
	go func() {
		defer close(ch)
		ch <- StreamEvent{ToolCalls: []ToolCallRequest{{ID: fmt.Sprintf("t%d", c.calls), Name: "read_file", Arguments: map[string]any{"path": "f.txt"}}}}
	}()
	return ch, nil
}

// blockingClient blocks until ctx is done, to exercise abort during a
// streaming call.
type blockingClient struct{ started chan struct{} }

func newBlockingClient() *blockingClient { return &blockingClient{started: make(chan struct{}, 8)} }

func (c *blockingClient) ChatStreaming(ctx context.Context, role Role, messages []ChatMessage, toolSpecs []tools.Spec) (<-chan StreamEvent, error) {
	c.started <- struct{}{}
	ch := make(chan StreamEvent)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func waitFor(t interface{ Fatalf(string, ...any) }, cond func() bool, timeout time.Duration, msg string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}
