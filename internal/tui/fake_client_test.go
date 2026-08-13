package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/agentdisc/goagentdisc/internal/orchestrator"
	"github.com/agentdisc/goagentdisc/internal/tools"
)

// fakeChatClient is a deterministic, instant ChatClient used by tui tests so
// they never touch a real model. Every role gets a short canned reply that
// mentions its role name, so tests can assert on transcript content.
type fakeChatClient struct{}

func (fakeChatClient) ChatStreaming(ctx context.Context, role orchestrator.Role, messages []orchestrator.ChatMessage, toolSpecs []tools.Spec) (<-chan orchestrator.StreamEvent, error) {
	ch := make(chan orchestrator.StreamEvent, 2)
	go func() {
		defer close(ch)
		ch <- orchestrator.StreamEvent{ContentDelta: fmt.Sprintf("Reply from %s.", role)}
	}()
	return ch, nil
}

// fakeBootstrap returns a Bootstrapper that hands back a fakeChatClient
// instantly, for tests that must not download or load a real model.
func fakeBootstrap(ctx context.Context, modelSource string, log BootLogFunc) (orchestrator.ChatClient, error) {
	return fakeChatClient{}, nil
}

// blockingChatClient never produces a response until its context is
// canceled, so tests can reliably catch a debate mid-turn (e.g. to exercise
// abort) without racing a fast fake reply.
type blockingChatClient struct{ started chan struct{} }

func newBlockingChatClient() *blockingChatClient {
	return &blockingChatClient{started: make(chan struct{}, 8)}
}

func (c *blockingChatClient) ChatStreaming(ctx context.Context, role orchestrator.Role, messages []orchestrator.ChatMessage, toolSpecs []tools.Spec) (<-chan orchestrator.StreamEvent, error) {
	select {
	case c.started <- struct{}{}:
	default:
	}
	ch := make(chan orchestrator.StreamEvent)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func blockingBootstrap(client *blockingChatClient) Bootstrapper {
	return func(ctx context.Context, modelSource string, log BootLogFunc) (orchestrator.ChatClient, error) {
		return client, nil
	}
}

// slowLoggingBootstrap returns a Bootstrapper that emits a couple of log
// lines (with a small delay so the form screen gets a chance to render them
// mid-bootstrap) before handing back a fakeChatClient.
func slowLoggingBootstrap(delay time.Duration) Bootstrapper {
	return func(ctx context.Context, modelSource string, log BootLogFunc) (orchestrator.ChatClient, error) {
		if log != nil {
			log("download-libraries: waiting to start download...", "tag", "v1.2.3")
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if log != nil {
			log("download-model: installed", "model", modelSource)
		}
		return fakeChatClient{}, nil
	}
}
