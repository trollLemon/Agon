package app

import (
	"context"
	"fmt"
	"time"

	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/tools"
)

// fakeEngine is a deterministic, instant Engine used by tui tests so they
// never touch a real model. Initialize is a no-op; every role gets a short
// canned reply that mentions its role name, so tests can assert on
// transcript content.
type fakeEngine struct{}

func (fakeEngine) Initialize(ctx context.Context, modelSource string, log orchestrator.LogFunc) error {
	return nil
}

func (fakeEngine) ChatStreaming(ctx context.Context, role orchestrator.Role, messages []orchestrator.ChatMessage, toolSpecs []tools.Spec) (<-chan orchestrator.StreamEvent, error) {
	ch := make(chan orchestrator.StreamEvent, 2)
	go func() {
		defer close(ch)
		ch <- orchestrator.StreamEvent{ContentDelta: fmt.Sprintf("Reply from %s.", role)}
	}()
	return ch, nil
}

// blockingEngine never produces a response until its context is canceled, so
// tests can reliably catch a debate mid-turn (e.g. to exercise abort)
// without racing a fast fake reply. Initialize succeeds immediately.
type blockingEngine struct{}

func (blockingEngine) Initialize(ctx context.Context, modelSource string, log orchestrator.LogFunc) error {
	return nil
}

func (blockingEngine) ChatStreaming(ctx context.Context, role orchestrator.Role, messages []orchestrator.ChatMessage, toolSpecs []tools.Spec) (<-chan orchestrator.StreamEvent, error) {
	ch := make(chan orchestrator.StreamEvent)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

// slowLoggingEngine emits a couple of Initialize log lines (with a small
// delay so the bootstrap screen gets a chance to render them mid-load)
// before succeeding, then serves fake replies. It exercises the
// progress-visible-while-loading path without a real download.
type slowLoggingEngine struct {
	fakeEngine
	delay time.Duration
}

func newSlowLoggingEngine(delay time.Duration) *slowLoggingEngine {
	return &slowLoggingEngine{delay: delay}
}

func (e *slowLoggingEngine) Initialize(ctx context.Context, modelSource string, log orchestrator.LogFunc) error {
	if log != nil {
		log("download-libraries: waiting to start download...", "tag", "v1.2.3")
	}
	select {
	case <-time.After(e.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	if log != nil {
		log("download-model: installed", "model", modelSource)
	}
	return nil
}
